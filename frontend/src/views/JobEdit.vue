<script setup>
// Zákazka: údaje, ekonomika z prepojených dokladov a súvisiace doklady.
// Nová ponuka alebo faktúra zo zákazky sa k nej priradí sama (?zakazka=).
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
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { blankJob, jobPayload, jobStatusOf, jobStatuses, marginText } from '../lib/jobs.js'

const props = defineProps({ id: { type: String, default: '' } })
const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('invoices.write'))

const job = ref(null) // zo servera (číslo, ekonomika)
const form = ref(blankJob())
const contacts = ref([])
const errors = ref({})
const busy = ref(false)
const dirty = ref(false)

async function load() {
  errors.value = {}
  contacts.value = await api.contacts(session.companyId).catch(() => [])
  if (props.id) {
    const j = await api.job(session.companyId, props.id)
    setJob(j)
  } else {
    const ct = route.query.kontakt ? contacts.value.find((c) => String(c.id) === String(route.query.kontakt)) : null
    job.value = null
    form.value = blankJob(ct)
  }
  dirty.value = false
}
function setJob(j) {
  job.value = j
  form.value = { id: j.id, name: j.name, contactId: j.contactId || 0, customer: j.customer, status: j.status, start: j.start, due: j.due,
    budget: j.budget && j.budget !== '0.00' ? j.budget : '', note: j.note }
}
onMounted(load)
watch(() => props.id, load)
watch(form, () => (dirty.value = true), { deep: true })

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
      <div class="figure"><span class="label">Náklady</span><span class="value">{{ formatMoney(s.costs) }}</span></div>
      <div class="figure"><span class="label">Zisk</span><span class="value" :class="{ loss: Number(s.profit) < 0 }">{{ formatMoney(s.profit) }}</span>
        <span class="sub">marža {{ marginText(s) }}</span></div>
    </section>
    <p v-if="s?.net" class="muted small">Ponuky, fakturácia a náklady sú bez DPH, úhrady s DPH. Zálohové faktúry sa do fakturácie neratajú (rátajú sa až vyúčtovacie).</p>

    <form class="form-grid fields" @submit.prevent="save">
      <FormField label="Názov zákazky" :span="2" :error="errors.name">
        <InputText v-model="form.name" size="small" :disabled="!canWrite" placeholder="Napr. Rekonštrukcia riadiaceho systému etiketovačky" />
      </FormField>
      <FormField label="Zákazník" :error="errors.contactId">
        <Select v-model="form.contactId" :options="contactOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
          :disabled="!canWrite" placeholder="Z adresára" @update:model-value="(v) => !v && (form.contactId = 0)" />
      </FormField>
      <FormField label="Plánovaná cena €" :error="errors.budget" :hint="s?.net ? 'bez DPH' : ''">
        <InputText v-model="form.budget" size="small" inputmode="decimal" :disabled="!canWrite" />
      </FormField>
      <FormField label="Stav" :span="2" :error="errors.status">
        <SelectButton v-model="form.status" :options="jobStatuses" option-label="label" option-value="value" :allow-empty="false" size="small" :disabled="!canWrite" />
      </FormField>
      <FormField label="Začiatok">
        <DatePicker :model-value="date('start').get()" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="!canWrite" @update:model-value="date('start').set($event)" />
      </FormField>
      <FormField label="Termín dokončenia" :error="errors.due" :hint="job?.closed ? `uzavretá ${formatDate(job.closed)}` : ''">
        <DatePicker :model-value="date('due').get()" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="!canWrite" @update:model-value="date('due').set($event)" />
      </FormField>
      <FormField label="Poznámka" :span="2">
        <Textarea v-model="form.note" rows="3" auto-resize :disabled="!canWrite" />
      </FormField>
    </form>

    <template v-if="job">
      <div v-if="canWrite" class="new-docs">
        <span class="muted">Nový doklad k zákazke:</span>
        <Button label="Ponuka" icon="pi pi-file-edit" size="small" outlined @click="newDoc('/ponuky/nova')" />
        <Button label="Faktúra" icon="pi pi-file" size="small" outlined @click="newDoc('/faktury/nova')" />
      </div>
      <RelatedDocs kind="job" :id="Number(job.id)" :can-write="canWrite" @changed="refresh" />
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
.fields {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.fields :deep(.p-selectbutton) {
  flex-wrap: wrap;
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
  .fields {
    grid-template-columns: minmax(0, 1fr);
  }
  .figure {
    flex: 1 1 45%;
    padding: 6px 10px;
  }
}
</style>
