<script setup>
// Tlačivá: vyplniť a vytlačiť (aj bez firmy). Vo firme sa vyplnené tlačivá
// ukladajú, robia sa z nich vzory, pokladničné doklady sa zapíšu do pokladne
// ako účtovný doklad a firma si môže zadať vlastné tlačivá.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import DatePicker from 'primevue/datepicker'
import Checkbox from 'primevue/checkbox'
import Drawer from 'primevue/drawer'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import FormSheet from '../components/FormSheet.vue'
import RelatedDocs from '../components/RelatedDocs.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatDate, fromISODate, toISODate } from '../lib/format.js'
import { formDocColumns } from '../lib/columns.js'
import { blankDef, blankItem, blankValues, fieldTypes, filterDocs, formMeta, fromTemplate, itemsForApi } from '../lib/forms.js'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const cid = computed(() => session.companyId || 0)
const canWrite = computed(() => !!cid.value && can('invoices.write'))
const canCash = computed(() => !!cid.value && can('cash.write'))

const catalog = ref({ forms: [], issuer: '', today: '' })
const docs = ref([])
const registers = ref([])
const tab = ref('new')
const tabs = computed(() =>
  cid.value
    ? [
        { value: 'new', label: 'Nové' },
        { value: 'saved', label: 'Uložené' },
        { value: 'templates', label: 'Vzory' },
      ]
    : [],
)
const loadError = ref('')

async function load() {
  try {
    const cat = await api.forms(cid.value)
    // uložené pred katalógom: otvorenie z adresy ?doc= ich potrebuje
    if (cid.value) docs.value = await api.formDocs(cid.value)
    catalog.value = cat
    if (cid.value) {
      if (can('cash.read')) registers.value = (await api.cashRegisters(cid.value).catch(() => [])).filter((r) => !r.archived)
    }
  } catch (e) {
    loadError.value = e.message
  }
}
onMounted(load)

// ---- vyplnenie ----
const editing = ref(null) // { form, id, title, template, cashId, cashNumber, values, items }
const preview = ref({ computed: null, errors: [] })
const errMap = computed(() => Object.fromEntries((preview.value.errors ?? []).map((e) => [e.field, e.message])))
const showErrors = ref(false)

function open(form, from = null) {
  showErrors.value = false
  if (from && !from.template) {
    editing.value = { form, id: from.id, title: from.title, template: false, cashId: from.cashId, cashNumber: from.cashNumber, values: { ...from.values }, items: (from.items ?? []).map((i) => ({ ...i })) }
  } else if (from) {
    const t = fromTemplate(from, form, { today: catalog.value.today })
    editing.value = { form, id: 0, title: '', template: false, values: t.values, items: t.items }
  } else {
    editing.value = { form, id: 0, title: '', template: false, values: blankValues(form, { issuer: catalog.value.issuer, today: catalog.value.today }), items: form.items ? [blankItem(), blankItem()] : [] }
  }
  router.replace({ query: from && !from.template ? { t: form.key, doc: from.id } : { t: form.key } })
}
function close() {
  editing.value = null
  router.replace({ query: {} })
}
const locked = computed(() => !!editing.value?.cashId)

let timer = null
let seq = 0
watch(
  () => editing.value && { k: editing.value.form.key, v: editing.value.values, i: editing.value.items },
  () => {
    clearTimeout(timer)
    if (!editing.value) return
    timer = setTimeout(async () => {
      const my = ++seq
      const e = editing.value
      try {
        const body = { formKey: e.form.key, values: e.values, items: itemsForApi(e.items) }
        const p = await api.formPreview(e.form.builtin ? 0 : cid.value, body)
        if (my === seq) preview.value = p
      } catch (err) {
        if (my === seq) preview.value = { computed: null, errors: [{ field: '', message: err.message }] }
      }
    }, 250)
  },
  { deep: true },
)

// dátum: v hodnotách ako ISO reťazec, v DatePicker ako Date
const dateModel = (key) => ({
  get: () => fromISODate(editing.value.values[key]),
  set: (d) => (editing.value.values[key] = toISODate(d) ?? ''),
})
const rateOptions = [{ value: '0', label: 'bez DPH' }, ...[23, 19, 5].map((r) => ({ value: String(r), label: `${r} %` }))]

// ---- tlač ----
const sheetBox = ref(null)
const scale = ref(1)
let ro = null
function fit() {
  const box = sheetBox.value
  const sheet = box?.querySelector('.sheet')
  if (!box || !sheet) return
  scale.value = Math.min(1, (box.clientWidth - 24) / sheet.offsetWidth)
  box.style.height = `${sheet.offsetHeight * scale.value + 24}px`
}
watch(
  () => editing.value?.form.key,
  async () => {
    await nextTick()
    ro?.disconnect()
    if (sheetBox.value) {
      ro = new ResizeObserver(fit)
      ro.observe(sheetBox.value)
      fit()
    }
  },
)
watch(preview, () => nextTick(fit))
onBeforeUnmount(() => ro?.disconnect())

let pageStyle = null
function print() {
  const f = editing.value.form
  const size = f.layout === 'cash' ? 'A5 landscape' : f.paper === 'A5' ? 'A5 portrait' : 'A4 portrait'
  pageStyle?.remove()
  pageStyle = document.createElement('style')
  pageStyle.textContent = `@page { size: ${size}; margin: 0 }`
  document.head.appendChild(pageStyle)
  window.print()
}

// ---- uloženie a pokladňa ----
const busy = ref(false)
function fail(e, summary) {
  const detail = e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message
  toast.add({ severity: 'error', summary, detail, life: 6000 })
}
async function save(asTemplate = false) {
  const e = editing.value
  busy.value = true
  try {
    const body = { id: asTemplate ? 0 : e.id, formKey: e.form.key, title: asTemplate ? `Vzor – ${e.title || e.form.name}` : e.title, template: asTemplate, values: e.values, items: itemsForApi(e.items) }
    const saved = await api.saveFormDoc(cid.value, body)
    if (!asTemplate) {
      e.id = saved.id
      e.title = saved.title
      router.replace({ query: { t: e.form.key, doc: saved.id } })
    }
    toast.add({ severity: 'success', summary: asTemplate ? 'Vzor uložený' : 'Tlačivo uložené', detail: saved.title, life: 2500 })
    docs.value = await api.formDocs(cid.value)
    return saved
  } catch (err) {
    fail(err, 'Uloženie zlyhalo')
    return null
  } finally {
    busy.value = false
  }
}
const registerId = ref(0)
const registerOptions = computed(() => registers.value.map((r) => ({ value: r.id, label: r.name + (r.currency && r.currency !== 'EUR' ? ` (${r.currency})` : '') })))
async function postCash() {
  showErrors.value = true
  if (preview.value.errors?.length) {
    toast.add({ severity: 'warn', summary: 'Doplňte tlačivo', detail: preview.value.errors[0].message, life: 4000 })
    return
  }
  const saved = await save(false)
  if (!saved) return
  busy.value = true
  try {
    const posted = await api.postFormDoc(cid.value, saved.id, registerId.value)
    Object.assign(editing.value, { cashId: posted.cashId, cashNumber: posted.cashNumber, values: posted.values, title: posted.title })
    toast.add({ severity: 'success', summary: `Zapísané do pokladne ako ${posted.cashNumber}`, life: 4000 })
    docs.value = await api.formDocs(cid.value)
  } catch (err) {
    fail(err, 'Zápis do pokladne zlyhal')
  } finally {
    busy.value = false
  }
}

// ---- uložené a vzory ----
const q = ref('')
const savedList = computed(() => filterDocs(docs.value, catalog.value.forms, q.value, tab.value === 'templates'))
const formOf = (d) => catalog.value.forms.find((f) => f.key === d.formKey)
const exportCols = computed(() => formDocColumns(catalog.value.forms))
function openDoc(d) {
  const f = formOf(d)
  if (f) open(f, d)
}
function editTemplate(d) {
  const f = formOf(d)
  if (!f) return
  open(f)
  Object.assign(editing.value, { id: d.id, title: d.title, template: true, values: { ...d.values }, items: (d.items ?? []).map((i) => ({ ...i })) })
}
async function saveTemplateEdit() {
  const e = editing.value
  busy.value = true
  try {
    await api.saveFormDoc(cid.value, { id: e.id, formKey: e.form.key, title: e.title, template: true, values: e.values, items: itemsForApi(e.items) })
    toast.add({ severity: 'success', summary: 'Vzor uložený', life: 2500 })
    docs.value = await api.formDocs(cid.value)
  } catch (err) {
    fail(err, 'Uloženie zlyhalo')
  } finally {
    busy.value = false
  }
}
function removeDoc(d) {
  confirm.require({
    message: `Zmazať ${d.template ? 'vzor' : 'tlačivo'} „${d.title}“?`,
    header: 'Zmazať',
    acceptLabel: 'Zmazať',
    rejectLabel: 'Zrušiť',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await api.deleteFormDoc(cid.value, d.id)
        docs.value = await api.formDocs(cid.value)
      } catch (e) {
        fail(e, 'Zmazanie zlyhalo')
      }
    },
  })
}

// ---- vlastné tlačivo ----
const def = ref(null)
const defErrors = ref({})
const defOpen = computed({ get: () => !!def.value, set: (v) => !v && (def.value = null) })
function openDef(f) {
  defErrors.value = {}
  def.value = f ? JSON.parse(JSON.stringify(f)) : blankDef()
}
function moveField(i, d) {
  const list = def.value.fields
  const j = i + d
  if (j < 0 || j >= list.length) return
  ;[list[i], list[j]] = [list[j], list[i]]
}
async function saveDef() {
  busy.value = true
  try {
    const saved = await api.saveFormDef(cid.value, def.value)
    toast.add({ severity: 'success', summary: 'Tlačivo uložené', detail: saved.name, life: 2500 })
    def.value = null
    catalog.value = await api.forms(cid.value)
  } catch (e) {
    defErrors.value = e instanceof ApiError ? e.fieldMap() : {}
    fail(e, 'Uloženie zlyhalo')
  } finally {
    busy.value = false
  }
}
function removeDef() {
  const d = def.value
  confirm.require({
    message: `Zmazať tlačivo „${d.name}“?`,
    header: 'Zmazať',
    acceptLabel: 'Zmazať',
    rejectLabel: 'Zrušiť',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await api.deleteFormDef(cid.value, d.key)
        def.value = null
        catalog.value = await api.forms(cid.value)
      } catch (e) {
        fail(e, 'Zmazanie zlyhalo')
      }
    },
  })
}

// otvorenie z adresy ?t=ppd
watch(
  () => catalog.value.forms.length,
  () => {
    const f = catalog.value.forms.find((x) => x.key === route.query.t)
    if (!f || editing.value) return
    // uložené tlačivo z adresy ?t=…&doc=12 (odkaz zo súvisiacich dokladov)
    const d = route.query.doc && docs.value.find((x) => String(x.id) === String(route.query.doc))
    open(f, d || null)
  },
)
const builtinForms = computed(() => catalog.value.forms.filter((f) => f.builtin))
const ownForms = computed(() => catalog.value.forms.filter((f) => !f.builtin))
</script>

<template>
  <div class="page forms-page">
    <!-- výber tlačiva a zoznamy -->
    <template v-if="!editing">
      <header class="head">
        <h1>Tlačivá</h1>
        <SelectButton v-if="tabs.length" v-model="tab" :options="tabs" option-label="label" option-value="value" :allow-empty="false" size="small" />
        <span class="spacer" />
        <Button v-if="canWrite && tab === 'new'" label="Vlastné tlačivo" icon="pi pi-plus" size="small" outlined @click="openDef()" />
      </header>
      <p v-if="loadError" class="error">{{ loadError }}</p>

      <template v-if="tab === 'new'">
        <p class="lead">
          Vyberte tlačivo, vyplňte ho a vytlačte alebo uložte ako PDF.
          <template v-if="cid">Vyplnené tlačivo môžete uložiť, urobiť z neho vzor a pokladničný doklad zapísať do pokladne ako účtovný doklad.</template>
          <template v-else>Ukladanie, vzory a zápis do pokladne sú vo firme.</template>
        </p>
        <ul class="cards">
          <li v-for="f in builtinForms" :key="f.key">
            <button type="button" class="card" @click="open(f)">
              <span class="paper" :class="f.layout === 'cash' ? 'land' : f.paper.toLowerCase()" aria-hidden="true" />
              <span class="txt"><b>{{ f.name }}</b><small>{{ f.description }}</small><small class="meta">{{ formMeta(f) }}</small></span>
            </button>
          </li>
        </ul>
        <template v-if="ownForms.length">
          <h2>Vlastné tlačivá</h2>
          <ul class="cards">
            <li v-for="f in ownForms" :key="f.key" class="own">
              <button type="button" class="card" @click="open(f)">
                <span class="paper" :class="f.paper.toLowerCase()" aria-hidden="true" />
                <span class="txt"><b>{{ f.name }}</b><small>{{ f.description || `${f.fields.length} polí` }}</small><small class="meta">{{ formMeta(f) }}</small></span>
              </button>
              <Button v-if="canWrite" icon="pi pi-pencil" text rounded size="small" class="edit-btn" :aria-label="`Upraviť ${f.name}`" @click="openDef(f)" />
            </li>
          </ul>
        </template>
      </template>

      <template v-else>
        <div class="tbar">
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="q" size="small" placeholder="Hľadať názov, odberateľa, číslo, položku" aria-label="Hľadať" />
          </IconField>
          <ExportButton :name="tab === 'templates' ? 'vzory-tlaciv' : 'tlaciva'" :columns="exportCols" :rows="savedList" />
        </div>
        <p v-if="!savedList.length" class="muted">
          {{ q ? 'Nič sa nenašlo.' : tab === 'templates' ? 'Zatiaľ žiadne vzory. Vyplňte tlačivo a dajte Uložiť ako vzor.' : 'Zatiaľ žiadne uložené tlačivá.' }}
        </p>
        <ul v-else class="docs">
          <li v-for="d in savedList" :key="d.id">
            <button type="button" class="doc-row" @click="openDoc(d)">
              <b>{{ d.title }}</b>
              <small>{{ formOf(d)?.name ?? d.formKey }}, {{ formatDate(d.updated) }}</small>
            </button>
            <span v-if="d.cashNumber" class="tag" title="Zapísané v pokladni">{{ d.cashNumber }}</span>
            <Button v-if="tab === 'templates'" label="Použiť" size="small" text @click="openDoc(d)" />
            <Button v-if="tab === 'templates' && canWrite" icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="`Upraviť vzor ${d.title}`" @click="editTemplate(d)" />
            <Button v-if="canWrite && !d.cashId" icon="pi pi-trash" text rounded size="small" severity="secondary" :aria-label="`Zmazať ${d.title}`" @click="removeDoc(d)" />
          </li>
        </ul>
      </template>
    </template>

    <!-- vyplnenie -->
    <template v-else>
      <header class="head">
        <Button icon="pi pi-arrow-left" text rounded size="small" aria-label="Späť na tlačivá" @click="close" />
        <h1>{{ editing.template ? 'Vzor: ' : '' }}{{ editing.form.name }}</h1>
        <span v-if="editing.cashNumber" class="tag">v pokladni {{ editing.cashNumber }}</span>
        <span class="spacer" />
        <Button label="Tlačiť" icon="pi pi-print" size="small" @click="print" />
      </header>

      <div class="edit">
        <form class="fill" @submit.prevent>
          <p v-if="locked" class="note">Tlačivo je zapísané v pokladni ako <RouterLink to="/pokladna">{{ editing.cashNumber }}</RouterLink>. Opravuje sa v Pokladni stornom a novým dokladom; vytlačiť ho môžete kedykoľvek.</p>
          <div class="form-grid">
            <FormField v-if="cid && (editing.id || editing.template)" label="Názov v zozname" :span="2">
              <InputText v-model="editing.title" size="small" :disabled="locked" />
            </FormField>
            <FormField v-for="f in editing.form.fields" :key="f.key" :label="f.label + (f.required ? ' *' : '')" :span="f.wide || f.type === 'textarea' ? 2 : 1"
              :error="showErrors ? errMap[f.key] : ''" :hint="f.hint">
              <Textarea v-if="f.type === 'textarea'" v-model="editing.values[f.key]" rows="3" auto-resize :disabled="locked" />
              <DatePicker v-else-if="f.type === 'date'" :model-value="dateModel(f.key).get()" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="locked"
                @update:model-value="dateModel(f.key).set($event)" />
              <Select v-else-if="f.type === 'rate'" v-model="editing.values[f.key]" :options="rateOptions" option-label="label" option-value="value" size="small" :disabled="locked" />
              <InputText v-else v-model="editing.values[f.key]" size="small" :inputmode="f.type === 'money' || f.type === 'number' ? 'decimal' : undefined" :disabled="locked" />
            </FormField>
          </div>

          <section v-if="editing.form.items" class="items-edit">
            <h2>Položky</h2>
            <div v-for="(it, i) in editing.items" :key="i" class="item">
              <InputText v-model="it.desc" size="small" placeholder="Popis" class="desc" :aria-label="`Popis položky ${i + 1}`" :invalid="showErrors && !!errMap[`items[${i}].desc`]" />
              <InputText v-model="it.qty" size="small" inputmode="decimal" class="qty" :aria-label="`Množstvo položky ${i + 1}`" />
              <InputText v-model="it.unit" size="small" class="unit" :aria-label="`Jednotka položky ${i + 1}`" />
              <InputText v-model="it.price" size="small" inputmode="decimal" placeholder="Cena" class="price" :aria-label="`Cena položky ${i + 1}`" />
              <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="`Odobrať položku ${i + 1}`" @click="editing.items.splice(i, 1)" />
            </div>
            <Button label="Položka" icon="pi pi-plus" text size="small" @click="editing.items.push(blankItem())" />
          </section>

          <div class="actions">
            <template v-if="editing.template">
              <Button label="Uložiť vzor" icon="pi pi-check" size="small" :loading="busy" @click="saveTemplateEdit" />
            </template>
            <template v-else-if="canWrite && !locked">
              <Button label="Uložiť" icon="pi pi-save" size="small" outlined :loading="busy" @click="save(false)" />
              <Button label="Uložiť ako vzor" icon="pi pi-clone" size="small" text :loading="busy" @click="save(true)" />
            </template>
            <template v-if="editing.form.cash && canCash && !locked && !editing.template">
              <span class="spacer" />
              <Select v-if="registerOptions.length > 1" v-model="registerId" :options="registerOptions" option-label="label" option-value="value" size="small" aria-label="Pokladňa" />
              <Button label="Zapísať do pokladne" icon="pi pi-book" size="small" :loading="busy" @click="postCash" />
            </template>
          </div>
          <p v-if="!cid" class="muted small">Bez firmy sa tlačivo neukladá — vytlačte ho alebo uložte ako PDF v dialógu tlače.</p>
          <RelatedDocs v-if="cid && editing.id && !editing.template" kind="form" :id="Number(editing.id)" :can-write="canWrite" />
        </form>

        <div class="preview">
          <div ref="sheetBox" class="sheet-box">
            <div class="form-print" :style="{ transform: `scale(${scale})` }">
              <FormSheet :form="editing.form" :values="editing.values" :items="editing.items" :calc="preview.computed" />
            </div>
          </div>
        </div>
        <!-- na tlač: hárok bez zmenšenia, mimo rozloženia aplikácie -->
        <Teleport to="body">
          <div class="form-print-root">
            <FormSheet :form="editing.form" :values="editing.values" :items="editing.items" :calc="preview.computed" />
          </div>
        </Teleport>
      </div>
    </template>

    <!-- vlastné tlačivo -->
    <Drawer v-model:visible="defOpen" position="right" class="def-panel" :header="def?.key ? 'Upraviť tlačivo' : 'Vlastné tlačivo'">
      <form v-if="def" class="def" @submit.prevent="saveDef">
        <div class="form-grid">
          <FormField label="Názov" :span="2" :error="defErrors.name">
            <InputText v-model="def.name" size="small" placeholder="napr. Preberací protokol" />
          </FormField>
          <FormField label="Popis" :span="2">
            <InputText v-model="def.description" size="small" />
          </FormField>
          <FormField label="Papier">
            <SelectButton v-model="def.paper" :options="['A4', 'A5']" :allow-empty="false" size="small" />
          </FormField>
          <FormField label="Tabuľka položiek">
            <label class="chk"><Checkbox v-model="def.items" binary /> s položkami</label>
          </FormField>
        </div>
        <h3>Polia</h3>
        <p v-if="defErrors.fields" class="error">{{ defErrors.fields }}</p>
        <div v-for="(f, i) in def.fields" :key="i" class="def-field">
          <InputText v-model="f.label" size="small" placeholder="Názov poľa" :invalid="!!defErrors[`fields[${i}].label`]" :aria-label="`Názov poľa ${i + 1}`" class="lbl" />
          <Select v-model="f.type" :options="fieldTypes" option-label="label" option-value="value" size="small" :aria-label="`Druh poľa ${i + 1}`" class="typ" />
          <label class="chk"><Checkbox v-model="f.required" binary /> povinné</label>
          <label class="chk"><Checkbox v-model="f.wide" binary /> celá šírka</label>
          <span class="moves">
            <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :aria-label="`Posunúť pole ${i + 1} vyššie`" :disabled="i === 0" @click="moveField(i, -1)" />
            <Button icon="pi pi-arrow-down" text rounded size="small" severity="secondary" :aria-label="`Posunúť pole ${i + 1} nižšie`" :disabled="i === def.fields.length - 1" @click="moveField(i, 1)" />
            <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="`Odobrať pole ${i + 1}`" @click="def.fields.splice(i, 1)" />
          </span>
        </div>
        <Button label="Pole" icon="pi pi-plus" text size="small" @click="def.fields.push({ key: '', label: '', type: 'text', required: false, wide: false })" />
        <div class="actions">
          <Button v-if="def.key" label="Zmazať" severity="danger" text size="small" @click="removeDef" />
          <span class="spacer" />
          <Button type="submit" label="Uložiť tlačivo" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Drawer>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
}
.head {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  flex-wrap: wrap;
  margin-bottom: 0.75rem;
}
.head h1 {
  margin: 0;
}
.spacer {
  flex: 1;
}
.lead {
  margin: 0 0 1rem;
  color: var(--ink-soft, #4a5866);
  max-width: 70ch;
}
h2 {
  font-size: 1rem;
  margin: 1.25rem 0 0.5rem;
}
.cards {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
  gap: 0.75rem;
}
.cards li {
  position: relative;
}
.card {
  width: 100%;
  height: 100%;
  display: flex;
  gap: 0.9rem;
  align-items: flex-start;
  text-align: left;
  font: inherit;
  color: inherit;
  background: #fff;
  border: 1px solid var(--line, #d8dee5);
  border-radius: 4px;
  padding: 0.8rem;
  cursor: pointer;
}
.card:hover {
  border-color: #2f5d8c;
}
.card:focus-visible {
  outline: 2px solid #2f5d8c;
  outline-offset: 1px;
}
.paper {
  flex: none;
  background: #f4f6f8;
  border: 1px solid #b9c3cf;
  box-shadow: 2px 2px 0 #e3e8ee;
  margin-top: 2px;
  background-image: repeating-linear-gradient(#f4f6f8 0 5px, #dfe5ec 5px 6px);
  background-size: 70% 100%;
  background-position: 50% 6px;
  background-repeat: no-repeat;
}
.card .paper.a4 {
  width: 30px;
  height: 42px;
}
.card .paper.a5 {
  width: 25px;
  height: 35px;
}
.card .paper.land {
  width: 40px;
  height: 28px;
}
.txt {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}
.txt b {
  color: #1f3a5c;
}
.txt small {
  color: var(--ink-soft, #4a5866);
}
.txt .meta {
  color: var(--ink-faint, #8a96a3);
}
.own .edit-btn {
  position: absolute;
  top: 0.3rem;
  right: 0.3rem;
}
.tbar {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
  flex-wrap: wrap;
}
.search {
  flex: 1 1 16rem;
  max-width: 28rem;
}
.search :deep(input) {
  width: 100%;
}
.docs {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--line, #d8dee5);
  max-width: 60rem;
}
.docs li {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  border-bottom: 1px solid var(--line, #d8dee5);
}
.doc-row {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  text-align: left;
  font: inherit;
  color: inherit;
  background: none;
  border: 0;
  padding: 0.55rem 0.3rem;
  cursor: pointer;
}
.doc-row b {
  color: #1f3a5c;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.doc-row small {
  color: var(--ink-faint, #8a96a3);
}
.doc-row:hover b {
  text-decoration: underline;
}
.tag {
  font-size: 0.78rem;
  padding: 0.1rem 0.45rem;
  border-radius: 3px;
  background: #eef3f8;
  color: #1f3a5c;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.edit {
  display: grid;
  grid-template-columns: minmax(20rem, 30rem) minmax(0, 1fr);
  gap: 1.5rem;
  align-items: start;
}
.fill .form-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.note {
  background: #f4f7fa;
  border-left: 3px solid #2f5d8c;
  padding: 0.5rem 0.7rem;
  margin: 0 0 0.75rem;
  font-size: 0.9rem;
}
.items-edit h2 {
  margin-top: 1rem;
}
.item {
  display: flex;
  gap: 0.3rem;
  align-items: center;
  margin-bottom: 0.3rem;
}
.item .desc {
  flex: 1 1 8rem;
  min-width: 0;
}
.item .qty {
  width: 4rem;
  text-align: right;
}
.item .unit {
  width: 3.2rem;
}
.item .price {
  width: 5.5rem;
  text-align: right;
}
.actions {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  flex-wrap: wrap;
  margin-top: 1rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--line, #d8dee5);
}
.edit > * {
  min-width: 0;
}
.preview {
  position: sticky;
  top: 0.5rem;
}
.sheet-box {
  background: #e9ecf0;
  border-radius: 4px;
  padding: 12px;
  overflow: hidden;
}
.form-print {
  transform-origin: top left;
  width: max-content;
  box-shadow: 0 1px 3px rgba(18, 24, 32, 0.12), 0 8px 28px rgba(18, 24, 32, 0.08);
}
.error {
  color: #b03a26;
}
.muted {
  color: var(--ink-faint, #8a96a3);
}
.small {
  font-size: 0.85rem;
}
.def h3 {
  font-size: 0.95rem;
  margin: 1rem 0 0.4rem;
}
.def-field {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.35rem 0.6rem;
  padding: 0.4rem 0;
  border-bottom: 1px solid #edf0f3;
}
.def-field .lbl {
  flex: 1 1 12rem;
}
.def-field .typ {
  width: 9rem;
}
.chk {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.9rem;
}
.moves {
  margin-left: auto;
}
@media (max-width: 1100px) {
  .edit {
    grid-template-columns: 1fr;
  }
  .preview {
    position: static;
  }
}
@media (max-width: 640px) {
  .fill .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .fill .form-grid > :deep(*) {
    grid-column: auto;
  }
}
</style>

<style>
.p-drawer.p-component.def-panel {
  width: min(560px, 100vw);
}
/* tlač: iba hárok tlačiva v skutočnej veľkosti */
.form-print-root {
  display: none;
}
@media print {
  body > *:not(.form-print-root) {
    display: none !important;
  }
  .form-print-root {
    display: block;
  }
}
</style>
