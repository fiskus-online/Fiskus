<script setup>
// Dodací list na tlač (A4): dodávateľ (firma), odberateľ, miesto dodania,
// položky — s cenami alebo bez nich — a podpisy odovzdal / prevzal.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import PrintFooter from '../components/PrintFooter.vue'
import CompanyLogo from '../components/CompanyLogo.vue'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'

const props = defineProps({ id: String })
const print = () => window.print()
const router = useRouter()
const note = ref(null)
const error = ref(null)
const prices = ref(true)
const company = computed(() => session.company ?? {})
const vat = computed(() => !!session.company?.vatPayer)

onMounted(async () => {
  try {
    note.value = await api.delivery(session.companyId, props.id)
    document.title = `Dodací list ${note.value.number || 'koncept'}`
  } catch (e) {
    error.value = e.message
  }
})
const addr = (c) => [c?.street, [c?.zip, c?.city].filter(Boolean).join(' ')].filter(Boolean).join(', ')
const ids = (c) => [c?.ico && `IČO ${c.ico}`, c?.dic && `DIČ ${c.dic}`, c?.icdph && `IČ DPH ${c.icdph}`].filter(Boolean).join(', ')
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="router.push(`/dodacie-listy/${id}`)" />
      <label class="prices"><Checkbox v-model="prices" binary /> s cenami</label>
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“</span>
      <Button icon="pi pi-print" label="Tlačiť / PDF" size="small" :disabled="!note" @click="print" />
    </nav>
    <p v-if="error" class="error">{{ error }}</p>

    <article v-else-if="note" class="sheet">
      <div v-if="note.status === 'cancelled'" class="stamp">Stornovaný</div>
      <div v-else-if="note.status === 'draft'" class="stamp">Koncept</div>
      <header class="head">
        <div>
          <CompanyLogo class="logo" height="12mm" />
          <p class="kind">Dodací list</p>
          <h1>{{ note.number || 'koncept' }}</h1>
        </div>
        <div class="company">
          <span>Dátum dodania {{ formatDate(note.date) }}</span>
          <span v-if="note.order">Objednávka {{ note.order }}</span>
        </div>
      </header>

      <div class="parties">
        <section>
          <h2>Dodávateľ</h2>
          <strong>{{ company.name }}</strong>
          <span>{{ addr(company) }}</span>
          <span>{{ ids(company) }}</span>
        </section>
        <section>
          <h2>Odberateľ</h2>
          <strong>{{ note.customer.name }}</strong>
          <span>{{ addr(note.customer) }}</span>
          <span>{{ ids(note.customer) }}</span>
          <template v-if="note.place"><h2 class="place">Miesto dodania</h2><span>{{ note.place }}</span></template>
        </section>
      </div>

      <table class="lines">
        <thead>
          <tr>
            <th class="l">#</th><th class="l">Položka</th><th>Množstvo</th><th class="l">MJ</th>
            <template v-if="prices"><th>Cena/MJ</th><th v-if="vat">DPH</th><th>Spolu</th></template>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(l, i) in note.lines" :key="i">
            <td class="l">{{ i + 1 }}</td><td class="l">{{ l.description }}</td><td>{{ formatAmount(l.quantity) }}</td><td class="l">{{ l.unit }}</td>
            <template v-if="prices"><td>{{ formatAmount(l.unitPrice) }}</td><td v-if="vat">{{ l.vatRate }} %</td><td>{{ formatAmount(l.amount) }}</td></template>
          </tr>
        </tbody>
      </table>

      <section v-if="prices" class="sum">
        <div v-if="vat"><span>{{ note.priceMode === 'gross' ? 'Ceny s DPH' : 'Ceny bez DPH' }} · základ {{ formatAmount(note.summary.base) }} · DPH {{ formatAmount(note.summary.vat) }}</span></div>
        <div>
          <span>Spolu</span>
          <strong class="grand">{{ formatMoney(note.summary.total) }}</strong>
        </div>
      </section>
      <p v-if="note.note" class="note">{{ note.note }}</p>

      <footer class="signs">
        <div><span>Odovzdal</span></div>
        <div><span>Prevzal{{ note.receiver ? ` — ${note.receiver}` : '' }}</span></div>
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
.note {
  margin: 4mm 0 0;
  white-space: pre-line;
}
.signs {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
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
