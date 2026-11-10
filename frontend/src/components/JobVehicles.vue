<script setup>
// Vozidlá zo zákaziek autoservisu: posledná návšteva, stav tachometra,
// STK/EK a ďalší servis. Súhrn vozidiel zostavuje server.
import { computed, onMounted, ref } from 'vue'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Checkbox from 'primevue/checkbox'
import ExportButton from './ExportButton.vue'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatDate, toISODate } from '../lib/format.js'
import { vehicleColumns } from '../lib/columns.js'
import { matches, searchTerms } from '../lib/search.js'
import { dueState } from '../lib/jobs.js'

const list = ref(null)
const search = ref('')
const dueOnly = ref(false)
const today = toISODate(new Date())
onMounted(async () => {
  list.value = await api.jobVehicles(session.companyId).catch(() => [])
})
const rows = computed(() => {
  const terms = searchTerms(search.value)
  return (list.value ?? []).filter((v) =>
    (!dueOnly.value || dueState(v.inspection, today) || dueState(v.nextService, today)) &&
    matches(terms, [v.plate, v.vin, v.make, v.model, v.customer, v.year]))
})
const km = (n) => (n ? `${Number(n).toLocaleString('sk-SK')} km` : '')
</script>

<template>
  <div class="stats">
    <div class="toolbar">
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať EČV, VIN, vozidlo, zákazníka" size="small" aria-label="Hľadať" />
      </IconField>
      <label class="check"><Checkbox v-model="dueOnly" binary /> <span>STK alebo servis do 30 dní</span></label>
      <span class="spacer" />
      <ExportButton name="Vozidlá" :columns="vehicleColumns" :rows="rows" />
    </div>
    <section class="sheet scroll">
      <table class="list">
        <thead>
          <tr>
            <th>EČV</th><th>Vozidlo</th><th class="hide-m">Zákazník</th><th class="hide-m">Posledná návšteva</th><th class="num hide-m">Tachometer</th>
            <th>STK/EK do</th><th class="hide-m">Servis</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="list && !rows.length"><td colspan="7" class="empty">{{ list.length ? 'Nič sa nenašlo.' : 'Zatiaľ žiadne vozidlá. Pribudnú so zákazkami druhu Autoservis.' }}</td></tr>
          <tr v-for="v in rows" :key="v.key">
            <td><RouterLink :to="`/zakazky/${v.lastJobId}`" class="plate">{{ v.plate || '—' }}</RouterLink>
              <span class="sub">{{ v.visits }} {{ v.visits === 1 ? 'návšteva' : v.visits < 5 && v.visits ? 'návštevy' : 'návštev' }}</span></td>
            <td>{{ [v.make, v.model].filter(Boolean).join(' ') }}<span v-if="v.year">, {{ v.year }}</span><span v-if="v.vin" class="sub">{{ v.vin }}</span></td>
            <td class="hide-m"><RouterLink v-if="v.contactId" :to="`/adresar/${v.contactId}`">{{ v.customer }}</RouterLink><template v-else>{{ v.customer }}</template></td>
            <td class="hide-m">{{ formatDate(v.last) }}</td>
            <td class="num hide-m">{{ km(v.mileage) }}</td>
            <td :class="dueState(v.inspection, today)">{{ formatDate(v.inspection) }}</td>
            <td class="hide-m" :class="dueState(v.nextService, today)">{{ formatDate(v.nextService) }}</td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<style scoped>
.stats {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.ledger {
  display: flex;
  flex-wrap: wrap;
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}
.figure {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 8px 20px 9px 14px;
  border-right: 1px solid var(--rule);
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
.sub {
  display: block;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.spacer {
  flex: 1;
}
.search {
  width: 260px;
}
.search :deep(input) {
  width: 100%;
}
.sheet {
  background: var(--sheet);
  border: 1px solid var(--rule);
  overflow: auto;
}
.list {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.list th,
.list td {
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
  text-align: left;
}
.list th {
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink-soft);
}
.list .num {
  text-align: right;
  white-space: nowrap;
}
.list a {
  color: var(--blue);
  text-decoration: none;
  font-weight: 600;
}
.list tfoot td {
  font-weight: 600;
  border-bottom: 0;
}
.grand {
  border-bottom: 3px double var(--ink);
}
.late {
  color: var(--tone-overdue);
  font-weight: 600;
}
.soon {
  font-weight: 600;
}
.plate {
  font-weight: 700;
  letter-spacing: 0.03em;
}
.check {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
}
.empty {
  text-align: center;
  color: var(--ink-soft);
  padding: 18px;
}
@media (max-width: 760px) {
  .hide-m {
    display: none;
  }
  .figure {
    flex: 1 1 45%;
    padding: 6px 10px;
  }
  .figure .value {
    font-size: var(--fs-lg);
  }
  .search {
    flex: 1;
    width: auto;
  }
  .spacer {
    display: none;
  }
  .list th,
  .list td {
    padding: 6px;
  }
}
</style>
