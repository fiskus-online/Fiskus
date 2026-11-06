<script setup>
// Mobil: odfotenie bločku eKasa. QR kód sa prečíta z fotky priamo v telefóne
// (jsQR), na server ide iba kód a zmenšená fotka. Funguje aj cez http v sieti
// (fotoaparát cez <input capture>, bez živého náhľadu kamery).
import { computed, ref, watch } from 'vue'
import jsQR from 'jsqr'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatMoney, formatDate } from '../lib/format.js'
import { findQR, imageReader, loadImage, payments, receiptId, shrinkImage } from '../lib/receipt.js'

const fileInput = ref(null)
const busy = ref('')
const error = ref('')
const manual = ref('')
const done = ref([]) // zapísané bločky v tejto relácii (najnovší hore)
// domácnosť: bloček sa zapíše ako pohyb rozdelený na položky (kategórie
// navrhne server, upraviť ich možno v programe); bez QR kódu sa číta fotka
const hh = computed(() => session.company?.bookkeeping === 'household')
const canWrite = computed(() => can(hh.value ? 'ledger.write' : 'purchases.write'))
const accounts = ref([])
const account = ref(null)
watch(hh, async (on) => {
  if (!on) return
  try {
    accounts.value = (await api.hhAccounts(session.companyId)).filter((a) => !a.closed)
    let saved = 0
    try {
      saved = Number(localStorage.getItem('fiskus.hhAccount'))
    } catch {
      /* súkromné okno */
    }
    account.value = accounts.value.find((a) => a.id === saved)?.id ?? accounts.value.find((a) => a.kind === 'cash')?.id ?? accounts.value[0]?.id ?? null
  } catch (e) {
    error.value = e.message
  }
}, { immediate: true })
function rememberAccount(v) {
  try {
    localStorage.setItem('fiskus.hhAccount', String(v))
  } catch {
    /* súkromné okno — nevadí */
  }
}

async function sendHousehold(qr, photo) {
  busy.value = 'send'
  try {
    const d = qr ? await api.hhReceiptQR(session.companyId, qr) : await api.hhReceiptPhoto(session.companyId, photo)
    const saved = await api.hhSaveEntry(session.companyId, { ...d.entry, accountId: account.value || d.entry.accountId })
    if (photo) await api.hhSavePhoto(session.companyId, saved.id, photo).catch(() => {})
    done.value = [{ id: saved.id, hh: true, name: saved.payee || 'Nákup', date: saved.date, items: saved.split?.length ?? 0, total: saved.amount.replace('-', ''), category: saved.category }, ...done.value].slice(0, 20)
    manual.value = ''
  } catch (err) {
    error.value = err instanceof ApiError && err.fields.length ? err.fields[0].message : err.message
  } finally {
    busy.value = ''
  }
}

function remembered() {
  try {
    return localStorage.getItem('fiskus.payment') || 'cash'
  } catch {
    return 'cash'
  }
}
const payment = ref(remembered())
function rememberPayment(v) {
  try {
    localStorage.setItem('fiskus.payment', v)
  } catch {
    /* súkromné okno — nevadí */
  }
}

async function onPhoto(e) {
  const file = e.target.files?.[0]
  e.target.value = ''
  if (!file) return
  error.value = ''
  busy.value = 'read'
  let url = ''
  try {
    const loaded = await loadImage(file)
    url = loaded.url
    const text = findQR(jsQR, loaded.img.naturalWidth, loaded.img.naturalHeight, imageReader(loaded.img))
    if (hh.value) {
      const photo = await shrinkImage(loaded.img)
      await sendHousehold(receiptId(text) ? text : '', photo)
      return
    }
    if (!text) {
      error.value = 'Na fotke sa nenašiel QR kód. Odfoťte ho zblízka, ostro a celý (aj s bielym okrajom).'
      return
    }
    const photo = await shrinkImage(loaded.img)
    await send(text, photo)
  } catch (err) {
    error.value = err.message
  } finally {
    if (url) URL.revokeObjectURL(url)
    busy.value = ''
  }
}

async function send(qr, photo) {
  busy.value = 'send'
  try {
    const r = await api.scanReceipt(session.companyId, { qr, payment: payment.value, photo })
    done.value = [{ ...r.purchase, items: r.receipt.items?.length ?? 0, total: r.receipt.total }, ...done.value].slice(0, 20)
    manual.value = ''
  } catch (err) {
    error.value = err instanceof ApiError && err.fields.length ? err.fields[0].message : err.message
  } finally {
    busy.value = ''
  }
}

function sendManual() {
  const id = receiptId(manual.value)
  if (!id) {
    error.value = 'Kód bločku začína O- a má 32 znakov za pomlčkou (je pod QR kódom).'
    return
  }
  error.value = ''
  if (hh.value) sendHousehold(id, null)
  else send(id, null)
}
</script>

<template>
  <div class="mobile">
    <header class="m-head">
      <svg viewBox="0 0 32 32" width="28" height="28" aria-hidden="true">
        <rect width="32" height="32" rx="7" fill="#f6f7f9" />
        <path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#1f3a5c" />
        <rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" />
      </svg>
      <div>
        <strong>Bločky</strong>
        <span>{{ session.company?.name }}</span>
      </div>
      <RouterLink :to="hh ? '/domacnost' : '/faktury'" class="full">Celý program</RouterLink>
    </header>

    <main class="m-body">
      <p v-if="!canWrite" class="m-error">Na zapisovanie dokladov nemáte oprávnenie.</p>
      <template v-else>
        <SelectButton v-if="hh" v-model="account" :options="accounts" option-label="name" option-value="id" :allow-empty="false" class="pay"
          aria-label="Zaplatené z účtu" @update:model-value="rememberAccount" />
        <SelectButton v-else v-model="payment" :options="payments" option-label="label" option-value="value" :allow-empty="false" class="pay"
          aria-label="Spôsob úhrady" @update:model-value="rememberPayment" />

        <button type="button" class="shoot" :disabled="!!busy" @click="fileInput.click()">
          <i class="pi" :class="busy ? 'pi-spin pi-spinner' : 'pi-camera'" aria-hidden="true" />
          <span>{{ busy === 'read' ? 'Čítam QR kód…' : busy === 'send' ? (hh ? 'Čítam a zapisujem bloček…' : 'Zapisujem bloček…') : 'Odfotiť bloček' }}</span>
          <small v-if="hh">Celý bloček aj s QR kódom — položky sa zaradia do kategórií</small>
          <small v-else>Odfoťte QR kód na bločku — zvyšok sa doplní z eKasy</small>
        </button>
        <input ref="fileInput" type="file" accept="image/*" capture="environment" class="hidden" @change="onPhoto" />

        <p v-if="error" class="m-error" role="alert"><i class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ error }}</p>

        <details class="manual">
          <summary>QR kód sa nedá odfotiť?</summary>
          <form class="manual-row" @submit.prevent="sendManual">
            <InputText v-model="manual" placeholder="O-…" aria-label="Kód bločku" autocapitalize="characters" spellcheck="false" />
            <Button type="submit" label="Zapísať" size="small" :disabled="!!busy" />
          </form>
        </details>

        <section v-if="done.length" class="list" aria-label="Zapísané bločky">
          <h2>Zapísané</h2>
          <template v-for="p in done" :key="p.id">
            <RouterLink v-if="p.hh" to="/domacnost/pohyby" class="row">
              <span class="who">{{ p.name }}<small>{{ formatDate(p.date) }}, {{ p.items }} položiek · {{ p.category }}</small></span>
              <strong class="sum">{{ formatMoney(p.total) }}</strong>
            </RouterLink>
            <RouterLink v-else :to="`/prijate/${p.id}`" class="row">
              <span class="who">{{ p.supplier.name }}<small>{{ formatDate(p.issueDate) }}, {{ p.number }}</small></span>
              <strong class="sum">{{ formatMoney(p.total) }}</strong>
            </RouterLink>
          </template>
        </section>
      </template>
    </main>
  </div>
</template>

<style scoped>
.mobile {
  min-height: 100%;
  background: var(--paper);
}
.m-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  background: var(--blue-deep);
  color: #fff;
}
.m-head div {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}
.m-head span {
  font-size: var(--fs-sm);
  color: #c9d5e3;
}
.full {
  margin-left: auto;
  color: #dbe4ee;
  font-size: var(--fs-sm);
}
.m-body {
  max-width: 480px;
  margin: 0 auto;
  padding: 16px;
}
.pay {
  display: flex;
  margin-bottom: 14px;
}
.pay :deep(.p-togglebutton) {
  flex: 1;
}
.shoot {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 28px 16px;
  border: 0;
  border-radius: 10px;
  background: var(--blue);
  color: #fff;
  font: inherit;
  cursor: pointer;
}
.shoot:disabled {
  opacity: 0.8;
}
.shoot .pi {
  font-size: 2.2rem;
}
.shoot span {
  font-size: var(--fs-lg);
  font-weight: 600;
}
.shoot small {
  color: #dbe4ee;
}
.shoot:focus-visible {
  outline: 3px solid var(--brass);
  outline-offset: 2px;
}
.hidden {
  display: none;
}
.m-error {
  margin-top: 12px;
  padding: 10px 12px;
  background: var(--tone-overdue-bg);
  color: var(--tone-overdue);
  border-radius: var(--radius);
}
.manual {
  margin-top: 14px;
  color: var(--ink-soft);
}
.manual summary {
  cursor: pointer;
  font-size: var(--fs-sm);
}
.manual-row {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.manual-row :deep(input) {
  flex: 1;
  min-width: 0;
}
.list {
  margin-top: 20px;
}
.list h2 {
  font-size: var(--fs-md);
  color: var(--ink-soft);
  margin-bottom: 6px;
}
.row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  margin-bottom: 6px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  color: var(--ink);
  text-decoration: none;
}
.who {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.who small {
  display: block;
  color: var(--ink-faint);
}
.sum {
  font-variant-numeric: tabular-nums;
  color: var(--tone-paid);
}
</style>
