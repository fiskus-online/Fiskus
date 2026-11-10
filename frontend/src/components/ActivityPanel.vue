<script setup>
// CRM: poznámky, telefonáty, e-maily, stretnutia a úlohy s termínom pri
// kontakte, zákazke alebo ponuke. Úlohy s termínom sú aj v kalendári.
import { computed, onMounted, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Textarea from 'primevue/textarea'
import SelectButton from 'primevue/selectbutton'
import DatePicker from 'primevue/datepicker'
import Checkbox from 'primevue/checkbox'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatDate, fromISODate, toISODate } from '../lib/format.js'
import { activityKind, activityKinds, activityPayload, blankActivity, dueText, sortActivities } from '../lib/activities.js'

const props = defineProps({
  contactId: { type: Number, default: 0 },
  jobId: { type: Number, default: 0 },
  quoteId: { type: Number, default: 0 },
  canWrite: { type: Boolean, default: false },
  showAbout: { type: Boolean, default: false }, // pri kontakte: z ktorej zákazky či ponuky
})
const toast = useToast()
const today = () => toISODate(new Date())
const scope = computed(() => ({ contactId: props.contactId, jobId: props.jobId, quoteId: props.quoteId }))
const list = ref([])
const form = ref(blankActivity(scope.value, today()))
const errors = ref({})
const busy = ref(false)
const open = ref(false)

async function load() {
  const q = props.jobId ? { job: props.jobId } : props.quoteId ? { quote: props.quoteId } : { contact: props.contactId }
  list.value = sortActivities(await api.activities(session.companyId, q).catch(() => []))
}
onMounted(load)
watch(scope, load)

function startNew(kind = 'note') {
  form.value = { ...blankActivity(scope.value, today()), kind }
  if (kind === 'task') form.value.due = toISODate(new Date(Date.now() + 7 * 86400000))
  errors.value = {}
  open.value = true
}
async function add() {
  busy.value = true
  errors.value = {}
  try {
    const x = await api.saveActivity(session.companyId, activityPayload(form.value))
    list.value = sortActivities([x, ...list.value])
    open.value = false
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Záznam sa neuložil', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
async function toggle(x) {
  try {
    const y = await api.activityDone(session.companyId, x.id, !x.done)
    list.value = sortActivities(list.value.map((a) => (a.id === y.id ? y : a)))
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  }
}
async function remove(x) {
  try {
    await api.deleteActivity(session.companyId, x.id)
    list.value = list.value.filter((a) => a.id !== x.id)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Záznam sa nezmazal', detail: e.message, life: 6000 })
  }
}
const openTasks = computed(() => list.value.filter((a) => a.kind === 'task' && !a.done).length)
const due = (iso) => fromISODate(iso)
</script>

<template>
  <section class="crm" aria-label="Záznamy a úlohy">
    <header class="ehead">
      <h2>Záznamy a úlohy</h2>
      <span v-if="openTasks" class="muted">{{ openTasks }} {{ openTasks === 1 ? 'úloha' : openTasks < 5 ? 'úlohy' : 'úloh' }}</span>
      <span class="spacer" />
      <template v-if="canWrite && !open">
        <Button label="Záznam" icon="pi pi-plus" size="small" text @click="startNew('note')" />
        <Button label="Úloha" icon="pi pi-check-square" size="small" text @click="startNew('task')" />
      </template>
    </header>

    <form v-if="open" class="composer" @submit.prevent="add">
      <SelectButton v-model="form.kind" :options="activityKinds" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Druh záznamu">
        <template #option="{ option }"><i :class="option.icon" :title="option.label" /><span class="kl">{{ option.label }}</span></template>
      </SelectButton>
      <Textarea v-model="form.text" rows="2" auto-resize :invalid="!!errors.text" autofocus
        :placeholder="form.kind === 'task' ? 'Čo treba urobiť, napr. ozvať sa k ponuke' : 'Čo sa dohodlo, kto volal…'" />
      <small v-if="errors.text" class="err">{{ errors.text }}</small>
      <div class="row">
        <label v-if="form.kind === 'task'" class="due">Termín
          <DatePicker :model-value="due(form.due)" date-format="d. m. yy" size="small" :invalid="!!errors.due" @update:model-value="form.due = toISODate($event) ?? ''" />
        </label>
        <label v-else class="due">Dátum
          <DatePicker :model-value="due(form.date)" date-format="d. m. yy" size="small" @update:model-value="form.date = toISODate($event) ?? ''" />
        </label>
        <span class="spacer" />
        <Button label="Zrušiť" severity="secondary" text size="small" @click="open = false" />
        <Button type="submit" label="Pridať" icon="pi pi-check" size="small" :loading="busy" />
      </div>
      <small v-if="errors.due || errors.contactId" class="err">{{ errors.due || errors.contactId }}</small>
    </form>

    <p v-if="!list.length && !open" class="muted small">Zatiaľ nič. Zapíšte telefonát, dohodu alebo úlohu s termínom — úloha sa ukáže aj v kalendári.</p>
    <ol v-else class="feed">
      <li v-for="x in list" :key="x.id" :class="{ done: x.done, late: x.late }">
        <span class="icon" :title="activityKind(x.kind).label">
          <Checkbox v-if="x.kind === 'task'" :model-value="x.done" binary :disabled="!canWrite" :aria-label="x.done ? 'Označiť ako nesplnenú' : 'Označiť ako splnenú'" @update:model-value="toggle(x)" />
          <i v-else :class="activityKind(x.kind).icon" />
        </span>
        <div class="body">
          <p class="text">{{ x.text }}</p>
          <span class="meta">
            <template v-if="x.kind === 'task'">
              <b v-if="!x.done" :class="{ lateText: x.late }">termín {{ formatDate(x.due) }} ({{ dueText(x.due, today()) }})</b>
              <span v-else>splnená {{ formatDate(x.doneDate) }}</span>
              ·
            </template>
            {{ activityKind(x.kind).label }} {{ formatDate(x.date) }}<template v-if="x.author && x.author !== 'Systém'"> · {{ x.author }}</template>
            <template v-if="showAbout && x.about && x.link && !x.link.startsWith('/adresar')"> · <RouterLink :to="x.link">{{ x.about }}</RouterLink></template>
          </span>
        </div>
        <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Zmazať záznam" @click="remove(x)" />
      </li>
    </ol>
  </section>
</template>

<style scoped>
.crm {
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
.small {
  margin: 0;
  font-size: var(--fs-sm);
}
.composer {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.composer :deep(.p-selectbutton) {
  flex-wrap: wrap;
}
.composer :deep(.p-togglebutton i) {
  margin-right: 5px;
}
.composer :deep(textarea) {
  width: 100%;
}
.row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.due {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.due :deep(input) {
  width: 120px;
}
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.feed {
  list-style: none;
  margin: 0;
  padding: 0;
}
.feed li {
  display: grid;
  grid-template-columns: 28px 1fr auto;
  gap: 8px;
  align-items: start;
  padding: 6px 0;
  border-bottom: 1px solid var(--rule);
}
.icon {
  display: flex;
  justify-content: center;
  padding-top: 2px;
  color: var(--blue);
}
.text {
  margin: 0;
  white-space: pre-line;
}
.done .text {
  color: var(--ink-soft);
  text-decoration: line-through;
}
.meta {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.meta a {
  color: var(--blue);
}
.lateText {
  color: var(--tone-overdue);
}
.late .icon {
  color: var(--tone-overdue);
}
@media (max-width: 760px) {
  .kl {
    display: none;
  }
  .composer :deep(.p-togglebutton i) {
    margin-right: 0;
  }
}
</style>
