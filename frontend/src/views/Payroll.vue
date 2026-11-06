<script setup>
// Mzdy — výplatná listina mesiaca: hrubá mzda, odvody, preddavok na daň,
// daňový bonus, čistá mzda a súčty na úhradu. Všetko počíta server; tu sa
// zadávajú iba mesačné údaje (hodiny, odmeny, náhrady, PN, zrážky).
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Drawer from 'primevue/drawer'
import InputText from 'primevue/inputtext'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatMoney } from '../lib/format.js'
import { payrollColumns } from '../lib/columns.js'
import { contractShort, employeeName, inputFromForm, inputToForm, insurerLabel, periodLabel, shiftMonth } from '../lib/payroll.js'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))

const now = new Date()
const year = computed(() => Number(route.query.rok) || now.getFullYear())
const month = computed(() => Number(route.query.mesiac) || now.getMonth() + 1)
const view = ref(null)
const error = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    view.value = await api.payroll(session.companyId, year.value, month.value)
    error.value = ''
  } catch (e) {
    view.value = null
    error.value = e.fields?.[0]?.message ?? e.message
  } finally {
    loading.value = false
  }
}
watch([year, month], load, { immediate: true })
function go(d) {
  const p = shiftMonth(year.value, month.value, d)
  router.replace({ query: { rok: p.year, mesiac: p.month } })
}
const rows = computed(() => view.value?.rows ?? [])
const t = computed(() => view.value?.totals)
const editable = computed(() => canWrite.value && view.value && !view.value.closed)

// ---- detail zamestnanca: výplatná páska a mesačné údaje ----
const openId = ref(null)
const row = computed(() => rows.value.find((r) => r.employee.id === openId.value))
const form = ref(null)
const errors = ref({})
const busy = ref(false)
function open(r) {
  openId.value = r.employee.id
  form.value = inputToForm(r.input)
  errors.value = {}
}
const drawer = computed({ get: () => !!row.value, set: (v) => !v && (openId.value = null) })
async function saveInput() {
  busy.value = true
  try {
    const inputs = rows.value.map((r) => (r.employee.id === openId.value ? inputFromForm(form.value) : r.input))
    view.value = await api.savePayroll(session.companyId, year.value, month.value, inputs)
    form.value = inputToForm(row.value.input)
    errors.value = {}
    toast.add({ severity: 'success', summary: 'Prepočítané', life: 1500 })
  } catch (e) {
    const idx = rows.value.findIndex((r) => r.employee.id === openId.value)
    const all = e instanceof ApiError ? e.fieldMap() : {}
    errors.value = Object.fromEntries(Object.entries(all).filter(([k]) => k.startsWith(`inputs.${idx}.`)).map(([k, v]) => [k.split('.').pop(), v]))
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Neuložené', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

// ---- uzavretie a otvorenie mesiaca ----
function closeMonth() {
  confirm.require({
    header: `Uzavrieť mzdy — ${periodLabel(year.value, month.value)}`,
    message: 'Výsledky sa uložia tak, ako sú, a neprepočítajú sa ani po zmene údajov zamestnanca. Mesiac sa dá otvoriť s uvedením dôvodu.',
    acceptProps: { label: 'Uzavrieť', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      try {
        view.value = await api.closePayroll(session.companyId, year.value, month.value)
        toast.add({ severity: 'success', summary: 'Mzdy sú uzavreté', life: 3000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Mzdy sa neuzavreli', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
      }
    },
  })
}
const reopening = ref(false)
const reason = ref('')
const reasonErr = ref('')
async function reopen() {
  try {
    view.value = await api.reopenPayroll(session.companyId, year.value, month.value, reason.value)
    reopening.value = false
    reason.value = ''
    reasonErr.value = ''
  } catch (e) {
    reasonErr.value = e.fields?.[0]?.message ?? e.message
  }
}
const health = computed(() => Object.entries(t.value?.health ?? {}).sort())
const fmtClosed = (s) => (s ? new Date(s).toLocaleDateString('sk-SK') : '')
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Mzdy</h1>
      <div class="period">
        <Button icon="pi pi-chevron-left" text size="small" aria-label="Predchádzajúci mesiac" @click="go(-1)" />
        <strong>{{ periodLabel(year, month) }}</strong>
        <Button icon="pi pi-chevron-right" text size="small" aria-label="Nasledujúci mesiac" @click="go(1)" />
      </div>
      <span v-if="view?.closed" class="tag" data-tone="paid" :title="`uzavrel ${view.closedBy} ${fmtClosed(view.closedAt)}`"><span class="tag-text"><i class="pi pi-lock" /> uzavreté</span></span>
      <div class="spacer" />
      <ExportButton v-if="rows.length" :name="`Mzdy ${periodLabel(year, month)}`" :columns="payrollColumns" :rows="rows" />
      <Button v-if="rows.length" label="Výplatné pásky" icon="pi pi-print" severity="secondary" size="small" outlined
        @click="router.push({ path: '/mzdy/tlac', query: { rok: year, mesiac: month } })" />
      <template v-if="canWrite && view">
        <Button v-if="view.closed" label="Otvoriť…" icon="pi pi-lock-open" text size="small" @click="reopening = true" />
        <Button v-else-if="rows.length" label="Uzavrieť mesiac" icon="pi pi-lock" size="small" @click="closeMonth" />
      </template>
    </header>

    <p v-if="error" class="err-box">{{ error }}</p>
    <p v-else-if="view && !rows.length" class="empty">
      V tomto mesiaci nikto nepracuje. <RouterLink to="/zamestnanci">Pridajte zamestnancov</RouterLink> — mzdy sa potom vypočítajú samy.
    </p>

    <section v-if="t && rows.length" class="sums" aria-label="Na úhradu">
      <div><span>Sociálna poisťovňa</span><strong class="num">{{ formatMoney(t.social) }}</strong><small>zamestnanci {{ formatAmount(t.socialEmployee) }} + firma {{ formatAmount(t.socialEmployer) }}</small></div>
      <div v-for="[ins, v] in health" :key="ins"><span>{{ insurerLabel(ins) }}</span><strong class="num">{{ formatMoney(v) }}</strong><small>zdravotné poistenie</small></div>
      <div><span>Daňový úrad</span><strong class="num">{{ formatMoney(t.tax) }}</strong><small>preddavky po daňovom bonuse</small></div>
      <div><span>Výplaty</span><strong class="num">{{ formatMoney(t.toPay) }}</strong><small>čistá mzda {{ formatAmount(t.net) }}</small></div>
      <div class="cost"><span>Cena práce</span><strong class="num"><span class="ledger">{{ formatMoney(t.cost) }}</span></strong><small>hrubé mzdy {{ formatAmount(t.gross) }}</small></div>
    </section>

    <div v-if="rows.length" class="wrap">
      <table class="pl">
        <thead>
          <tr>
            <th>Zamestnanec</th><th class="num">Hrubá mzda</th><th class="num">Odvody zam.</th><th class="num">Preddavok</th><th class="num">Bonus</th>
            <th class="num">Čistá mzda</th><th class="num">Na výplatu</th><th class="num">Odvody firmy</th><th class="num">Cena práce</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.employee.id" tabindex="0" @click="open(r)" @keydown.enter="open(r)">
            <td class="name">
              <strong>{{ employeeName(r.employee) }}</strong>
              <small>{{ contractShort(r.employee.contract) }}<template v-if="r.employee.hourly"> · {{ formatAmount(r.input.hours) || 0 }} h</template></small>
              <i v-if="r.result.notes?.length" class="pi pi-info-circle note" :title="r.result.notes.join('\n')" aria-hidden="true" />
            </td>
            <td class="num">{{ formatAmount(r.result.gross) }}</td>
            <td class="num">{{ formatAmount(r.result.contrib) }}</td>
            <td class="num">{{ formatAmount(r.result.tax) }}</td>
            <td class="num">{{ Number(r.result.taxBonus) ? formatAmount(r.result.taxBonus) : '' }}</td>
            <td class="num">{{ formatAmount(r.result.net) }}</td>
            <td class="num strong">{{ formatAmount(r.result.toPay) }}</td>
            <td class="num">{{ formatAmount(r.result.contribER) }}</td>
            <td class="num">{{ formatAmount(r.result.cost) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="rows.length" class="foot">Sadzby a limity podľa legislatívy {{ year }}. Kliknutím na zamestnanca zadáte hodiny, odmeny, dovolenku, PN a zrážky a uvidíte výplatnú pásku.</p>

    <Drawer v-model:visible="drawer" position="right" class="pl-panel" :header="row ? employeeName(row.employee) : ''">
      <template v-if="row">
        <p class="sub">{{ contractShort(row.employee.contract) }} · {{ insurerLabel(row.employee.insurer) }}<template v-if="row.employee.nczd"> · NČZD</template></p>
        <form v-if="form" class="form-grid in-form" @submit.prevent="saveInput">
          <FormField v-if="row.employee.hourly" label="Odpracované hodiny" :error="errors.hours"><InputText v-model="form.hours" size="small" inputmode="decimal" class="num" :disabled="!editable" /></FormField>
          <FormField label="Základná mzda" :error="errors.base" hint="prázdne = podľa zmluvy"><InputText v-model="form.base" size="small" inputmode="decimal" class="num" :placeholder="formatAmount(row.result.baseWage)" :disabled="!editable" /></FormField>
          <FormField label="Odmeny a príplatky" :error="errors.extra"><InputText v-model="form.extra" size="small" inputmode="decimal" class="num" :disabled="!editable" /></FormField>
          <FormField label="Náhrada mzdy" :error="errors.leave" hint="dovolenka, sviatok"><InputText v-model="form.leave" size="small" inputmode="decimal" class="num" :disabled="!editable" /></FormField>
          <FormField label="Náhrada pri PN" :error="errors.sickPay" hint="bez odvodov, zdaňuje sa"><InputText v-model="form.sickPay" size="small" inputmode="decimal" class="num" :disabled="!editable" /></FormField>
          <FormField label="Zrážky" :error="errors.deduction" hint="z čistej mzdy"><InputText v-model="form.deduction" size="small" inputmode="decimal" class="num" :disabled="!editable" /></FormField>
          <FormField label="Poznámka" :span="2"><InputText v-model="form.note" size="small" :disabled="!editable" /></FormField>
          <div v-if="editable" class="actions"><Button type="submit" label="Prepočítať" icon="pi pi-refresh" size="small" :loading="busy" /></div>
        </form>

        <ul v-if="row.result.notes?.length" class="notes">
          <li v-for="(n, i) in row.result.notes" :key="i"><i class="pi pi-info-circle" aria-hidden="true" /> {{ n }}</li>
        </ul>

        <table class="slip">
          <tbody>
            <tr><td>Základná mzda</td><td class="num">{{ formatAmount(row.result.baseWage) }}</td></tr>
            <tr v-if="Number(row.input.extra)"><td>Odmeny a príplatky</td><td class="num">{{ formatAmount(row.input.extra) }}</td></tr>
            <tr v-if="Number(row.input.leave)"><td>Náhrada mzdy</td><td class="num">{{ formatAmount(row.input.leave) }}</td></tr>
            <tr class="sum"><td>Hrubá mzda</td><td class="num">{{ formatAmount(row.result.gross) }}</td></tr>
            <tr v-if="Number(row.result.sickPay)"><td>Náhrada príjmu pri PN</td><td class="num">{{ formatAmount(row.result.sickPay) }}</td></tr>
            <tr class="hd"><td colspan="2">Poistné zamestnanca</td></tr>
            <tr v-for="f in row.result.social.filter((x) => Number(x.employee))" :key="f.fund"><td class="in">{{ f.name }}</td><td class="num">−{{ formatAmount(f.employee) }}</td></tr>
            <tr><td class="in">Zdravotné poistenie</td><td class="num">−{{ formatAmount(row.result.healthEmployee) }}</td></tr>
            <tr class="sum"><td>Základ dane</td><td class="num">{{ formatAmount(row.result.taxBase) }}</td></tr>
            <tr v-if="Number(row.result.nczd)"><td class="in">Nezdaniteľná časť</td><td class="num">{{ formatAmount(row.result.nczd) }}</td></tr>
            <tr><td>Preddavok na daň</td><td class="num">{{ Number(row.result.tax) ? '−' : '' }}{{ formatAmount(row.result.tax) }}</td></tr>
            <tr v-if="Number(row.result.taxBonus)"><td>Daňový bonus na deti</td><td class="num">+{{ formatAmount(row.result.taxBonus) }}</td></tr>
            <tr class="sum"><td>Čistá mzda</td><td class="num">{{ formatAmount(row.result.net) }}</td></tr>
            <tr v-if="Number(row.result.deduction)"><td>Zrážky</td><td class="num">−{{ formatAmount(row.result.deduction) }}</td></tr>
            <tr class="total"><td>Na výplatu</td><td class="num"><span class="ledger">{{ formatMoney(row.result.toPay) }}</span></td></tr>
            <tr class="hd"><td colspan="2">Platí firma navyše</td></tr>
            <tr v-for="f in row.result.social.filter((x) => Number(x.employer))" :key="'er' + f.fund"><td class="in">{{ f.name }}</td><td class="num">{{ formatAmount(f.employer) }}</td></tr>
            <tr><td class="in">Zdravotné poistenie</td><td class="num">{{ formatAmount(row.result.healthEmployer) }}</td></tr>
            <tr class="sum"><td>Cena práce</td><td class="num">{{ formatAmount(row.result.cost) }}</td></tr>
          </tbody>
        </table>
      </template>
    </Drawer>

    <Dialog v-model:visible="reopening" :header="`Otvoriť mzdy — ${periodLabel(year, month)}`" modal :style="{ width: '28rem' }" :breakpoints="{ '480px': '96vw' }">
      <p class="dlg">Otvorené mzdy sa prepočítajú podľa aktuálnych údajov. Dôvod sa zapíše do auditu.</p>
      <label class="dlg-l">Dôvod
        <InputText v-model="reason" size="small" fluid autofocus :invalid="!!reasonErr" placeholder="napr. dodatočná odmena" />
      </label>
      <small v-if="reasonErr" class="err">{{ reasonErr }}</small>
      <template #footer>
        <Button label="Zrušiť" text size="small" @click="reopening = false" />
        <Button label="Otvoriť" icon="pi pi-lock-open" severity="danger" size="small" @click="reopen" />
      </template>
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
.period {
  display: flex;
  align-items: center;
  gap: 2px;
}
.period strong {
  min-width: 8.5rem;
  text-align: center;
  font-size: var(--fs-md);
}
.spacer {
  flex: 1;
}
.empty,
.foot {
  color: var(--ink-soft);
  margin: 0;
  font-size: var(--fs-sm);
}
.err-box {
  color: var(--tone-overdue);
  margin: 0;
}
.sums {
  display: flex;
  flex-wrap: wrap;
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}
.sums > div {
  display: flex;
  flex-direction: column;
  padding: 8px 18px 9px 14px;
  border-right: 1px solid var(--rule);
  flex: 1 1 9rem;
}
.sums span {
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.sums strong {
  font-size: var(--fs-lg);
}
.sums small {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  font-variant-numeric: tabular-nums;
}
.ledger {
  border-bottom: 3px double currentColor;
}
.wrap {
  overflow-x: auto;
}
.pl {
  width: 100%;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  font-size: var(--fs-sm);
}
.pl th {
  text-align: left;
  font-weight: 600;
  color: var(--ink-soft);
  padding: 6px 10px;
  border-bottom: 2px solid var(--ink);
  white-space: nowrap;
}
.pl td {
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
  white-space: nowrap;
}
.pl .num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.pl .strong {
  font-weight: 600;
}
.pl tbody tr {
  cursor: pointer;
}
.pl tbody tr:hover,
.pl tbody tr:focus-visible {
  background: var(--paper);
  outline: none;
}
.pl small {
  display: block;
  color: var(--ink-soft);
}
.pl .name {
  position: relative;
  padding-right: 24px;
}
.note {
  position: absolute;
  right: 6px;
  top: 8px;
  color: var(--tone-draft);
}
.sub {
  margin: 0 0 10px;
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.in-form {
  grid-template-columns: 1fr 1fr;
  margin-bottom: 12px;
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  justify-content: flex-end;
}
.notes {
  list-style: none;
  padding: 8px 10px;
  margin: 0 0 12px;
  background: var(--tone-draft-bg);
  font-size: var(--fs-sm);
}
.notes .pi {
  color: var(--tone-draft);
}
.slip {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.slip td {
  padding: 3px 0;
}
.slip .num {
  text-align: right;
}
.slip .in {
  padding-left: 12px;
  color: var(--ink-soft);
}
.slip .hd td {
  padding-top: 10px;
  font-size: var(--fs-xs);
  color: var(--ink-soft);
  font-weight: 600;
}
.slip .sum td {
  border-top: 1px solid var(--rule);
  font-weight: 600;
}
.slip .total td {
  border-top: 2px solid var(--ink);
  font-weight: 700;
  font-size: var(--fs-md);
  padding: 6px 0;
}
.dlg {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  margin: 0 0 10px;
}
.dlg-l {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: var(--fs-sm);
}
.err {
  color: var(--tone-overdue);
}
@media (max-width: 640px) {
  .sums > div {
    flex: 1 1 45%;
    min-width: 0;
  }
  .pl th:nth-child(n + 3):not(:nth-child(7)),
  .pl td:nth-child(n + 3):not(:nth-child(7)) {
    display: none;
  }
}
</style>

<style>
.pl-panel.p-drawer {
  width: 30rem;
}
@media (max-width: 600px) {
  .pl-panel.p-drawer {
    width: 100vw !important;
  }
}
</style>
