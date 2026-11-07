<script setup>
// Kalkulačky pre každého — fungujú aj bez založenej firmy. Výpočty robí
// server (/api/calc), obrazovka iba skladá vstup a zobrazuje výsledok.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import DatePicker from 'primevue/datepicker'
import Checkbox from 'primevue/checkbox'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { formatAmount } from '../lib/format.js'
import { loanScheduleColumns, savingsYearColumns } from '../lib/columns.js'
import {
  calculators, contractOptions, debtorOptions, defaults, filterSavingsYears, filterSchedule, pensionOptions, resultRows, toRequest,
} from '../lib/calc.js'

const route = useRoute()
const router = useRouter()
const info = ref({})
const forms = ref(defaults(new Date()))
const active = computed({
  get: () => (calculators.some((c) => c.key === route.query.k) ? route.query.k : 'vat'),
  set: (k) => router.replace({ query: { k } }),
})
const current = computed(() => calculators.find((c) => c.key === active.value))
const result = ref(null)
const error = ref('')
const errors = ref({})
const busy = ref(false)

onMounted(async () => {
  try {
    info.value = await api.calcInfo()
    forms.value = defaults(new Date(), info.value)
  } catch (e) {
    error.value = e.message
  }
})

const endpoints = { vat: 'vat', late: 'late-interest', loan: 'loan', savings: 'savings', wage: 'wage', fx: 'fx' }
let timer = null
let seq = 0
function compute() {
  clearTimeout(timer)
  timer = setTimeout(async () => {
    const key = active.value
    const body = toRequest(key, forms.value[key])
    const my = ++seq
    if (!body) {
      result.value = null
      errors.value = {}
      error.value = ''
      return
    }
    busy.value = true
    try {
      const r = await api.calc(endpoints[key], body)
      if (my !== seq) return
      result.value = { key, data: r }
      errors.value = {}
      error.value = ''
    } catch (e) {
      if (my !== seq) return
      result.value = null
      errors.value = e instanceof ApiError ? e.fieldMap() : {}
      error.value = e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message
    } finally {
      if (my === seq) busy.value = false
    }
  }, 250)
}
watch([forms, active], compute, { deep: true, immediate: true })

const f = computed(() => forms.value[active.value])
const rows = computed(() => (result.value?.key === active.value ? resultRows(active.value, result.value.data, f.value) : []))
const vatRates = computed(() => (info.value.vatRates ?? [23, 19, 5]).filter((r) => r > 0).map((r) => ({ value: r, label: `${r} %` })))
const yearOptions = computed(() => (info.value.wageYears ?? []).map((y) => ({ value: y, label: String(y) })))
const monthOptions = Array.from({ length: 12 }, (_, i) => ({ value: i + 1, label: new Date(2026, i, 1).toLocaleString('sk-SK', { month: 'long' }) }))
const currencyOptions = computed(() => (info.value.currencies ?? []).filter((c) => c.code !== 'EUR').map((c) => ({ value: c.code, label: `${c.code} – ${c.name}` })))
const lawHint = computed(() => {
  const l = info.value.law
  if (!l) return ''
  return `Sadzba ECB dnes ${formatAmount(l.ecb)} %. Podnikatelia ECB + ${formatAmount(l.businessMargin)} p. b. a paušál ${formatAmount(l.flatFee)} €, občania ECB + ${formatAmount(l.consumerMargin)} p. b. Sadzba platná k prvému dňu omeškania sa nemení.`
})

// zoznamy pod výsledkom: splátkový kalendár, sporenie po rokoch
const q = ref('')
watch(active, () => (q.value = ''))
const schedule = computed(() => (result.value?.key === 'loan' ? filterSchedule(result.value.data.schedule, q.value) : []))
const savingYears = computed(() => (result.value?.key === 'savings' ? filterSavingsYears(result.value.data.years, q.value) : []))

function addChild() {
  forms.value.wage.children.push(5)
}
</script>

<template>
  <div class="page calc">
    <header class="head">
      <h1>Kalkulačky</h1>
      <Select v-model="active" :options="calculators" option-label="label" option-value="key" size="small" class="pick" aria-label="Kalkulačka" />
    </header>

    <div class="layout">
      <nav class="list" aria-label="Kalkulačky">
        <button v-for="c in calculators" :key="c.key" type="button" :class="{ on: c.key === active }" :aria-current="c.key === active ? 'page' : undefined" @click="active = c.key">
          <i class="pi" :class="c.icon" aria-hidden="true" />
          <span>{{ c.label }}</span>
        </button>
      </nav>

      <section class="work">
        <p class="hint">{{ current.hint }}</p>
        <div class="grid">
          <form class="form-grid" @submit.prevent>
            <!-- DPH -->
            <template v-if="active === 'vat'">
              <FormField label="Zadaná suma" :span="2">
                <SelectButton v-model="f.gross" :options="[{ value: false, label: 'bez DPH' }, { value: true, label: 's DPH' }]" option-label="label" option-value="value" :allow-empty="false" size="small" />
              </FormField>
              <FormField label="Suma" :error="errors.amount">
                <InputText v-model="f.amount" inputmode="decimal" size="small" />
              </FormField>
              <FormField label="Sadzba DPH" :error="errors.rate">
                <SelectButton v-model="f.rate" :options="vatRates" option-label="label" option-value="value" :allow-empty="false" size="small" />
              </FormField>
            </template>

            <!-- úrok z omeškania -->
            <template v-else-if="active === 'late'">
              <FormField label="Vzťah" :span="2" :hint="lawHint">
                <Select v-model="f.debtor" :options="debtorOptions" option-label="label" option-value="value" size="small" fluid />
              </FormField>
              <FormField label="Dlžná suma" :error="errors.amount">
                <InputText v-model="f.amount" inputmode="decimal" size="small" />
              </FormField>
              <FormField v-if="f.debtor === 'custom'" label="Sadzba % p. a." :error="errors.rate">
                <InputText v-model="f.rate" inputmode="decimal" size="small" />
              </FormField>
              <span v-else />
              <FormField label="Splatné do" :error="errors.due">
                <DatePicker v-model="f.due" date-format="d. m. yy" size="small" fluid />
              </FormField>
              <FormField label="Zaplatené (alebo ku dňu)" :error="errors.paid">
                <DatePicker v-model="f.paid" date-format="d. m. yy" size="small" fluid />
              </FormField>
            </template>

            <!-- úver -->
            <template v-else-if="active === 'loan'">
              <FormField label="Výška úveru" :error="errors.amount">
                <InputText v-model="f.amount" inputmode="decimal" size="small" />
              </FormField>
              <FormField label="Úroková sadzba % p. a." :error="errors.rate">
                <InputText v-model="f.rate" inputmode="decimal" size="small" />
              </FormField>
              <FormField label="Počet mesačných splátok" :error="errors.months" :hint="f.months >= 12 ? `${formatAmount(f.months / 12).replace(',00', '')} r.` : ''">
                <InputNumber v-model="f.months" :min="1" :max="600" size="small" />
              </FormField>
              <span />
              <FormField label="Poplatok za poskytnutie" :error="errors.fee">
                <InputText v-model="f.fee" inputmode="decimal" size="small" placeholder="0" />
              </FormField>
              <FormField label="Mesačný poplatok" :error="errors.monthlyFee">
                <InputText v-model="f.monthlyFee" inputmode="decimal" size="small" placeholder="0" />
              </FormField>
            </template>

            <!-- sporenie -->
            <template v-else-if="active === 'savings'">
              <FormField label="Počiatočný vklad" :error="errors.initial">
                <InputText v-model="f.initial" inputmode="decimal" size="small" />
              </FormField>
              <FormField label="Mesačný vklad" :error="errors.monthly">
                <InputText v-model="f.monthly" inputmode="decimal" size="small" />
              </FormField>
              <FormField label="Úrok % p. a." :error="errors.rate">
                <InputText v-model="f.rate" inputmode="decimal" size="small" />
              </FormField>
              <FormField label="Doba sporenia (roky)" :error="errors.months">
                <InputNumber v-model="f.years" :min="1" :max="50" size="small" />
              </FormField>
              <FormField label="Daň z úrokov %" :error="errors.tax" hint="banka zrazí 19 %">
                <InputText v-model="f.tax" inputmode="decimal" size="small" />
              </FormField>
            </template>

            <!-- mzda -->
            <template v-else-if="active === 'wage'">
              <FormField label="Hrubá mzda" :error="errors.gross">
                <InputText v-model="f.gross" inputmode="decimal" size="small" />
              </FormField>
              <FormField label="Obdobie" :error="errors.year">
                <div class="pair">
                  <Select v-model="f.month" :options="monthOptions" option-label="label" option-value="value" size="small" aria-label="Mesiac" />
                  <Select v-model="f.year" :options="yearOptions" option-label="label" option-value="value" size="small" aria-label="Rok" />
                </div>
              </FormField>
              <FormField label="Pracovný vzťah" :error="errors.contract">
                <Select v-model="f.contract" :options="contractOptions" option-label="label" option-value="value" size="small" fluid />
              </FormField>
              <FormField label="Dôchodok">
                <Select v-model="f.pension" :options="pensionOptions" option-label="label" option-value="value" size="small" fluid />
              </FormField>
              <FormField label="Daň" :span="2">
                <div class="checks">
                  <label><Checkbox v-model="f.nczd" binary /> Podpísané vyhlásenie (nezdaniteľná časť)</label>
                  <label v-if="f.contract !== 'tpp' && f.contract !== 'dobps'"><Checkbox v-model="f.regular" binary /> Pravidelný mesačný príjem z dohody</label>
                </div>
              </FormField>
              <FormField label="Deti na daňový bonus (vek)" :span="2" :error="Object.entries(errors).find(([k]) => k.startsWith('children'))?.[1]">
                <div class="children">
                  <span v-for="(c, i) in f.children" :key="i" class="child">
                    <InputNumber v-model="f.children[i]" :min="0" :max="25" size="small" suffix=" r." :aria-label="`Vek ${i + 1}. dieťaťa`" />
                    <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="`Odobrať ${i + 1}. dieťa`" @click="f.children.splice(i, 1)" />
                  </span>
                  <Button label="Dieťa" icon="pi pi-plus" text size="small" @click="addChild" />
                </div>
              </FormField>
            </template>

            <!-- kurzy -->
            <template v-else-if="active === 'fx'">
              <FormField label="Smer" :span="2">
                <SelectButton v-model="f.toEur" :options="[{ value: false, label: 'z eur' }, { value: true, label: 'na eurá' }]" option-label="label" option-value="value" :allow-empty="false" size="small" />
              </FormField>
              <FormField :label="f.toEur ? `Suma v ${f.currency}` : 'Suma v eurách'" :error="errors.amount">
                <InputText v-model="f.amount" inputmode="decimal" size="small" />
              </FormField>
              <FormField label="Mena" :error="errors.currency">
                <Select v-model="f.currency" :options="currencyOptions" option-label="label" option-value="value" filter size="small" fluid />
              </FormField>
              <FormField label="Kurz k dátumu" :error="errors.date">
                <DatePicker v-model="f.date" date-format="d. m. yy" size="small" fluid />
              </FormField>
            </template>
          </form>

          <aside class="result" :class="{ busy }" aria-live="polite">
            <h2>Výsledok</h2>
            <p v-if="error" class="error">{{ error }}</p>
            <p v-else-if="!rows.length" class="muted">Doplňte údaje vľavo.</p>
            <dl v-else>
              <template v-for="r in rows" :key="r.label">
                <div class="r" :class="{ total: r.total, after: r.after, strong: r.strong }">
                  <dt>{{ r.label }}<small v-if="r.sub">{{ r.sub }}</small></dt>
                  <dd><span v-if="r.minus" class="sign">−</span><span v-if="r.plus" class="sign">+</span>{{ r.value }}</dd>
                </div>
              </template>
            </dl>
          </aside>
        </div>

        <section v-if="schedule.length || savingYears.length || q" class="table-block">
          <div class="tbar">
            <h2>{{ active === 'loan' ? 'Splátkový kalendár' : 'Sporenie po rokoch' }}</h2>
            <span class="spacer" />
            <IconField>
              <InputIcon class="pi pi-search" />
              <InputText v-model="q" size="small" placeholder="Hľadať" aria-label="Hľadať" />
            </IconField>
            <ExportButton v-if="active === 'loan'" name="splatkovy-kalendar" :columns="loanScheduleColumns" :rows="schedule" />
            <ExportButton v-else name="sporenie" :columns="savingsYearColumns" :rows="savingYears" />
          </div>
          <div class="scroll">
            <table v-if="active === 'loan'">
              <thead><tr><th>Splátka</th><th>Spolu</th><th>Istina</th><th>Úrok</th><th class="opt">Poplatok</th><th>Zostatok</th></tr></thead>
              <tbody>
                <tr v-for="r in schedule" :key="r.n">
                  <td>{{ r.n }}.</td><td>{{ formatAmount(r.payment) }}</td><td>{{ formatAmount(r.principal) }}</td><td>{{ formatAmount(r.interest) }}</td>
                  <td class="opt">{{ formatAmount(r.fee) }}</td><td>{{ formatAmount(r.balance) }}</td>
                </tr>
              </tbody>
            </table>
            <table v-else>
              <thead><tr><th>Rok</th><th>Vklady</th><th>Čisté úroky</th><th>Zostatok</th></tr></thead>
              <tbody>
                <tr v-for="r in savingYears" :key="r.year">
                  <td>{{ r.year }}.</td><td>{{ formatAmount(r.deposits) }}</td><td>{{ formatAmount(r.interest) }}</td><td>{{ formatAmount(r.balance) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </section>
    </div>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 1200px;
}
.form-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.head {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.75rem;
}
.head h1 {
  margin: 0;
}
.pick {
  display: none;
}
.layout {
  display: grid;
  grid-template-columns: 12rem 1fr;
  gap: 1.25rem;
  align-items: start;
}
.list {
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--line, #d8dee5);
  padding-right: 0.5rem;
}
.list button {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  border: 0;
  background: none;
  font: inherit;
  text-align: left;
  padding: 0.45rem 0.6rem;
  border-radius: 4px;
  color: #1f3a5c;
  cursor: pointer;
}
.list button:hover {
  background: #eef3f8;
}
.list button.on {
  background: #1f3a5c;
  color: #fff;
}
.list button:focus-visible {
  outline: 2px solid #2f5d8c;
  outline-offset: 1px;
}
.hint {
  margin: 0 0 0.75rem;
  color: var(--ink-soft, #4a5866);
}
.grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(16rem, 22rem);
  gap: 1.5rem;
  align-items: start;
}
.pair {
  display: flex;
  gap: 0.4rem;
}
.pair > * {
  flex: 1;
  min-width: 0;
}
.checks {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}
.checks label {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}
.children {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.4rem;
}
.child {
  display: inline-flex;
  align-items: center;
}
.child :deep(input) {
  width: 4.5rem;
}
.result {
  border: 1px solid var(--line, #d8dee5);
  border-radius: 4px;
  padding: 0.9rem 1rem;
  background: #fbfcfd;
  transition: opacity 0.15s;
}
.result.busy {
  opacity: 0.7;
}
.result h2 {
  margin: 0 0 0.6rem;
  font-size: 1rem;
}
.result dl {
  margin: 0;
}
.r {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 1rem;
  padding: 0.3rem 0;
  border-bottom: 1px solid #edf0f3;
}
.r dt small {
  display: block;
  font-size: 0.75rem;
  color: var(--ink-faint, #8a96a3);
}
.r dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
  text-align: right;
  white-space: nowrap;
}
.r.strong dd {
  font-weight: 600;
}
.r.total {
  border-bottom: 0;
  padding-top: 0.5rem;
  font-weight: 700;
  font-size: 1.1rem;
}
.r.total dd {
  border-bottom: 3px double currentColor;
}
.r.after:first-of-type,
.r.total + .r.after {
  margin-top: 0.8rem;
  border-top: 1px solid var(--line, #d8dee5);
}
.sign {
  margin-right: 0.15rem;
  color: var(--ink-faint, #8a96a3);
}
.error {
  color: #b03a26;
  margin: 0;
}
.muted {
  color: var(--ink-faint, #8a96a3);
  margin: 0;
}
.table-block {
  margin-top: 1.5rem;
}
.tbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 0.5rem;
}
.tbar h2 {
  margin: 0;
  font-size: 1rem;
}
.spacer {
  flex: 1;
}
.scroll {
  max-height: 26rem;
  overflow: auto;
  border: 1px solid var(--line, #d8dee5);
  border-radius: 4px;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
  font-size: 0.9rem;
}
th,
td {
  padding: 0.3rem 0.6rem;
  text-align: right;
  border-bottom: 1px solid #edf0f3;
}
th:first-child,
td:first-child {
  text-align: left;
}
thead th {
  position: sticky;
  top: 0;
  background: #f4f6f8;
  font-weight: 600;
}

@media (max-width: 900px) {
  .grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 640px) {
  .layout {
    grid-template-columns: 1fr;
  }
  .list {
    display: none;
  }
  .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .form-grid > :deep(*) {
    grid-column: auto;
  }
  .pick {
    display: inline-flex;
    flex: 1;
    min-width: 0;
  }
  .opt {
    display: none;
  }
  .tbar :deep(.p-iconfield) {
    flex: 1 1 8rem;
  }
}
</style>
