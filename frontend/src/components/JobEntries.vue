<script setup>
// Výkaz práce alebo materiál zákazky: hustá tabuľka na úpravu. Vyfakturované
// riadky sú len na čítanie (opravujú sa dobropisom alebo stornom faktúry).
// Sumy riadkov a súčty počíta server; nový riadok má sumu až po uložení.
import { computed } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import { formatAmount, fromISODate, toISODate } from '../lib/format.js'
import { blankMaterial, blankWork, pickItem } from '../lib/jobs.js'

const props = defineProps({
  kind: { type: String, required: true }, // work | material
  form: { type: Object, required: true }, // formulár zákazky (rows = form.work / form.material)
  errors: { type: Object, default: () => ({}) },
  readonly: { type: Boolean, default: false },
  items: { type: Array, default: () => [] }, // cenník (materiál)
  people: { type: Array, default: () => [] }, // návrhy pracovníkov
  hours: { type: String, default: '' }, // súčet hodín zo servera
  total: { type: String, default: '' }, // súčet nákladov zo servera
})

const work = computed(() => props.kind === 'work')
const rows = computed(() => (work.value ? props.form.work : props.form.material))
const today = () => toISODate(new Date())
const err = (i, f) => props.errors[`${props.kind}[${i}].${f}`]
const rowErr = (i) =>
  Object.entries(props.errors).find(([k]) => k.startsWith(`${props.kind}[${i}].`))?.[1] ?? ''
const listErr = computed(() => props.errors[props.kind] ?? '')

function add() {
  rows.value.push(work.value ? blankWork(props.form, today()) : blankMaterial(today()))
}
function removeRow(i) {
  rows.value.splice(i, 1)
}
const itemOptions = computed(() =>
  props.items.filter((it) => !it.archived).map((it) => ({ value: it.id, label: it.code ? `${it.code} · ${it.name}` : it.name })),
)
function setItem(i, id) {
  rows.value[i] = pickItem(rows.value[i], props.items.find((it) => it.id === id) ?? null)
}
const peopleId = computed(() => `people-${props.kind}`)
</script>

<template>
  <section class="entries" :aria-label="work ? 'Výkaz práce' : 'Materiál'">
    <header class="ehead">
      <h2>{{ work ? 'Výkaz práce' : 'Materiál' }}</h2>
      <span v-if="work && Number(hours)" class="muted">{{ formatAmount(hours) }} h</span>
      <span class="spacer" />
      <Button v-if="!readonly" :label="work ? 'Práca' : 'Materiál'" icon="pi pi-plus" size="small" text @click="add" />
    </header>
    <p v-if="!rows.length" class="empty muted">
      {{ work ? 'Zapíšte, kto a koľko hodín na zákazke pracoval. Z výkazu sa dá urobiť faktúra.' : 'Materiál z cenníka vedený na sklade sa vydá zo skladu výdajkou zákazky.' }}
    </p>
    <div v-else class="grid" :class="kind" role="table">
      <div class="row thead" role="row">
        <span role="columnheader">Dátum</span>
        <template v-if="work">
          <span role="columnheader">Kto</span>
          <span role="columnheader">Popis práce</span>
          <span role="columnheader" class="r">Hodiny</span>
          <span role="columnheader" class="r">Sadzba €/h</span>
          <span role="columnheader" class="r">Náklad €/h</span>
        </template>
        <template v-else>
          <span role="columnheader">Položka</span>
          <span role="columnheader" class="r">Množstvo</span>
          <span role="columnheader">MJ</span>
          <span role="columnheader" class="r">Cena €/MJ</span>
          <span role="columnheader" class="r">Nákup €/MJ</span>
        </template>
        <span role="columnheader" class="r">Spolu</span>
        <span role="columnheader" />
      </div>
      <div v-for="(r, i) in rows" :key="r.id || `n${i}`" class="row" :class="{ billed: r.invoiceId }" role="row">
        <span class="cell" data-label="Dátum">
          <DatePicker :model-value="fromISODate(r.date)" date-format="d. m. yy" size="small" fluid :disabled="readonly || !!r.invoiceId"
            :invalid="!!err(i, 'date')" @update:model-value="r.date = toISODate($event) ?? ''" />
        </span>
        <template v-if="work">
          <span class="cell" data-label="Kto">
            <InputText v-model="r.person" size="small" :list="peopleId" :disabled="readonly || !!r.invoiceId" placeholder="Meno" />
          </span>
          <span class="cell wide" data-label="Popis práce">
            <InputText v-model="r.description" size="small" :disabled="readonly || !!r.invoiceId" :invalid="!!err(i, 'description')" placeholder="Čo sa robilo" />
          </span>
          <span class="cell" data-label="Hodiny">
            <InputText v-model="r.hours" size="small" inputmode="decimal" class="r" :disabled="readonly || !!r.invoiceId" :invalid="!!err(i, 'hours')" />
          </span>
          <span class="cell" data-label="Sadzba €/h">
            <InputText v-model="r.rate" size="small" inputmode="decimal" class="r" :disabled="readonly || !!r.invoiceId" :invalid="!!err(i, 'rate')" />
          </span>
          <span class="cell" data-label="Náklad €/h">
            <InputText v-model="r.costRate" size="small" inputmode="decimal" class="r" :disabled="readonly || !!r.invoiceId" placeholder="0" />
          </span>
        </template>
        <template v-else>
          <span class="cell wide item" data-label="Položka">
            <Select :model-value="r.itemId || null" :options="itemOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
              :disabled="readonly || !!r.invoiceId" placeholder="Z cenníka…" @update:model-value="setItem(i, $event)" />
            <InputText v-if="!r.itemId" v-model="r.name" size="small" :disabled="readonly || !!r.invoiceId" :invalid="!!err(i, 'name')" placeholder="alebo názov materiálu" />
            <span v-else-if="r.stock" class="stock">vydané zo skladu</span>
          </span>
          <span class="cell" data-label="Množstvo">
            <InputText v-model="r.quantity" size="small" inputmode="decimal" class="r" :disabled="readonly || !!r.invoiceId" :invalid="!!err(i, 'quantity')" />
          </span>
          <span class="cell" data-label="MJ">
            <InputText v-model="r.unit" size="small" :disabled="readonly || !!r.invoiceId || !!r.itemId" />
          </span>
          <span class="cell" data-label="Cena €/MJ">
            <InputText v-model="r.unitPrice" size="small" inputmode="decimal" class="r" :disabled="readonly || !!r.invoiceId" :invalid="!!err(i, 'unitPrice')" />
          </span>
          <span class="cell" data-label="Nákup €/MJ">
            <span v-if="r.stock" class="ro r" title="Náklad je z ocenenia skladu">{{ formatAmount(r.cost) }} spolu</span>
            <InputText v-else v-model="r.unitCost" size="small" inputmode="decimal" class="r" :disabled="readonly || !!r.invoiceId" placeholder="0" />
          </span>
        </template>
        <span class="cell sum" data-label="Spolu">{{ r.id && r.total ? formatAmount(r.total) : '—' }}</span>
        <span class="cell act">
          <RouterLink v-if="r.invoiceId" :to="`/faktury/${r.invoiceId}`" class="inv" :title="`Vyfakturované na ${r.invoiceNumber}`">{{ r.invoiceNumber }}</RouterLink>
          <Button v-else-if="!readonly" icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Odstrániť riadok" @click="removeRow(i)" />
        </span>
        <small v-if="rowErr(i)" class="rerr">{{ rowErr(i) }}</small>
      </div>
      <div v-if="!work && Number(total)" class="row tfoot" role="row">
        <span class="foot-label">Náklad materiálu</span>
        <span class="cell sum">{{ formatAmount(total) }}</span>
        <span />
      </div>
    </div>
    <small v-if="listErr" class="rerr">{{ listErr }}</small>
    <datalist v-if="work" :id="peopleId">
      <option v-for="p in people" :key="p" :value="p" />
    </datalist>
  </section>
</template>

<style scoped>
.entries {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ehead {
  display: flex;
  align-items: baseline;
  gap: 10px;
  border-bottom: 1px solid var(--rule);
  padding-bottom: 2px;
}
.ehead h2 {
  margin: 0;
  font-size: var(--fs-md);
}
.spacer {
  flex: 1;
}
.muted {
  color: var(--ink-soft);
}
.empty {
  margin: 0;
  font-size: var(--fs-sm);
}
.grid {
  display: flex;
  flex-direction: column;
  font-variant-numeric: tabular-nums;
}
.row {
  display: grid;
  gap: 6px;
  align-items: center;
  padding: 3px 0;
  border-bottom: 1px solid var(--rule);
}
.grid.work .row {
  grid-template-columns: 118px 130px minmax(160px, 1fr) 72px 88px 88px 92px 92px;
}
.grid.material .row {
  grid-template-columns: 118px minmax(200px, 1fr) 84px 56px 88px 96px 92px 92px;
}
.grid .row.tfoot {
  grid-template-columns: 1fr 92px 92px;
  border-bottom: 0;
}
.thead {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.cell {
  min-width: 0;
}
.cell :deep(input) {
  width: 100%;
}
.item {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.stock {
  font-size: var(--fs-xs);
  color: var(--tone-paid);
}
.r,
.cell :deep(input.r) {
  text-align: right;
}
.sum {
  text-align: right;
  font-weight: 600;
}
.ro {
  display: block;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.act {
  display: flex;
  justify-content: flex-end;
}
.inv {
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--blue);
  text-decoration: none;
  white-space: nowrap;
}
.billed {
  background: var(--tone-paid-bg);
}
.rerr {
  grid-column: 1 / -1;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.foot-label {
  text-align: right;
  color: var(--ink-soft);
}
@media (max-width: 760px) {
  .thead {
    display: none;
  }
  .grid.work .row,
  .grid.material .row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    padding: 8px 0;
  }
  .grid .row.tfoot {
    grid-template-columns: 1fr auto;
  }
  .cell[data-label]::before {
    content: attr(data-label);
    display: block;
    font-size: var(--fs-xs);
    color: var(--ink-faint);
  }
  .cell.wide {
    grid-column: 1 / -1;
  }
  .sum {
    text-align: left;
  }
}
</style>
