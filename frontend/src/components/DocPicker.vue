<script setup>
// Výber dokladu na prepojenie: druh, hľadanie (číslo, partner, suma),
// najnovšie prvé. Zoznamy sa načítajú až pri výbere druhu.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { candidates, linkKinds, pickable } from '../lib/links.js'
import { formatDate, formatMoney } from '../lib/format.js'

const props = defineProps({
  header: { type: String, default: 'Prepojiť doklad' },
  kinds: { type: Array, default: () => linkKinds.map((k) => k.value) },
  self: { type: Object, default: null }, // { kind, id }
  linked: { type: Array, default: () => [] },
  // filter kandidátov (napr. iba vystavené faktúry)
  accept: { type: Function, default: null },
})
const visible = defineModel('visible', { type: Boolean, default: false })
const emit = defineEmits(['pick'])

const options = computed(() => linkKinds.filter((k) => props.kinds.includes(k.value) && can(k.perm)))
const kind = ref('')
const q = ref('')
const loading = ref(false)
const error = ref('')
const cache = {}
const list = ref([])

async function load(k) {
  if (!k) return
  if (cache[k]) {
    list.value = cache[k]
    return
  }
  loading.value = true
  error.value = ''
  try {
    const cid = session.companyId
    let rows = []
    if (k === 'invoice') rows = candidates(k, await api.invoices(cid, {}))
    else if (k === 'quote') rows = candidates(k, await api.quotes(cid))
    else if (k === 'purchase') rows = candidates(k, await api.purchases(cid, {}))
    else if (k === 'job') rows = candidates(k, await api.jobs(cid))
    else if (k === 'form') {
      const [docs, cat] = await Promise.all([api.formDocs(cid), api.forms(cid)])
      rows = candidates(k, docs, cat.forms)
    }
    cache[k] = rows
    list.value = rows
  } catch (e) {
    error.value = e.message
    list.value = []
  } finally {
    loading.value = false
  }
}

watch(visible, (v) => {
  if (!v) return
  q.value = ''
  if (!options.value.find((o) => o.value === kind.value)) kind.value = options.value[0]?.value ?? ''
  load(kind.value)
})
watch(kind, load)

const shown = computed(() => {
  const rows = props.accept ? list.value.filter(props.accept) : list.value
  return pickable(rows, { self: props.self, linked: props.linked, q: q.value })
})

function pick(c) {
  emit('pick', c)
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="header" :style="{ width: '40rem' }" :breakpoints="{ '760px': '96vw' }" class="doc-picker">
    <div class="pick">
      <SelectButton v-if="options.length > 1" v-model="kind" :options="options" option-label="short" option-value="value" :allow-empty="false" size="small"
        aria-label="Druh dokladu" />
      <InputText v-model="q" size="small" fluid placeholder="Hľadať číslo, partnera, sumu…" aria-label="Hľadať doklad" autofocus />
      <p v-if="error" class="err">{{ error }}</p>
      <p v-else-if="loading" class="muted">Načítavam…</p>
      <p v-else-if="!shown.length" class="muted">Nič sa nenašlo.</p>
      <ul v-else class="cands">
        <li v-for="c in shown" :key="c.kind + c.id">
          <button type="button" class="cand" @click="pick(c)">
            <span class="docnum">{{ c.number }}<small v-if="c.type"> · {{ c.type }}</small></span>
            <span class="who">{{ c.partner }}</span>
            <span class="date">{{ c.date ? formatDate(c.date) : '' }}</span>
            <span class="sum">{{ c.total != null && c.total !== '' ? formatMoney(c.total) : '' }}</span>
          </button>
        </li>
      </ul>
    </div>
  </Dialog>
</template>

<style scoped>
.pick {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}
.cands {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 22rem;
  overflow: auto;
  border-top: 1px solid var(--rule, #dde3ea);
}
.cand {
  display: grid;
  grid-template-columns: minmax(7rem, auto) minmax(0, 1fr) auto auto;
  gap: 0.75rem;
  align-items: baseline;
  width: 100%;
  padding: 0.5rem 0.25rem;
  border: 0;
  border-bottom: 1px solid var(--rule, #dde3ea);
  background: none;
  font: inherit;
  text-align: left;
  color: var(--ink);
  cursor: pointer;
}
.cand:hover,
.cand:focus-visible {
  background: #eef3f8;
  outline: none;
}
.docnum {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.docnum small {
  font-weight: 400;
  color: var(--ink-soft);
}
.who {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.date,
.sum {
  font-variant-numeric: tabular-nums;
  color: var(--ink-soft);
  text-align: right;
}
.muted {
  margin: 0;
  color: var(--ink-soft);
}
.err {
  margin: 0;
  color: var(--tone-overdue);
}
@media (max-width: 560px) {
  .cand {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.1rem 0.75rem;
  }
  .docnum {
    grid-row: 1;
    grid-column: 1;
  }
  .date {
    grid-row: 1;
    grid-column: 2;
  }
  .who {
    grid-row: 2;
    grid-column: 1;
  }
  .sum {
    grid-row: 2;
    grid-column: 2;
  }
}
</style>
