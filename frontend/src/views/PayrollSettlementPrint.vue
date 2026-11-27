<script setup>
// Doklad o vykonanom ročnom zúčtovaní preddavkov na daň (pre zamestnanca);
// ?id= vytlačí jedného zamestnanca, inak všetkých so žiadosťou.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { employeeName, settlementLines, settlementOutcome } from '../lib/payroll.js'

const route = useRoute()
const router = useRouter()
const year = Number(route.query.rok)
const view = ref(null)
const error = ref('')
const company = computed(() => session.company ?? {})
const rows = computed(() =>
  (view.value?.list ?? []).filter((r) => (route.query.id ? r.employee.id === Number(route.query.id) : r.request.requested)),
)
onMounted(async () => {
  try {
    view.value = await api.settlements(session.companyId, year)
    document.title = `Ročné zúčtovanie ${year}`
  } catch (e) {
    error.value = e.fields?.[0]?.message ?? e.message
  }
})
const back = () => router.push({ path: '/rocne-zuctovanie', query: { rok: year } })
const print = () => window.print()
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="back" />
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“</span>
      <Button icon="pi pi-print" label="Tlačiť / PDF" size="small" :disabled="!rows.length" @click="print" />
    </nav>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-else-if="view && !rows.length" class="error">Žiadny zamestnanec nepožiadal o ročné zúčtovanie.</p>

    <article v-for="r in rows" :key="r.employee.id" class="sheet">
      <header class="head">
        <div>
          <p class="kind">Ročné zúčtovanie preddavkov na daň z príjmov zo závislej činnosti</p>
          <h1>za rok {{ year }}</h1>
        </div>
        <div class="company">
          <strong>{{ company.name }}</strong>
          <span v-if="company.ico">IČO {{ company.ico }}</span>
          <span v-if="company.dic">DIČ {{ company.dic }}</span>
        </div>
      </header>
      <dl class="who">
        <div><dt>Zamestnanec</dt><dd>{{ employeeName(r.employee) }}</dd></div>
        <div><dt>Rodné číslo</dt><dd>{{ r.employee.rc || '—' }}</dd></div>
        <div><dt>Adresa</dt><dd>{{ [r.employee.street, [r.employee.zip, r.employee.city].filter(Boolean).join(' ')].filter(Boolean).join(', ') || '—' }}</dd></div>
        <div><dt>Dátum vykonania</dt><dd>{{ r.request.date ? formatDate(r.request.date) : '—' }}</dd></div>
      </dl>
      <table class="rz">
        <tbody>
          <tr v-for="[code, text] in settlementLines" :key="code" :class="{ sub: code.match(/[a-c]$/) }">
            <td class="code">{{ code }}</td>
            <td>{{ text }}</td>
            <td class="num">{{ formatAmount(r.rows[code]) }}</td>
          </tr>
          <tr class="total">
            <td colspan="2">Výsledok: {{ settlementOutcome(r.result) }}</td>
            <td class="num"><span class="ledger">{{ formatMoney(r.result) }}</span></td>
          </tr>
        </tbody>
      </table>
      <p v-if="r.request.spouse" class="foot">Nezdaniteľná časť na manžela/manželku {{ r.request.spouse.name }} za {{ r.request.spouse.months }} mes.</p>
      <footer class="sign">
        <span>Vyhotovil: ……………………………</span>
        <span>Podpis zamestnávateľa: ……………………………</span>
      </footer>
    </article>
  </div>
</template>

<style scoped>
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
.sheet {
  width: 210mm;
  max-width: 100%;
  min-height: 287mm;
  margin: 24px auto 0;
  padding: 14mm 16mm;
  background: #fff;
  color: #1d2633;
  font-size: 10pt;
  line-height: 1.4;
  box-shadow: 0 1px 3px rgba(18, 24, 32, 0.12), 0 8px 28px rgba(18, 24, 32, 0.08);
  break-after: page;
}
.head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 2px solid #1f3a5c;
  padding-bottom: 6px;
}
.kind {
  margin: 0;
  font-size: 9pt;
  color: #4a5a6c;
}
h1 {
  margin: 0;
  font-size: 16pt;
  color: #1f3a5c;
}
.company {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-size: 9pt;
}
.who {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 4px 16px;
  margin: 10px 0;
}
.who dt {
  font-size: 7.5pt;
  color: #4a5a6c;
}
.who dd {
  margin: 0;
}
.rz {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.rz td {
  padding: 3px 4px;
  border-bottom: 1px solid #d5dbe2;
}
.rz .sub td {
  color: #4a5a6c;
}
.rz .code {
  width: 3rem;
  color: #4a5a6c;
}
.num {
  text-align: right;
  white-space: nowrap;
}
.rz .total td {
  border-top: 2px solid #1d2633;
  border-bottom: none;
  font-weight: 700;
  padding-top: 6px;
}
.ledger {
  border-bottom: 3px double currentColor;
}
.foot {
  font-size: 9pt;
  color: #4a5a6c;
}
.sign {
  display: flex;
  justify-content: space-between;
  margin-top: 24mm;
  font-size: 9pt;
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
    margin: 0;
    box-shadow: none;
    min-height: auto;
  }
}
</style>
