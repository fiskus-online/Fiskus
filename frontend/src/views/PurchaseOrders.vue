<script setup>
// Objednávky dodávateľom: stav, očakávané dodanie a suma bez DPH.
// Súčty v hlavičke sú súčtom už vypočítaných súm zo servera.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import SelectButton from 'primevue/selectbutton'
import { api } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, toISODate } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { porderColumns } from '../lib/columns.js'
import { matchesPorderView, porderLate, porderStatusOf, porderViews } from '../lib/porders.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()
const today = toISODate(new Date())
const view = ref('open')
const search = ref('')
const searchEl = ref(null)
const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.porders(session.companyId)
    loadError.value = null
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
onMounted(load)

const visibleRows = computed(() => {
  const terms = searchTerms(search.value)
  return rows.value.filter((r) =>
    matchesPorderView(r, view.value) &&
    matches(terms, [r.number, r.supplier, r.note, porderStatusOf(r).label, r.total, ...(r.lines ?? []).flatMap((l) => [l.name, l.supplierCode])]))
})

const ledger = computed(() => {
  const open = rows.value.filter((r) => matchesPorderView(r, 'open'))
  return {
    openCount: open.length,
    openTotal: open.reduce((a, r) => a + Number(r.total || 0), 0),
    lateCount: open.filter((r) => porderLate(r, today)).length,
    drafts: rows.value.filter((r) => r.status === 'draft').length,
  }
})

const totalsRow = computed(() => [{ number: `Spolu ${visibleRows.value.length}`, supplier: '', total: visibleRows.value.reduce((a, r) => a + Number(r.total || 0), 0).toFixed(2), _totals: true }])

const columnDefs = [
  { field: 'number', headerName: 'Číslo', width: 120, cellClass: (p) => (p.data._totals ? '' : 'cell-number') },
  { field: 'supplier', headerName: 'Dodávateľ', flex: 1, minWidth: 160,
    // súčtový riadok vracia počet, aby sa bunka na mobile prekreslila
    valueGetter: (p) => (p.data._totals ? (narrow.matches ? p.data.number : '') : p.data.supplier) },
  { field: 'date', headerName: 'Dátum', valueFormatter: (p) => formatDate(p.value), width: 100 },
  { field: 'expected', headerName: 'Dodanie', valueFormatter: (p) => formatDate(p.value), width: 100 },
  {
    colId: 'status',
    headerName: 'Stav',
    width: 190,
    valueGetter: (p) => (p.data._totals ? '' : porderStatusOf(p.data, today).label),
    cellRenderer: (p) => {
      if (p.data._totals) return ''
      const s = porderStatusOf(p.data, today)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
  { colId: 'lines', headerName: 'Položiek', width: 90, type: 'rightAligned', valueGetter: (p) => (p.data._totals ? '' : p.data.lines?.length ?? 0) },
  { colId: 'total', headerName: 'Suma bez DPH', width: 130, type: 'rightAligned', cellClass: 'ag-right-aligned-cell cell-total',
    valueGetter: (p) => Number(p.data.total || 0), valueFormatter: (p) => formatAmount(p.value) },
]
const defaultColDef = { sortable: true, resizable: true }
const rowClassRules = { 'row-cancelled': (p) => p.data.status === 'cancelled', 'row-overdue': (p) => !p.data._totals && porderLate(p.data, today) }

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['number', 'date', 'expected', 'lines'], !narrow.matches)
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
  router.push(`/objednavky/${e.data.id}`)
}
function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (typing || e.ctrlKey || e.metaKey || e.altKey) return
  if (e.key === 'n' && can('purchases.write')) {
    e.preventDefault()
    router.push('/objednavky/nova')
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
      <h1>Objednávky</h1>
      <div class="spacer" />
      <Button v-if="can('purchases.read')" class="reorder-btn" label="Na objednanie" aria-label="Na objednanie" icon="pi pi-box" size="small" severity="secondary" outlined @click="router.push('/sklad?tab=reorder')" />
      <Button v-if="can('purchases.write')" size="small" @click="router.push('/objednavky/nova')">
        <template #default>
          <i class="pi pi-plus" /><span>Nová objednávka</span><kbd class="hint">N</kbd>
        </template>
      </Button>
    </header>

    <section class="ledger" aria-label="Prehľad objednávok">
      <button class="figure figure-link" :disabled="!ledger.openCount" @click="view = 'open'">
        <span class="label">Otvorené ({{ ledger.openCount }}), bez DPH</span>
        <span class="value">{{ formatMoney(ledger.openTotal) }}</span>
      </button>
      <div class="figure">
        <span class="label">Mešká dodanie</span>
        <span class="value" :class="{ late: ledger.lateCount }">{{ ledger.lateCount }}</span>
      </div>
      <div class="figure">
        <span class="label">Rozpracované (neodoslané)</span>
        <span class="value">{{ ledger.drafts }}</span>
      </div>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="porderViews" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText ref="searchEl" v-model="search" placeholder="Hľadať číslo, dodávateľa, položku" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton name="Objednávky" :columns="porderColumns" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <div class="grid-wrap">
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
        :get-row-id="(p) => (p.data._totals ? 'totals' : String(p.data.id))"
        :overlay-no-rows-template="'<span class=&quot;empty&quot;>Žiadne objednávky. Novú založíte klávesom N alebo zo skladu cez Na objednanie.</span>'"
        @grid-ready="onGridReady"
        @row-clicked="open"
      />
    </div>
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
  .toolbar > :first-child {
    flex-basis: 100%;
  }
  .reorder-btn :deep(.p-button-label) {
    display: none;
  }
}
</style>
