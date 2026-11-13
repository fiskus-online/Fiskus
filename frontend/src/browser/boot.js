// Fiškus v prehliadači: spustenie pred zobrazením aplikácie.
//
//   stránka ──fetch /api/…──▶ service worker (sw.js)
//                                 │ postMessage
//   stránka (tento súbor) ◀───────┘
//      │ postMessage
//      ▼
//   web worker (worker.js): Fiškus v Go (WebAssembly) + SQLite (OPFS)
//
// Rozhranie tak volá rovnaké API ako pri nainštalovanom programe.
import { downloadURL } from '../lib/install.js'
import { missingFeatures, progressShare, progressText } from './support.js'

const mark = `<svg viewBox="0 0 32 32" width="40" height="40" aria-hidden="true">
  <rect width="32" height="32" rx="7" fill="#1f3a5c" />
  <path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#f6f7f9" />
  <rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" /></svg>`

const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c])

function screen(el, body) {
  el.innerHTML = `<div class="boot">${mark}<h1>Fiškus</h1>${body}</div>`
}

function problem(el, title, text, retry) {
  screen(el, `<p class="boot-title">${esc(title)}</p><p>${text}</p>
    <p class="boot-actions">${retry ? '<button type="button" class="boot-btn" data-retry>Skúsiť znova</button>' : ''}
    <a class="boot-link" href="${downloadURL}">Stiahnuť nainštalovaný Fiškus</a></p>`)
  el.querySelector('[data-retry]')?.addEventListener('click', () => location.reload())
}

/** Jedna karta naraz: databázu v úložisku prehliadača môže mať otvorenú iba jedna. */
function lockTab() {
  return new Promise((resolve) => {
    navigator.locks.request('fiskus-databaza', { ifAvailable: true }, (lock) => {
      resolve(!!lock)
      return lock ? new Promise(() => {}) : undefined // zámok drží stránka až do zatvorenia
    })
  })
}

async function serviceWorker() {
  await navigator.serviceWorker.register('/sw.js', { scope: '/' })
  if (!navigator.serviceWorker.controller) {
    // prvé otvorenie: počkať, kým service worker prevezme stránku
    await new Promise((resolve) => navigator.serviceWorker.addEventListener('controllerchange', resolve, { once: true }))
  }
}

/** Spustí Fiškus; vráti true, keď sa môže zobraziť aplikácia. */
export async function boot(el) {
  const missing = missingFeatures()
  if (missing.length) {
    problem(el, 'Tento prehliadač Fiškus nespustí', `Chýba mu: ${esc(missing.join(', '))}. Použite aktuálny Chrome, Edge, Firefox alebo Safari, alebo si Fiškus nainštalujte do počítača.`)
    return false
  }
  if (!(await lockTab())) {
    problem(el, 'Fiškus je už otvorený', 'Fiškus v prehliadači môže byť otvorený iba v jednej karte. Prepnite sa do nej, alebo ju zatvorte a skúste znova.', true)
    return false
  }

  const draw = (loaded = 0, total = 0) => {
    const share = progressShare(loaded, total)
    screen(el, `<p class="boot-title" aria-live="polite">${progressText(loaded, total)}</p>
      <div class="boot-bar${share === null ? ' boot-bar-busy' : ''}"><span style="width:${Math.round((share ?? 0.3) * 100)}%"></span></div>
      <p class="boot-note">Údaje zostávajú iba v tomto počítači.</p>`)
  }
  draw()
  navigator.storage.persist?.().catch(() => {})

  try {
    await serviceWorker()
  } catch (e) {
    problem(el, 'Fiškus sa nespustil', `Service worker sa nepodarilo zaregistrovať (${esc(e.message)}). V súkromnom okne niektoré prehliadače úložisko nedovolia.`, true)
    return false
  }

  const worker = new Worker(new URL('./worker.js', import.meta.url), { type: 'module' })
  // požiadavky na API zo service workera → worker; odpoveď ide priamo cez port
  navigator.serviceWorker.addEventListener('message', (e) => {
    if (e.data?.type === 'fiskus-api') {
      const req = e.data.req
      worker.postMessage({ type: 'api', req }, req.body ? [e.ports[0], req.body] : [e.ports[0]])
    } else if (e.data?.type === 'fiskus-owner?') {
      e.ports[0]?.postMessage(true) // túto kartu obsluhuje worker
    }
  })
  navigator.serviceWorker.startMessages()

  const ready = await new Promise((resolve) => {
    worker.onmessage = (e) => {
      const m = e.data
      if (m.type === 'progress') draw(m.loaded, m.total)
      else if (m.type === 'ready') {
        globalThis.fiskusStorage = m.storage // opfs | indexeddb | memory (pozri worker.js)
        resolve(true)
      }
      else if (m.type === 'failed') {
        problem(el, 'Fiškus sa nespustil', esc(m.message), true)
        resolve(false)
      }
    }
    worker.onerror = (e) => {
      problem(el, 'Fiškus sa nespustil', esc(e.message || 'Chyba pri načítaní programu.'), true)
      resolve(false)
    }
  })
  if (ready) navigator.serviceWorker.controller?.postMessage({ type: 'fiskus-owner' })
  return ready
}
