<script setup>
// Súvisiace doklady: prepojenia s inými dokladmi (ručné sa dajú zrušiť,
// tie, ktoré vyplývajú z dokladu — faktúra z ponuky, dobropis… — nie).
import { onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { docPath } from '../lib/links.js'
import { formatDate, formatMoney } from '../lib/format.js'
import DocPicker from './DocPicker.vue'

const props = defineProps({
  kind: { type: String, required: true },
  id: { type: Number, required: true },
  canWrite: { type: Boolean, default: false },
})
const emit = defineEmits(['changed'])
const toast = useToast()
const rows = ref([])
const loaded = ref(false)
const picking = ref(false)
const busy = ref(0)

async function load() {
  if (!props.id) return
  try {
    rows.value = await api.docLinks(session.companyId, props.kind, props.id)
  } catch {
    rows.value = []
  } finally {
    loaded.value = true
  }
}
onMounted(load)
watch(() => [props.kind, props.id], load)

async function pick(c) {
  try {
    rows.value = await api.linkDoc(session.companyId, props.kind, props.id, { kind: c.kind, id: c.id })
    picking.value = false
    toast.add({ severity: 'success', summary: `Prepojené: ${c.number}`, life: 2500 })
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Prepojenie zlyhalo', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
  }
}

async function unlink(r) {
  busy.value = r.linkId
  try {
    rows.value = await api.unlinkDoc(session.companyId, props.kind, props.id, r.linkId)
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Prepojenie sa nezrušilo', detail: e.message, life: 6000 })
  } finally {
    busy.value = 0
  }
}

defineExpose({ load })
</script>

<template>
  <fieldset v-if="loaded && (rows.length || canWrite)" class="block form-section related">
    <legend>Súvisiace doklady <span v-if="rows.length" class="muted">{{ rows.length }}</span></legend>
    <ul v-if="rows.length" class="rel">
      <li v-for="r in rows" :key="r.kind + r.id">
        <RouterLink :to="docPath(r)" class="doc">
          <span class="type">{{ r.type }}</span>
          <strong class="docnum">{{ r.number || 'koncept' }}</strong>
        </RouterLink>
        <span class="who">{{ r.partner }}<small v-if="r.relation" class="rel-why"> · {{ r.relation }}</small></span>
        <span class="date">{{ r.date ? formatDate(r.date) : '' }}</span>
        <span class="sum">{{ r.total && r.total !== '0.00' ? formatMoney(r.total, r.currency) : '' }}</span>
        <Button v-if="canWrite && r.linkId" icon="pi pi-times" text rounded size="small" severity="secondary" :loading="busy === r.linkId"
          :aria-label="`Zrušiť prepojenie s ${r.number}`" v-tooltip.left="'Zrušiť prepojenie'" @click="unlink(r)" />
        <span v-else class="fixed" />
      </li>
    </ul>
    <p v-else class="muted empty">Žiadne. Prepojte ponuku, dodací list, objednávku alebo prijatú faktúru, ktoré k dokladu patria.</p>
    <Button v-if="canWrite" label="Prepojiť doklad" icon="pi pi-link" size="small" text @click="picking = true" />
    <DocPicker v-model:visible="picking" :self="{ kind, id }" :linked="rows" @pick="pick" />
  </fieldset>
</template>

<style scoped>
.related {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.35rem;
}
.rel {
  list-style: none;
  margin: 0;
  padding: 0;
  width: 100%;
}
.rel li {
  display: grid;
  grid-template-columns: minmax(9rem, auto) minmax(0, 1fr) auto auto 2rem;
  gap: 0.75rem;
  align-items: center;
  padding: 0.3rem 0;
  border-bottom: 1px solid var(--rule, #dde3ea);
}
.doc {
  display: flex;
  flex-direction: column;
  text-decoration: none;
  color: var(--blue);
  line-height: 1.2;
}
.type {
  font-size: var(--fs-sm, 0.8rem);
  color: var(--ink-soft);
}
.docnum {
  font-variant-numeric: tabular-nums;
}
.who {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rel-why {
  color: var(--ink-soft);
}
.date,
.sum {
  font-variant-numeric: tabular-nums;
  text-align: right;
  color: var(--ink-soft);
}
.sum {
  color: var(--ink);
}
.muted {
  color: var(--ink-soft);
  font-weight: 400;
}
.empty {
  margin: 0;
  font-size: var(--fs-sm, 0.85rem);
}
@media (max-width: 640px) {
  .rel li {
    grid-template-columns: minmax(0, 1fr) auto 2rem;
    gap: 0.1rem 0.6rem;
  }
  .who {
    grid-column: 1;
    grid-row: 2;
  }
  .date {
    display: none;
  }
  .sum {
    grid-row: 1;
    grid-column: 2;
  }
}
</style>
