// Snímky obrazovky do návodu (docs/obrazky) a na web (site/obrazky) z ukážkových
// dát. Spustenie: make snimky (zostaví Fiškus, spustí ukážku a tento skript).
// Ručne: BASE=http://127.0.0.1:8110 node tools/snimky/snimky.cjs
const { chromium } = require('playwright')
const fs = require('node:fs')
const path = require('node:path')

const BASE = process.env.BASE || 'http://127.0.0.1:8110'
const ROOT = path.resolve(__dirname, '../..')
const DOCS = path.join(ROOT, 'docs/obrazky')
const SITE = path.join(ROOT, 'site/obrazky')
const only = process.argv.slice(2)

async function api(p, opts = {}) {
  const r = await fetch(BASE + p, { ...opts, headers: { 'X-Fiskus': '1', 'Content-Type': 'application/json', ...(opts.headers || {}) } })
  if (!r.ok) throw new Error(`${p}: ${r.status}`)
  return r.json()
}

// kliknutie na prvý viditeľný text
const click = (text) => async (p) => {
  await p.getByText(text, { exact: true }).first().click()
  await p.waitForTimeout(900)
}

async function main() {
  const companies = await api('/api/companies')
  const firm = companies.find((c) => c.bookkeeping !== 'household')
  const home = companies.find((c) => c.bookkeeping === 'household')
  const F = `/api/companies/${firm.id}`
  const inv = await api(`${F}/invoices`)
  const byNum = (n) => inv.find((x) => x.number === n)?.id
  const pdp = byNum('FA20260041') ?? inv.find((x) => x.status === 'issued').id
  const proforma = inv.find((x) => x.kind === 'proforma' && x.status === 'issued')?.id
  const quotes = await api(`${F}/quotes`)
  const quote = quotes.find((q) => q.status === 'issued')?.id
  const purchases = await api(`${F}/purchases`)
  const foreign = purchases.find((x) => x.currency && x.currency !== 'EUR')?.id ?? purchases[0].id
  const items = await api(`${F}/items`)
  const stocked = items.find((x) => x.ean)?.id ?? items[0].id
  const assets = (await api(`${F}/assets`)).items
  const car = assets.find((a) => /Transit|auto|vozidlo/i.test(a.name))?.id ?? assets[0].id

  // názov, cesta, firma, akcia, voľby (mobil, celá stránka, web)
  const shots = [
    // úvod webu: program v širokom okne
    ['uvod', '/faktury', firm, null, { site: true, siteOnly: true, wide: true }],
    ['uvod-mobil', '/faktury', firm, null, { site: true, siteOnly: true, mobile: true }],
    // prehľad firmy
    ['prehlad', '/prehlad', firm, null, { full: true }],
    // predaj
    ['faktury', '/faktury', firm, null, { site: true }],
    ['faktura', `/faktury/${pdp}`, firm, null, { site: true }],
    ['faktura-tlac', `/faktury/${pdp}/tlac`, firm, null, { site: true, full: true }],
    ['zalohova-faktura', `/faktury/${proforma}`, firm],
    ['ponuky', '/ponuky', firm],
    ['ponuka', `/ponuky/${quote}`, firm],
    ['pravidelna-fakturacia', '/pravidelne', firm],
    ['upomienky', '/upomienky', firm],
    ['cennik', '/cennik', firm],
    ['e-faktury', '/nastavenia/e-faktury', firm, null, { site: true }],
    // nákup
    ['prijate', '/prijate', firm],
    ['prijata-faktura', `/prijate/${foreign}`, firm],
    // sklad
    ['sklad', '/sklad', firm],
    ['sklad-doklady', '/sklad', firm, click('Doklady')],
    ['karta-produktu', `/cennik/${stocked}/karta`, firm],
    // peniaze
    ['banka', '/banka', firm, null, { site: true }],
    ['pokladna', '/pokladna', firm],
    // dane
    ['dph', '/dph', firm, null, { site: true }],
    ['kontrolny-vykaz', '/kontrolny-vykaz', firm],
    ['suhrnny-vykaz', '/suhrnny-vykaz', firm],
    ['dan-z-prijmov', '/dan-z-prijmov', firm],
    ['kalendar', '/kalendar', firm, null, { site: true }],
    // mzdy
    ['mzdy', '/mzdy', firm, null, { site: true }],
    ['zamestnanci', '/zamestnanci', firm],
    // účtovníctvo (jednoduché)
    ['penazny-dennik', '/dennik', firm, null, { site: true }],
    ['pohladavky-zavazky', '/pohladavky-zavazky', firm],
    ['vykazy-ju', '/vykazy-ju', firm],
    ['majetok', '/majetok', firm],
    ['majetok-karta', `/majetok/${car}`, firm],
    ['kniha-jazd', '/kniha-jazd', firm],
    ['kniha-jazd-gps', '/kniha-jazd', firm, async (p) => {
      await api(`${F}/logbook/places`, { method: 'PUT', body: JSON.stringify([
        { name: 'Sídlo, Banská Bystrica', lat: 48.7363, lon: 19.1462 },
        { name: 'Zvolen – Hotel Lesná', lat: 48.5767, lon: 19.1263 },
      ]) })
      await p.getByRole('button', { name: 'Jazdy zo záznamu GPS' }).click()
      await p.setInputFiles('.gps-panel input[type=file]', path.join(__dirname, 'jazdy.gpx'))
      await p.waitForSelector('.drafts li')
      await p.locator('.purpose input').first().fill('Montáž rozvádzača – Hotel Lesná')
      await p.locator('.purpose input').nth(1).fill('Návrat zo stavby')
      await p.locator('.gps .lead').click()
      await p.waitForTimeout(500)
    }],
    // kalkulačky
    ['kalkulacky', '/kalkulacky?k=loan', firm, null, { site: true }],
    ['kalkulacky-mzda', '/kalkulacky?k=wage', firm, async (p) => {
      await p.getByRole('button', { name: 'Dieťa', exact: true }).click()
    }, { mobile: true, full: true }],
    ['pokladna-uzavierka', '/pokladna', firm, async (p) => {
      await p.getByRole('button', { name: 'Uzávierky e-kasy' }).click()
      await p.getByRole('button', { name: 'Nová uzávierka' }).click()
      const inputs = p.locator('.closings-panel .form-grid input')
      await inputs.nth(1).fill('217')
      await inputs.nth(2).fill('88812345678900001')
      await p.getByLabel('Obrat 23 %').fill('1 254,60')
      await p.getByLabel('Obrat 5 %').fill('86,10')
      await p.getByPlaceholder('0').fill('612,40')
      await p.waitForTimeout(800)
    }],
    ['banka-fio', '/banka', firm, async (p) => {
      await p.locator('header .p-button:has(.pi-link)').click()
      await p.locator('#fio-token').fill('x'.repeat(64))
      await p.waitForTimeout(500)
    }, { mobile: true }],
    ['kurzove-rozdiely', '/hlavna-kniha', firm, async (p) => {
      await p.getByText('Súvaha', { exact: true }).first().click()
      await p.waitForTimeout(500)
      await p.getByRole('button', { name: /Kurzové rozdiely/ }).click()
      await p.waitForTimeout(600)
    }],
    ['prijata-colne-rozhodnutie', '/prijate', firm, async (p) => {
      const imp = await api(`${F}/purchases`, { method: 'POST', body: JSON.stringify({ supplierNumber: 'SZ-2026-1187', supplier: { name: 'Shenzhen Meanwell Parts Co.', country: 'CN' },
        issueDate: '2026-10-28', receivedDate: '2026-11-02', dueDate: '2026-11-20', supplyKind: 'goods', expense: 'stock', rates: [{ vatRate: 0, base: '1240' }] }) })
      const cu = await api(`${F}/purchases`, { method: 'POST', body: JSON.stringify({ supplierNumber: '26SK5800000048213A7', supplier: { name: 'Colný úrad Bratislava', country: 'SK' },
        issueDate: '2026-11-03', receivedDate: '2026-11-03', dueDate: '2026-11-13', expense: 'stock', rates: [{ vatRate: 23, base: '1314.40' }], customs: { duty: '49.60', importId: imp.id } }) })
      await p.goto(`${BASE}/prijate/${cu.id}`, { waitUntil: 'networkidle' })
      await p.waitForTimeout(600)
      await p.locator('.customs').scrollIntoViewIfNeeded()
    }],
    // tlačivá
    ['tlaciva', '/tlaciva', firm],
    ['tlaciva-ppd', '/tlaciva?t=ppd', firm, async (p) => {
      await p.locator('textarea').nth(1).fill('Ján Novák\nHlavná 12, 974 01 Banská Bystrica')
      await p.getByLabel('Účel platby', { exact: false }).fill('Tržba za predaj materiálu')
      await p.getByLabel('Suma €', { exact: false }).fill('246')
      await p.getByLabel('Vyhotovil', { exact: false }).fill('Ján Novák')
      await p.locator('.fill .p-select').first().click()
      await p.getByRole('option', { name: '23 %' }).click()
      await p.waitForTimeout(800)
    }],
    ['tlaciva-dodaci-list', '/tlaciva?t=dodaci-list', firm, async (p) => {
      await p.getByLabel('Číslo', { exact: true }).fill('DL-2026-031')
      await p.locator('textarea').nth(1).fill('Hotel Lesná s.r.o.\nLesná 5, 960 01 Zvolen\nIČO 36123456')
      const rows = [['Kábel CYKY 3x2,5', '50', 'm', '1,19'], ['Istič B16 1-pól', '6', 'ks', '4,80']]
      for (const [i, r] of rows.entries()) {
        const inputs = p.locator('.item').nth(i).locator('input')
        for (const [j, v] of r.entries()) await inputs.nth(j).fill(v)
      }
      await p.waitForTimeout(800)
    }],
    // e-shopy, mobil, nastavenia
    ['e-shopy', '/nastavenia/e-shopy', firm],
    ['mobil-v-sieti', '/nastavenia/mobil', firm],
    ['nastavenia', '/nastavenia', firm],
    ['firma', '/nastavenia/firma', firm],
    ['firmy', '/firmy', firm],
    ['prenos-dat', '/prenos', firm],
    ['audit', '/audit', firm],
    ['legislativa', '/legislativa', firm],
    ['ai', '/ai', firm],
    ['bezpecnost', '/nastavenia/bezpecnost', firm],
    ['adresar', '/adresar', firm],
    // domácnosť
    ['domacnost', '/domacnost', home, null, { site: true, full: true }],
    ['domacnost-pohyby', '/domacnost/pohyby', home],
    ['domacnost-blocek', '/domacnost/pohyby', home, click('Lidl'), { site: true }],
    ['domacnost-zavazky', '/domacnost/zavazky', home],
    ['domacnost-plan', '/domacnost/zavazky', home, click('Plán po mesiacoch'), { site: true }],
    ['domacnost-ucty', '/domacnost/ucty', home],
    ['domacnost-rozpocet', '/domacnost/rozpocet', home],
    // mobil (telefón)
    ['mobil-domacnost', '/domacnost', home, null, { mobile: true, site: true }],
    ['mobil-faktura', `/faktury/${pdp}`, firm, null, { mobile: true }],
  ]
  // podvojné účtovníctvo: tá istá ukážková firma prepnutá na podvojné
  const double = [
    ['uctovy-rozvrh', '/uctovy-rozvrh', firm],
    ['hlavna-kniha', '/hlavna-kniha', firm, null, { site: true }],
    ['hlavna-kniha-ucty', '/hlavna-kniha', firm, click('Hlavná kniha')],
    ['suvaha', '/hlavna-kniha', firm, click('Súvaha')],
    ['vysledovka', '/hlavna-kniha', firm, click('Výsledovka')],
    ['majetok-odpisy', `/majetok/${car}`, firm],
  ]

  fs.mkdirSync(DOCS, { recursive: true })
  fs.mkdirSync(SITE, { recursive: true })
  const browser = await chromium.launch()
  const run = async (list) => {
    for (const [name, url, company, action, o = {}] of list) {
      if (only.length && !only.includes(name)) continue
      const vp = o.mobile ? { width: 390, height: 844 } : o.wide ? { width: 1440, height: 900 } : { width: 1280, height: 800 }
      const ctx = await browser.newContext({ viewport: vp, deviceScaleFactor: o.mobile ? 2 : 1, locale: 'sk-SK' })
      await ctx.addInitScript((id) => localStorage.setItem('fiskus.companyId', String(id)), company.id)
      const p = await ctx.newPage()
      p.on('pageerror', (e) => console.log(`  ${name}: chyba stránky ${e.message}`))
      await p.goto(BASE + url, { waitUntil: 'networkidle' })
      await p.waitForTimeout(700)
      if (action) await action(p)
      await p.waitForLoadState('networkidle')
      await p.waitForTimeout(300)
      const file = path.join(DOCS, `${name}.png`)
      await p.screenshot({ path: file, fullPage: !!o.full })
      if (o.site) fs.copyFileSync(file, path.join(SITE, `${name}.png`))
      if (o.siteOnly) fs.rmSync(file) // iba na web, nie do návodu
      console.log(`  ${name}`)
      await ctx.close()
    }
  }
  await run(shots)
  await api(`/api/companies/${firm.id}`, { method: 'PUT', body: JSON.stringify({ ...firm, bookkeeping: 'double' }) })
  await run(double)
  await browser.close()
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
