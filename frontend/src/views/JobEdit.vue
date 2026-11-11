<script setup>
// Zákazka: údaje, ekonomika z prepojených dokladov a súvisiace doklady.
// Nová ponuka alebo faktúra zo zákazky sa k nej priradí sama (?zakazka=).
// Výkaz práce a materiál sa dajú vyfakturovať jedným tlačidlom.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import DatePicker from 'primevue/datepicker'
import FormField from '../components/FormField.vue'
import RelatedDocs from '../components/RelatedDocs.vue'
import JobEntries from '../components/JobEntries.vue'
import ActivityPanel from '../components/ActivityPanel.vue'
import JobKindDetails from '../components/JobKindDetails.vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { printTitle } from '../lib/jobPrint.js'
import { blankJob, contactAddress, fillFromVehicle, jobForm, jobKinds, jobPayload, jobStatusOf, jobStatuses, marginText, setKind, vehicleHistory } from '../lib/jobs.js'

const props = defineProps({ id: { type: String, default: '' } })
const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('invoices.write'))

const job = ref(null) // zo servera (číslo, ekonomika)
const form = ref(blankJob())
const contacts = ref([])
const items = ref([])
const employees = ref([])
const errors = ref({})
const busy = ref(false)
// zmenené = formulár sa líši od posledne načítaného (porovnanie tela na uloženie)
const saved = ref('')
const snapshot = () => JSON.stringify(jobPayload(form.value))
const dirty = computed({ get: () => snapshot() !== saved.value, set: (v) => !v && (saved.value = snapshot()) })

async function load() {
  errors.value = {}
  ;[contacts.value, items.value, employees.value] = await Promise.all([
    api.contacts(session.companyId).catch(() => []),
    api.items(session.companyId).catch(() => []),
    can('ledger.read') ? api.employees(session.companyId).catch(() => []) : [],
  ])
  if (props.id) {
    const j = await api.job(session.companyId, props.id)
    setJob(j)
  } else {
    const ct = route.query.kontakt ? contacts.value.find((c) => String(c.id) === String(route.query.kontakt)) : null
    job.value = null
    form.value = blankJob(ct, jobKinds.some((k) => k.value === route.query.druh) ? route.query.druh : lastKind())
  }
  dirty.value = false
}
function setJob(j) {
  job.value = j
  form.value = jobForm(j)
}
onMounted(load)
watch(() => props.id, load)

// druh zákazky: nová zákazka dostane naposledy použitý (pamätá sa v prehliadači)
function lastKind() {
  try {
    const k = localStorage.getItem('fiskus.jobKind') ?? ''
    return jobKinds.some((x) => x.value === k) ? k : ''
  } catch {
    return ''
  }
}
function rememberKind(k) {
  try {
    localStorage.setItem('fiskus.jobKind', k)
  } catch {
    /* prehliadač bez úložiska */
  }
}
const contactOf = (id) => contacts.value.find((c) => c.id === id) ?? null
const changeKind = (k) => (form.value = setKind(form.value, k ?? '', contactOf(form.value.contactId)))
// montáž: miesto sa predvyplní adresou zákazníka, kým je prázdne
watch(() => form.value.contactId, (id) => {
  if (form.value.kind === 'install' && form.value.site && !form.value.site.address) form.value.site.address = contactAddress(contactOf(id))
})
// autoservis: známe vozidlo podľa EČV doplní údaje a zákazníka, pod ním história opráv
const vehicles = ref(null)
const allJobs = ref([])
async function loadVehicles() {
  if (vehicles.value) return
  ;[vehicles.value, allJobs.value] = await Promise.all([
    api.jobVehicles(session.companyId).catch(() => []),
    api.jobs(session.companyId).catch(() => []),
  ])
}
watch(() => form.value.kind, (k) => k === 'auto' && loadVehicles(), { immediate: true })
async function onPlate() {
  await loadVehicles()
  const filled = canWrite.value && fillFromVehicle(form.value, vehicles.value)
  if (filled && JSON.stringify(filled) !== JSON.stringify(form.value)) {
    form.value = filled
    toast.add({ severity: 'info', summary: 'Známe vozidlo', detail: 'Údaje sa doplnili z predchádzajúcej zákazky.', life: 3000 })
  }
}
const history = computed(() => (form.value.kind === 'auto' ? vehicleHistory(allJobs.value, form.value.vehicle, job.value?.id ?? 0) : []))

const namePlaceholder = computed(() => ({
  auto: 'Napr. Výmena bŕzd a servisná prehliadka',
  install: 'Napr. Montáž tepelného čerpadla',
  service: 'Napr. Oprava frekvenčného meniča – nejde štart',
})[form.value.kind] ?? 'Napr. Rekonštrukcia riadiaceho systému etiketovačky')

const date = (k) => ({
  get: () => fromISODate(form.value[k]),
  set: (d) => (form.value[k] = toISODate(d) ?? ''),
})

async function save() {
  busy.value = true
  errors.value = {}
  try {
    const saved = await api.saveJob(session.companyId, jobPayload(form.value), job.value?.id)
    const wasNew = !job.value
    if (wasNew) rememberKind(saved.kind ?? '')
    setJob(saved)
    dirty.value = false
    toast.add({ severity: 'success', summary: wasNew ? `Zákazka ${saved.number} založená` : 'Zákazka uložená', life: 2500 })
    if (wasNew) router.replace(`/zakazky/${saved.id}`)
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Zákazka sa neuložila', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

// tlač: neuložené zmeny sa najprv uložia, aby boli na papieri
async function printJob() {
  if (dirty.value && canWrite.value) await save()
  if (!dirty.value) router.push(`/zakazky/${job.value.id}/tlac`)
}

function remove() {
  confirm.require({
    header: 'Zmazať zákazku',
    message: `Zmazať zákazku ${job.value.number}? Doklady ostanú, zruší sa iba ich priradenie.`,
    acceptProps: { label: 'Zmazať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      await api.deleteJob(session.companyId, job.value.id)
      router.push('/zakazky')
    },
  })
}

async function refresh() {
  if (job.value) setJob(await api.job(session.companyId, job.value.id))
  dirty.value = false
}

// vyfakturovanie práce a materiálu: koncept faktúry, potom ho otvoríme
const invoicing = ref(false)
async function invoiceWork() {
  invoicing.value = true
  try {
    const inv = await api.invoiceJob(session.companyId, job.value.id)
    toast.add({ severity: 'success', summary: 'Koncept faktúry pripravený', detail: 'Skontrolujte ho a vystavte.', life: 3000 })
    router.push(`/faktury/${inv.id}`)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Faktúra sa nevytvorila', detail: e.fieldMap ? Object.values(e.fieldMap()).join(' ') || e.message : e.message, life: 6000 })
  } finally {
    invoicing.value = false
  }
}
const people = computed(() => {
  const names = new Set(employees.value.map((e) => [e.firstName, e.lastName].filter(Boolean).join(' ')).filter(Boolean))
  for (const w of form.value.work) if (w.person) names.add(w.person)
  return [...names]
})

// rozpad nákladov (iba nenulové časti)
const costParts = computed(() =>
  [['práca', s.value?.workCost], ['materiál', s.value?.materialCost], ['tovar zo skladu', s.value?.goodsCost]]
    .filter(([, v]) => Number(v))
    .map(([l, v]) => `${l} ${formatAmount(v)}`)
    .join(' · '),
)
const status = computed(() => (job.value ? jobStatusOf(job.value) : null))
const s = computed(() => job.value?.summary)
const contactOptions = computed(() => contacts.value.map((c) => ({ value: c.id, label: c.name + (c.ico ? ` (${c.ico})` : '') })))
const newDoc = (path) => {
  const q = new URLSearchParams({ zakazka: job.value.id })
  if (job.value.contactId) q.set('kontakt', job.value.contactId)
  router.push(`${path}?${q}`)
}
</script>

<template>
  <div class="page">
    <header class="head">
      <Button icon="pi pi-arrow-left" text rounded size="small" aria-label="Späť na zákazky" @click="router.push('/zakazky')" />
      <h1>{{ job ? `Zákazka ${job.number}` : 'Nová zákazka' }}</h1>
      <span v-if="status" class="tag" :data-tone="status.tone"><span class="tag-text">{{ status.label }}</span></span>
      <span class="spacer" />
      <Button v-if="job" :label="printTitle(form.kind)" icon="pi pi-print" text size="small" @click="printJob" />
      <template v-if="canWrite">
        <Button v-if="job" label="Zmazať" severity="danger" text size="small" @click="remove" />
        <Button label="Uložiť" icon="pi pi-check" size="small" :loading="busy" :disabled="job && !dirty" @click="save" />
      </template>
    </header>

    <section v-if="s" class="ledger" aria-label="Ekonomika zákazky">
      <div class="figure"><span class="label">Ponúknuté</span><span class="value">{{ formatMoney(s.offered) }}</span></div>
      <div class="figure"><span class="label">Vyfakturované</span><span class="value">{{ formatMoney(s.invoiced) }}</span></div>
      <div class="figure"><span class="label">Uhradené</span><span class="value">{{ formatMoney(s.paid) }}</span>
        <span v-if="Number(s.unpaid)" class="sub late">na úhradu {{ formatMoney(s.unpaid) }}</span></div>
      <div v-if="Number(s.unbilled) || Number(s.hours)" class="figure"><span class="label">Nevyfakturované</span><span class="value">{{ formatMoney(s.unbilled) }}</span>
        <span v-if="Number(s.hours)" class="sub">odpracované {{ formatAmount(s.hours) }} h</span></div>
      <div class="figure"><span class="label">Náklady</span><span class="value">{{ formatMoney(s.costs) }}</span>
        <span v-if="costParts" class="sub">{{ costParts }}</span></div>
      <div class="figure"><span class="label">Zisk</span><span class="value" :class="{ loss: Number(s.profit) < 0 }">{{ formatMoney(s.profit) }}</span>
        <span class="sub">marža {{ marginText(s) }}</span></div>
    </section>
    <p v-if="s?.net" class="muted small">Ponuky, fakturácia a náklady sú bez DPH, úhrady s DPH. Zálohové faktúry sa do fakturácie neratajú (rátajú sa až vyúčtovacie).</p>

    <form class="form-grid fields" @submit.prevent="save">
      <FormField label="Druh" :span="4" :error="errors.kind">
        <SelectButton :model-value="form.kind" :options="jobKinds" option-label="label" option-value="value" :allow-empty="false" size="small" class="kinds"
          :disabled="!canWrite" aria-label="Druh zákazky" @update:model-value="changeKind">
          <template #option="{ option }"><i :class="option.icon" /><span>{{ option.label }}</span></template>
        </SelectButton>
      </FormField>
      <FormField label="Názov zákazky" :span="2" :error="errors.name">
        <InputText v-model="form.name" size="small" :disabled="!canWrite" :placeholder="namePlaceholder" />
      </FormField>
      <FormField label="Zákazník" :span="2" :error="errors.contactId">
        <Select v-model="form.contactId" :options="contactOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
          :disabled="!canWrite" placeholder="Z adresára" @update:model-value="(v) => !v && (form.contactId = 0)" />
      </FormField>
      <FormField label="Plánovaná cena €" :error="errors.budget" :hint="s?.net ? 'bez DPH' : ''">
        <InputText v-model="form.budget" size="small" inputmode="decimal" :disabled="!canWrite" />
      </FormField>
      <FormField label="Hodinová sadzba €" :error="errors.rate" :hint="s?.net ? 'bez DPH, predvolená pre výkaz práce' : 'predvolená pre výkaz práce'">
        <InputText v-model="form.rate" size="small" inputmode="decimal" :disabled="!canWrite" />
      </FormField>
      <FormField label="Stav" :span="4" :error="errors.status">
        <SelectButton v-model="form.status" :options="jobStatuses" option-label="label" option-value="value" :allow-empty="false" size="small" :disabled="!canWrite" />
      </FormField>
      <FormField label="Začiatok">
        <DatePicker :model-value="date('start').get()" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="!canWrite" @update:model-value="date('start').set($event)" />
      </FormField>
      <FormField label="Termín dokončenia" :error="errors.due" :hint="job?.closed ? `uzavretá ${formatDate(job.closed)}` : ''">
        <DatePicker :model-value="date('due').get()" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="!canWrite" @update:model-value="date('due').set($event)" />
      </FormField>
      <FormField label="Poznámka" :span="4">
        <Textarea v-model="form.note" rows="3" auto-resize :disabled="!canWrite" />
      </FormField>
    </form>

    <JobKindDetails :form="form" :errors="errors" :readonly="!canWrite" @plate="onPlate" />
    <section v-if="history.length" class="history" aria-label="História vozidla">
      <h2>História vozidla</h2>
      <ul>
        <li v-for="h in history" :key="h.id">
          <RouterLink :to="`/zakazky/${h.id}`">{{ h.number }}</RouterLink>
          <span>{{ formatDate(h.start || h.created) }}</span>
          <span class="hname">{{ h.name }}</span>
          <span v-if="h.vehicle?.mileage" class="muted">{{ Number(h.vehicle.mileage).toLocaleString('sk-SK') }} km</span>
          <span class="tag" :data-tone="jobStatusOf(h).tone"><span class="tag-text">{{ jobStatusOf(h).label }}</span></span>
        </li>
      </ul>
    </section>

    <JobEntries kind="work" :form="form" :errors="errors" :readonly="!canWrite" :people="people" :hours="s?.hours ?? ''" />
    <JobEntries kind="material" :form="form" :errors="errors" :readonly="!canWrite" :items="items" :total="s?.materialCost ?? ''" />
    <div v-if="canWrite && (form.work.length || form.material.length)" class="new-docs">
      <Button v-if="dirty || !job" label="Uložiť výkaz" icon="pi pi-check" size="small" :loading="busy" @click="save" />
      <template v-else-if="Number(s?.unbilled)">
        <Button label="Vyfakturovať prácu a materiál" icon="pi pi-file" size="small" :loading="invoicing" @click="invoiceWork" />
        <span class="muted">{{ formatMoney(s.unbilled) }}{{ s.net ? ' bez DPH' : '' }} do konceptu faktúry</span>
      </template>
      <span v-else class="muted">Všetka práca a materiál sú vyfakturované.</span>
    </div>

    <template v-if="job">
      <div v-if="canWrite" class="new-docs">
        <span class="muted">Nový doklad k zákazke:</span>
        <Button label="Ponuka" icon="pi pi-file-edit" size="small" outlined @click="newDoc('/ponuky/nova')" />
        <Button label="Faktúra" icon="pi pi-file" size="small" outlined @click="newDoc('/faktury/nova')" />
        <Button label="Dodací list" icon="pi pi-truck" size="small" outlined @click="newDoc('/dodacie-listy/novy')" />
      </div>
      <RelatedDocs kind="job" :id="Number(job.id)" :can-write="canWrite" @changed="refresh" />
      <ActivityPanel :job-id="Number(job.id)" :can-write="canWrite" />
    </template>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px 24px;
  max-width: 1100px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.spacer {
  flex: 1;
}
.ledger {
  display: flex;
  flex-wrap: wrap;
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}
.figure {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 8px 20px 9px 14px;
  border-right: 1px solid var(--rule);
}
.figure .label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.figure .value {
  font-size: var(--fs-lg);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.figure .value.loss,
.sub.late {
  color: var(--tone-overdue);
}
.figure .sub {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.fields :deep(.p-selectbutton) {
  flex-wrap: wrap;
}
.kinds :deep(.p-togglebutton i) {
  margin-right: 5px;
}
.history h2 {
  margin: 0;
  font-size: var(--fs-md);
  border-bottom: 1px solid var(--rule);
  padding-bottom: 2px;
}
.history ul {
  list-style: none;
  margin: 0;
  padding: 0;
}
.history li {
  display: flex;
  align-items: baseline;
  gap: 10px;
  flex-wrap: wrap;
  padding: 4px 0;
  border-bottom: 1px solid var(--rule);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.history a {
  font-weight: 600;
  color: var(--blue);
  text-decoration: none;
}
.history .hname {
  flex: 1;
  min-width: 140px;
}
.new-docs {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.muted {
  color: var(--ink-soft);
}
.small {
  margin: 0;
  font-size: var(--fs-sm);
}
@media (max-width: 640px) {
  .page {
    padding: 10px 10px 20px;
  }
  .figure {
    flex: 1 1 45%;
    padding: 6px 10px;
  }
}
</style>
