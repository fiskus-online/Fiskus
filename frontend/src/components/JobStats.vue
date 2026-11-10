<script setup>
// Ziskovosť zákaziek podľa zákazníka za rok. Súčty a marže počíta server.
import { computed, onMounted, ref, watch } from 'vue'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import ExportButton from './ExportButton.vue'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatMoney } from '../lib/format.js'
import { jobStatsColumns } from '../lib/columns.js'
import { matches, searchTerms } from '../lib/search.js'
import { pct } from '../lib/jobs.js'

const year = ref(new Date().getFullYear())
const st = ref(null)
const search = ref('')
async function load() {
  st.value = await api.jobStats(session.companyId, year.value || undefined).catch(() => null)
}
onMounted(load)
watch(year, load)
const years = computed(() => [{ value: 0, label: 'Všetky roky' }, ...[...new Set([year.value, ...(st.value?.years ?? [])])].filter(Boolean).sort((a, b) => b - a).map((y) => ({ value: y, label: String(y) }))])
const rows = computed(() => {
  const terms = searchTerms(search.value)
  return (st.value?.rows ?? []).filter((r) => matches(terms, [r.customer]))
})
</script>

<template>
  <div class="stats">
    <section v-if="st" class="ledger" aria-label="Súhrn zákaziek">
      <div class="figure"><span class="label">Zákazky {{ year || 'spolu' }}</span><span class="value">{{ st.total.jobs }}</span>
        <span class="sub">bežiace {{ st.total.open }} · dokončené {{ st.total.done }} · stratené {{ st.total.lost }}</span></div>
      <div class="figure"><span class="label">Úspešnosť</span><span class="value">{{ pct(st.winRate) }}</span><span class="sub">dokončené z uzavretých</span></div>
      <div class="figure"><span class="label">Vyfakturované{{ st.net ? ' (bez DPH)' : '' }}</span><span class="value">{{ formatMoney(st.total.invoiced) }}</span></div>
      <div class="figure"><span class="label">Zisk</span><span class="value">{{ formatMoney(st.total.profit) }}</span><span class="sub">marža {{ pct(st.total.margin) }}</span></div>
    </section>
    <div class="toolbar">
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať zákazníka" size="small" aria-label="Hľadať" />
      </IconField>
      <span class="spacer" />
      <ExportButton name="Zákazky podľa zákazníka" :columns="jobStatsColumns" :rows="rows" />
    </div>
    <section class="sheet scroll">
      <table class="list">
        <thead>
          <tr>
            <th>Zákazník</th><th class="num">Zákaziek</th><th class="num hide-m">Vyfakturované</th><th class="num hide-m">Náklady</th>
            <th class="num">Zisk</th><th class="num">Marža</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="st && !rows.length"><td colspan="6" class="empty">Žiadne zákazky{{ year ? ` v roku ${year}` : '' }}.</td></tr>
          <tr v-for="r in rows" :key="r.contactId || r.customer">
            <td>
              <RouterLink v-if="r.contactId" :to="`/adresar/${r.contactId}`">{{ r.customer }}</RouterLink><template v-else>{{ r.customer }}</template>
              <span class="sub">{{ r.jobs }} {{ r.jobs === 1 ? 'zákazka' : r.jobs < 5 ? 'zákazky' : 'zákaziek' }}<template v-if="Number(r.unbilled)"> · nevyfakturované {{ formatAmount(r.unbilled) }}</template></span>
            </td>
            <td class="num">{{ r.jobs }}</td>
            <td class="num hide-m">{{ formatAmount(r.invoiced) }}</td>
            <td class="num hide-m">{{ formatAmount(r.costs) }}</td>
            <td class="num" :class="{ loss: Number(r.profit) < 0 }"><b>{{ formatAmount(r.profit) }}</b></td>
            <td class="num">{{ pct(r.margin) }}</td>
          </tr>
        </tbody>
        <tfoot v-if="st && rows.length > 1 && !search">
          <tr>
            <td>Spolu</td><td class="num">{{ st.total.jobs }}</td><td class="num hide-m">{{ formatAmount(st.total.invoiced) }}</td>
            <td class="num hide-m">{{ formatAmount(st.total.costs) }}</td><td class="num"><span class="grand">{{ formatAmount(st.total.profit) }}</span></td>
            <td class="num">{{ pct(st.total.margin) }}</td>
          </tr>
        </tfoot>
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
.loss {
  color: var(--tone-overdue);
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
