<script setup>
// Domácnosť — prehľad: zostatky účtov, čerpanie rozpočtu v mesiaci, príjmy
// a výdavky po mesiacoch a posledné pohyby. Všetky sumy počíta server.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import {
  accountIcon, budgetTone, budgetWidth, chartBars, entryCategory, entryType, monthNames, newEntry,
} from '../lib/household.js'
import HouseholdEntryDialog from '../components/HouseholdEntryDialog.vue'
import ObligationPayDialog from '../components/ObligationPayDialog.vue'
import HouseholdReceiptDialog from '../components/HouseholdReceiptDialog.vue'
import UpcomingEvents from '../components/UpcomingEvents.vue'
import { toISODate } from '../lib/format.js'

const router = useRouter()
const canWrite = computed(() => can('ledger.write'))
const now = new Date()
const year = ref(now.getFullYear())
const month = ref(now.getMonth() + 1)
const years = [0, 1, 2].map((d) => ({ label: String(now.getFullYear() - d), value: now.getFullYear() - d }))
const months = monthNames.map((m, i) => ({ label: m, value: i + 1 }))

const ov = ref(null)
const entries = ref([])
const categories = ref([])
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  try {
    const [o, es, cats] = await Promise.all([
      api.hhSummary(session.companyId, year.value, month.value),
      api.hhEntries(session.companyId, year.value),
      api.hhCategories(session.companyId),
    ])
    ov.value = o
    entries.value = es
    categories.value = cats
    error.value = ''
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
watch([year, month], load)
onMounted(load)

// termíny z kalendára (vlastné, dane…) na 14 dní; splátky sú v Najbližších platbách
const cal = ref(null)
onMounted(async () => {
  const from = toISODate(new Date())
  const to = toISODate(new Date(Date.now() + 14 * 86400000))
  cal.value = await api.calendar(session.companyId, from, to).catch(() => null)
})
const notPay = (e) => e.kind !== 'pay' && e.kind !== 'receive' && e.kind !== 'holiday' && !e.done
const calEvents = computed(() => (cal.value?.events ?? []).filter(notPay))
const calOverdue = computed(() => (cal.value?.overdue ?? []).filter(notPay))

const accounts = computed(() => ov.value?.accounts ?? [])
const openAccounts = computed(() => accounts.value.filter((a) => !a.closed))
const bars = computed(() => chartBars(ov.value?.income, ov.value?.expense))
const recent = computed(() => [...entries.value].sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : b.id - a.id)).slice(0, 8))
const monthCats = computed(() =>
  (ov.value?.categories ?? []).filter((c) => !c.income && (Number(c.total) !== 0 || Number(c.budget) > 0)),
)
const incomeCats = computed(() => (ov.value?.categories ?? []).filter((c) => c.income && Number(c.total) !== 0))
const payees = computed(() => [...new Set(entries.value.map((e) => e.payee).filter(Boolean))])

const editing = ref(null)
const receiptOpen = ref(false)
function add(type) {
  editing.value = newEntry(type, accounts.value)
}
function show(cat) {
  router.push({ path: '/domacnost/pohyby', query: { kategoria: cat, rok: year.value } })
}
const amountClass = (e) => entryType(e)

// najbližšie splátky záväzkov (do mesiaca, aj po splatnosti)
const upcoming = computed(() => ov.value?.upcoming ?? [])
const paying = ref(null)
function pay(d) {
  paying.value = { ...d, id: d.obligationId, amount: d.rest }
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>{{ session.company?.name || 'Domácnosť' }}</h1>
      <div class="spacer" />
      <Select v-model="month" :options="months" option-label="label" option-value="value" size="small" aria-label="Mesiac" />
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <template v-if="canWrite && openAccounts.length">
        <Button label="Výdavok" icon="pi pi-minus" size="small" @click="add('expense')" />
        <Button label="Príjem" icon="pi pi-plus" size="small" severity="secondary" outlined @click="add('income')" />
        <Button label="Bloček" icon="pi pi-receipt" size="small" severity="secondary" outlined @click="receiptOpen = true" />
        <Button label="Prevod" icon="pi pi-arrow-right-arrow-left" size="small" severity="secondary" text @click="add('transfer')" />
      </template>
    </header>
    <p v-if="error" class="error">{{ error }}</p>

    <section v-if="ov && !accounts.length" class="start">
      <h2>Začnite účtami domácnosti</h2>
      <p>
        Pridajte, kde máte peniaze: bežný účet, peňaženku, kreditnú kartu, sporenie. Pri každom zadáte počiatočný stav — od neho
        Fiškus počíta zostatky. Potom zapisujte výdavky a príjmy alebo nahrajte výpis z banky.
      </p>
      <Button v-if="canWrite" label="Pridať účet" icon="pi pi-plus" size="small" @click="router.push('/domacnost/ucty')" />
    </section>

    <template v-else-if="ov">
      <!-- zostatky -->
      <section class="balances" aria-label="Zostatky účtov">
        <button v-for="a in openAccounts" :key="a.id" class="acc" type="button" @click="router.push({ path: '/domacnost/pohyby', query: { ucet: a.id } })">
          <span class="acc-name"><i class="pi" :class="accountIcon(a.kind)" aria-hidden="true" />{{ a.name }}</span>
          <span class="acc-bal num" :class="{ neg: Number(a.balance) < 0 }">{{ formatMoney(a.balance) }}</span>
        </button>
        <div class="acc total">
          <span class="acc-name">Spolu</span>
          <span class="acc-bal num"><span class="ledger">{{ formatMoney(ov.total) }}</span></span>
        </div>
      </section>

      <div class="cols">
      <!-- najbližšie platby -->
      <section class="panel upcoming" aria-label="Najbližšie platby">
        <header>
          <h2>Najbližšie platby</h2>
          <router-link to="/domacnost/zavazky" class="link">Záväzky a platby</router-link>
        </header>
        <p v-if="!upcoming.length" class="empty">Na najbližší mesiac nič na zaplatenie.</p>
        <table v-else class="moves">
          <tbody>
            <tr v-for="d in upcoming" :key="d.obligationId + '-' + d.n" :class="{ late: d.status === 'overdue' }" @click="router.push(`/domacnost/zavazky/${d.obligationId}`)">
              <td class="d">{{ formatDate(d.date) }}</td>
              <td class="w"><span class="who">{{ d.name }}</span><small>{{ d.status === 'overdue' ? 'po splatnosti' : d.kind === 'lent' ? 'má vrátiť' : d.creditor }}</small></td>
              <td class="num amt" :class="d.kind === 'lent' ? 'income' : 'expense'">{{ formatAmount(d.rest) }}</td>
              <td class="pay" @click.stop>
                <Button v-if="canWrite" :label="d.kind === 'lent' ? 'Prijať' : 'Zaplatiť'" size="small" text @click="pay(d)" />
              </td>
            </tr>
          </tbody>
        </table>
      </section>
      <UpcomingEvents :events="calEvents" :overdue="calOverdue" :today="cal?.today ?? ''" />
      </div>

      <div class="cols">
        <!-- rozpočet mesiaca -->
        <section class="panel budget" :aria-label="`Rozpočet – ${monthNames[ov.month - 1]}`">
          <header>
            <h2>Výdavky a rozpočet — {{ monthNames[ov.month - 1] }}</h2>
            <router-link v-if="canWrite" to="/domacnost/rozpocet" class="link">Upraviť rozpočet</router-link>
          </header>
          <p v-if="!ov.budget.length" class="empty">V tomto mesiaci zatiaľ žiadne výdavky.</p>
          <ul v-else class="blist">
            <li v-for="b in ov.budget" :key="b.name" :data-tone="budgetTone(b)">
              <button type="button" class="bname" @click="show(b.name)">{{ b.name }}</button>
              <span class="bamt num">
                {{ formatAmount(b.actual) }}<template v-if="Number(b.budget) > 0"> / {{ formatAmount(b.budget) }}</template>
              </span>
              <span class="bar" :class="{ nobudget: !(Number(b.budget) > 0) }"><span :style="{ width: budgetWidth(b) + '%' }" /></span>
              <small v-if="Number(b.budget) > 0" class="bleft">
                <template v-if="b.over">prečerpané o {{ formatMoney(String(-Number(b.left))) }}</template>
                <template v-else>zostáva {{ formatMoney(b.left) }}</template>
              </small>
            </li>
          </ul>
        </section>

        <!-- rok -->
        <section class="panel year" :aria-label="`Rok ${ov.year}`">
          <header><h2>Príjmy a výdavky {{ ov.year }}</h2></header>
          <div class="chart" role="img" :aria-label="`Graf príjmov a výdavkov po mesiacoch roka ${ov.year}`">
            <div v-for="(b, i) in bars" :key="b.month" class="mcol" :class="{ cur: i + 1 === ov.month }" @click="month = i + 1">
              <div class="pair">
                <span class="in" :style="{ height: b.income + '%' }" :title="`Príjmy ${monthNames[i]}: ${formatMoney(ov.income[i])}`" />
                <span class="out" :style="{ height: b.expense + '%' }" :title="`Výdavky ${monthNames[i]}: ${formatMoney(ov.expense[i])}`" />
              </div>
              <small>{{ b.month }}</small>
            </div>
          </div>
          <dl class="totals">
            <div><dt><i class="dot in" />Príjmy</dt><dd class="num">{{ formatMoney(ov.totalIn) }}</dd></div>
            <div><dt><i class="dot out" />Výdavky</dt><dd class="num">{{ formatMoney(ov.totalOut) }}</dd></div>
            <div class="net"><dt>Rozdiel</dt><dd class="num" :class="{ neg: Number(ov.net) < 0 }"><span class="ledger">{{ formatMoney(ov.net) }}</span></dd></div>
          </dl>
          <table v-if="incomeCats.length || monthCats.length" class="cats">
            <thead><tr><th>Kategória</th><th class="num">{{ monthNames[ov.month - 1] }}</th><th class="num">Rok</th></tr></thead>
            <tbody>
              <tr v-for="c in incomeCats" :key="'i' + c.name" class="inc" @click="show(c.name)">
                <td>{{ c.name }}</td><td class="num">{{ formatAmount(c.months[ov.month - 1]) }}</td><td class="num">{{ formatAmount(c.total) }}</td>
              </tr>
              <tr v-for="c in monthCats" :key="'e' + c.name" @click="show(c.name)">
                <td>{{ c.name }}</td><td class="num">{{ formatAmount(c.months[ov.month - 1]) }}</td><td class="num">{{ formatAmount(c.total) }}</td>
              </tr>
            </tbody>
          </table>
        </section>
      </div>

      <!-- posledné pohyby -->
      <section class="panel recent" aria-label="Posledné pohyby">
        <header>
          <h2>Posledné pohyby</h2>
          <router-link to="/domacnost/pohyby" class="link">Všetky pohyby</router-link>
        </header>
        <p v-if="!recent.length" class="empty">Zatiaľ žiadne pohyby. Zapíšte výdavok alebo nahrajte výpis v Pohyboch.</p>
        <table v-else class="moves">
          <tbody>
            <tr v-for="e in recent" :key="e.id" @click="editing = e">
              <td class="d">{{ formatDate(e.date) }}</td>
              <td class="w"><span class="who">{{ e.payee || e.note || '—' }}</span><small>{{ entryCategory(e, accounts) }}</small></td>
              <td class="num amt" :class="amountClass(e)">{{ formatAmount(e.amount) }}</td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>

    <HouseholdEntryDialog :entry="editing" :accounts="accounts" :categories="categories" :payees="payees" :can-write="canWrite"
      @close="editing = null" @saved="load" />
    <HouseholdReceiptDialog v-model:visible="receiptOpen" :accounts="accounts" :categories="categories" @saved="load" />
    <ObligationPayDialog :target="paying" :accounts="accounts" @close="paying = null" @paid="load" />
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
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
.start {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 18px;
  max-width: 64ch;
}
.start h2 {
  font-size: var(--fs-lg);
  margin-bottom: 6px;
}
.start p {
  color: var(--ink-soft);
  margin-bottom: 12px;
}

/* zostatky — pás ako hlavička knihy */
.balances {
  display: flex;
  flex-wrap: wrap;
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.acc {
  flex: 1 1 160px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 9px 12px;
  border: 0;
  border-right: 1px solid var(--rule-soft);
  background: none;
  text-align: left;
  font: inherit;
  color: inherit;
  cursor: pointer;
}
.acc:hover {
  background: var(--blue-wash);
}
.acc-name {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  display: flex;
  align-items: center;
  gap: 6px;
}
.acc-name .pi {
  font-size: 0.8rem;
  color: var(--blue);
}
.acc-bal {
  font-size: var(--fs-lg);
  font-weight: 600;
  text-align: left;
}
.acc.total {
  cursor: default;
  background: var(--blue-wash);
  border-right: 0;
}
.acc.total .acc-bal {
  color: var(--blue-deep);
}
.ledger {
  border-bottom: 3px double currentColor;
  padding-bottom: 1px;
}
.neg {
  color: var(--tone-overdue);
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
.link {
  margin-left: auto;
  font-size: var(--fs-sm);
  color: var(--blue);
}
.empty {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}

/* rozpočet */
.blist {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.blist li {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 3px 10px;
  --c: var(--blue);
}
.blist li[data-tone='near'] {
  --c: var(--tone-draft);
}
.blist li[data-tone='over'] {
  --c: var(--tone-overdue);
}
.bname {
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  color: var(--ink);
  text-align: left;
  cursor: pointer;
}
.bname:hover {
  text-decoration: underline;
}
.bamt {
  font-size: var(--fs-sm);
}
.blist li[data-tone='over'] .bamt {
  color: var(--tone-overdue);
  font-weight: 600;
}
.bar {
  grid-column: 1 / -1;
  height: 6px;
  background: var(--rule-soft);
  overflow: hidden;
}
.bar span {
  display: block;
  height: 100%;
  background: var(--c);
}
.bar.nobudget {
  height: 3px;
}
.bar.nobudget span {
  background: var(--ink-faint);
  opacity: 0.3;
}
.bleft {
  grid-column: 1 / -1;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.blist li[data-tone='over'] .bleft {
  color: var(--tone-overdue);
}

/* graf */
.chart {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 4px;
  height: 120px;
  border-bottom: 1px solid var(--rule);
  margin-bottom: 8px;
}
.mcol {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  cursor: pointer;
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
  max-width: 12px;
  min-height: 0;
}
.pair .in {
  background: var(--blue);
}
.pair .out {
  background: var(--tone-overdue);
  opacity: 0.85;
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
.totals {
  display: flex;
  gap: 18px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.totals div {
  display: flex;
  flex-direction: column;
}
.totals dt {
  font-size: var(--fs-xs);
  color: var(--ink-soft);
  display: flex;
  align-items: center;
  gap: 5px;
}
.totals dd {
  margin: 0;
  font-weight: 600;
  text-align: left;
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
  background: var(--tone-overdue);
}
.cats,
.moves {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
}
.cats th {
  font-weight: 600;
  color: var(--ink-soft);
  text-align: left;
  border-bottom: 1px solid var(--rule);
  padding: 3px 4px;
}
.cats th.num {
  text-align: right;
}
.cats td,
.moves td {
  padding: 4px;
  border-bottom: 1px solid var(--rule-soft);
}
.cats tr,
.moves tr {
  cursor: pointer;
}
.cats tbody tr:hover,
.moves tr:hover {
  background: var(--blue-wash);
}
.cats tr.inc td:first-child {
  color: var(--blue-deep);
}
.moves .d {
  width: 90px;
  color: var(--ink-soft);
  white-space: nowrap;
}
.moves .w {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.who {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.moves small {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.amt {
  font-weight: 600;
}
.amt.expense {
  color: var(--tone-overdue);
}
.amt.income {
  color: var(--tone-paid);
}
.upcoming tr.late .d,
.upcoming tr.late small {
  color: var(--tone-overdue);
  font-weight: 600;
}
.moves .pay {
  width: 1%;
  white-space: nowrap;
  text-align: right;
}
.amt.transfer {
  color: var(--ink-faint);
}

@media (max-width: 860px) {
  .cols {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 560px) {
  .head h1 {
    width: 100%;
  }
  .acc {
    flex-basis: 45%;
    border-bottom: 1px solid var(--rule-soft);
  }
  .acc.total {
    flex-basis: 100%;
  }
}
</style>
