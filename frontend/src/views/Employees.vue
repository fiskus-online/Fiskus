<script setup>
// Mzdy — zamestnanci a dohodári: pracovný vzťah, mzda, poistenie, daň.
import { computed, onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { employeeColumns } from '../lib/columns.js'
import {
  contractLabel, contractShort, contracts, employeeFromForm, employeeName, employeeState, employeeToForm, filterEmployees, insurerLabel, insurers,
  monthNames, newEmployee, oopAllowed, pensions,
} from '../lib/payroll.js'

const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))
const list = ref([])
const loading = ref(true)
const search = ref('')
const all = ref(false)
const views = [{ label: 'Aktuálni', value: false }, { label: 'Všetci', value: true }]
const now = new Date()
const ym = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`

async function load() {
  loading.value = true
  try {
    list.value = await api.employees(session.companyId)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Zamestnanci sa nenačítali', detail: e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}
onMounted(load)
const visible = computed(() => filterEmployees(list.value, { search: search.value, all: all.value, ym }))

// ---- formulár ----
const form = ref(null)
const errors = ref({})
const busy = ref(false)
function edit(e) {
  errors.value = {}
  const f = e ? employeeToForm(e) : newEmployee()
  form.value = { ...f, startDate: fromISODate(f.start), endDate: f.end ? fromISODate(f.end) : null }
}
const agreement = computed(() => form.value && form.value.contract !== 'tpp')
function addChild() {
  form.value.children.push({ name: '', rc: '', months: Array(12).fill(false) })
}
async function save() {
  busy.value = true
  try {
    const f = form.value
    const body = employeeFromForm({ ...f, start: f.startDate ? toISODate(f.startDate) : '', end: f.endDate ? toISODate(f.endDate) : '' })
    delete body.startDate
    delete body.endDate
    await api.saveEmployee(session.companyId, body)
    toast.add({ severity: 'success', summary: f.id ? 'Zamestnanec uložený' : 'Zamestnanec pridaný', life: 2000 })
    form.value = null
    await load()
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Zamestnanec sa neuložil', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
function remove(e) {
  confirm.require({
    header: 'Zmazať zamestnanca',
    message: `Zmazať ${employeeName(e)}? Zamestnanca s uzavretou mzdou nezmažete — zadajte mu koniec pracovného vzťahu.`,
    acceptProps: { label: 'Zmazať', severity: 'danger', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.deleteEmployee(session.companyId, e.id)
        form.value = null
        await load()
      } catch (err) {
        toast.add({ severity: 'error', summary: 'Zamestnanec sa nezmazal', detail: err.fields?.[0]?.message ?? err.message, life: 7000 })
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Zamestnanci</h1>
      <SelectButton v-model="all" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <div class="spacer" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať meno, pozíciu, dohodu" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton name="Zamestnanci" :columns="employeeColumns" :rows="visible" />
      <Button v-if="canWrite" label="Pridať zamestnanca" icon="pi pi-plus" size="small" @click="edit(null)" />
    </header>

    <p v-if="!loading && !list.length" class="empty">
      Pridajte zamestnancov v pracovnom pomere aj dohodárov. Fiškus z ich údajov vypočíta každý mesiac odvody do Sociálnej poisťovne a zdravotnej poisťovne,
      preddavok na daň, daňový bonus a čistú mzdu.
    </p>
    <p v-else-if="!loading && !visible.length" class="empty">Nikto nevyhovuje. <button class="lnk" type="button" @click="(all = true), (search = '')">Zobraziť všetkých</button></p>

    <table v-if="visible.length" class="emps">
      <thead>
        <tr><th>Meno</th><th>Vzťah</th><th>Od – do</th><th class="num">Mzda</th><th>Poisťovňa</th><th>Daň</th><th>Stav</th></tr>
      </thead>
      <tbody>
        <tr v-for="e in visible" :key="e.id" tabindex="0" @click="edit(e)" @keydown.enter="edit(e)">
          <td class="name"><strong>{{ employeeName(e) }}</strong><small v-if="e.position">{{ e.position }}</small></td>
          <td><span :title="contractLabel(e.contract)">{{ contractShort(e.contract) }}</span><small v-if="e.contract !== 'tpp'">{{ e.regular ? 'pravidelný príjem' : 'nepravidelný príjem' }}</small></td>
          <td>{{ formatDate(e.start) }} – {{ e.end ? formatDate(e.end) : '' }}</td>
          <td class="num">{{ formatMoney(e.wage) }}<small v-if="e.hourly">/ hod</small></td>
          <td>{{ insurerLabel(e.insurer) }}</td>
          <td><span v-if="e.nczd">NČZD</span><span v-if="e.bonus"> · bonus {{ e.children?.length }}</span></td>
          <td><span class="tag" :data-tone="employeeState(e, ym).tone"><span class="tag-text">{{ employeeState(e, ym).label }}</span></span></td>
        </tr>
      </tbody>
    </table>

    <Dialog :visible="!!form" :header="form?.id ? employeeName(form) || 'Zamestnanec' : 'Nový zamestnanec'" modal :style="{ width: '46rem' }" :breakpoints="{ '720px': '100vw' }"
      @update:visible="(v) => !v && (form = null)">
      <form v-if="form" class="form-grid emp-form" @submit.prevent="save">
        <h3 class="sec">Osobné údaje</h3>
        <FormField label="Titul" :error="errors.title"><InputText v-model="form.title" size="small" /></FormField>
        <FormField label="Meno" :error="errors.firstName"><InputText v-model="form.firstName" size="small" /></FormField>
        <FormField label="Priezvisko" :span="2" :error="errors.lastName"><InputText v-model="form.lastName" size="small" /></FormField>
        <FormField label="Rodné číslo" :error="errors.rc"><InputText v-model="form.rc" size="small" placeholder="850101/1234" /></FormField>
        <FormField label="IBAN na výplatu" :span="2" :error="errors.iban"><InputText v-model="form.iban" size="small" /></FormField>
        <FormField label="E-mail" :error="errors.email"><InputText v-model="form.email" size="small" type="email" /></FormField>
        <FormField label="Ulica a číslo" :span="2" :error="errors.street"><InputText v-model="form.street" size="small" /></FormField>
        <FormField label="PSČ" :error="errors.zip"><InputText v-model="form.zip" size="small" /></FormField>
        <FormField label="Obec" :error="errors.city"><InputText v-model="form.city" size="small" /></FormField>

        <h3 class="sec">Pracovný vzťah</h3>
        <FormField label="Druh" :span="2" :error="errors.contract">
          <Select v-model="form.contract" :options="contracts" option-label="label" option-value="value" size="small" fluid />
        </FormField>
        <FormField label="Pozícia" :span="2" :error="errors.position"><InputText v-model="form.position" size="small" /></FormField>
        <FormField label="Nástup" :error="errors.start"><DatePicker v-model="form.startDate" date-format="d. m. yy" size="small" fluid /></FormField>
        <FormField label="Koniec" :error="errors.end" hint="prázdne = na neurčito"><DatePicker v-model="form.endDate" date-format="d. m. yy" size="small" fluid show-clear /></FormField>
        <FormField label="Odmeňovanie" :span="2">
          <SelectButton v-model="form.hourly" :options="[{ label: 'Mesačne', value: false }, { label: 'Za hodinu', value: true }]" option-label="label" option-value="value" :allow-empty="false" size="small" />
        </FormField>
        <FormField :label="form.hourly ? 'Hodinová sadzba €' : agreement ? 'Mesačná odmena €' : 'Mesačná mzda €'" :error="errors.wage">
          <InputText v-model="form.wage" size="small" inputmode="decimal" class="num" placeholder="0,00" />
        </FormField>
        <FormField label="Týždenný úväzok (hod)" :error="errors.hours"><InputText v-model="form.hours" size="small" inputmode="decimal" class="num" /></FormField>
        <FormField v-if="agreement && form.contract !== 'dobps'" label="Pravidelný mesačný príjem" :span="2" hint="dohoda na pravidelnú mesačnú odmenu — poistenie ako pri pracovnom pomere">
          <ToggleSwitch v-model="form.regular" />
        </FormField>

        <h3 class="sec">Poistenie a daň</h3>
        <FormField label="Zdravotná poisťovňa" :error="errors.insurer">
          <Select v-model="form.insurer" :options="insurers" option-label="label" option-value="value" size="small" fluid />
        </FormField>
        <FormField label="Dôchodok" :span="2" :error="errors.pension">
          <Select v-model="form.pension" :options="pensions" option-label="label" option-value="value" size="small" fluid placeholder="Bez dôchodku" />
        </FormField>
        <FormField label="ZŤP" hint="znížené zdravotné poistenie"><ToggleSwitch v-model="form.ztp" /></FormField>
        <FormField v-if="oopAllowed(form)" label="Odvodová výnimka 200 €" :span="2" :error="errors.oop" hint="sociálne poistenie až zo sumy nad 200 € — iba na jednej dohode">
          <ToggleSwitch v-model="form.oop" />
        </FormField>
        <FormField label="Vyhlásenie na NČZD" :span="2" hint="nezdaniteľná časť základu dane sa uplatňuje u nás">
          <ToggleSwitch v-model="form.nczd" />
        </FormField>
        <FormField label="Daňový bonus na deti" :span="2" :error="errors.children"><ToggleSwitch v-model="form.bonus" /></FormField>
        <div v-if="form.bonus" class="kids">
          <div v-for="(c, i) in form.children" :key="i" class="kid">
            <FormField label="Meno dieťaťa" :error="errors[`children.${i}.name`]"><InputText v-model="c.name" size="small" /></FormField>
            <FormField label="Rodné číslo" :error="errors[`children.${i}.rc`]"><InputText v-model="c.rc" size="small" /></FormField>
            <fieldset class="months">
              <legend>Mesiace v domácnosti (nič = celý rok)</legend>
              <label v-for="(mn, k) in monthNames" :key="k" :title="mn"><Checkbox v-model="c.months[k]" binary />{{ k + 1 }}</label>
            </fieldset>
            <Button icon="pi pi-trash" text severity="danger" size="small" aria-label="Odobrať dieťa" @click="form.children.splice(i, 1)" />
          </div>
          <Button label="Pridať dieťa" icon="pi pi-plus" text size="small" @click="addChild" />
        </div>
        <FormField label="Poznámka" :span="4"><InputText v-model="form.note" size="small" /></FormField>

        <div class="actions">
          <Button v-if="form.id && canWrite" label="Zmazať" icon="pi pi-trash" severity="danger" text size="small" @click="remove(form)" />
          <span class="spacer" />
          <Button label="Zrušiť" text size="small" @click="form = null" />
          <Button v-if="canWrite" type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
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
.empty {
  color: var(--ink-soft);
  max-width: 46rem;
  margin: 0;
}
.lnk {
  border: 0;
  background: none;
  color: var(--blue);
  cursor: pointer;
  padding: 0;
  font: inherit;
  text-decoration: underline;
}
.emps {
  width: 100%;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  font-size: var(--fs-sm);
}
.emps th {
  text-align: left;
  font-weight: 600;
  color: var(--ink-soft);
  padding: 6px 10px;
  border-bottom: 2px solid var(--ink);
  white-space: nowrap;
}
.emps td {
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
  vertical-align: top;
}
.emps tbody tr {
  cursor: pointer;
}
.emps tbody tr:hover,
.emps tbody tr:focus-visible {
  background: var(--paper);
  outline: none;
}
.emps small {
  display: block;
  color: var(--ink-soft);
}
.emps .num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.emps .num small {
  display: inline;
  margin-left: 2px;
}
.sec {
  grid-column: 1 / -1;
  font-size: var(--fs-md);
  margin: 6px 0 0;
  padding-top: 6px;
  border-top: 1px solid var(--rule);
}
.sec:first-child {
  border-top: 0;
  padding-top: 0;
  margin-top: 0;
}
.kids {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.kid {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: 4px 10px;
  align-items: end;
  padding: 8px;
  background: var(--paper);
}
.months {
  grid-column: 1 / -1;
  border: 0;
  padding: 0;
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  font-size: var(--fs-xs);
}
.months legend {
  color: var(--ink-soft);
  margin-bottom: 2px;
}
.months label {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.kid > .p-button {
  grid-row: 1;
  grid-column: 3;
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  gap: 8px;
  align-items: center;
}
@media (max-width: 640px) {
  .emps thead {
    display: none;
  }
  .emps tr {
    display: grid;
    grid-template-columns: 1fr auto;
    padding: 8px 10px;
    border-bottom: 1px solid var(--rule);
  }
  .emps td {
    border: 0;
    padding: 1px 0;
  }
  .emps td:nth-child(3),
  .emps td:nth-child(5),
  .emps td:nth-child(6) {
    display: none;
  }
  .emps td.name {
    grid-column: 1;
  }
  .kid {
    grid-template-columns: 1fr auto;
  }
  .kid > .p-button {
    grid-column: 2;
  }
}
</style>
