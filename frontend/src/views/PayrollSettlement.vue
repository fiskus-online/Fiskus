<script setup>
// Ročné zúčtovanie preddavkov na daň zo závislej činnosti (§ 38 ZDP):
// výpočet zo súčtu uzavretých mesiacov, žiadosť a doložené údaje.
// Všetky sumy počíta server.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { settlementColumns } from '../lib/columns.js'
import { contractShort, employeeName, filterSettlements, settleMonthOptions, settlementFromForm, settlementLines, settlementOutcome, settlementToForm } from '../lib/payroll.js'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const canWrite = computed(() => can('ledger.write'))
const thisYear = new Date().getFullYear()
// v prvom štvrťroku sa zúčtúva predchádzajúci rok
const year = ref(Number(route.query.rok) || (new Date().getMonth() < 4 ? thisYear - 1 : thisYear))
const years = [thisYear - 1, thisYear].map((y) => ({ label: String(y), value: y }))
const view = ref(null)
const loading = ref(true)
const search = ref('')
const onlyRequested = ref(false)

async function load() {
  loading.value = true
  try {
    view.value = await api.settlements(session.companyId, year.value)
  } catch (e) {
    view.value = null
    toast.add({ severity: 'error', summary: 'Ročné zúčtovanie sa nenačítalo', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(year, (y) => {
  router.replace({ query: { rok: y } })
  load()
})
const visible = computed(() => filterSettlements(view.value?.list, { search: search.value, requested: onlyRequested.value }))
const monthsLabel = (ms) => (ms.length === 12 ? 'celý rok' : `${ms.length} mes.`)

// ---- detail a žiadosť ----
const current = ref(null)
const form = ref(null)
const errors = ref({})
const busy = ref(false)
function open(r) {
  current.value = r
  form.value = settlementToForm(r.request)
  errors.value = {}
}
async function save() {
  busy.value = true
  try {
    view.value = await api.saveSettlement(session.companyId, year.value, current.value.employee.id, settlementFromForm(form.value))
    current.value = view.value.list.find((r) => r.employee.id === current.value.employee.id)
    errors.value = {}
    toast.add({ severity: 'success', summary: 'Uložené', life: 2500 })
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Neuložené', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
const printOne = (r) => router.push({ path: '/rocne-zuctovanie/tlac', query: { rok: year.value, id: r.employee.id } })
</script>

<template>
  <section class="page">
    <header class="head">
      <h1>Ročné zúčtovanie</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať zamestnanca" size="small" aria-label="Hľadať" />
      </IconField>
      <label class="only"><ToggleSwitch v-model="onlyRequested" /> iba žiadosti</label>
      <ExportButton :name="`Ročné zúčtovanie ${year}`" :columns="settlementColumns" :rows="visible" />
    </header>

    <p v-if="view" class="info">
      Zamestnávateľ vykoná ročné zúčtovanie na písomnú žiadosť zamestnanca do {{ formatDate(view.deadline) }}. Výpočet je zo súčtu uzavretých mesiacov
      roka; údaje od iných zamestnávateľov, manžela/manželku a III. pilier doplníte podľa dokladov zamestnanca.
      <strong v-if="view.open.length" class="warn">Neuzavreté mesiace: {{ view.open.join(', ') }} — výsledok je predbežný.</strong>
    </p>

    <p v-if="loading" class="muted">Načítavam…</p>
    <p v-else-if="view && !view.list.length" class="muted">V roku {{ year }} nie sú uzavreté mzdy.</p>
    <div v-else-if="view" class="wrap">
      <table class="list">
        <thead>
          <tr>
            <th>Zamestnanec</th>
            <th class="hide-m">Obdobie</th>
            <th class="num hide-m">Zdaniteľná mzda</th>
            <th class="num hide-m">Daň</th>
            <th class="num hide-m">Preddavky</th>
            <th class="num">Zrazí / vráti</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in visible" :key="r.employee.id" tabindex="0" @click="open(r)" @keydown.enter="open(r)">
            <td>
              <div class="who">{{ employeeName(r.employee) }}</div>
              <div class="sub">
                {{ contractShort(r.employee.contract) }}
                <span v-if="r.request.requested" class="tag">žiadosť</span>
              </div>
            </td>
            <td class="hide-m">{{ monthsLabel(r.months) }}</td>
            <td class="num hide-m">{{ formatAmount(r.rows['05']) }}</td>
            <td class="num hide-m">{{ formatAmount(r.rows['06']) }}</td>
            <td class="num hide-m">{{ formatAmount(r.rows['18']) }}</td>
            <td class="num" :class="{ minus: Number(r.result) < 0 }">
              {{ formatAmount(r.result) }}
              <div class="sub">{{ settlementOutcome(r.result) }}</div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Dialog :visible="!!current" :header="current ? `Ročné zúčtovanie ${year} — ${employeeName(current.employee)}` : ''" modal :style="{ width: '52rem' }"
      :breakpoints="{ '760px': '96vw' }" @update:visible="(v) => !v && (current = null)">
      <div v-if="current" class="detail">
        <form class="req" @submit.prevent="save">
          <label class="chk"><Checkbox v-model="form.requested" binary /> Zamestnanec písomne požiadal o ročné zúčtovanie</label>
          <div class="form-grid">
            <FormField label="Príspevky na III. pilier (DDS) €" :error="errors.dds" hint="najviac 180 € ročne"><InputText v-model="form.dds" size="small" inputmode="decimal" fluid :disabled="!canWrite" /></FormField>
            <FormField label="Dátum vykonania" :error="errors.date"><DatePicker :model-value="form.date ? fromISODate(form.date) : null" date-format="d. m. yy" show-icon icon-display="input" size="small" fluid :disabled="!canWrite" @update:model-value="(d) => (form.date = d ? toISODate(d) : '')" /></FormField>
          </div>
          <FormField label="Vyrovnať vo mzde" :error="errors.settleMonth" hint="preplatok vráti alebo nedoplatok zrazí v zvolenej mzde">
            <Select v-model="form.settleMonth" :options="settleMonthOptions(year)" option-label="label" option-value="value" size="small" fluid :disabled="!canWrite" />
          </FormField>
          <label class="chk"><Checkbox v-model="form.spouse" binary :disabled="!canWrite" /> Nezdaniteľná časť na manžela/manželku</label>
          <div v-if="form.spouse" class="form-grid">
            <FormField label="Meno manžela/manželky"><InputText v-model="form.spouseName" size="small" fluid :disabled="!canWrite" /></FormField>
            <FormField label="Mesiace" :error="errors['spouse.months']"><InputNumber v-model="form.spouseMonths" :min="1" :max="12" size="small" fluid :disabled="!canWrite" /></FormField>
            <FormField label="Vlastný príjem manžela/ky €" hint="znížený o poistné"><InputText v-model="form.spouseIncome" size="small" inputmode="decimal" fluid :disabled="!canWrite" /></FormField>
          </div>
          <p class="sec">Iní zamestnávatelia v roku (z ich potvrdení)</p>
          <div class="form-grid three">
            <FormField label="Príjmy €" :error="errors.otherIncome"><InputText v-model="form.otherIncome" size="small" inputmode="decimal" fluid :disabled="!canWrite" /></FormField>
            <FormField label="Sociálne poistenie €" :error="errors.otherSocial"><InputText v-model="form.otherSocial" size="small" inputmode="decimal" fluid :disabled="!canWrite" /></FormField>
            <FormField label="Zdravotné poistenie €" :error="errors.otherHealth"><InputText v-model="form.otherHealth" size="small" inputmode="decimal" fluid :disabled="!canWrite" /></FormField>
            <FormField label="Preddavky na daň €" :error="errors.otherPrepaid"><InputText v-model="form.otherPrepaid" size="small" inputmode="decimal" fluid :disabled="!canWrite" /></FormField>
            <FormField label="Vyplatený daňový bonus €" :error="errors.otherBonus"><InputText v-model="form.otherBonus" size="small" inputmode="decimal" fluid :disabled="!canWrite" /></FormField>
          </div>
          <div v-if="canWrite" class="actions"><Button type="submit" label="Uložiť a prepočítať" icon="pi pi-refresh" size="small" :loading="busy" /></div>
        </form>
        <table class="rz">
          <tbody>
            <tr v-for="[code, text] in settlementLines" :key="code" :class="{ main: !code.match(/[a-c]$/) }">
              <td class="code">{{ code }}</td>
              <td>{{ text }}</td>
              <td class="num">{{ formatAmount(current.rows[code]) }}</td>
            </tr>
            <tr class="total">
              <td colspan="2">{{ settlementOutcome(current.result) }}</td>
              <td class="num"><span class="ledger">{{ formatMoney(current.result) }}</span></td>
            </tr>
          </tbody>
        </table>
        <p v-for="n in current.notes" :key="n" class="note">{{ n }}</p>
      </div>
      <template #footer>
        <Button label="Zavrieť" text size="small" @click="current = null" />
        <Button label="Tlačiť doklad" icon="pi pi-print" severity="secondary" size="small" @click="printOne(current)" />
      </template>
    </Dialog>
  </section>
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
  margin: 0;
}
.spacer {
  flex: 1;
}
.only {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
}
.info {
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  max-width: 80ch;
}
.warn {
  color: var(--tone-overdue);
  font-weight: 600;
}
.muted {
  color: var(--ink-soft);
}
.wrap {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
  overflow-x: auto;
}
.list {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.list th {
  text-align: left;
  font-weight: 500;
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
}
.list td {
  padding: 7px 10px;
  border-bottom: 1px solid var(--rule);
  vertical-align: top;
}
.list tbody tr {
  cursor: pointer;
}
.list tbody tr:hover,
.list tbody tr:focus-visible {
  background: var(--paper);
}
.num {
  text-align: right;
  white-space: nowrap;
}
th.num {
  text-align: right;
}
.minus {
  color: var(--blue);
}
.who {
  font-weight: 600;
}
.sub {
  color: var(--ink-soft);
  font-size: var(--fs-xs);
}
.tag {
  border: 1px solid var(--rule);
  padding: 0 4px;
  margin-left: 4px;
  border-radius: 2px;
}
.detail {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}
.req {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.chk {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
}
.sec {
  margin: 6px 0 0;
  font-size: var(--fs-sm);
  font-weight: 600;
}
.three {
  grid-template-columns: 1fr 1fr;
}
.actions {
  display: flex;
  justify-content: flex-end;
}
.rz {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
  align-self: start;
}
.rz td {
  padding: 3px 4px;
  border-bottom: 1px solid var(--rule);
  color: var(--ink-soft);
}
.rz .main td {
  color: var(--ink);
}
.rz .code {
  width: 2.6rem;
  color: var(--ink-faint);
}
.rz .total td {
  border-top: 2px solid var(--ink);
  border-bottom: none;
  font-weight: 700;
  color: var(--ink);
  padding-top: 6px;
}
.ledger {
  border-bottom: 3px double currentColor;
}
.note {
  grid-column: 1 / -1;
  margin: 0;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
@media (max-width: 760px) {
  .hide-m {
    display: none;
  }
  .detail {
    grid-template-columns: 1fr;
  }
  .search {
    flex: 1 1 10rem;
  }
}
</style>
