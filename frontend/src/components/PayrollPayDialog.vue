<script setup>
// Príkaz na úhradu miezd: výplaty, Sociálna poisťovňa, zdravotné poisťovne
// a daňový úrad z uzavretého mesiaca. Čo sa smie zaradiť, rozhoduje server
// (pole problem); sumy sú hotové zo servera.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import { useToast } from 'primevue/usetoast'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatMoney, toISODate } from '../lib/format.js'
import { saveBlob } from '../lib/export.js'
import { insurerLabel, payDefault, paySettingsForm, payTotal, periodLabel } from '../lib/payroll.js'

const props = defineProps({ year: { type: Number, required: true }, month: { type: Number, required: true } })
const visible = defineModel('visible', { type: Boolean, default: false })
const toast = useToast()
const canWrite = computed(() => can('ledger.write'))

const data = ref(null)
const selected = ref([])
const date = ref(new Date())
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const editing = ref(false)
const form = ref(paySettingsForm())
const errors = ref({})

const pays = computed(() => data.value?.payments ?? [])
const eligible = computed(() => pays.value.filter((p) => !p.problem))
const allOn = computed(() => eligible.value.length > 0 && selected.value.length === eligible.value.length)
const total = computed(() => payTotal(pays.value, selected.value))
const companyIBAN = computed(() => session.company?.iban ?? '')

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.payrollPayments(session.companyId, props.year, props.month)
    selected.value = payDefault(data.value.payments)
    form.value = paySettingsForm(data.value.settings)
    // prvý raz bez platobných údajov → formulár hneď otvorený
    const s = data.value.settings ?? {}
    editing.value = !s.socialIban && !s.taxOud && !Object.keys(s.health ?? {}).length
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
watch(visible, (v) => v && load())

function toggleAll() {
  selected.value = allOn.value ? [] : eligible.value.map((p) => p.key)
}

async function saveSettings() {
  busy.value = true
  try {
    await api.savePayrollPaySettings(session.companyId, form.value)
    errors.value = {}
    await load()
    editing.value = false
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) error.value = e.message
  } finally {
    busy.value = false
  }
}

async function downloadOrder() {
  busy.value = true
  error.value = ''
  try {
    const { blob, name } = await api.payrollPaymentOrder(session.companyId, props.year, props.month, selected.value, toISODate(date.value))
    saveBlob(blob, name)
    toast.add({ severity: 'success', summary: 'Príkaz stiahnutý', detail: `${name} nahrajte v internet bankingu ako hromadný príkaz.`, life: 8000 })
    visible.value = false
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
const kindLabel = { wage: 'výplata', social: 'poistné', health: 'zdravotné poistenie', tax: 'preddavok dane' }
</script>

<template>
  <Dialog v-model:visible="visible" :header="`Príkaz na úhradu — mzdy ${periodLabel(year, month)}`" modal :style="{ width: '52rem' }" :breakpoints="{ '760px': '96vw' }">
    <div class="pp-head">
      <div class="from">
        <span class="label">Z účtu</span>
        <span v-if="companyIBAN" class="iban">{{ companyIBAN }}</span>
        <span v-else class="missing">Firma nemá IBAN — doplňte ho v nastaveniach.</span>
      </div>
      <label class="when">
        <span class="label">Dátum splatnosti</span>
        <DatePicker v-model="date" :min-date="new Date()" date-format="d. m. yy" show-icon icon-display="input" size="small" />
      </label>
      <div class="spacer" />
      <Button v-if="canWrite && !editing" label="Platobné údaje…" icon="pi pi-pencil" text size="small" @click="editing = true" />
    </div>

    <form v-if="editing" class="settings" @submit.prevent="saveSettings">
      <p class="note">Účet pobočky a variabilný symbol vám pridelila Sociálna poisťovňa, symboly zdravotných poisťovní nájdete v ich oznámení o registrácii. Preddavky dane idú na účet s predčíslím 500259 a vaším osobným účtom daňovníka (OÚD).</p>
      <div class="form-grid sg">
        <FormField label="Sociálna poisťovňa — IBAN pobočky" :error="errors.socialIban"><InputText v-model="form.socialIban" size="small" fluid /></FormField>
        <FormField label="Variabilný symbol (10 číslic)" :error="errors.socialVs"><InputText v-model="form.socialVs" size="small" inputmode="numeric" fluid /></FormField>
        <template v-for="(h, code) in form.health" :key="code">
          <FormField :label="`${insurerLabel(code)} — IBAN`" :error="errors[`health.${code}.iban`]"><InputText v-model="h.iban" size="small" fluid /></FormField>
          <FormField label="Variabilný symbol" :error="errors[`health.${code}.vs`]"><InputText v-model="h.vs" size="small" inputmode="numeric" fluid /></FormField>
        </template>
        <FormField label="Osobný účet daňovníka (OÚD)" :error="errors.taxOud" hint="základné číslo účtu, bez predčíslia"><InputText v-model="form.taxOud" size="small" inputmode="numeric" fluid /></FormField>
      </div>
      <div class="actions">
        <Button label="Zrušiť" text size="small" @click="editing = false" />
        <Button type="submit" label="Uložiť údaje" icon="pi pi-check" size="small" :loading="busy" />
      </div>
    </form>

    <p v-if="loading" class="muted">Načítavam úhrady…</p>
    <p v-else-if="data && !data.closed" class="muted">Príkaz sa zostaví z uzavretých miezd — najprv mesiac uzavrite.</p>
    <div v-else-if="pays.length" class="pp-wrap">
      <table class="pp">
        <thead>
          <tr>
            <th class="chk"><Checkbox :model-value="allOn" binary aria-label="Vybrať všetky" @update:model-value="toggleAll" /></th>
            <th>Príjemca</th>
            <th class="sym">Symboly</th>
            <th class="acc">IBAN</th>
            <th class="num">Suma</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in pays" :key="p.key" :class="{ off: p.problem }">
            <td class="chk"><Checkbox v-model="selected" :value="p.key" :disabled="!!p.problem" :aria-label="`Zaradiť ${p.name}`" /></td>
            <td>
              <div class="who">{{ p.name }}</div>
              <div class="sub">{{ kindLabel[p.kind] }} · {{ p.info }}</div>
              <div v-if="p.problem" class="problem">{{ p.problem }}</div>
            </td>
            <td class="sym"><template v-if="p.vs">VS {{ p.vs }}</template><template v-if="p.ss"><br />ŠS {{ p.ss }}</template><template v-if="p.ks"><br />KS {{ p.ks }}</template></td>
            <td class="acc">{{ p.iban }}</td>
            <td class="num">{{ formatAmount(p.amount) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <template #footer>
      <div class="pp-footer">
        <div class="pp-foot">
          <span class="sum">Vybrané {{ selected.length }} · spolu <strong>{{ formatMoney(total) }}</strong></span>
          <Button label="Zrušiť" severity="secondary" text size="small" @click="visible = false" />
          <Button icon="pi pi-download" label="Stiahnuť príkaz (XML)" size="small" :loading="busy" :disabled="!selected.length || !companyIBAN" @click="downloadOrder" />
        </div>
        <p class="hint">Preddavky dane sa odvádzajú do 5 dní od výplaty, poistné do 8. dňa nasledujúceho mesiaca. Zrážky zo mzdy uhraďte oprávneným osobám samostatne.</p>
      </div>
    </template>
  </Dialog>
</template>

<style scoped>
.pp-head {
  display: flex;
  gap: 24px;
  align-items: flex-end;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.from,
.when {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.spacer {
  flex: 1;
}
.label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.iban {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.missing {
  color: var(--tone-overdue);
}
.settings {
  background: var(--paper);
  border: 1px solid var(--rule);
  padding: 10px 12px;
  margin-bottom: 12px;
}
.note {
  margin: 0 0 8px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.sg {
  grid-template-columns: 2fr 1fr;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  margin-top: 8px;
}
.pp-wrap {
  max-height: 52vh;
  overflow: auto;
  border-top: 2px solid var(--ink);
}
.pp {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.pp th {
  position: sticky;
  top: 0;
  background: var(--sheet);
  text-align: left;
  font-weight: 500;
  color: var(--ink-soft);
  padding: 5px 6px;
  border-bottom: 1px solid var(--rule);
}
.pp td {
  padding: 6px;
  border-bottom: 1px solid var(--rule);
  vertical-align: top;
}
.pp .chk {
  width: 32px;
}
.pp .num {
  text-align: right;
  white-space: nowrap;
}
.pp .sym,
.pp .acc {
  white-space: nowrap;
  color: var(--ink-soft);
}
.who {
  font-weight: 600;
}
.sub {
  color: var(--ink-soft);
}
.problem,
tr.off .problem {
  color: var(--tone-overdue);
}
tr.off td:not(:first-child) {
  color: var(--ink-faint);
}
.pp-footer {
  width: 100%;
}
.pp-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.sum {
  margin-right: auto;
}
.hint {
  margin: 8px 0 0;
  text-align: right;
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.muted {
  color: var(--ink-soft);
}
.error {
  color: var(--tone-overdue);
  margin: 8px 0 0;
}
@media (max-width: 760px) {
  .pp .sym,
  .pp .acc {
    display: none;
  }
  .sg {
    grid-template-columns: 1fr;
  }
  .pp-foot {
    flex-wrap: wrap;
  }
  .sum {
    width: 100%;
  }
}
</style>
