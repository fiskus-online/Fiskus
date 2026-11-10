<script setup>
// Dodacie listy: stav, odberateľ, suma; vybrané vydané dodacie listy
// jedného odberateľa sa dajú dať na zbernú faktúru. Súčty v hlavičke sú
// súčtom už vypočítaných súm zo servera.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
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
import { deliveryColumns } from '../lib/columns.js'
import { deliveryStatusOf, deliveryViews, invoiceBlocker, matchesDeliveryView } from '../lib/deliveries.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()
const toast = useToast()
const view = ref('issued')
const search = ref('')
const searchEl = ref(null)
const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.deliveries(session.companyId)
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
    matchesDeliveryView(r, view.value) &&
    matches(terms, [r.number, r.customer?.name, r.customer?.ico, r.place, r.order, r.note, r.receiver, r.invoiceNumber, deliveryStatusOf(r).label, r.summary?.total,
      ...(r.lines ?? []).map((l) => l.description)]))
})

const ledger = computed(() => {
  const open = rows.value.filter((r) => r.status === 'issued')
  return {
    openCount: open.length,
    openTotal: open.reduce((a, r) => a + Number(r.summary?.total || 0), 0),
    customers: new Set(open.map((r) => r.contactId || r.customer?.name)).size,
    drafts: rows.value.filter((r) => r.status === 'draft').length,
  }
})

const totalsRow = computed(() => [{ number: `Spolu ${visibleRows.value.length}`, customer: {}, summary: { total: visibleRows.value.reduce((a, r) => a + Number(r.summary?.total || 0), 0).toFixed(2) }, _totals: true }])

const columnDefs = [
  { field: 'number', headerName: 'Číslo', width: 130, cellClass: (p) => (p.data._totals ? '' : 'cell-number'), valueGetter: (p) => p.data.number || (p.data._totals ? '' : 'koncept') },
  { colId: 'customer', headerName: 'Odberateľ', flex: 1, minWidth: 120,
    // na mobile s číslom; súčtový riadok vracia počet, aby sa bunka prekreslila
    valueGetter: (p) => {
      if (p.data._totals) return narrow.matches ? p.data.number : ''
      return narrow.matches ? `${p.data.number || 'koncept'} · ${p.data.customer?.name ?? ''}` : p.data.customer?.name
    } },
  { field: 'date', headerName: 'Dátum', valueFormatter: (p) => formatDate(p.value), width: 100 },
  { field: 'place', headerName: 'Miesto dodania', width: 180 },
  {
    colId: 'status',
    headerName: 'Stav',
    width: 150,
    valueGetter: (p) => (p.data._totals ? '' : deliveryStatusOf(p.data).label),
    cellRenderer: (p) => {
      if (p.data._totals) return ''
      const s = deliveryStatusOf(p.data)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
  { field: 'invoiceNumber', headerName: 'Faktúra', width: 120 },
  { colId: 'total', headerName: 'Spolu', width: 120, type: 'rightAligned', cellClass: 'ag-right-aligned-cell cell-total',
    valueGetter: (p) => Number(p.data.summary?.total || 0), valueFormatter: (p) => formatAmount(p.value) },
]
const defaultColDef = { sortable: true, resizable: true }
const rowClassRules = { 'row-cancelled': (p) => p.data.status === 'cancelled' }
const rowSelection = { mode: 'multiRow', checkboxes: true, headerCheckbox: false, enableClickSelection: false, isRowSelectable: (node) => node.data?.status === 'issued' }
const selected = shallowRef([])
const onSelection = () => (selected.value = gridApi.value?.getSelectedRows() ?? [])
const blocker = computed(() => invoiceBlocker(selected.value))
const invoicing = ref(false)
async function invoiceSelected() {
  invoicing.value = true
  try {
    const inv = await api.invoiceDeliveries(session.companyId, selected.value.map((n) => n.id))
    toast.add({ severity: 'success', summary: 'Koncept faktúry pripravený', detail: 'Skontrolujte ho a vystavte.', life: 3000 })
    router.push(`/faktury/${inv.id}`)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Faktúra sa nevytvorila', detail: e.message, life: 6000 })
  } finally {
    invoicing.value = false
  }
}

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['number', 'date', 'place', 'invoiceNumber'], !narrow.matches)
  gridApi.value?.setColumnWidths([{ key: 'status', newWidth: narrow.matches ? 104 : 150 }, { key: 'total', newWidth: narrow.matches ? 90 : 120 }])
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
  router.push(`/dodacie-listy/${e.data.id}`)
}
function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (typing || e.ctrlKey || e.metaKey || e.altKey) return
  if (e.key === 'n' && can('invoices.write')) {
    e.preventDefault()
    router.push('/dodacie-listy/novy')
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
      <h1>Dodacie listy</h1>
      <div class="spacer" />
      <Button v-if="can('invoices.write')" size="small" @click="router.push('/dodacie-listy/novy')">
        <template #default>
          <i class="pi pi-plus" /><span>Nový dodací list</span><kbd class="hint">N</kbd>
        </template>
      </Button>
    </header>

    <section class="ledger" aria-label="Prehľad dodacích listov">
      <button class="figure figure-link" :disabled="!ledger.openCount" @click="view = 'issued'">
        <span class="label">Na fakturáciu ({{ ledger.openCount }})</span>
        <span class="value">{{ formatMoney(ledger.openTotal) }}</span>
      </button>
      <div class="figure">
        <span class="label">Odberateľov</span>
        <span class="value">{{ ledger.customers }}</span>
      </div>
      <button class="figure figure-link" :disabled="!ledger.drafts" @click="view = 'draft'">
        <span class="label">Koncepty</span>
        <span class="value">{{ ledger.drafts }}</span>
      </button>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="deliveryViews" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText ref="searchEl" v-model="search" placeholder="Hľadať číslo, odberateľa, položku" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton name="Dodacie listy" :columns="deliveryColumns" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>
    <div v-if="can('invoices.write') && selected.length" class="selbar">
      <span>Vybrané: {{ selected.length }}</span>
      <Button label="Vyfakturovať vybrané" icon="pi pi-file" size="small" :disabled="!!blocker" :loading="invoicing" @click="invoiceSelected" />
      <span v-if="blocker" class="muted">{{ blocker }}</span>
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
        :row-selection="rowSelection"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => (p.data._totals ? 'totals' : String(p.data.id))"
        :overlay-no-rows-template="'<span class=&quot;empty&quot;>Žiadne dodacie listy. Nový založíte klávesom N alebo z ponuky či zákazky.</span>'"
        @grid-ready="onGridReady"
        @selection-changed="onSelection"
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
.selbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  padding: 6px 10px;
  background: var(--blue-wash);
  border: 1px solid var(--rule);
}
.muted {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
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
