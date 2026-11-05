<script setup>
// Domácnosť — pohyby: zoznam s filtrom, hľadaním a exportom, zápis pohybu,
// import bankového výpisu a prenos výberov zo živnosti.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { householdEntryColumns } from '../lib/columns.js'
import {
  accountName, entryCategory, entryType, entryTypes, filterEntries, listTotals, monthNames, newEntry,
} from '../lib/household.js'
import ExportButton from '../components/ExportButton.vue'
import HouseholdEntryDialog from '../components/HouseholdEntryDialog.vue'

const route = useRoute()
const toast = useToast()
const canWrite = computed(() => can('ledger.write'))
const now = new Date()
const year = ref(Number(route.query.rok) || now.getFullYear())
const years = [0, 1, 2, 3].map((d) => ({ label: String(now.getFullYear() - d), value: now.getFullYear() - d }))
const filter = ref({ accountId: Number(route.query.ucet) || null, type: null, category: route.query.kategoria || null, month: null, search: '' })

const rows = shallowRef([])
const accounts = ref([])
const categories = ref([])
const loading = ref(true)
const loadError = ref('')

async function load() {
  loading.value = true
  try {
    const [es, accs, cats] = await Promise.all([
      api.hhEntries(session.companyId, year.value),
      api.hhAccounts(session.companyId),
      api.hhCategories(session.companyId),
    ])
    rows.value = es
    accounts.value = accs
    categories.value = cats
    loadError.value = ''
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
watch(year, load)
onMounted(load)

const visible = computed(() => filterEntries(rows.value, filter.value, accounts.value))
const totals = computed(() => listTotals(visible.value))
const payees = computed(() => [...new Set(rows.value.map((e) => e.payee).filter(Boolean))])
const categoryOptions = computed(() => categories.value.map((c) => ({ label: c.name, value: c.name })))
const monthOptions = monthNames.map((m, i) => ({ label: m, value: i + 1 }))
const filtered = computed(() => !!(filter.value.accountId || filter.value.type || filter.value.category || filter.value.month || filter.value.search))
function resetFilter() {
  filter.value = { accountId: null, type: null, category: null, month: null, search: '' }
}

const columnDefs = computed(() => [
  { field: 'date', headerName: 'Dátum', valueFormatter: (p) => formatDate(p.value), width: 100, minWidth: 96, sort: 'desc' },
  { colId: 'account', headerName: 'Účet', valueGetter: (p) => accountName(accounts.value, p.data.accountId), width: 130 },
  { colId: 'category', headerName: 'Kategória', valueGetter: (p) => entryCategory(p.data, accounts.value), width: 170 },
  { field: 'payee', headerName: 'Obchod / komu', flex: 1, minWidth: 140 },
  { field: 'note', headerName: 'Poznámka', flex: 1, minWidth: 120 },
  {
    colId: 'amount',
    headerName: 'Suma €',
    valueGetter: (p) => Number(p.data.amount),
    valueFormatter: (p) => formatAmount(p.data.amount),
    cellClass: (p) => ['cell-number', 'ag-right-aligned-cell', 'amt-' + entryType(p.data)],
    type: 'rightAligned',
    width: 110,
  },
  { colId: 'source', headerName: '', valueGetter: (p) => (p.data.source?.startsWith('bank:') ? 'výpis' : p.data.source?.startsWith('biz:') ? 'živnosť' : ''), width: 74, cellClass: 'cell-src' },
])
const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['account', 'category', 'note', 'source'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)
onBeforeUnmount(() => narrow.removeEventListener('change', applyResponsive))

// ---- pohyb ----
const editing = ref(null)
function add(type) {
  const e = newEntry(type, accounts.value)
  if (filter.value.accountId) e.accountId = filter.value.accountId
  editing.value = e
}

// ---- výpis ----
const importOpen = ref(false)
const importAccount = ref(null)
const importBusy = ref(false)
const fileInput = ref(null)
const bankAccounts = computed(() => accounts.value.filter((a) => !a.closed && a.kind !== 'cash'))
function openImport() {
  importAccount.value = filter.value.accountId || bankAccounts.value[0]?.id || null
  importOpen.value = true
}
async function importFile(ev) {
  const file = ev.target.files?.[0]
  ev.target.value = ''
  if (!file || !importAccount.value) return
  importBusy.value = true
  try {
    const r = await api.hhImportBank(session.companyId, importAccount.value, file)
    importOpen.value = false
    const parts = [`${r.added} nových pohybov`]
    if (r.skipped) parts.push(`${r.skipped} už bolo`)
    if (r.foreign) parts.push(`${r.foreign} v cudzej mene vynechaných`)
    toast.add({ severity: 'success', summary: 'Výpis načítaný', detail: parts.join(', ') + '. Kategórie skontrolujte.', life: 6000 })
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Výpis sa nenačítal', detail: e.fields?.[0]?.message ?? e.message, life: 8000 })
  } finally {
    importBusy.value = false
  }
}

// ---- zo živnosti ----
const drawsOpen = ref(false)
const draws = ref([])
const drawsBusy = ref(false)
const drawAccount = ref(null)
const picked = ref([])
async function openDraws() {
  drawsOpen.value = true
  drawsBusy.value = true
  drawAccount.value = filter.value.accountId || accounts.value.find((a) => !a.closed)?.id || null
  try {
    draws.value = await api.hhDraws(session.companyId, year.value)
    picked.value = draws.value.filter((d) => !d.imported).map((d) => d.companyId + ':' + d.key)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Výbery sa nenačítali', detail: e.message, life: 6000 })
    drawsOpen.value = false
  } finally {
    drawsBusy.value = false
  }
}
async function importDraws() {
  drawsBusy.value = true
  try {
    const items = draws.value.filter((d) => picked.value.includes(d.companyId + ':' + d.key)).map((d) => ({ companyId: d.companyId, key: d.key }))
    const r = await api.hhImportDraws(session.companyId, drawAccount.value, year.value, items)
    drawsOpen.value = false
    toast.add({ severity: 'success', summary: 'Prenesené zo živnosti', detail: `${r.added} pohybov`, life: 4000 })
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Prenos zlyhal', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
  } finally {
    drawsBusy.value = false
  }
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Pohyby</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <template v-if="canWrite && accounts.length">
        <Button label="Výdavok" icon="pi pi-minus" size="small" @click="add('expense')" />
        <Button label="Príjem" icon="pi pi-plus" size="small" severity="secondary" outlined @click="add('income')" />
        <Button label="Prevod" icon="pi pi-arrow-right-arrow-left" size="small" severity="secondary" text @click="add('transfer')" />
        <Button label="Nahrať výpis" icon="pi pi-upload" size="small" severity="secondary" text @click="openImport" />
        <Button label="Zo živnosti" icon="pi pi-briefcase" size="small" severity="secondary" text @click="openDraws" />
      </template>
    </header>

    <div class="toolbar">
      <Select v-model="filter.accountId" :options="accounts" option-label="name" option-value="id" size="small" show-clear placeholder="Všetky účty" aria-label="Účet" />
      <Select v-model="filter.type" :options="entryTypes" option-label="label" option-value="value" size="small" show-clear placeholder="Všetky druhy" aria-label="Druh" />
      <Select v-model="filter.category" :options="categoryOptions" option-label="label" option-value="value" size="small" filter show-clear placeholder="Všetky kategórie" aria-label="Kategória" />
      <Select v-model="filter.month" :options="monthOptions" option-label="label" option-value="value" size="small" show-clear placeholder="Celý rok" aria-label="Mesiac" />
      <Button v-if="filtered" label="Zrušiť filter" icon="pi pi-filter-slash" text size="small" @click="resetFilter" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="filter.search" placeholder="Hľadať obchod, kategóriu, sumu" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`Domácnosť ${year}`" :columns="householdEntryColumns(accounts)" :rows="() => gridRows(gridApi, visible)" />
    </div>
    <p v-if="loadError" class="error">{{ loadError }}</p>
    <p v-else-if="!loading && !accounts.length" class="hint">
      Najprv pridajte účty domácnosti v časti <router-link to="/domacnost/ucty">Účty</router-link>.
    </p>

    <div class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visible"
        :column-defs="columnDefs"
        :default-col-def="{ sortable: true, resizable: true }"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => String(p.data.id)"
        :overlay-no-rows-template="'<span>Žiadne pohyby. Zapíšte výdavok alebo nahrajte výpis z banky.</span>'"
        @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
        @row-clicked="(e) => (editing = e.data)"
      />
    </div>
    <footer class="sums" aria-label="Súčty zoznamu">
      <span>{{ totals.count }} pohybov</span>
      <span>Príjmy <strong class="num inc">{{ formatMoney(totals.income) }}</strong></span>
      <span>Výdavky <strong class="num out">{{ formatMoney(totals.expense) }}</strong></span>
      <small>bez prevodov medzi účtami</small>
    </footer>

    <HouseholdEntryDialog :entry="editing" :accounts="accounts" :categories="categories" :payees="payees" :can-write="canWrite"
      @close="editing = null" @saved="load" />

    <Dialog v-model:visible="importOpen" header="Nahrať výpis z banky" modal :style="{ width: '30rem' }" :breakpoints="{ '560px': '94vw' }">
      <p class="dlg-p">
        Výpis v tvare XML (camt.053), GPC/ABO alebo CSV/XLSX z internetbankingu. Kategórie Fiškus navrhne podľa obchodu — podľa toho,
        ako ste ich zaradili predtým. Opakované nahratie toho istého výpisu nič nezdvojí.
      </p>
      <label class="dlg-l">Účet
        <Select v-model="importAccount" :options="bankAccounts" option-label="name" option-value="id" size="small" fluid placeholder="Vyberte účet" />
      </label>
      <input ref="fileInput" type="file" accept=".xml,.gpc,.abo,.csv,.txt,.xlsx" class="visually-hidden" @change="importFile" />
      <template #footer>
        <Button label="Zrušiť" text size="small" @click="importOpen = false" />
        <Button label="Vybrať súbor" icon="pi pi-upload" size="small" :disabled="!importAccount" :loading="importBusy" @click="fileInput.click()" />
      </template>
    </Dialog>

    <Dialog v-model:visible="drawsOpen" header="Peniaze zo živnosti" modal :style="{ width: '40rem' }" :breakpoints="{ '640px': '96vw' }">
      <p class="dlg-p">
        Výbery a vklady podnikateľa z peňažného denníka vašej živnosti za rok {{ year }} (stĺpce „mimo základu dane“). Výber sa
        v domácnosti zapíše ako príjem, vklad do živnosti ako výdavok.
      </p>
      <p v-if="!drawsBusy && !draws.length" class="dlg-p muted">
        V peňažnom denníku živnosti za {{ year }} nie sú žiadne výbery ani vklady. Výber z účtu alebo pokladne zaraďte v denníku do stĺpca
        „Iné výdavky mimo základu dane“.
      </p>
      <table v-else class="draws">
        <tbody>
          <tr v-for="d in draws" :key="d.companyId + ':' + d.key" :class="{ done: d.imported }">
            <td><Checkbox v-model="picked" :value="d.companyId + ':' + d.key" :disabled="d.imported" :aria-label="d.text || d.key" /></td>
            <td class="d">{{ formatDate(d.date) }}</td>
            <td class="t"><span>{{ d.text || (Number(d.amount) > 0 ? 'Výber' : 'Vklad') }}</span><small>{{ d.companyName }}<template v-if="d.imported"> · už prenesené</template></small></td>
            <td class="num" :class="Number(d.amount) > 0 ? 'inc' : 'out'">{{ formatAmount(d.amount) }}</td>
          </tr>
        </tbody>
      </table>
      <label v-if="draws.length" class="dlg-l">Na účet domácnosti
        <Select v-model="drawAccount" :options="accounts.filter((a) => !a.closed)" option-label="name" option-value="id" size="small" fluid />
      </label>
      <template #footer>
        <Button label="Zrušiť" text size="small" @click="drawsOpen = false" />
        <Button label="Preniesť" icon="pi pi-check" size="small" :disabled="!picked.length || !drawAccount" :loading="drawsBusy" @click="importDraws" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 14px 16px 12px;
  gap: 10px;
  overflow: auto;
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.search {
  margin-left: auto;
  width: 260px;
}
.search :deep(input) {
  width: 100%;
}
.error {
  color: var(--tone-overdue);
}
.hint {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.grid-wrap {
  flex: 1;
  min-height: 320px;
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.grid {
  height: 100%;
}
:deep(.amt-expense) {
  color: var(--tone-overdue);
}
:deep(.amt-income) {
  color: var(--tone-paid);
}
:deep(.amt-transfer) {
  color: var(--ink-faint);
}
:deep(.cell-src) {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.sums {
  display: flex;
  gap: 18px;
  align-items: baseline;
  flex-wrap: wrap;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.sums strong {
  color: var(--ink);
}
.sums .inc {
  color: var(--tone-paid);
}
.sums .out {
  color: var(--tone-overdue);
}
.sums small {
  color: var(--ink-faint);
}
.dlg-p {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  margin-bottom: 10px;
}
.muted {
  color: var(--ink-faint);
}
.dlg-l {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.draws {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  margin-bottom: 10px;
}
.draws td {
  padding: 5px 4px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: middle;
}
.draws .d {
  white-space: nowrap;
  color: var(--ink-soft);
}
.draws .t {
  display: flex;
  flex-direction: column;
}
.draws small {
  color: var(--ink-faint);
}
.draws .inc {
  color: var(--tone-paid);
  font-weight: 600;
}
.draws .out {
  color: var(--tone-overdue);
  font-weight: 600;
}
.draws tr.done {
  opacity: 0.55;
}
@media (max-width: 760px) {
  .search {
    width: 100%;
    margin-left: 0;
  }
  .toolbar :deep(.p-select) {
    flex: 1 1 140px;
  }
}
</style>
