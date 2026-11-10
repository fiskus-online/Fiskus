<script setup>
// Zákazka na tlač (A4) podľa druhu: zákazkový list (autoservis, všeobecná),
// montážny protokol alebo servisný list. Údaje druhu, výkaz práce
// a materiál (s cenami alebo bez nich), vyhlásenie a podpisy. Sumy sú zo servera.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import PrintFooter from '../components/PrintFooter.vue'
import CompanyLogo from '../components/CompanyLogo.vue'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { blankRows, detailRows, printSigns, printTitle } from '../lib/jobPrint.js'

const props = defineProps({ id: String })
const print = () => window.print()
const router = useRouter()
const job = ref(null)
const customer = ref(null)
const error = ref(null)
const prices = ref(true)
const company = computed(() => session.company ?? {})
const vat = computed(() => !!session.company?.vatPayer)

onMounted(async () => {
  try {
    job.value = await api.job(session.companyId, props.id)
    if (job.value.contactId) customer.value = await api.contact(session.companyId, job.value.contactId).catch(() => null)
    document.title = `${printTitle(job.value.kind)} ${job.value.number}`
  } catch (e) {
    error.value = e.message
  }
})
const title = computed(() => printTitle(job.value?.kind))
const rows = computed(() => detailRows(job.value))
const signs = computed(() => printSigns(job.value?.kind))
const blanks = computed(() => blankRows(job.value))
const hasEntries = computed(() => !!(job.value?.work?.length || job.value?.material?.length))
const s = computed(() => job.value?.summary ?? {})
const addr = (c) => [c?.street, [c?.zip, c?.city].filter(Boolean).join(' ')].filter(Boolean).join(', ')
const ids = (c) => [c?.ico && `IČO ${c.ico}`, c?.dic && `DIČ ${c.dic}`, c?.icdph && `IČ DPH ${c.icdph}`].filter(Boolean).join(', ')
const q = (v) => formatAmount(v).replace(/,00$/, '')
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="router.push(`/zakazky/${id}`)" />
      <label class="prices"><Checkbox v-model="prices" binary /> s cenami</label>
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“</span>
      <Button icon="pi pi-print" label="Tlačiť / PDF" size="small" :disabled="!job" @click="print" />
    </nav>
    <p v-if="error" class="error">{{ error }}</p>

    <article v-else-if="job" class="sheet">
      <header class="head">
        <div>
          <CompanyLogo class="logo" height="12mm" />
          <p class="kind">{{ title }}</p>
          <h1>{{ job.number }}</h1>
        </div>
        <div class="company">
          <span>Prijaté {{ formatDate(job.start || job.created) }}</span>
          <span v-if="job.due">Termín {{ formatDate(job.due) }}</span>
          <span v-if="job.closed">Ukončené {{ formatDate(job.closed) }}</span>
        </div>
      </header>

      <div class="parties">
        <section>
          <h2>Zhotoviteľ</h2>
          <strong>{{ company.name }}</strong>
          <span>{{ addr(company) }}</span>
          <span>{{ ids(company) }}</span>
          <span v-if="company.phone || company.email">{{ [company.phone, company.email].filter(Boolean).join(', ') }}</span>
        </section>
        <section>
          <h2>Zákazník</h2>
          <strong>{{ customer?.name || job.customer || '' }}</strong>
          <template v-if="customer">
            <span>{{ addr(customer) }}</span>
            <span>{{ ids(customer) }}</span>
            <span v-if="customer.phone || customer.email">{{ [customer.phone, customer.email].filter(Boolean).join(', ') }}</span>
          </template>
        </section>
      </div>

      <p class="subject">{{ job.name }}</p>
      <dl v-if="rows.length" class="rows">
        <div v-for="r in rows" :key="r.label" :class="{ wide: r.wide }"><dt>{{ r.label }}</dt><dd>{{ r.value }}</dd></div>
      </dl>

      <h3>{{ job.kind === 'install' || job.material?.length ? 'Vykonané práce a materiál' : 'Vykonané práce' }}</h3>
      <table class="lines">
        <thead>
          <tr>
            <th class="l">Dátum</th><th class="l">Popis</th><th>Množstvo</th><th class="l">MJ</th>
            <template v-if="prices"><th>Cena/MJ</th><th>Spolu</th></template>
          </tr>
        </thead>
        <tbody>
          <tr v-for="w in job.work" :key="'w' + w.id">
            <td class="l">{{ formatDate(w.date) }}</td><td class="l">{{ w.description }}{{ w.person ? ` (${w.person})` : '' }}</td>
            <td>{{ q(w.hours) }}</td><td class="l">hod</td>
            <template v-if="prices"><td>{{ formatAmount(w.rate) }}</td><td>{{ formatAmount(w.total) }}</td></template>
          </tr>
          <tr v-for="m in job.material" :key="'m' + m.id">
            <td class="l">{{ formatDate(m.date) }}</td><td class="l">{{ m.name }}</td>
            <td>{{ q(m.quantity) }}</td><td class="l">{{ m.unit }}</td>
            <template v-if="prices"><td>{{ formatAmount(m.unitPrice) }}</td><td>{{ formatAmount(m.total) }}</td></template>
          </tr>
          <tr v-for="n in blanks" :key="'b' + n"><td class="blank" :colspan="prices ? 6 : 4" /></tr>
        </tbody>
      </table>

      <section v-if="prices && hasEntries" class="sum">
        <div><span>Práca {{ formatAmount(s.workTotal) }} · materiál {{ formatAmount(s.materialTotal) }}{{ vat ? ' · ceny bez DPH' : '' }}</span></div>
        <div>
          <span>Spolu</span>
          <strong class="grand">{{ formatMoney(s.reported) }}</strong>
        </div>
      </section>
      <p v-if="job.note" class="note">{{ job.note }}</p>

      <p v-if="signs.statement" class="statement">{{ signs.statement }}</p>
      <div v-if="signs.remarks" class="remarks" />

      <footer class="signs" :style="{ '--signs': signs.signs.length }">
        <div v-for="t in signs.signs" :key="t"><span>{{ t }}</span></div>
      </footer>
      <PrintFooter />
    </article>
  </div>
</template>

<style scoped>
.lines {
  width: 100%;
  border-collapse: collapse;
  margin: 10px 0 0;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
}
.lines th,
.lines td {
  border-bottom: 1px solid #d5dbe2;
  padding: 3px 6px;
  text-align: right;
}
.lines th.l,
.lines td.l {
  text-align: left;
}
.lines th {
  font-weight: 500;
  color: #4a5a6c;
}
.lines tfoot td {
  text-align: left;
  border: 0;
  color: #4a5a6c;
}
.print-root {
  min-height: 100%;
  background: #e9ecf0;
  padding: 0 0 40px;
}
.toolbar {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  background: var(--sheet);
  border-bottom: 1px solid var(--rule);
}
.prices {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
}
.spacer {
  flex: 1;
}
.tip {
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.error {
  text-align: center;
  color: var(--tone-overdue);
  padding: 40px;
}
/* polovica A4 na šírku hárku: 210 × 148 mm */
.sheet {
  position: relative;
  width: 210mm;
  min-height: 270mm;
  margin: 24px auto 0;
  padding: 12mm 14mm;
  background: #fff;
  color: #1d2633;
  font-size: 10pt;
  line-height: 1.4;
  box-shadow: 0 1px 3px rgba(18, 24, 32, 0.12), 0 8px 28px rgba(18, 24, 32, 0.08);
  overflow: hidden;
}
.stamp {
  position: absolute;
  top: 50mm;
  left: 50%;
  transform: translateX(-50%) rotate(-14deg);
  font-size: 40pt;
  font-weight: 700;
  color: rgba(176, 58, 38, 0.15);
  border: 4px solid rgba(176, 58, 38, 0.15);
  padding: 1mm 6mm;
}
.head {
  display: flex;
  justify-content: space-between;
  gap: 10mm;
  border-bottom: 2px solid #1f3a5c;
  padding-bottom: 4mm;
}
.logo {
  margin-bottom: 3mm;
}
.kind {
  margin: 0;
  font-size: 9pt;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #4e5866;
}
h1 {
  margin: 0;
  font-size: 20pt;
  color: #1f3a5c;
}
.company {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  text-align: right;
  font-size: 9pt;
}
.company strong {
  font-size: 10.5pt;
}
.rows {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 3mm 8mm;
  margin: 6mm 0;
}
.rows div {
  display: flex;
  flex-direction: column;
  border-bottom: 1px solid #dce1e8;
  padding-bottom: 1.5mm;
}
.rows .wide {
  grid-column: 1 / -1;
}
.rows dt {
  font-size: 8pt;
  color: #6b7685;
}
.rows dd {
  margin: 0;
  font-weight: 600;
}
.sum {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2mm;
}
.sum div {
  display: flex;
  align-items: baseline;
  gap: 6mm;
}
.sum span {
  color: #4e5866;
}
.grand {
  font-size: 18pt;
  font-variant-numeric: tabular-nums;
  border-bottom: 3pt double #1d2633;
  line-height: 1.15;
}
.words {
  margin: 0;
  font-style: italic;
  color: #4e5866;
}
.parties {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8mm;
  margin: 6mm 0 2mm;
}
.parties section {
  display: flex;
  flex-direction: column;
  font-size: 9.5pt;
}
.parties h2 {
  margin: 0 0 1mm;
  font-size: 8pt;
  font-weight: 500;
  color: #6b7685;
}
.parties .place {
  margin-top: 2mm;
}
.parties strong {
  font-size: 11pt;
}
.subject {
  margin: 4mm 0 0;
  font-size: 12pt;
  font-weight: 600;
}
h3 {
  margin: 5mm 0 0;
  font-size: 9pt;
  font-weight: 600;
  color: #1f3a5c;
  border-bottom: 1px solid #1f3a5c;
  padding-bottom: 0.5mm;
}
.rows dd {
  white-space: pre-line;
}
.lines td.blank {
  height: 7mm;
}
.statement {
  margin: 5mm 0 0;
  font-size: 9pt;
}
.remarks {
  height: 14mm;
  border-bottom: 1px solid #dce1e8;
}
.note {
  margin: 4mm 0 0;
  white-space: pre-line;
}
.signs {
  display: grid;
  grid-template-columns: repeat(var(--signs, 2), 1fr);
  gap: 6mm;
  margin-top: 16mm;
}
.signs div {
  border-top: 1px solid #1d2633;
  padding-top: 1mm;
  font-size: 8pt;
  color: #4e5866;
  text-align: center;
}
@media (max-width: 860px) {
  .print-root {
    overflow-x: auto;
  }
  .tip {
    display: none;
  }
  .sheet {
    margin: 12px;
  }
}
@media print {
  .print-root {
    background: none;
    padding: 0;
  }
  .toolbar {
    display: none;
  }
  .sheet {
    width: auto;
    margin: 0;
    padding: 0;
    box-shadow: none;
  }
}
</style>
