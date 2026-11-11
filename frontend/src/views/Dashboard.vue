<script setup>
// Prehľad firmy: peniaze, pohľadávky a záväzky, príjmy a výdavky po
// mesiacoch, najbližšie termíny a rozpracovaná práca na jednej stránke.
// Všetky sumy počíta server (časti bez práva používateľa chýbajú).
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import UpcomingEvents from '../components/UpcomingEvents.vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { monthNames } from '../lib/household.js'
import { dueText, monthBars, openItemLink, workRows } from '../lib/dashboard.js'

const router = useRouter()
const now = new Date()
const year = ref(now.getFullYear())
const years = [0, 1, 2].map((d) => ({ label: String(now.getFullYear() - d), value: now.getFullYear() - d }))
const d = ref(null)
const error = ref('')

async function load() {
  try {
    d.value = await api.dashboard(session.companyId, year.value)
    error.value = ''
  } catch (e) {
    error.value = e.message
  }
}
onMounted(load)
watch(year, load)

const bars = computed(() => monthBars(d.value?.months ?? []))
const work = computed(() => workRows(d.value?.work))
const thisYear = computed(() => year.value === now.getFullYear())
const curMonth = now.getMonth()
const net = computed(() => (d.value?.net ? ' bez DPH' : ''))
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>{{ session.company?.name || 'Prehľad' }}</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <Button v-if="can('invoices.write')" label="Faktúra" icon="pi pi-plus" size="small" @click="router.push('/faktury/nova')" />
      <Button v-if="can('purchases.write')" label="Prijatá faktúra" icon="pi pi-file-import" size="small" severity="secondary" outlined
        @click="router.push('/prijate/nova')" />
    </header>
    <p v-if="error" class="error">{{ error }}</p>

    <template v-if="d">
      <!-- hlavné čísla -->
      <section class="ledger" aria-label="Hlavné čísla">
        <RouterLink v-if="d.money" class="fig" to="/banka">
          <span class="label">Peniaze</span>
          <span class="value">{{ formatMoney(d.money.total) }}</span>
          <span class="sub">banka a pokladňa{{ thisYear ? '' : ` k 31. 12. ${d.year}` }}</span>
        </RouterLink>
        <RouterLink v-if="d.receivables" class="fig" to="/pohladavky-zavazky">
          <span class="label">Pohľadávky ({{ d.receivables.count }})</span>
          <span class="value">{{ formatMoney(d.receivables.total) }}</span>
          <span v-if="d.receivables.overdueCount" class="sub late">po splatnosti {{ formatMoney(d.receivables.overdue) }}</span>
          <span v-else class="sub">nič po splatnosti</span>
        </RouterLink>
        <RouterLink v-if="d.payables" class="fig" to="/pohladavky-zavazky">
          <span class="label">Záväzky ({{ d.payables.count }})</span>
          <span class="value">{{ formatMoney(d.payables.total) }}</span>
          <span v-if="d.payables.overdueCount" class="sub late">po splatnosti {{ formatMoney(d.payables.overdue) }}</span>
          <span v-else class="sub">nič po splatnosti</span>
        </RouterLink>
        <template v-if="d.months">
          <div class="fig">
            <span class="label">Príjmy {{ d.year }}</span>
            <span class="value">{{ formatMoney(d.income) }}</span>
            <span class="sub">vydané faktúry{{ net }}</span>
          </div>
          <div class="fig">
            <span class="label">Výdavky {{ d.year }}</span>
            <span class="value">{{ formatMoney(d.expense) }}</span>
            <span class="sub">prijaté faktúry{{ net }}</span>
          </div>
          <div class="fig result">
            <span class="label">Rozdiel</span>
            <span class="value"><span class="double">{{ formatMoney(d.result) }}</span></span>
            <span class="sub">príjmy − výdavky</span>
          </div>
        </template>
      </section>

      <div class="cols">
        <!-- graf -->
        <section v-if="d.months" class="panel" :aria-label="`Príjmy a výdavky ${d.year}`">
          <header><h2>Príjmy a výdavky {{ d.year }}</h2><span class="muted">{{ d.net ? 'bez DPH' : 'faktúry' }}</span></header>
          <div class="chart" role="img" :aria-label="`Graf príjmov a výdavkov po mesiacoch roka ${d.year}`">
            <div v-for="(b, i) in bars" :key="b.month" class="mcol" :class="{ cur: thisYear && i === curMonth }">
              <div class="pair">
                <span class="in" :style="{ height: b.income + '%' }" :title="`Príjmy ${monthNames[i]}: ${formatMoney(d.months[i].income)}`" />
                <span class="out" :style="{ height: b.expense + '%' }" :title="`Výdavky ${monthNames[i]}: ${formatMoney(d.months[i].expense)}`" />
              </div>
              <small>{{ b.month }}</small>
            </div>
          </div>
          <p class="legend"><i class="dot in" />príjmy <i class="dot out" />výdavky</p>
        </section>

        <UpcomingEvents :events="d.events" :overdue="d.overdue" :today="d.today" />
      </div>

      <div class="cols">
        <section v-if="d.receivables" class="panel" aria-label="Na úhradu od odberateľov">
          <header><h2>Neuhradené vydané</h2><RouterLink to="/faktury" class="link">Vydané faktúry</RouterLink></header>
          <p v-if="!d.receivables.top.length" class="empty">Všetko je uhradené.</p>
          <table v-else class="rows">
            <tbody>
              <tr v-for="it in d.receivables.top" :key="it.kind + it.id" :class="{ late: it.daysOverdue > 0 }" @click="router.push(openItemLink(it))">
                <td class="w"><span class="t">{{ it.partner }}</span><small>{{ it.number }} · splatná {{ formatDate(it.dueDate) }}</small></td>
                <td class="num">{{ formatAmount(it.remainingEur) }}<small>{{ dueText(it) }}</small></td>
              </tr>
            </tbody>
          </table>
        </section>
        <section v-if="d.payables" class="panel" aria-label="Na úhradu dodávateľom">
          <header><h2>Neuhradené prijaté</h2><RouterLink to="/prijate" class="link">Prijaté faktúry</RouterLink></header>
          <p v-if="!d.payables.top.length" class="empty">Všetko je zaplatené.</p>
          <table v-else class="rows">
            <tbody>
              <tr v-for="it in d.payables.top" :key="it.kind + it.id" :class="{ late: it.daysOverdue > 0 }" @click="router.push(openItemLink(it))">
                <td class="w"><span class="t">{{ it.partner }}</span><small>{{ it.number }} · splatná {{ formatDate(it.dueDate) }}</small></td>
                <td class="num">{{ formatAmount(it.remainingEur) }}<small>{{ dueText(it) }}</small></td>
              </tr>
            </tbody>
          </table>
        </section>
      </div>

      <div class="cols">
        <section v-if="d.money" class="panel" aria-label="Peniaze">
          <header><h2>Peniaze</h2><RouterLink to="/banka" class="link">Banka</RouterLink></header>
          <p v-if="!d.money.items.length" class="empty">Zatiaľ žiadny pohyb v banke ani v pokladni.</p>
          <table v-else class="rows">
            <tbody>
              <tr v-for="m in d.money.items" :key="m.kind + m.label" @click="router.push(m.kind === 'cash' ? '/pokladna' : '/banka')">
                <td class="w"><span class="t"><i class="pi" :class="m.kind === 'cash' ? 'pi-wallet' : 'pi-building-columns'" aria-hidden="true" /> {{ m.label }}</span>
                  <small>{{ m.kind === 'cash' ? 'pokladnica' : 'bankový účet' }}<template v-if="m.account"> · {{ m.account }}</template></small></td>
                <td class="num">{{ formatAmount(m.amount) }}</td>
              </tr>
              <tr class="total"><td>Spolu</td><td class="num"><span class="double">{{ formatAmount(d.money.total) }}</span></td></tr>
            </tbody>
          </table>
        </section>
        <section v-if="d.work" class="panel" aria-label="Rozpracované">
          <header><h2>Rozpracované</h2></header>
          <p v-if="!work.length" class="empty">Nič rozpracované — žiadne koncepty, bežiace zákazky ani objednávky.</p>
          <table v-else class="rows">
            <tbody>
              <tr v-for="r in work" :key="r.to + r.label" :class="{ late: r.late }" @click="router.push(r.to)">
                <td class="w"><span class="t">{{ r.label }}</span><small v-if="r.sub">{{ r.sub }}</small></td>
                <td class="num">{{ r.amount ? formatAmount(r.amount) : '' }}</td>
              </tr>
            </tbody>
          </table>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 1180px;
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
}
.error {
  color: var(--tone-overdue);
}
.ledger {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}
.fig {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 8px 14px 9px;
  border-right: 1px solid var(--rule);
  color: inherit;
  text-decoration: none;
  min-width: 0;
}
a.fig:hover {
  background: var(--blue-wash);
}
.fig .label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.fig .value {
  font-size: var(--fs-xl);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.fig .sub {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.fig .sub.late {
  color: var(--tone-overdue);
  font-weight: 600;
}
.fig.result {
  background: var(--blue-wash);
}
.double {
  border-bottom: 3px double currentColor;
  padding-bottom: 1px;
}
.fig.result .value {
  margin-bottom: 4px;
}
.cols {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 12px;
}
.panel {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 10px 12px 12px;
  min-width: 0;
}
.panel > header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 8px;
}
.panel h2 {
  font-size: var(--fs-md);
  font-weight: 600;
  color: var(--blue-deep);
}
.muted {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.link {
  margin-left: auto;
  font-size: var(--fs-sm);
  color: var(--blue);
}
.empty {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.chart {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 4px;
  height: 220px;
  border-bottom: 1px solid var(--rule);
}
.mcol {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.pair {
  flex: 1;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  gap: 2px;
}
.pair span {
  width: 42%;
  max-width: 14px;
}
.pair .in {
  background: var(--blue);
}
.pair .out {
  background: var(--ink-faint);
}
.mcol small {
  text-align: center;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  padding-top: 2px;
}
.mcol.cur small {
  color: var(--ink);
  font-weight: 700;
  border-top: 2px solid var(--blue-deep);
}
.legend {
  margin: 6px 0 0;
  font-size: var(--fs-xs);
  color: var(--ink-soft);
  display: flex;
  align-items: center;
  gap: 5px;
}
.dot {
  width: 8px;
  height: 8px;
  display: inline-block;
}
.dot.in {
  background: var(--blue);
}
.dot.out {
  background: var(--ink-faint);
  margin-left: 8px;
}
.rows {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.rows td {
  padding: 5px 4px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: top;
}
.rows tr {
  cursor: pointer;
}
.rows tr:hover {
  background: var(--blue-wash);
}
.rows tr.total {
  cursor: default;
  font-weight: 600;
}
.rows tr.total:hover {
  background: none;
}
.w {
  min-width: 0;
}
.t {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.t .pi {
  font-size: 0.8rem;
  color: var(--blue);
}
.rows small {
  display: block;
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.num {
  text-align: right;
  white-space: nowrap;
  font-weight: 600;
}
.late .num small,
.late .w small {
  color: var(--tone-overdue);
  font-weight: 600;
}
@media (max-width: 860px) {
  .cols {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 560px) {
  .page {
    padding: 10px 10px 20px;
  }
  .head h1 {
    width: 100%;
  }
  .ledger {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .fig .value {
    font-size: var(--fs-lg);
  }
}
</style>
