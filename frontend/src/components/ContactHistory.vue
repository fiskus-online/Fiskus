<script setup>
// CRM pri kontakte: jeho zákazky a posledné doklady (ponuky a faktúry).
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatDate, formatMoney, statusOf } from '../lib/format.js'
import { jobStatusOf } from '../lib/jobs.js'
import { quoteStatusOf } from '../lib/quotes.js'
import ActivityPanel from './ActivityPanel.vue'

const props = defineProps({ contactId: { type: Number, required: true } })
const jobs = ref([])
const docs = ref([])
const loaded = ref(false)

async function load() {
  loaded.value = false
  if (!can('invoices.read')) return
  const [js, invs, qs] = await Promise.all([
    api.jobs(session.companyId).catch(() => []),
    api.invoices(session.companyId, {}).catch(() => []),
    api.quotes(session.companyId).catch(() => []),
  ])
  const mine = (x) => x.contactId === props.contactId
  jobs.value = js.filter(mine)
  docs.value = [
    ...invs.filter(mine).map((i) => ({ key: 'i' + i.id, path: `/faktury/${i.id}`, type: 'Faktúra', number: i.number || 'koncept', date: i.issueDate, total: i.summary?.total, currency: i.currency, status: statusOf(i) })),
    ...qs.filter(mine).map((q) => ({ key: 'q' + q.id, path: `/ponuky/${q.id}`, type: 'Ponuka', number: q.number || 'koncept', date: q.issueDate, total: q.summary?.total, currency: q.currency, status: quoteStatusOf(q) })),
  ]
    .sort((a, b) => String(b.date ?? '').localeCompare(String(a.date ?? '')))
    .slice(0, 12)
  loaded.value = true
}
onMounted(load)
watch(() => props.contactId, load)
const empty = computed(() => loaded.value && !jobs.value.length && !docs.value.length)
</script>

<template>
  <ActivityPanel class="crm-panel" :contact-id="contactId" :can-write="can('invoices.write')" show-about />
  <section v-if="loaded && !empty" class="hist" aria-label="Zákazky a doklady kontaktu">
    <template v-if="jobs.length">
      <h3>Zákazky</h3>
      <ul>
        <li v-for="j in jobs" :key="j.id">
          <RouterLink :to="`/zakazky/${j.id}`"><strong>{{ j.number }}</strong> {{ j.name }}</RouterLink>
          <span class="tag" :data-tone="jobStatusOf(j).tone"><span class="tag-text">{{ jobStatusOf(j).label }}</span></span>
        </li>
      </ul>
    </template>
    <template v-if="docs.length">
      <h3>Posledné doklady</h3>
      <ul>
        <li v-for="d in docs" :key="d.key">
          <RouterLink :to="d.path"><span class="muted">{{ d.type }}</span> <strong>{{ d.number }}</strong></RouterLink>
          <span class="muted">{{ formatDate(d.date) }}</span>
          <span class="sum">{{ formatMoney(d.total, d.currency) }}</span>
        </li>
      </ul>
    </template>
  </section>
</template>

<style scoped>
.crm-panel {
  margin-top: 10px;
}
.hist h3 {
  margin: 0.6rem 0 0.25rem;
  font-size: var(--fs-md, 0.95rem);
}
.hist ul {
  list-style: none;
  margin: 0;
  padding: 0;
}
.hist li {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.25rem 0;
  border-bottom: 1px solid var(--rule, #dde3ea);
  font-variant-numeric: tabular-nums;
}
.hist a {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--blue);
  text-decoration: none;
}
.muted {
  color: var(--ink-soft);
}
.sum {
  min-width: 6.5rem;
  text-align: right;
}
</style>
