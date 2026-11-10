<script setup>
// Nástenka zákaziek: stĺpce podľa stavu. Kartu presuniete myšou do iného
// stĺpca (zmení stav), kliknutím zákazku otvoríte. Sumy sú zo servera.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate } from '../lib/format.js'
import { boardColumns, movePayload } from '../lib/jobs.js'

const props = defineProps({ rows: { type: Array, required: true }, canWrite: { type: Boolean, default: false } })
const emit = defineEmits(['changed'])
const router = useRouter()
const toast = useToast()
const cols = computed(() => boardColumns(props.rows))
const dragging = ref(0)
const over = ref('')

function onDrop(status) {
  const row = props.rows.find((r) => r.id === dragging.value)
  over.value = ''
  dragging.value = 0
  if (!row || row.status === status || !props.canWrite) return
  move(row, status)
}
async function move(row, status) {
  try {
    await api.saveJob(session.companyId, movePayload(row, status), row.id)
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Stav sa nezmenil', detail: e.message, life: 6000 })
  }
}
const sum = (jobs) => jobs.reduce((a, j) => a + Number(j.budget || 0), 0)
</script>

<template>
  <div class="board" role="list" aria-label="Nástenka zákaziek">
    <section v-for="c in cols" :key="c.value" class="col" :class="{ over: over === c.value }" role="listitem" :aria-label="c.label"
      @dragover.prevent="over = c.value" @dragleave="over === c.value && (over = '')" @drop.prevent="onDrop(c.value)">
      <header class="chead">
        <span class="tag" :data-tone="c.tone"><span class="tag-text">{{ c.label }}</span></span>
        <span class="count">{{ c.jobs.length }}</span>
        <span class="spacer" />
        <span v-if="sum(c.jobs)" class="csum">{{ formatAmount(sum(c.jobs)) }}</span>
      </header>
      <article v-for="j in c.jobs" :key="j.id" class="card" :class="{ late: j.late }" :draggable="canWrite" tabindex="0"
        @dragstart="dragging = j.id" @dragend="dragging = 0; over = ''" @click="router.push(`/zakazky/${j.id}`)" @keydown.enter="router.push(`/zakazky/${j.id}`)">
        <span class="no">{{ j.number }}</span>
        <strong class="name">{{ j.name }}</strong>
        <span v-if="j.subject" class="subj">{{ j.subject }}</span>
        <span v-if="j.customer" class="cust">{{ j.customer }}</span>
        <span class="meta">
          <span v-if="j.due" :class="{ lateText: j.late }">termín {{ formatDate(j.due) }}</span>
          <span v-if="Number(j.budget)" class="amt">{{ formatAmount(j.budget) }}</span>
        </span>
      </article>
      <p v-if="!c.jobs.length" class="empty">Žiadne zákazky</p>
    </section>
  </div>
</template>

<style scoped>
.board {
  display: grid;
  grid-template-columns: repeat(5, minmax(200px, 1fr));
  gap: 10px;
  overflow-x: auto;
  flex: 1;
  min-height: 320px;
  align-items: start;
}
.col {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 160px;
  padding: 8px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
}
.col.over {
  background: var(--blue-wash);
}
.chead {
  display: flex;
  align-items: center;
  gap: 6px;
  padding-bottom: 4px;
}
.count {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.spacer {
  flex: 1;
}
.csum {
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
  color: var(--ink-soft);
}
.card {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 7px 9px;
  background: var(--paper, #fff);
  border: 1px solid var(--rule);
  border-left: 3px solid var(--blue);
  cursor: pointer;
}
.card:hover,
.card:focus-visible {
  border-color: var(--blue);
}
.card.late {
  border-left-color: var(--tone-overdue);
}
.no {
  font-size: var(--fs-xs);
  font-weight: 600;
  color: var(--blue);
}
.name {
  font-weight: 600;
  line-height: 1.25;
}
.cust {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.subj {
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--blue-deep);
}
.meta {
  display: flex;
  justify-content: space-between;
  gap: 6px;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  font-variant-numeric: tabular-nums;
}
.amt {
  color: var(--ink);
}
.lateText {
  color: var(--tone-overdue);
  font-weight: 600;
}
.empty {
  margin: 0;
  padding: 10px 0;
  text-align: center;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
@media (max-width: 760px) {
  .board {
    grid-template-columns: repeat(5, 78vw);
    scroll-snap-type: x mandatory;
  }
  .col {
    scroll-snap-align: start;
  }
}
</style>
