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
import { allYear, businessRate, lossesOut, lossRows, monthLabels, newChild, rcHint, visibleRows } from '../lib/taxreturn.js'

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
const moneyFields = ['empIncome', 'empInsurance', 'empPrepaid', 'dds', 'prepaid', 'empBonus']

function fill(t) {
  tr.value = t
  const f = { ...t.input, taxpayer: { ...t.input.taxpayer } }
  for (const k of moneyFields) f[k] = num(f[k])
  f.children = (t.input.children ?? []).map((c) => ({ ...c, months: [...(c.months ?? Array(12).fill(false))] }))
  f.spouseOn = !!t.input.spouse
  f.spouse = t.input.spouse ? { ...t.input.spouse, ownIncome: num(t.input.spouse.ownIncome) } : { name: '', rc: '', ownIncome: null, months: 12 }
  f.lossRows = lossRows(t.year, t.input.losses ?? [])
  f.attachments = t.input.attachments ?? 0
  form.value = f
}

// deti: celý rok alebo vybrané mesiace
function toggleAllYear(c, on) {
  c.months = Array(12).fill(false)
  if (!on) c.months[new Date().getMonth()] = true
}
const childErr = (i, k) => errors.value[`children.${i}.${k}`] || (k === 'rc' ? rcHint(form.value.children[i].rc) : '')

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
    body.spouse = body.spouseOn ? { ...body.spouse, ownIncome: str(body.spouse.ownIncome), months: body.spouse.months || 0 } : null
    body.losses = lossesOut(body.lossRows)
    body.attachments = body.attachments || 0
    delete body.spouseOn
    delete body.lossRows
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
                  {{ r.back ? `${r.label} za ${year - r.back}` : r.label }}
                  <small v-if="r.code === '96'" class="hint">{{ businessRate(tr.rows) }}</small>
                  <small v-if="r.code === '40' && form.flatExpenses" class="hint">60 % z príjmov (najviac 20 000 €) + zaplatené poistné</small>
                </td>
                <td class="num"><span :class="{ grand: r.total }">{{ formatAmount(tr.rows[r.code] ?? '0') }}</span></td>
              </tr>
            </tbody>
          </table>
          <p class="hint">
            Príjmy a výdavky sú z peňažného denníka, tabuľka 1a z výkazu o majetku a záväzkoch. Zahraničné príjmy a prenájom doplníte vo
            formulári po načítaní XML — formulár daň prepočíta.
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
            <legend>Deti — daňový bonus</legend>
            <p class="lead">
              Vyživované deti, ktoré s vami žili v domácnosti. Bonus je {{ year >= 2025 ? '100 € mesačne do 15 rokov a 50 € od 15 do 18 rokov' : '' }};
              vek Fiškus určí z rodného čísla. Pri vysokom základe dane sa bonus znižuje, najviac je percento zo základu podľa počtu detí.
            </p>
            <div v-for="(c, i) in form.children" :key="i" class="child">
              <div class="form-grid">
                <FormField label="Meno a priezvisko" :span="2" :error="err(`children.${i}.name`)">
                  <InputText v-model="c.name" size="small" :invalid="!!err(`children.${i}.name`)" :disabled="!canWrite" />
                </FormField>
                <FormField label="Rodné číslo" :error="childErr(i, 'rc')">
                  <InputText v-model="c.rc" size="small" inputmode="numeric" placeholder="RRMMDD/XXXX" :invalid="!!childErr(i, 'rc')" :disabled="!canWrite" />
                </FormField>
                <FormField label="V domácnosti">
                  <label class="check inline"><Checkbox :model-value="allYear(c.months)" binary :disabled="!canWrite" @update:model-value="(v) => toggleAllYear(c, v)" /> <span>celý rok</span></label>
                </FormField>
              </div>
              <div v-if="!allYear(c.months)" class="months" role="group" :aria-label="`Mesiace — ${c.name || 'dieťa'}`">
                <label v-for="(ml, mi) in monthLabels" :key="mi" class="mo" :class="{ on: c.months[mi] }">
                  <input v-model="c.months[mi]" type="checkbox" :disabled="!canWrite" /> {{ ml }}
                </label>
              </div>
              <Button v-if="canWrite" icon="pi pi-times" text size="small" severity="secondary" class="rm" :aria-label="`Odstrániť ${c.name || 'dieťa'}`"
                @click="form.children.splice(i, 1)" />
            </div>
            <Button v-if="canWrite" label="Pridať dieťa" icon="pi pi-plus" text size="small" @click="form.children.push(newChild())" />
          </fieldset>

          <fieldset class="sheet form-section">
            <legend>Manžel / manželka</legend>
            <label class="check"><Checkbox v-model="form.spouseOn" binary :disabled="!canWrite" />
              <span>Uplatňujem nezdaniteľnú časť na manžela/ku</span></label>
            <p class="lead">
              Iba ak žil/a s vami v domácnosti a staral/a sa o dieťa do 3 rokov (6 pri dlhodobo nepriaznivom zdraví), poberal/a príspevok
              na opatrovanie, bol/a uchádzačom o zamestnanie alebo je osobou so zdravotným postihnutím.
            </p>
            <div v-if="form.spouseOn" class="form-grid">
              <FormField label="Meno a priezvisko" :span="2" :error="err('spouse.name')">
                <InputText v-model="form.spouse.name" size="small" :invalid="!!err('spouse.name')" :disabled="!canWrite" />
              </FormField>
              <FormField label="Rodné číslo" :span="2" :error="err('spouse.rc') || rcHint(form.spouse.rc)">
                <InputText v-model="form.spouse.rc" size="small" inputmode="numeric" :invalid="!!(err('spouse.rc') || rcHint(form.spouse.rc))" :disabled="!canWrite" />
              </FormField>
              <FormField label="Vlastný príjem za rok" :span="2" :error="err('spouse.ownIncome')" hint="po odpočítaní poistného; bez prídavkov a rodičovského">
                <InputNumber v-model="form.spouse.ownIncome" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" />
              </FormField>
              <FormField label="Počet mesiacov" :span="2" :error="err('spouse.months')" hint="keď boli splnené podmienky">
                <InputNumber v-model="form.spouse.months" :min="1" :max="12" size="small" show-buttons :disabled="!canWrite" />
              </FormField>
            </div>
          </fieldset>

          <fieldset class="sheet form-section">
            <legend>Daňové straty z minulých rokov</legend>
            <p class="lead">Strata z priznania za posledných 5 rokov. Fiškus ju odpočíta od základu dane od najstaršej.</p>
            <table class="losses">
              <thead><tr><th>Rok</th><th>Vykázaná strata</th><th>Už odpočítané</th></tr></thead>
              <tbody>
                <tr v-for="(l, i) in form.lossRows" :key="l.year">
                  <td>{{ l.year }}</td>
                  <td><InputNumber v-model="l.amount" :min="0" :min-fraction-digits="2" locale="sk-SK" size="small" :disabled="!canWrite" :aria-label="`Strata ${l.year}`" /></td>
                  <td>
                    <InputNumber v-if="i < 4" v-model="l.used" :min="0" :min-fraction-digits="2" locale="sk-SK" size="small" :disabled="!canWrite" :aria-label="`Odpočítané ${l.year}`" />
                    <span v-else class="na">—</span>
                    <small v-if="err(`losses.${i}.used`)" class="ferr">{{ err(`losses.${i}.used`) }}</small>
                  </td>
                </tr>
              </tbody>
            </table>
          </fieldset>

          <fieldset class="sheet form-section">
            <legend>Zamestnanie (ak ste mali)</legend>
            <div class="form-grid">
              <FormField label="Úhrn príjmov (r. 36)" :span="2"><InputNumber v-model="form.empIncome" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Povinné poistné (r. 37)" :span="2"><InputNumber v-model="form.empInsurance" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Zrazené preddavky (r. 131)" :span="2"><InputNumber v-model="form.empPrepaid" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" /></FormField>
              <FormField label="Bonus na deti od zamestnávateľa (r. 119)" :span="2" :error="err('empBonus')"><InputNumber v-model="form.empBonus" :min="0" :min-fraction-digits="2" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" /></FormField>
            </div>
          </fieldset>

          <fieldset class="sheet form-section">
            <legend>Podiel dane, preplatok a prílohy</legend>
            <div class="form-grid">
              <FormField label="IČO prijímateľa 2 %" :error="err('ngoIco')"><InputText v-model="form.ngoIco" size="small" inputmode="numeric" :invalid="!!err('ngoIco')" :disabled="!canWrite" /></FormField>
              <FormField label="Názov prijímateľa" :span="3"><InputText v-model="form.ngoName" size="small" :disabled="!canWrite" /></FormField>
              <label class="check"><Checkbox v-model="form.volunteer" binary :disabled="!canWrite" /> <span>Som dobrovoľník — 3 %</span></label>
              <FormField label="IBAN na vrátenie preplatku a bonusu" :span="2" hint="preplatok alebo bonus vyšší ako daň">
                <InputText v-model="form.refundIban" size="small" :disabled="!canWrite" />
              </FormField>
              <FormField label="Počet príloh" :error="err('attachments')" hint="napr. potvrdenie od zamestnávateľa">
                <InputNumber v-model="form.attachments" :min="0" :max="99" size="small" show-buttons :disabled="!canWrite" />
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
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  gap: 8px;
}
.actions {
  display: flex;
  justify-content: flex-end;
}
.lead {
  margin: 0 0 10px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  max-width: 75ch;
}
.check.inline {
  grid-column: auto;
  min-height: 2rem;
}
.child {
  position: relative;
  padding: 8px 34px 8px 0;
  border-bottom: 1px solid var(--rule-soft);
  margin-bottom: 6px;
}
.child .rm {
  position: absolute;
  top: 26px;
  right: 0;
}
.months {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 6px;
}
.mo {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 2px 6px;
  border: 1px solid var(--rule);
  font-size: var(--fs-xs);
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}
.mo.on {
  background: var(--blue-wash);
  border-color: var(--blue);
  color: var(--blue-deep);
}
.losses {
  border-collapse: collapse;
  font-size: var(--fs-sm);
}
.losses th {
  text-align: left;
  color: var(--ink-soft);
  font-weight: 600;
  padding: 2px 6px 4px 0;
}
.losses td {
  padding: 2px 6px 2px 0;
}
.losses :deep(input) {
  width: 8.5rem;
  max-width: 38vw;
  text-align: right;
}
.na {
  color: var(--ink-faint);
  padding-left: 8px;
}
.ferr {
  display: block;
  color: var(--tone-overdue);
  font-size: var(--fs-xs);
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
