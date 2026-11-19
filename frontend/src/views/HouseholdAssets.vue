<script setup>
// Domácnosť — majetok a čistý majetok: účty + majetok (byt, auto, fondy
// s ručne zadanou hodnotou) + požičané − dlhy zo záväzkov. Všetky sumy
// počíta server (/household/networth); tu sa iba zobrazujú.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import ExportButton from '../components/ExportButton.vue'
import FormField from '../components/FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { assetBody, assetIcon, assetKindLabel, assetKinds, netWorthBars } from '../lib/household.js'
import { householdAssetColumns } from '../lib/columns.js'
import { matches, searchTerms } from '../lib/search.js'

const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))
const report = ref(null)
const search = ref('')

async function load() {
  try {
    report.value = await api.hhNetWorth(session.companyId, 12)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Majetok sa nenačítal', detail: e.message, life: 6000 })
  }
}
onMounted(load)

const now = computed(() => report.value?.now)
const assets = computed(() => report.value?.assets ?? [])
const terms = computed(() => searchTerms(search.value))
const visible = computed(() => assets.value.filter((a) => matches(terms.value, [a.name, a.note, assetKindLabel(a.kind)])))
const debts = computed(() => report.value?.debts ?? [])
const nonzero = (v) => Number(v ?? 0) !== 0

// graf vývoja
const W = 720
const H = 120
const chart = computed(() => netWorthBars(report.value?.history ?? [], W, H))
const monthLabel = (n, i, all) => (i === all.length - 1 ? 'dnes' : formatDate(n.date).replace(/^\d+\. /, '').replace(/ \d{2}(\d{2})$/, " '$1"))

// ---- formulár majetku ----
const form = ref(null)
const errors = ref({})
const busy = ref(false)
function edit(a) {
  errors.value = {}
  const today = toISODate(new Date())
  form.value = a
    ? { id: a.id, name: a.name, kind: a.kind, note: a.note, values: a.values.map((v) => ({ date: v.date, value: String(v.value).replace('.', ',') })) }
    : { id: 0, name: '', kind: 'vehicle', note: '', values: [{ date: today, value: '' }] }
}
function addValue() {
  form.value.values.push({ date: toISODate(new Date()), value: '' })
}
async function save() {
  busy.value = true
  try {
    await api.hhSaveAsset(session.companyId, assetBody(form.value))
    form.value = null
    await load()
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Neuložené', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
function remove() {
  const f = form.value
  confirm.require({
    header: 'Zmazať majetok',
    message: `Zmazať „${f.name}“ aj s históriou hodnôt? Predaný majetok radšej nechajte a pridajte hodnotu 0 k dátumu predaja — v histórii čistého majetku ostane.`,
    acceptProps: { label: 'Zmazať', severity: 'danger', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.hhDeleteAsset(session.companyId, f.id)
        form.value = null
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Nezmazané', detail: e.message, life: 6000 })
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Majetok</h1>
      <div class="spacer" />
      <ExportButton v-if="assets.length" name="Majetok domácnosti" :columns="householdAssetColumns" :rows="visible" />
      <Button v-if="canWrite" label="Pridať majetok" icon="pi pi-plus" size="small" @click="edit(null)" />
    </header>

    <div v-if="now" class="top">
      <section class="panel worth" aria-label="Čistý majetok">
        <table>
          <tbody>
            <tr><th><RouterLink to="/domacnost/ucty">Peniaze na účtoch</RouterLink></th><td class="num">{{ formatMoney(now.accounts) }}</td></tr>
            <tr><th>Majetok</th><td class="num">{{ formatMoney(now.assets) }}</td></tr>
            <tr v-if="nonzero(now.receivables)"><th><RouterLink to="/domacnost/zavazky">Požičané, ešte nevrátené</RouterLink></th>
              <td class="num">{{ formatMoney(now.receivables) }}</td></tr>
            <tr><th><RouterLink to="/domacnost/zavazky">Dlhy (zostatok pôžičiek)</RouterLink></th><td class="num neg">−{{ formatMoney(now.debts) }}</td></tr>
            <tr class="total"><th>Čistý majetok</th><td class="num"><span class="ledger">{{ formatMoney(now.total) }}</span></td></tr>
          </tbody>
        </table>
      </section>
      <section class="panel trend" aria-label="Vývoj čistého majetku za 12 mesiacov">
        <h2>Vývoj za rok</h2>
        <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" role="img" aria-hidden="true">
          <rect v-for="(b, i) in chart.bars" :key="i" :x="b.x" :y="b.y" :width="b.w" :height="b.h" :class="{ neg: b.neg, now: i === chart.bars.length - 1 }">
            <title>{{ formatDate(report.history[i].date) }}: {{ formatMoney(report.history[i].total) }}</title>
          </rect>
          <line v-if="chart.zero !== null" x1="0" :x2="W" :y1="chart.zero" :y2="chart.zero" class="zero" />
        </svg>
        <div class="months">
          <span v-for="(n, i) in report.history" :key="i">{{ monthLabel(n, i, report.history) }}</span>
        </div>
        <p v-if="chart.lo >= 0" class="range">
          najmenej {{ formatMoney(report.history[chart.lo].total) }} ({{ formatDate(report.history[chart.lo].date) }}),
          najviac {{ formatMoney(report.history[chart.hi].total) }} ({{ formatDate(report.history[chart.hi].date) }})
        </p>
      </section>
    </div>

    <section class="panel">
      <div class="sub">
        <h2>Majetok mimo účtov</h2>
        <span class="spacer" />
        <InputText v-if="assets.length > 5" v-model="search" placeholder="Hľadať majetok" size="small" aria-label="Hľadať majetok" class="search" />
      </div>
      <p v-if="!assets.length" class="help">
        Byt, auto, fondy alebo dôchodkové sporenie. Hodnotu zadávate ručne k dátumu (odhad, výpis z fondu) a kedykoľvek pridáte novú — história
        ostane. Dlhy sa berú zo záväzkov, netreba ich zadávať.
      </p>
      <table v-else class="list">
        <tbody>
          <tr v-for="a in visible" :key="a.id" :class="{ click: canWrite }" @click="canWrite && edit(a)">
            <td><i class="pi" :class="assetIcon(a.kind)" aria-hidden="true" /> {{ a.name }}<small>{{ assetKindLabel(a.kind) }}</small></td>
            <td class="when">k {{ formatDate(a.values.at(-1)?.date) }}</td>
            <td class="num">{{ formatMoney(a.value) }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="debts.length" class="panel">
      <div class="sub"><h2>Dlhy a požičané</h2></div>
      <table class="list">
        <tbody>
          <tr v-for="d in debts" :key="d.id" class="click" @click="router.push(`/domacnost/zavazky/${d.id}`)">
            <td><i class="pi" :class="d.lent ? 'pi-arrow-down-left' : 'pi-arrow-up-right'" aria-hidden="true" /> {{ d.name }}
              <small>{{ d.lent ? 'požičané niekomu' : 'zostáva splatiť' }}</small></td>
            <td class="num" :class="{ neg: !d.lent }">{{ d.lent ? '' : '−' }}{{ formatMoney(d.owed) }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <Dialog :visible="!!form" :header="form?.id ? 'Majetok' : 'Nový majetok'" modal :style="{ width: '32rem' }" :breakpoints="{ '560px': '96vw' }"
      @update:visible="(v) => !v && (form = null)">
      <form v-if="form" class="form-grid" @submit.prevent="save">
        <FormField label="Názov" :error="errors.name" :span="2">
          <InputText v-model="form.name" size="small" fluid placeholder="napr. Škoda Octavia, Byt Fončorda" autofocus />
        </FormField>
        <FormField label="Druh" :error="errors.kind" :span="2">
          <Select v-model="form.kind" :options="assetKinds" option-label="label" option-value="value" size="small" fluid />
        </FormField>
        <div class="values">
          <span class="lbl">Hodnota k dátumu</span>
          <div v-for="(v, i) in form.values" :key="i" class="vrow">
            <DatePicker :model-value="fromISODate(v.date)" date-format="d. m. yy" size="small" :aria-label="`Dátum hodnoty ${i + 1}`"
              @update:model-value="(d) => (v.date = d ? toISODate(d) : '')" />
            <InputText v-model="v.value" size="small" inputmode="decimal" placeholder="0,00" class="num" :aria-label="`Hodnota ${i + 1}`"
              :invalid="!!errors[`values.${i}`]" />
            <Button icon="pi pi-times" text size="small" severity="secondary" aria-label="Odobrať hodnotu" :disabled="form.values.length === 1"
              @click="form.values.splice(i, 1)" />
            <small v-if="errors[`values.${i}`]" class="err">{{ errors[`values.${i}`] }}</small>
          </div>
          <small v-if="errors.values" class="err">{{ errors.values }}</small>
          <Button label="Pridať hodnotu" icon="pi pi-plus" text size="small" @click="addValue" />
          <small class="hint">Platí posledná hodnota k dátumu. Predaný majetok: pridajte hodnotu 0 k dátumu predaja.</small>
        </div>
        <FormField label="Poznámka" :span="2">
          <Textarea v-model="form.note" rows="2" auto-resize fluid />
        </FormField>
        <div class="actions">
          <Button v-if="form.id" label="Zmazať" icon="pi pi-trash" text severity="danger" size="small" @click="remove" />
          <span class="spacer" />
          <Button label="Zrušiť" text size="small" @click="form = null" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.head,
.sub {
  display: flex;
  align-items: center;
  gap: 10px;
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
}
.help {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  max-width: 80ch;
}
.panel {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 10px 12px;
}
.panel h2 {
  font-size: var(--fs-md);
  color: var(--blue-deep);
}
.top {
  display: grid;
  grid-template-columns: minmax(280px, 380px) minmax(0, 1fr);
  gap: 12px;
}
.worth table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.worth th {
  text-align: left;
  font-weight: 400;
  color: var(--ink-soft);
  padding: 4px 0;
}
.worth th a {
  color: inherit;
}
.worth td {
  padding: 4px 0;
}
.worth .total th,
.worth .total td {
  border-top: 1px solid var(--ink);
  font-weight: 700;
  color: var(--ink);
  padding-top: 6px;
  font-size: var(--fs-lg);
}
.ledger {
  border-bottom: 3px double currentColor;
}
.num {
  text-align: right;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.neg {
  color: var(--tone-overdue);
}
.trend svg {
  width: 100%;
  height: 130px;
  display: block;
  margin-top: 6px;
}
.trend rect {
  fill: #9fb6cf;
}
.trend rect.now {
  fill: var(--blue);
}
.trend rect.neg {
  fill: var(--tone-overdue);
}
.trend .zero {
  stroke: var(--ink-faint);
  stroke-width: 1;
}
.months {
  display: flex;
  margin-top: 3px;
}
.months span {
  flex: 1;
  min-width: 0;
  text-align: center;
  font-size: var(--fs-xs);
  color: var(--ink-soft);
  white-space: nowrap;
}
.range {
  margin-top: 4px;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.list {
  width: 100%;
  border-collapse: collapse;
  margin-top: 4px;
}
.list td {
  padding: 6px 4px;
  border-top: 1px solid var(--rule);
}
.list small {
  margin-left: 8px;
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.list .pi {
  color: var(--blue);
  margin-right: 4px;
}
.list .when {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  white-space: nowrap;
}
.click {
  cursor: pointer;
}
.click:hover {
  background: var(--blue-wash);
}
.search {
  width: 200px;
}
.values {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.values .lbl {
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
}
.vrow {
  display: grid;
  grid-template-columns: 9rem 1fr auto;
  gap: 6px;
  align-items: center;
}
.vrow .err {
  grid-column: 1 / -1;
}
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-xs);
}
.hint {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  gap: 6px;
  align-items: center;
}
@media (max-width: 760px) {
  .top {
    grid-template-columns: minmax(0, 1fr);
  }
  .months span:nth-child(even) {
    visibility: hidden;
  }
  .list small {
    display: block;
    margin-left: 22px;
  }
  .list .when {
    display: none;
  }
}
</style>
