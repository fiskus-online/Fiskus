<script setup>
// Prijaté objednávky od zákazníkov: stav, termín dodania, odberateľ, suma.
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
import { salesOrderColumns } from '../lib/columns.js'
import { isActive, isLate, matchesSalesOrderView, salesOrderStatusOf, salesOrderViews } from '../lib/sorders.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()
const today = toISODate(new Date())
const view = ref('active')
const search = ref('')
const searchEl = ref(null)
const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.salesOrders(session.companyId)
    loadError.value = null
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
onMounted(load)

const lateOnly = ref(false)
const visibleRows = computed(() => {
  const terms = searchTerms(search.value)
  return rows.value.filter((r) =>
    matchesSalesOrderView(r, view.value) && (!lateOnly.value || isLate(r, today)) &&
    matches(terms, [r.number, r.customer?.name, r.customer?.ico, r.ref, r.place, r.note, salesOrderStatusOf(r, today).label, r.summary?.total,
      ...(r.fulfils ?? []).map((f) => f.number), ...(r.lines ?? []).map((l) => l.description)]))
})

const ledger = computed(() => {
  const open = rows.value.filter(isActive)
  return {
    openCount: open.length,
    openTotal: open.reduce((a, r) => a + Number(r.summary?.total || 0), 0),
    late: open.filter((r) => isLate(r, today)).length,
    customers: new Set(open.map((r) => r.contactId || r.customer?.name)).size,
  }
})
function showLate() {
  view.value = 'active'
  lateOnly.value = !lateOnly.value
}

const totalsRow = computed(() => [{ number: `Spolu ${visibleRows.value.length}`, customer: {}, summary: { total: visibleRows.value.reduce((a, r) => a + Number(r.summary?.total || 0), 0).toFixed(2) }, _totals: true }])

const columnDefs = [
  { field: 'number', headerName: 'Číslo', width: 130, cellClass: (p) => (p.data._totals ? '' : 'cell-number') },
  { colId: 'customer', headerName: 'Odberateľ', flex: 1, minWidth: 120,
    // na mobile s číslom; súčtový riadok vracia počet, aby sa bunka prekreslila
    valueGetter: (p) => {
      if (p.data._totals) return narrow.matches ? p.data.number : ''
      return narrow.matches ? `${p.data.number} · ${p.data.customer?.name ?? ''}` : p.data.customer?.name
    } },
  { field: 'date', headerName: 'Prijatá', valueFormatter: (p) => formatDate(p.value), width: 100 },
  { field: 'due', headerName: 'Dodať do', valueFormatter: (p) => formatDate(p.value), width: 100,
    cellClassRules: { 'cell-late': (p) => !p.data._totals && isLate(p.data, today) } },
  { field: 'ref', headerName: 'Číslo u zákazníka', width: 150 },
  {
    colId: 'status',
    headerName: 'Stav',
    width: 190,
    valueGetter: (p) => (p.data._totals ? '' : salesOrderStatusOf(p.data, today).label),
    cellRenderer: (p) => {
      if (p.data._totals) return ''
      const s = salesOrderStatusOf(p.data, today)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
  { colId: 'total', headerName: 'Spolu', width: 120, type: 'rightAligned', cellClass: 'ag-right-aligned-cell cell-total',
    valueGetter: (p) => Number(p.data.summary?.total || 0), valueFormatter: (p) => formatAmount(p.value) },
]
const defaultColDef = { sortable: true, resizable: true }
const rowClassRules = { 'row-cancelled': (p) => p.data.status === 'cancelled' }

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['number', 'date', 'ref', 'due'], !narrow.matches)
  gridApi.value?.setColumnWidths([{ key: 'status', newWidth: narrow.matches ? 140 : 190 }, { key: 'total', newWidth: narrow.matches ? 90 : 120 }])
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
  router.push(`/prijate-objednavky/${e.data.id}`)
}
function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (typing || e.ctrlKey || e.metaKey || e.altKey) return
  if (e.key === 'n' && can('invoices.write')) {
    e.preventDefault()
    router.push('/prijate-objednavky/nova')
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
      <h1>Prijaté objednávky</h1>
      <div class="spacer" />
      <Button v-if="can('invoices.write')" size="small" @click="router.push('/prijate-objednavky/nova')">
        <template #default>
          <i class="pi pi-plus" /><span>Nová objednávka</span><kbd class="hint">N</kbd>
        </template>
      </Button>
    </header>

    <section class="ledger" aria-label="Prehľad objednávok">
      <button class="figure figure-link" :disabled="!ledger.openCount" @click="view = 'active'; lateOnly = false">
        <span class="label">Na dodanie ({{ ledger.openCount }})</span>
        <span class="value">{{ formatMoney(ledger.openTotal) }}</span>
      </button>
      <button class="figure figure-link" :class="{ on: lateOnly }" :disabled="!ledger.late" @click="showLate">
        <span class="label">Po termíne</span>
        <span class="value" :class="{ late: ledger.late }">{{ ledger.late }}</span>
      </button>
      <div class="figure">
        <span class="label">Odberateľov</span>
        <span class="value">{{ ledger.customers }}</span>
      </div>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="salesOrderViews" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť"
        @update:model-value="lateOnly = false" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText ref="searchEl" v-model="search" placeholder="Hľadať číslo, odberateľa, položku" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton name="Prijaté objednávky" :columns="salesOrderColumns" :rows="() => gridRows(gridApi, visibleRows)" />
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
        :overlay-no-rows-template="'<span class=&quot;empty&quot;>Žiadne objednávky. Novú zapíšete klávesom N alebo z cenovej ponuky.</span>'"
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
.figure-link.on {
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
.grid :deep(.cell-late) {
  color: var(--tone-overdue);
  font-weight: 600;
}
.grid :deep(.cell-total) {
  font-weight: 600;
}
.grid :deep(.ag-cell-wrapper),
.grid :deep(.ag-cell) {
  line-height: 1.25;
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
}
</style>
