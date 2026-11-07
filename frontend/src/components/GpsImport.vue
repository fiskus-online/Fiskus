<script setup>
// Import jázd zo záznamu GPS (GPX alebo CSV z mobilu či GPS jednotky v aute).
// Jazdy navrhne server, tu sa doplní účel a skontrolujú kilometre.
import { computed, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import AutoComplete from 'primevue/autocomplete'
import SelectButton from 'primevue/selectbutton'
import { api, ApiError } from '../lib/api.js'
import { formatDate } from '../lib/format.js'
import { addPlace, chainOdometer, gpsDrafts, gpsToSave, isCoordName, tripKinds } from '../lib/logbook.js'

const props = defineProps({
  cid: { type: Number, required: true },
  vehicle: { type: Object, default: null },
  purposes: { type: Array, default: () => [] },
})
const visible = defineModel('visible', { type: Boolean, default: false })
const emit = defineEmits(['saved'])
const toast = useToast()

const file = ref(null)
const preview = ref(null)
const drafts = ref([])
const places = ref([])
const busy = ref(false)
const error = ref('')
const fieldErrors = ref({})
const naming = ref(null) // { point, name }

watch(visible, async (v) => {
  if (!v) return
  file.value = null
  preview.value = null
  drafts.value = []
  error.value = ''
  fieldErrors.value = {}
  try {
    places.value = await api.gpsPlaces(props.cid)
  } catch {
    places.value = []
  }
})

async function load() {
  if (!file.value) return
  busy.value = true
  error.value = ''
  try {
    preview.value = await api.gpsPreview(props.cid, props.vehicle.id, file.value)
    const old = new Map(drafts.value.map((d) => [d.date + d.depart, d]))
    // po pomenovaní miesta sa zachová to, čo už používateľ vyplnil
    drafts.value = chainOdometer(
      gpsDrafts(preview.value).map((d) => {
        const o = old.get(d.date + d.depart)
        return o ? { ...d, purpose: o.purpose, kind: o.kind, km: o.km, pick: o.pick && !d.duplicate } : d
      }),
    )
  } catch (e) {
    error.value = e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message
    preview.value = null
    drafts.value = []
  } finally {
    busy.value = false
  }
}

function pickFile(ev) {
  file.value = ev.target.files?.[0] ?? null
  drafts.value = []
  load()
}

const recalc = () => (drafts.value = chainOdometer(drafts.value))
const picked = computed(() => drafts.value.filter((d) => d.pick))
const pickedKm = computed(() => picked.value.reduce((s, d) => s + (Number(d.km) || 0), 0))
const purposeHits = ref([])
const suggest = (q) => {
  const s = q.toLowerCase()
  purposeHits.value = props.purposes.filter((x) => x.toLowerCase().includes(s)).slice(0, 10)
}

function startNaming(point) {
  naming.value = { point, name: '' }
}
async function saveName() {
  const n = naming.value
  if (!n?.name.trim()) return
  busy.value = true
  try {
    places.value = await api.gpsSavePlaces(props.cid, addPlace(places.value, n.point, n.name))
    naming.value = null
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Miesto sa neuložilo', detail: e.message, life: 5000 })
  } finally {
    busy.value = false
  }
}
async function removePlace(p) {
  places.value = await api.gpsSavePlaces(props.cid, places.value.filter((x) => x !== p))
  if (file.value) await load()
}

async function save() {
  busy.value = true
  fieldErrors.value = {}
  try {
    const saved = await api.gpsSave(props.cid, props.vehicle.id, gpsToSave(drafts.value))
    toast.add({ severity: 'success', summary: `Zapísané jazdy: ${saved.length}`, life: 3000 })
    visible.value = false
    emit('saved')
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      // pole „trips[1].purpose“ patrí k druhej vybranej jazde
      const keys = picked.value.map((d) => d.key)
      for (const f of e.fields) {
        const m = /^trips\[(\d+)\]\.(\w+)/.exec(f.field)
        if (m) fieldErrors.value[`${keys[+m[1]]}.${m[2]}`] = f.message
      }
      toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené jazdy', detail: e.fields[0].message, life: 5000 })
    } else toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
const err = (d, f) => fieldErrors.value[`${d.key}.${f}`]
</script>

<template>
  <Drawer v-model:visible="visible" position="right" class="gps-panel" header="Jazdy zo záznamu GPS">
    <div class="gps">
      <p class="lead">
        Nahrajte trasu z mobilu alebo z GPS jednotky v aute (GPX, alebo CSV so stĺpcami čas, šírka, dĺžka).
        Fiškus ju rozdelí na jazdy podľa zastávok dlhších ako 10 minút a navrhne odkiaľ, kam a koľko kilometrov.
      </p>
      <label class="file">
        <input type="file" accept=".gpx,.csv,.txt,application/gpx+xml,text/csv" @change="pickFile" />
        <span class="p-button p-button-sm p-button-outlined"><i class="pi pi-upload" /> {{ file ? 'Iný súbor' : 'Vybrať súbor' }}</span>
        <span v-if="file" class="fname">{{ file.name }}<template v-if="preview">, {{ preview.points }} bodov</template></span>
      </label>
      <p v-if="error" class="error">{{ error }}</p>

      <p v-if="preview && !drafts.length" class="muted">V zázname nie je žiadna jazda dlhšia ako 300 m.</p>

      <ol v-if="drafts.length" class="drafts">
        <li v-for="d in drafts" :key="d.key" :class="{ off: !d.pick, dup: d.duplicate }">
          <div class="row1">
            <Checkbox v-model="d.pick" binary :input-id="`gp${d.key}`" @update:model-value="recalc" />
            <label :for="`gp${d.key}`" class="when">{{ formatDate(d.date) }} <span class="time">{{ d.depart }}–{{ d.arrive }}</span></label>
            <span v-if="d.duplicate" class="tag">už v knihe</span>
            <span class="spacer" />
            <InputNumber v-model="d.km" :min="0" :max="5000" size="small" suffix=" km" class="km" :disabled="!d.pick" aria-label="Kilometre" @update:model-value="recalc" />
          </div>
          <div class="route">
            <template v-for="(end, i) in [{ name: d.from, point: d.fromPoint }, { name: d.to, point: d.toPoint }]" :key="i">
              <span v-if="i" class="arrow">→</span>
              <span v-if="!isCoordName(end.name)" class="place">{{ end.name }}</span>
              <button v-else type="button" class="coord" title="Pomenovať miesto" @click="startNaming(end.point)">
                <i class="pi pi-map-marker" /> {{ end.name }}
              </button>
            </template>
          </div>
          <template v-if="d.pick">
            <div class="row2">
              <SelectButton v-model="d.kind" :options="tripKinds" option-label="label" option-value="value" :allow-empty="false" size="small" />
              <AutoComplete v-model="d.purpose" :suggestions="purposeHits" :show-empty-message="false" size="small" fluid class="purpose" :invalid="!!err(d, 'purpose')"
                :placeholder="d.kind === 'private' ? 'Účel (nepovinné)' : 'Účel jazdy'" @complete="(e) => suggest(e.query)" />
            </div>
            <p v-if="err(d, 'purpose')" class="error small">{{ err(d, 'purpose') }}</p>
            <p v-if="d.odoStart" class="odo">Tachometer {{ d.odoStart }} – {{ d.odoEnd }} km</p>
          </template>
        </li>
      </ol>

      <form v-if="naming" class="naming" @submit.prevent="saveName">
        <span>Miesto {{ naming.point[0].toFixed(5) }}, {{ naming.point[1].toFixed(5) }}</span>
        <InputText v-model="naming.name" size="small" placeholder="napr. Sklad Zvolen" autofocus aria-label="Názov miesta" />
        <Button type="submit" label="Uložiť" size="small" :loading="busy" />
        <Button label="Zrušiť" text size="small" severity="secondary" @click="naming = null" />
      </form>

      <details v-if="places.length" class="places">
        <summary>Pomenované miesta ({{ places.length }})</summary>
        <ul>
          <li v-for="p in places" :key="p.name + p.lat">
            <span>{{ p.name }}</span>
            <small class="muted">{{ p.lat.toFixed(4) }}, {{ p.lon.toFixed(4) }}</small>
            <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="`Odstrániť ${p.name}`" @click="removePlace(p)" />
          </li>
        </ul>
      </details>

      <div v-if="drafts.length" class="actions">
        <span class="sum">Vybrané {{ picked.length }} z {{ drafts.length }}, spolu {{ pickedKm }} km</span>
        <span class="spacer" />
        <Button :label="`Zapísať jazdy (${picked.length})`" icon="pi pi-check" size="small" :disabled="!picked.length" :loading="busy" @click="save" />
      </div>
    </div>
  </Drawer>
</template>

<style scoped>
.gps {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.lead {
  margin: 0;
  color: var(--ink-soft, #4a5866);
  font-size: 0.9rem;
}
.file {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  flex-wrap: wrap;
  cursor: pointer;
}
.file input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
}
.file input:focus-visible + .p-button {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
.file .p-button {
  gap: 0.4rem;
}
.fname {
  font-size: 0.85rem;
  color: var(--ink-soft, #4a5866);
  overflow-wrap: anywhere;
}
.drafts {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--line, #d8dee5);
}
.drafts li {
  padding: 0.6rem 0;
  border-bottom: 1px solid var(--line, #d8dee5);
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.drafts li.off .route,
.drafts li.off .when {
  color: var(--ink-faint, #8a96a3);
}
.row1,
.row2 {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.row2 {
  flex-wrap: wrap;
}
.when {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}
.time {
  font-weight: 400;
  color: var(--ink-soft, #4a5866);
}
.tag {
  font-size: 0.75rem;
  padding: 0.05rem 0.4rem;
  border-radius: 3px;
  background: #eef1f4;
  color: #4a5866;
}
.spacer {
  flex: 1;
}
.km :deep(input) {
  width: 5.5rem;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.route {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.35rem;
  padding-left: 1.85rem;
}
.arrow {
  color: var(--ink-faint, #8a96a3);
}
.coord {
  border: 1px dashed #2f5d8c;
  background: none;
  color: #2f5d8c;
  border-radius: 3px;
  padding: 0.05rem 0.35rem;
  font: inherit;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}
.coord:hover {
  background: #eef3f8;
}
.row2 {
  padding-left: 1.85rem;
}
.purpose {
  flex: 1 1 12rem;
}
.odo,
.error.small {
  margin: 0 0 0 1.85rem;
  font-size: 0.8rem;
  font-variant-numeric: tabular-nums;
}
.odo {
  color: var(--ink-soft, #4a5866);
}
.naming {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem;
  background: #f4f7fa;
  border: 1px solid #d8dee5;
  border-radius: 4px;
  position: sticky;
  bottom: 3.2rem;
}
.naming span {
  width: 100%;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
}
.naming :deep(input) {
  flex: 1 1 10rem;
}
.places summary {
  cursor: pointer;
  font-size: 0.9rem;
  color: #2f5d8c;
}
.places ul {
  list-style: none;
  margin: 0.4rem 0 0;
  padding: 0;
}
.places li {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.places li span {
  flex: 1;
}
.actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  position: sticky;
  bottom: 0;
  background: var(--p-drawer-background, #fff);
  padding: 0.6rem 0;
  border-top: 1px solid var(--line, #d8dee5);
}
.sum {
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
}
.error {
  color: #b03a26;
  margin: 0;
}
.muted {
  color: var(--ink-faint, #8a96a3);
}
</style>

<style>
.p-drawer.p-component.gps-panel {
  width: min(600px, 100vw);
}
</style>
