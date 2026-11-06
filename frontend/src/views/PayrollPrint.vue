<script setup>
// Výplatné pásky mesiaca na tlač (dve na hárok A4); ?id= vytlačí jednu.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { contractLabel, employeeName, insurerLabel, periodLabel } from '../lib/payroll.js'

const route = useRoute()
const router = useRouter()
const year = Number(route.query.rok)
const month = Number(route.query.mesiac)
const view = ref(null)
const error = ref('')
const company = computed(() => session.company ?? {})
const rows = computed(() => (view.value?.rows ?? []).filter((r) => !route.query.id || r.employee.id === Number(route.query.id)))

onMounted(async () => {
  try {
    view.value = await api.payroll(session.companyId, year, month)
    document.title = `Výplatné pásky ${periodLabel(year, month)}`
  } catch (e) {
    error.value = e.fields?.[0]?.message ?? e.message
  }
})
const back = () => router.push({ path: '/mzdy', query: { rok: year, mesiac: month } })
const nz = (v) => Number(v) !== 0
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

    <article v-for="r in rows" :key="r.employee.id" class="sheet">
      <header class="head">
        <div>
          <p class="kind">Výplatná páska</p>
          <h1>{{ periodLabel(year, month) }}</h1>
        </div>
        <div class="company">
          <strong>{{ company.name }}</strong>
          <span v-if="company.ico">IČO {{ company.ico }}</span>
        </div>
      </header>
      <dl class="who">
        <div><dt>Zamestnanec</dt><dd>{{ employeeName(r.employee) }}</dd></div>
        <div><dt>Pracovný vzťah</dt><dd>{{ contractLabel(r.employee.contract) }}<template v-if="r.employee.position">, {{ r.employee.position }}</template></dd></div>
        <div><dt>Od</dt><dd>{{ formatDate(r.employee.start) }}</dd></div>
        <div><dt>Zdravotná poisťovňa</dt><dd>{{ insurerLabel(r.employee.insurer) }}</dd></div>
      </dl>
      <div class="cols">
        <table>
          <tbody>
            <tr v-if="r.employee.hourly"><td>Odpracované hodiny</td><td>{{ formatAmount(r.input.hours) }}</td></tr>
            <tr><td>Základná mzda</td><td>{{ formatAmount(r.result.baseWage) }}</td></tr>
            <tr v-if="nz(r.input.extra)"><td>Odmeny a príplatky</td><td>{{ formatAmount(r.input.extra) }}</td></tr>
            <tr v-if="nz(r.input.leave)"><td>Náhrada mzdy</td><td>{{ formatAmount(r.input.leave) }}</td></tr>
            <tr class="sum"><td>Hrubá mzda</td><td>{{ formatAmount(r.result.gross) }}</td></tr>
            <tr v-if="nz(r.result.sickPay)"><td>Náhrada príjmu pri PN</td><td>{{ formatAmount(r.result.sickPay) }}</td></tr>
            <tr v-for="f in r.result.social.filter((x) => nz(x.employee))" :key="f.fund"><td class="in">{{ f.name }} poistenie</td><td>−{{ formatAmount(f.employee) }}</td></tr>
            <tr><td class="in">Zdravotné poistenie</td><td>−{{ formatAmount(r.result.healthEmployee) }}</td></tr>
          </tbody>
        </table>
        <table>
          <tbody>
            <tr><td>Základ dane</td><td>{{ formatAmount(r.result.taxBase) }}</td></tr>
            <tr v-if="nz(r.result.nczd)"><td class="in">Nezdaniteľná časť</td><td>{{ formatAmount(r.result.nczd) }}</td></tr>
            <tr><td>Preddavok na daň</td><td>{{ nz(r.result.tax) ? '−' : '' }}{{ formatAmount(r.result.tax) }}</td></tr>
            <tr v-if="nz(r.result.taxBonus)"><td>Daňový bonus na deti</td><td>+{{ formatAmount(r.result.taxBonus) }}</td></tr>
            <tr class="sum"><td>Čistá mzda</td><td>{{ formatAmount(r.result.net) }}</td></tr>
            <tr v-if="nz(r.result.deduction)"><td>Zrážky</td><td>−{{ formatAmount(r.result.deduction) }}</td></tr>
            <tr class="total"><td>Na výplatu</td><td><span class="ledger">{{ formatMoney(r.result.toPay) }}</span></td></tr>
            <tr v-if="r.employee.iban"><td class="in">na účet</td><td class="iban">{{ r.employee.iban }}</td></tr>
          </tbody>
        </table>
      </div>
      <p class="er">Zamestnávateľ za vás navyše odviedol poistné {{ formatMoney(r.result.contribER) }} (sociálne {{ formatAmount(r.result.socialEmployer) }}, zdravotné {{ formatAmount(r.result.healthEmployer) }}). Cena práce {{ formatMoney(r.result.cost) }}.</p>
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
/* polovica A4: dve pásky na hárok */
.sheet {
  width: 210mm;
  max-width: 100%;
  min-height: 140mm;
  margin: 24px auto 0;
  padding: 10mm 14mm;
  background: #fff;
  color: #1d2633;
  font-size: 9.5pt;
  line-height: 1.4;
  box-shadow: 0 1px 3px rgba(18, 24, 32, 0.12), 0 8px 28px rgba(18, 24, 32, 0.08);
  break-inside: avoid;
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
  font-size: 8.5pt;
  color: #4a5a6c;
}
h1 {
  margin: 0;
  font-size: 15pt;
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
  grid-template-columns: repeat(4, 1fr);
  gap: 4px 12px;
  margin: 8px 0;
}
.who dt {
  font-size: 7.5pt;
  color: #4a5a6c;
}
.who dd {
  margin: 0;
}
.cols {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10mm;
}
.cols table {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.cols td {
  padding: 2px 0;
}
.cols td:last-child {
  text-align: right;
  white-space: nowrap;
}
.cols .in {
  padding-left: 8px;
  color: #4a5a6c;
}
.cols .sum td {
  border-top: 1px solid #d5dbe2;
  font-weight: 600;
}
.cols .total td {
  border-top: 2px solid #1d2633;
  font-weight: 700;
  font-size: 11pt;
  padding-top: 4px;
}
.ledger {
  border-bottom: 3px double currentColor;
}
.iban {
  font-size: 8.5pt;
}
.er {
  margin: 8px 0 0;
  font-size: 8pt;
  color: #4a5a6c;
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
    box-shadow: none;
    margin: 0;
    width: auto;
    min-height: 0;
    height: 148mm;
    page-break-inside: avoid;
  }
}
@media (max-width: 640px) {
  .sheet {
    padding: 16px;
  }
  .who,
  .cols {
    grid-template-columns: 1fr 1fr;
  }
  .cols {
    grid-template-columns: 1fr;
    gap: 8px;
  }
}
</style>
