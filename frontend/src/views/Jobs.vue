<script setup>
// Zákazky (jednoduché CRM): stav, termín a ekonomika z prepojených dokladov.
// Súčty v hlavičke sú súčtom už vypočítaných súm zo servera.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import SelectButton from 'primevue/selectbutton'
import { api } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { crmJobColumns } from '../lib/columns.js'
import { jobStatusOf, jobViews, marginText, matchesJobView } from '../lib/jobs.js'
import ExportButton from '../components/ExportButton.vue'
import JobBoard from '../components/JobBoard.vue'
import JobStats from '../components/JobStats.vue'

const router = useRouter()
const view = ref('open')
// zobrazenie: zoznam, nástenka podľa stavu alebo ziskovosť podľa zákazníka (pamätá sa v prehliadači)
const modes = [
  { value: 'list', label: 'Zoznam', icon: 'pi pi-list' },
  { value: 'board', label: 'Nástenka', icon: 'pi pi-th-large' },
  { value: 'stats', label: 'Podľa zákazníka', icon: 'pi pi-chart-bar' },
]
const savedMode = (() => {
  try {
    return localStorage.getItem('fiskus.jobsMode') || 'list'
  } catch {
    return 'list'
  }
})()
const mode = ref(modes.some((m) => m.value === savedMode) ? savedMode : 'list')
watch(mode, (m) => {
  try {
    localStorage.setItem('fiskus.jobsMode', m)
  } catch {
    /* prehliadač bez úložiska */
  }
})
const search = ref('')
const searchEl = ref(null)
const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.jobs(session.companyId)
    loadError.value = null
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
onMounted(load)

const boardRows = computed(() => {
  const terms = searchTerms(search.value)
  return rows.value.filter((r) => matches(terms, [r.number, r.name, r.customer, r.note, jobStatusOf(r).label, r.budget]))
})
const visibleRows = computed(() => {
  const terms = searchTerms(search.value)
  return rows.value.filter((r) => matchesJobView(r, view.value) && matches(terms, [r.number, r.name, r.customer, r.note, jobStatusOf(r).label, r.budget]))
})

const add = (list, k) => list.reduce((a, r) => a + Number(r.summary?.[k] ?? 0), 0)
const ledger = computed(() => {
  const open = rows.value.filter((r) => matchesJobView(r, 'open'))
  return {
    openCount: open.length,
    lateCount: rows.value.filter((r) => r.late).length,
    pipeline: open.reduce((a, r) => a + Number(r.budget || 0), 0),
    unpaid: add(rows.value, 'unpaid'),
    profit: add(rows.value, 'profit'),
    net: rows.value[0]?.summary?.net ?? false,
  }
})

const totalsRow = computed(() => {
  const list = visibleRows.value
  const sum = (k) => add(list, k).toFixed(2)
  return [{ number: `Spolu ${list.length}`, name: '', customer: '', budget: list.reduce((a, r) => a + Number(r.budget || 0), 0).toFixed(2),
    summary: { offered: sum('offered'), invoiced: sum('invoiced'), paid: sum('paid'), unpaid: sum('unpaid'), costs: sum('costs'), profit: sum('profit') }, _totals: true }]
})

const amountCol = (key, header, extra = {}) => ({
  colId: key,
  headerName: header,
  valueGetter: (p) => Number(key === 'budget' ? p.data.budget : p.data.summary?.[key] ?? 0),
  valueFormatter: (p) => (p.value ? formatAmount(p.value) : ''),
  type: 'rightAligned',
  width: 116,
  ...extra,
})

const columnDefs = [
  { field: 'number', headerName: 'Číslo', width: 110, cellClass: (p) => (p.data._totals ? '' : 'cell-number') },
  {
    colId: 'name',
    headerName: 'Zákazka',
    // súčtový riadok vracia počet, aby sa bunka pri zmene prekreslila
    valueGetter: (p) => (p.data._totals ? p.data.number : p.data.name),
    cellRenderer: (p) => {
      if (p.data._totals) return narrow.matches ? p.data.number : ''
      const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c])
      const no = narrow.matches && p.data.number ? `<span class="job-no">${esc(p.data.number)}</span> ` : ''
      return `<span class="job-name">${no}${esc(p.data.name)}</span>${p.data.customer ? `<span class="job-cust">${esc(p.data.customer)}</span>` : ''}`
    },
    flex: 1,
    minWidth: 160,
  },
  {
    colId: 'status',
    headerName: 'Stav',
    width: 190,
    valueGetter: (p) => (p.data._totals ? '' : jobStatusOf(p.data).label),
    cellRenderer: (p) => {
      if (p.data._totals) return ''
      const s = jobStatusOf(p.data)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
  { field: 'due', headerName: 'Termín', valueFormatter: (p) => formatDate(p.value), width: 100 },
  amountCol('budget', 'Plán'),
  amountCol('invoiced', 'Vyfakturované'),
  amountCol('unpaid', 'Na úhradu'),
  amountCol('costs', 'Náklady'),
  amountCol('profit', 'Zisk', { cellClass: 'ag-right-aligned-cell cell-total' }),
  { colId: 'margin', headerName: 'Marža', width: 84, type: 'rightAligned', valueGetter: (p) => (p.data._totals ? '' : marginText(p.data.summary)) },
]
const defaultColDef = { sortable: true, resizable: true }
const rowClassRules = { 'row-cancelled': (p) => p.data.status === 'lost', 'row-overdue': (p) => !!p.data.late }

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['number', 'due', 'budget', 'unpaid', 'costs', 'margin', 'invoiced'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
  gridApi.value?.refreshCells({ force: true })
}
function onGridReady(e) {
  gridApi.value = e.api
  applyResponsive()
}
narrow.addEventListener('change', applyResponsive)

function open(e) {
  if (e.data?._totals || !e.data?.id) return
  router.push(`/zakazky/${e.data.id}`)
}
function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (typing || e.ctrlKey || e.metaKey || e.altKey) return
  if (e.key === 'n' && can('invoices.write')) {
    e.preventDefault()
    router.push('/zakazky/nova')
  } else if (e.key === '/') {
    e.preventDefault()
    searchEl.value?.$el?.focus()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  narrow.removeEventListener('change', applyResponsive)
})
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Zákazky</h1>
      <SelectButton v-model="mode" :options="modes" option-label="label" option-value="value" :allow-empty="false" size="small" class="modes" aria-label="Zobrazenie">
        <template #option="{ option }"><i :class="option.icon" :title="option.label" /><span class="ml">{{ option.label }}</span></template>
      </SelectButton>
      <div class="spacer" />
      <Button v-if="can('invoices.write')" size="small" @click="router.push('/zakazky/nova')">
        <template #default>
          <i class="pi pi-plus" /><span>Nová zákazka</span><kbd class="hint">N</kbd>
        </template>
      </Button>
    </header>

    <JobStats v-if="mode === 'stats'" />
    <template v-else>
    <section class="ledger" aria-label="Prehľad zákaziek">
      <button class="figure figure-link" :disabled="!ledger.openCount" @click="view = 'open'">
        <span class="label">Bežiace ({{ ledger.openCount }}), plánovaná cena</span>
        <span class="value">{{ formatMoney(ledger.pipeline) }}</span>
      </button>
      <button class="figure figure-link" :disabled="!ledger.lateCount" @click="view = 'late'">
        <span class="label">Po termíne</span>
        <span class="value" :class="{ late: ledger.lateCount }">{{ ledger.lateCount }}</span>
      </button>
      <div class="figure">
        <span class="label">Na úhradu od zákazníkov</span>
        <span class="value">{{ formatMoney(ledger.unpaid) }}</span>
      </div>
      <div class="figure">
        <span class="label">Zisk zo zákaziek{{ ledger.net ? ' (bez DPH)' : '' }}</span>
        <span class="value">{{ formatMoney(ledger.profit) }}</span>
      </div>
    </section>

    <div class="toolbar">
      <SelectButton v-if="mode === 'list'" v-model="view" :options="jobViews" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText ref="searchEl" v-model="search" placeholder="Hľadať číslo, názov, zákazníka" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton name="Zákazky" :columns="crmJobColumns" :rows="() => (mode === 'board' ? boardRows : gridRows(gridApi, visibleRows))" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <JobBoard v-if="mode === 'board'" :rows="boardRows" :can-write="can('invoices.write')" @changed="load" />
    <div v-else class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visibleRows"
        :pinned-bottom-row-data="totalsRow"
        :column-defs="columnDefs"
        :default-col-def="defaultColDef"
        :row-class-rules="rowClassRules"
        :locale-text="skGridLocale"
        :loading="loading"
        :row-height="44"
        :get-row-id="(p) => (p.data._totals ? 'totals' : String(p.data.id))"
        :overlay-no-rows-template="'<span class=&quot;empty&quot;>Žiadne zákazky. Novú založíte klávesom N.</span>'"
        @grid-ready="onGridReady"
        @row-clicked="open"
      />
    </div>
    </template>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 14px 16px 12px;
  gap: 10px;
}
.head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
}
.modes :deep(.p-togglebutton i) {
  margin-right: 5px;
}
.hint {
  margin-left: 6px;
  background: rgba(255, 255, 255, 0.16);
  border-color: rgba(255, 255, 255, 0.3);
  color: #fff;
}
.head :deep(.p-button) {
  gap: 6px;
}
.ledger {
  display: flex;
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}
.figure {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 8px 20px 9px 14px;
  border: 0;
  border-right: 1px solid var(--rule);
  background: none;
  font: inherit;
  color: inherit;
  text-align: left;
}
.figure .label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.figure .value {
  font-size: var(--fs-xl);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.figure .value.late {
  color: var(--tone-overdue);
}
.figure-link {
  cursor: pointer;
}
.figure-link:hover:not(:disabled) {
  background: var(--blue-wash);
}
.figure-link:disabled {
  cursor: default;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.search {
  margin-left: auto;
  width: 280px;
}
.search :deep(input) {
  width: 100%;
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
.grid :deep(.ag-row) {
  cursor: pointer;
}
.grid :deep(.cell-number) {
  font-weight: 600;
  color: var(--blue-deep);
}
.grid :deep(.cell-total) {
  font-weight: 600;
}
.grid :deep(.ag-cell-wrapper),
.grid :deep(.ag-cell) {
  line-height: 1.25;
}
.grid :deep(.job-name) {
  display: block;
  padding-top: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.grid :deep(.job-no) {
  font-weight: 600;
  color: var(--blue);
}
.grid :deep(.job-cust) {
  display: block;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  overflow: hidden;
  text-overflow: ellipsis;
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 8px;
  }
  .head h1 {
    font-size: var(--fs-lg);
  }
  .hint {
    display: none;
  }
  .ml {
    display: none;
  }
  .modes :deep(.p-togglebutton i) {
    margin-right: 0;
  }
  .ledger {
    overflow-x: auto;
  }
  .figure {
    padding: 6px 12px 7px 10px;
    flex: none;
  }
  .figure .value {
    font-size: var(--fs-lg);
  }
  .toolbar :deep(.p-selectbutton) {
    overflow-x: auto;
    max-width: 100%;
    flex-wrap: nowrap;
  }
  .toolbar :deep(.p-togglebutton) {
    flex: none;
    white-space: nowrap;
  }
  .search {
    flex: 1;
    width: auto;
    min-width: 0;
    margin-left: 0;
  }
}
</style>
