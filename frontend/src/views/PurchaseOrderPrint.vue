<script setup>
// Objednávka dodávateľovi na tlač (A4): odberateľ (firma), dodávateľ,
// položky s kódom dodávateľa a nákupnou cenou bez DPH. Sumy sú zo servera.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import PrintFooter from '../components/PrintFooter.vue'
import CompanyLogo from '../components/CompanyLogo.vue'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'

const props = defineProps({ id: String })
const print = () => window.print()
const router = useRouter()
const order = ref(null)
const supplier = ref(null)
const error = ref(null)
const company = computed(() => session.company ?? {})

onMounted(async () => {
  try {
    order.value = await api.porder(session.companyId, props.id)
    document.title = `Objednávka ${order.value.number}`
    if (order.value.contactId) supplier.value = await api.contact(session.companyId, order.value.contactId).catch(() => null)
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
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="router.push(`/objednavky/${id}`)" />
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“</span>
      <Button icon="pi pi-print" label="Tlačiť / PDF" size="small" :disabled="!order" @click="print" />
    </nav>
    <p v-if="error" class="error">{{ error }}</p>

    <article v-else-if="order" class="sheet">
      <div v-if="order.status === 'cancelled'" class="stamp">Zrušená</div>
      <header class="head">
        <div>
          <CompanyLogo class="logo" height="12mm" />
          <p class="kind">Objednávka</p>
          <h1>{{ order.number }}</h1>
        </div>
        <div class="company">
          <span>Dátum {{ formatDate(order.date) }}</span>
          <span v-if="order.expected">Termín dodania {{ formatDate(order.expected) }}</span>
        </div>
      </header>

      <div class="parties">
        <section>
          <h2>Odberateľ</h2>
          <strong>{{ company.name }}</strong>
          <span>{{ addr(company) }}</span>
          <span>{{ ids(company) }}</span>
          <span v-if="company.email || company.phone">{{ [company.phone, company.email].filter(Boolean).join(', ') }}</span>
        </section>
        <section>
          <h2>Dodávateľ</h2>
          <strong>{{ order.supplier }}</strong>
          <template v-if="supplier">
            <span>{{ addr(supplier) }}</span>
            <span>{{ ids(supplier) }}</span>
          </template>
        </section>
      </div>

      <p>Objednávame u vás:</p>
      <table class="lines">
        <thead>
          <tr><th class="l">#</th><th class="l">Kód</th><th class="l">Položka</th><th>Množstvo</th><th class="l">MJ</th><th>Cena/MJ</th><th>Spolu</th></tr>
        </thead>
        <tbody>
          <tr v-for="(l, i) in order.lines" :key="i">
            <td class="l">{{ i + 1 }}</td><td class="l">{{ l.supplierCode }}</td><td class="l">{{ l.name }}</td>
            <td>{{ formatAmount(l.quantity) }}</td><td class="l">{{ l.unit }}</td>
            <td>{{ Number(l.unitPrice) ? formatAmount(l.unitPrice) : '' }}</td><td>{{ Number(l.total) ? formatAmount(l.total) : '' }}</td>
          </tr>
        </tbody>
      </table>

      <section v-if="Number(order.total)" class="sum">
        <div>
          <span>Spolu bez DPH (orientačne)</span>
          <strong class="grand">{{ formatMoney(order.total) }}</strong>
        </div>
      </section>
      <p v-if="order.note" class="note">{{ order.note }}</p>

      <footer class="signs">
        <div><span>Vystavil</span></div>
        <div><span>Potvrdenie dodávateľa</span></div>
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
