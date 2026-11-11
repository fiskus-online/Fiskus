<script setup>
// Najbližšie termíny z kalendára (a zmeškané) na úvodnej stránke firmy aj
// domácnosti. Termíny počíta server; tu sa iba zobrazia s preklikom.
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { formatAmount, formatDate } from '../lib/format.js'
import { kindLabel, relative } from '../lib/calendar.js'

const props = defineProps({
  events: { type: Array, default: () => [] },
  overdue: { type: Array, default: () => [] },
  today: { type: String, default: '' },
  limit: { type: Number, default: 8 },
})
const router = useRouter()
// zmeškané najviac 3, spolu najviac limit riadkov
const rows = computed(() => {
  const late = props.overdue.slice(-3).map((e) => ({ ...e, late: true }))
  return [...late, ...props.events].slice(0, props.limit)
})
const more = computed(() => props.events.length + Math.min(props.overdue.length, 3) - rows.value.length)
const open = (e) => e.link && router.push(e.link)
</script>

<template>
  <section class="panel" aria-label="Najbližšie termíny">
    <header>
      <h2>Termíny</h2>
      <span class="muted">najbližších 14 dní</span>
      <RouterLink to="/kalendar" class="link">Kalendár</RouterLink>
    </header>
    <p v-if="!rows.length" class="empty">Žiadne termíny na najbližšie dva týždne.</p>
    <table v-else class="ev">
      <tbody>
        <tr v-for="e in rows" :key="e.id + e.date" :class="{ late: e.late, click: !!e.link }" @click="open(e)">
          <td class="d">{{ formatDate(e.date) }}<small>{{ e.late ? 'zmeškané' : relative(today, e.date) }}</small></td>
          <td class="w"><span class="t">{{ e.title }}</span><small>{{ kindLabel(e.kind) }}<template v-if="e.detail"> · {{ e.detail }}</template></small></td>
          <td class="num">{{ e.amount ? formatAmount(e.amount) : '' }}</td>
        </tr>
      </tbody>
    </table>
    <RouterLink v-if="more > 0" to="/kalendar" class="more">ďalšie termíny ({{ more }})</RouterLink>
  </section>
</template>

<style scoped>
.panel {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 10px 12px 12px;
  min-width: 0;
}
header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 8px;
}
h2 {
  font-size: var(--fs-md);
  font-weight: 600;
  color: var(--blue-deep);
}
.muted {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.link {
  margin-left: auto;
  font-size: var(--fs-sm);
  color: var(--blue);
}
.empty {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.ev {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.ev td {
  padding: 4px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: top;
}
.ev tr.click {
  cursor: pointer;
}
.ev tr.click:hover {
  background: var(--blue-wash);
}
.d {
  width: 92px;
  white-space: nowrap;
}
.d small,
.w small {
  display: block;
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.w {
  min-width: 0;
}
.t {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
}
.num {
  text-align: right;
  white-space: nowrap;
  font-weight: 600;
}
.more {
  display: inline-block;
  margin-top: 6px;
  font-size: var(--fs-sm);
  color: var(--blue);
}
.late .d,
.late .d small {
  color: var(--tone-overdue);
  font-weight: 600;
}
</style>
