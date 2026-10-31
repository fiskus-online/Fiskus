<script setup>
// Daňové priznanie k dani z príjmov FO typ B: sumy z peňažného denníka,
// údaje daňovníka a výpočet dane zo servera, XML na načítanie do formulára
// DPFOB na portáli finančnej správy.
import { computed, onMounted, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import FormField from '../components/FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount } from '../lib/format.js'
import { saveBlob } from '../lib/export.js'
import { businessRate, visibleRows } from '../lib/taxreturn.js'

const toast = useToast()
const canWrite = computed(() => can('ledger.write'))
const thisYear = new Date().getFullYear()
// priznanie za minulý rok sa podáva do 31. 3. — v prvej polovici roka ponúkni ten
const year = ref(new Date().getMonth() < 6 ? thisYear - 1 : thisYear)
const tr = ref(null)
const form = ref(null)
const errors = ref({})
const busy = ref(false)
const loadError = ref('')

const num = (s) => (s === undefined || s === null || s === '' || Number(s) === 0 ? null : Number(s))
const str = (n) => (n === null || n === undefined ? '0' : String(n))
const moneyFields = ['empIncome', 'empInsurance', 'empPrepaid', 'dds', 'prepaid']

function fill(t) {
  tr.value = t
  const f = { ...t.input, taxpayer: { ...t.input.taxpayer } }
  for (const k of moneyFields) f[k] = num(f[k])
  form.value = f
}

async function load() {
  loadError.value = ''
  try {
    fill(await api.taxReturn(session.companyId, year.value))
  } catch (e) {
    tr.value = null
    loadError.value = e.message
  }
}
onMounted(load)
watch(year, load)

const yearOptions = computed(() => (tr.value?.years ?? [thisYear, thisYear - 1]).map((y) => ({ label: String(y), value: y })))
const rows = computed(() => (tr.value ? visibleRows(tr.value.rows) : []))
const expenseModes = [
  { label: 'Skutočné výdavky', value: false },
  { label: 'Paušálne 60 %', value: true },
]

async function save(quiet = false) {
  busy.value = true
  try {
    const body = { ...form.value }
    for (const k of moneyFields) body[k] = str(body[k])
    fill(await api.saveTaxReturn(session.companyId, year.value, body))
    errors.value = {}
    if (!quiet) toast.add({ severity: 'success', summary: 'Údaje uložené, daň prepočítaná', life: 2500 })
    return true
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      errors.value = e.fieldMap()
      toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené polia', detail: e.fields[0].message, life: 5000 })
    } else {
      toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
    }
    return false
  } finally {
    busy.value = false
  }
}

async function downloadXML() {
  if (canWrite.value && !(await save(true))) return
  try {
    const { blob, name } = await api.taxReturnXML(session.companyId, year.value)
    saveBlob(blob, name)
    toast.add({ severity: 'success', summary: `Uložené ${name}`, detail: 'Načítajte ho vo formulári DPFOB na portáli finančnej správy (Načítať zo súboru).', life: 8000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'XML sa nevytvorilo', detail: e.message, life: 6000 })
  }
}
const err = (k) => errors.value[k]
</script>

<template>
  <div class="taxret">
    <header class="head">
      <h1>Daň z príjmov <span class="sub">priznanie FO typ B</span></h1>
      <Select v-model="year" :options="yearOptions" option-label="label" option-value="value" size="small" aria-label="Zdaňovacie obdobie" />
      <div class="spacer" />
      <Button v-if="tr" label="Stiahnuť XML pre eDane" icon="pi pi-download" size="small" :loading="busy" @click="downloadXML" />
    </header>

    <p v-if="loadError" class="warn">{{ loadError }}</p>

    <template v-if="tr && form">
      <ul v-if="tr.notes?.length" class="notes">
        <li v-for="n in tr.notes" :key="n"><i class="pi pi-exclamation-circle" aria-hidden="true" /> {{ n }}</li>
      </ul>

      <div class="body">
        <!-- výpočet ako list z účtovnej knihy -->
        <section class="sheet ledger" aria-label="Výpočet dane">
          <h2>Výpočet za {{ year }}</h2>
          <table>
            <tbody>
              <tr v-for="r in rows" :key="r.code" :class="{ sum: r.sum, total: r.total }">
                <td class="code">{{ r.code }}</td>
                <td>
                  {{ r.label }}
                  <small v-if="r.code === '96'" class="hint">{{ businessRate(tr.rows) }}</small>
                  <small v-if="r.code === '40' && form.flatExpenses" class="hint">60 % z príjmov (najviac 20 000 €) + zaplatené poistné</small>
                </td>
                <td class="num"><span :class="{ grand: r.total }">{{ formatAmount(tr.rows[r.code] ?? '0') }}</span></td>
              </tr>
            </tbody>
          </table>
          <p class="hint">
            Príjmy a výdavky sú z peňažného denníka, tabuľka 1a z výkazu o majetku a záväzkoch. Deti (daňový bonus), manžela/ku a prílohy
            doplníte vo formulári po načítaní XML — formulár daň prepočíta.
          </p>
        </section>

        <!-- údaje, ktoré zadáva daňovník -->
        <div class="forms">
          <fieldset class="sheet form-section">
            <legend>Daňovník</legend>
            <div class="form-grid">
              <FormField label="Meno" :span="2"><InputText v-model="form.taxpayer.name" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Priezvisko" :span="2"><InputText v-model="form.taxpayer.surname" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="DIČ" :error="err('taxpayer.dic')"><InputText v-model="form.taxpayer.dic" size="small" inputmode="numeric" :invalid="!!err('taxpayer.dic')" :disabled="!canWrite" /></FormField>
              <FormField label="Titul pred"><InputText v-model="form.taxpayer.title" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Titul za"><InputText v-model="form.taxpayer.titleAfter" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Dátum narodenia" :error="err('taxpayer.birthDate')" hint="iba bez DIČ">
                <InputText v-model="form.taxpayer.birthDate" size="small" placeholder="DD.MM.RRRR" :invalid="!!err('taxpayer.birthDate')" :disabled="!canWrite" />
              </FormField>
              <FormField label="Ulica (trvalý pobyt)" :span="3"><InputText v-model="form.taxpayer.street" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Číslo"><InputText v-model="form.taxpayer.number" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="PSČ"><InputText v-model="form.taxpayer.zip" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Obec" :span="2"><InputText v-model="form.taxpayer.city" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Štát"><InputText v-model="form.taxpayer.country" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="SK NACE" :error="err('taxpayer.nace')" hint="napr. 43210">
                <InputText v-model="form.taxpayer.nace" size="small" inputmode="numeric" :invalid="!!err('taxpayer.nace')" :disabled="!canWrite" />
              </FormField>
              <FormField label="Činnosť s najvyšším príjmom" :span="3"><InputText v-model="form.taxpayer.activity" size="small" :disabled="!canWrite" /></FormField>
            </div>
          </fieldset>

          <fieldset class="sheet form-section">
            <legend>Podnikanie a preddavky</legend>
            <SelectButton v-model="form.flatExpenses" :options="expenseModes" option-label="label" option-value="value" :allow-empty="false" size="small"
              :disabled="!canWrite" aria-label="Výdavky" class="modes" />
            <div class="form-grid">
              <FormField label="Zaplatené preddavky na daň" :span="2" :error="err('prepaid')" hint="z podnikania, za rok">
                <InputNumber v-model="form.prepaid" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" />
              </FormField>
              <FormField label="Doplnkové dôchodkové sporenie" :span="2" :error="err('dds')" hint="zaplatené príspevky, uzná sa najviac 180 €">
                <InputNumber v-model="form.dds" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" />
              </FormField>
            </div>
          </fieldset>

          <fieldset class="sheet form-section">
            <legend>Zamestnanie (ak ste mali)</legend>
            <div class="form-grid">
              <FormField label="Úhrn príjmov (r. 36)" :span="2"><InputNumber v-model="form.empIncome" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Povinné poistné (r. 37)" :span="2"><InputNumber v-model="form.empInsurance" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Zrazené preddavky (r. 131)" :span="2"><InputNumber v-model="form.empPrepaid" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" /></FormField>
            </div>
          </fieldset>

          <fieldset class="sheet form-section">
            <legend>Podiel dane a preplatok</legend>
            <div class="form-grid">
              <FormField label="IČO prijímateľa 2 %" :error="err('ngoIco')"><InputText v-model="form.ngoIco" size="small" inputmode="numeric" :invalid="!!err('ngoIco')" :disabled="!canWrite" /></FormField>
              <FormField label="Názov prijímateľa" :span="3"><InputText v-model="form.ngoName" size="small" :disabled="!canWrite" /></FormField>
              <label class="check"><Checkbox v-model="form.volunteer" binary :disabled="!canWrite" /> <span>Som dobrovoľník — 3 %</span></label>
              <FormField label="IBAN na vrátenie preplatku" :span="2" hint="iba ak vám vznikne preplatok">
                <InputText v-model="form.refundIban" size="small" :disabled="!canWrite" />
              </FormField>
              <FormField label="Dátum vyhlásenia" :error="err('date')"><InputText v-model="form.date" size="small" placeholder="DD.MM.RRRR" :invalid="!!err('date')" :disabled="!canWrite" /></FormField>
            </div>
          </fieldset>

          <div v-if="canWrite" class="actions">
            <Button label="Uložiť a prepočítať" icon="pi pi-refresh" severity="secondary" outlined size="small" :loading="busy" @click="save()" />
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.taxret {
  padding: 14px 16px 24px;
  max-width: 1280px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
}
.sub {
  font-size: var(--fs-md);
  font-weight: 400;
  color: var(--ink-faint);
}
.spacer {
  flex: 1;
}
.notes {
  list-style: none;
  margin: 12px 0 0;
  padding: 8px 12px;
  background: var(--tone-draft-bg);
  color: var(--tone-draft);
  border-left: 3px solid var(--tone-draft);
}
.notes li + li {
  margin-top: 4px;
}
.warn {
  margin-top: 12px;
  color: var(--tone-overdue);
}
.body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.25fr);
  gap: 14px;
  margin-top: 14px;
  align-items: start;
}
.sheet {
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.ledger {
  border-top: 3px solid var(--blue-deep);
  padding: 12px 14px;
  position: sticky;
  top: 10px;
}
.ledger h2 {
  font-size: var(--fs-md);
  margin-bottom: 6px;
}
.ledger table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.ledger td {
  padding: 5px 4px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: top;
}
.ledger .code {
  width: 40px;
  color: var(--ink-faint);
  font-variant-numeric: tabular-nums;
}
.ledger .num {
  text-align: right;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.ledger tr.sum td {
  font-weight: 600;
  border-top: 1px solid var(--rule);
}
.ledger tr.total td {
  font-weight: 700;
  font-size: var(--fs-lg);
  border-bottom: 0;
  padding-top: 10px;
}
.grand {
  text-decoration: underline double;
  text-underline-offset: 4px;
}
.hint {
  display: block;
  margin-top: 4px;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  font-weight: 400;
}
.forms {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}
.form-section {
  margin: 0;
  padding: 12px 14px;
}
.modes {
  margin-bottom: 10px;
}
.check {
  grid-column: span 4;
  display: flex;
  align-items: center;
  gap: 8px;
}
.actions {
  display: flex;
  justify-content: flex-end;
}
@media (max-width: 980px) {
  .body {
    grid-template-columns: minmax(0, 1fr);
  }
  .ledger {
    position: static;
  }
}
@media (max-width: 760px) {
  .taxret {
    padding: 10px 10px 20px;
  }
  .head h1 {
    font-size: var(--fs-lg);
  }
  .sub {
    display: block;
  }
}
</style>
