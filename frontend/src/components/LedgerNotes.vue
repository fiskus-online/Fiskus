<script setup>
// Poznámky k účtovnej závierke malej účtovnej jednotky (Úč POD 3-01):
// text zadáva účtovná jednotka, Fiškus navrhne zásady oceňovania
// a prehľad čísel pripojí pri tlači.
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import { useToast } from 'primevue/usetoast'
import FormField from './FormField.vue'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { fromISODate, toISODate } from '../lib/format.js'
import { notesBody, notesForm } from '../lib/notes.js'

const props = defineProps({ year: { type: Number, required: true }, canWrite: { type: Boolean, default: false } })
const router = useRouter()
const toast = useToast()
const notes = ref(null)
const f = ref(notesForm(null))
const errors = ref({})
const busy = ref(false)
const saved = ref('')

async function load() {
  try {
    notes.value = await api.ledgerNotes(session.companyId, props.year)
    f.value = notesForm(notes.value)
    saved.value = JSON.stringify(f.value)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Poznámky sa nenačítali', detail: e.message, life: 6000 })
  }
}
watch(() => [props.year, session.companyId], load, { immediate: true })
const dirty = computed(() => notes.value && JSON.stringify(f.value) !== saved.value)
const small = computed(() => notes.value?.form === 'pod')
const approved = computed({
  get: () => fromISODate(f.value.prevApproved),
  set: (d) => (f.value.prevApproved = d ? toISODate(d) : ''),
})

async function save() {
  errors.value = {}
  const body = notesBody(f.value)
  if (!body) {
    errors.value = { liabilities5y: 'Zadajte sumu, napr. 12 000,00' }
    return
  }
  busy.value = true
  try {
    notes.value = await api.saveLedgerNotes(session.companyId, props.year, body)
    f.value = notesForm(notes.value)
    saved.value = JSON.stringify(f.value)
    toast.add({ severity: 'success', summary: 'Poznámky uložené', life: 2500 })
  } catch (e) {
    for (const x of e.fields ?? []) errors.value[x.field] = x.message
    if (!e.fields?.length) toast.add({ severity: 'error', summary: 'Poznámky sa neuložili', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
const print = () => router.push({ path: '/hlavna-kniha/poznamky', query: { rok: props.year } })
const usePolicies = () => (f.value.policies = notes.value?.defaultPolicies ?? '')
</script>

<template>
  <section class="sheet notes">
    <p v-if="notes && !small" class="hint">
      Poznámky sú pripravené pre malú účtovnú jednotku (Úč POD 3-01). Vzor závierky zvolíte na karte Súvaha.
    </p>
    <template v-else-if="notes">
      <div class="bar">
        <p class="hint">
          Poznámky Úč POD 3-01 k závierke {{ year }}. Prázdnu časť Fiškus do poznámok neuvedie. Prehľad súvahy a výkazu ziskov a strát
          s minulým rokom pripojí sám.
        </p>
        <div class="acts">
          <Button label="Tlačiť / PDF" icon="pi pi-print" size="small" severity="secondary" outlined :disabled="dirty" @click="print" />
          <Button v-if="canWrite" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" :disabled="!dirty" @click="save" />
        </div>
      </div>

      <fieldset class="form-section" :disabled="!canWrite">
        <legend>Čl. I. Všeobecné údaje</legend>
        <div class="form-grid">
          <FormField label="Opis činnosti" :span="2"><Textarea v-model="f.activity" rows="2" auto-resize fluid /></FormField>
          <FormField label="Závierka za minulý rok schválená" :error="errors.prevApproved">
            <DatePicker v-model="approved" date-format="d. m. yy" show-icon fluid />
          </FormField>
          <FormField label="Priemerný prepočítaný počet zamestnancov" :error="errors.employees"><InputText v-model="f.employees" fluid /></FormField>
          <FormField label="Skupina, konsolidovaná závierka" :span="2" hint="Ak je firma dcérskou alebo materskou účtovnou jednotkou">
            <Textarea v-model="f.group" rows="2" auto-resize fluid />
          </FormField>
        </div>
      </fieldset>

      <fieldset class="form-section" :disabled="!canWrite">
        <legend>Čl. II. Orgány účtovnej jednotky</legend>
        <FormField label="Záruky, pôžičky a plnenia členom štatutárneho a iných orgánov">
          <Textarea v-model="f.bodies" rows="2" auto-resize fluid />
        </FormField>
      </fieldset>

      <fieldset class="form-section" :disabled="!canWrite">
        <legend>Čl. III. Účtovné zásady a metódy</legend>
        <label class="check"><Checkbox v-model="f.notGoingConcern" binary /> <span>Firma nebude nepretržite pokračovať v činnosti (likvidácia, zrušenie)</span></label>
        <FormField label="Zásady a spôsob oceňovania" :hint="f.policies ? '' : 'Prázdne = návrh Fiškusu podľa nastavení programu (zobrazený nižšie)'">
          <Textarea v-model="f.policies" rows="5" auto-resize fluid :placeholder="notes.defaultPolicies" />
        </FormField>
        <Button v-if="canWrite && !f.policies" label="Upraviť návrh Fiškusu" text size="small" @click="usePolicies" />
        <FormField label="Opravy významných chýb minulých období"><Textarea v-model="f.errors" rows="2" auto-resize fluid /></FormField>
      </fieldset>

      <fieldset class="form-section" :disabled="!canWrite">
        <legend>Čl. IV. Súvaha a výkaz ziskov a strát</legend>
        <div class="form-grid">
          <FormField label="Záväzky so splatnosťou nad 5 rokov (€)" :error="errors.liabilities5y"><InputText v-model="f.liabilities5y" inputmode="decimal" fluid /></FormField>
          <FormField label="Zabezpečené záväzky (výška, spôsob zabezpečenia)" :span="2"><Textarea v-model="f.secured" rows="2" auto-resize fluid /></FormField>
          <FormField label="Výnosy a náklady mimoriadnej veľkosti alebo výskytu" :span="2"><Textarea v-model="f.unusual" rows="2" auto-resize fluid /></FormField>
        </div>
      </fieldset>

      <fieldset class="form-section" :disabled="!canWrite">
        <legend>Čl. V – VII. Iné aktíva a pasíva, udalosti po závierke, ostatné</legend>
        <FormField label="Podmienený majetok a podmienené záväzky (súdne spory, záruky, ručenie)"><Textarea v-model="f.contingent" rows="2" auto-resize fluid /></FormField>
        <FormField label="Iné finančné povinnosti a podsúvahové položky (prenajatý majetok, odpísané pohľadávky)"><Textarea v-model="f.offBalance" rows="2" auto-resize fluid /></FormField>
        <FormField label="Významné udalosti po 31. 12. do zostavenia závierky"><Textarea v-model="f.events" rows="2" auto-resize fluid /></FormField>
        <FormField label="Ostatné informácie"><Textarea v-model="f.other" rows="2" auto-resize fluid /></FormField>
      </fieldset>
    </template>
  </section>
</template>

<style scoped>
.notes .bar {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
}
.notes .bar .hint {
  flex: 1 1 20rem;
  margin: 0;
}
.notes .acts {
  display: flex;
  gap: 8px;
}
.hint {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.form-section {
  border: 0;
  border-top: 1px solid var(--rule);
  margin: 14px 0 0;
  padding: 10px 0 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.form-section legend {
  font-weight: 600;
  color: var(--ink);
  padding: 0 6px 0 0;
}
.check {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: var(--fs-sm);
}
</style>
