<script setup>
// Objednávka dodávateľovi: dodávateľ, položky s nákupnou cenou, odoslanie,
// príjem na sklad (aj čiastočný) a súvisiace doklady. Sumy počíta server.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import FormField from '../components/FormField.vue'
import RelatedDocs from '../components/RelatedDocs.vue'
import SendMailDialog from '../components/SendMailDialog.vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { blankPorder, porderForm, porderLine, porderPayload, porderStatusOf, receiveDraft, receivePayload, remaining } from '../lib/porders.js'

const props = defineProps({ id: { type: String, default: '' } })
const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('purchases.write'))
const today = () => toISODate(new Date())

const order = ref(null) // zo servera (číslo, stav, sumy)
const form = ref(blankPorder(null, today()))
const contacts = ref([])
const items = ref([])
const warehouses = ref([])
const errors = ref({})
const busy = ref(false)
const saved = ref('')
const snapshot = () => JSON.stringify(porderPayload(form.value))
const dirty = computed(() => snapshot() !== saved.value)

async function load() {
  errors.value = {}
  ;[contacts.value, items.value, warehouses.value] = await Promise.all([
    api.contacts(session.companyId).catch(() => []),
    api.items(session.companyId).catch(() => []),
    api.stock(session.companyId).then((s) => s.warehouses ?? []).catch(() => []),
  ])
  if (props.id) {
    setOrder(await api.porder(session.companyId, props.id))
  } else {
    const ct = route.query.kontakt ? contacts.value.find((c) => String(c.id) === String(route.query.kontakt)) : null
    order.value = null
    form.value = blankPorder(ct, today())
    saved.value = snapshot()
  }
}
// e-mail dodávateľovi s PDF; rozpracovaná sa odoslaním označí ako odoslaná
const mailOpen = ref(false)
async function openMail() {
  if (dirty.value && !(await save())) return
  mailOpen.value = true
}
async function mailed() {
  setOrder(await api.porder(session.companyId, order.value.id))
}
function setOrder(o) {
  order.value = o
  form.value = porderForm(o)
  saved.value = snapshot()
}
onMounted(load)
watch(() => props.id, load)

const editable = computed(() => canWrite.value && (!order.value || ['draft', 'sent', 'partial'].includes(order.value.status)))
const status = computed(() => (order.value ? porderStatusOf(order.value, today()) : null))

async function save() {
  busy.value = true
  errors.value = {}
  try {
    const o = await api.savePorder(session.companyId, porderPayload(form.value), order.value?.id)
    const wasNew = !order.value
    setOrder(o)
    toast.add({ severity: 'success', summary: wasNew ? `Objednávka ${o.number} založená` : 'Objednávka uložená', life: 2500 })
    if (wasNew) router.replace(`/objednavky/${o.id}`)
    return true
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Objednávka sa neuložila', detail: e.message, life: 6000 })
    return false
  } finally {
    busy.value = false
  }
}

async function action(name, done) {
  if (dirty.value && !(await save())) return
  try {
    setOrder(await api.porderAction(session.companyId, order.value.id, name))
    toast.add({ severity: 'success', summary: done, life: 2500 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  }
}
const send = () => action('send', 'Objednávka je označená ako odoslaná')
function cancel() {
  confirm.require({
    header: 'Zrušiť objednávku',
    message: `Zrušiť objednávku ${order.value.number}? Čo ešte neprišlo, sa už nebude čakať.`,
    acceptProps: { label: 'Zrušiť objednávku', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: () => action('cancel', 'Objednávka zrušená'),
  })
}
function remove() {
  confirm.require({
    header: 'Zmazať objednávku',
    message: `Zmazať objednávku ${order.value.number}?`,
    acceptProps: { label: 'Zmazať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deletePorder(session.companyId, order.value.id)
        router.push('/objednavky')
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Objednávka sa nezmazala', detail: e.message, life: 6000 })
      }
    },
  })
}
async function print() {
  if (dirty.value && !(await save())) return
  router.push(`/objednavky/${order.value.id}/tlac`)
}

// riadky
const itemOptions = computed(() =>
  items.value.filter((it) => !it.archived).map((it) => ({ value: it.id, label: it.code ? `${it.code} · ${it.name}` : it.name })),
)
const addLine = () => form.value.lines.push(porderLine())
function setItem(i, id) {
  const it = items.value.find((x) => x.id === id) ?? null
  const old = form.value.lines[i]
  form.value.lines[i] = { ...porderLine(it), quantity: old.quantity, received: old.received ?? '0', remaining: old.remaining }
}
const err = (i, f) => errors.value[`lines[${i}].${f}`]
const locked = (l) => !editable.value || Number(l.received || 0) > 0
const contactOptions = computed(() => contacts.value.map((c) => ({ value: c.id, label: c.name + (c.ico ? ` (${c.ico})` : '') })))
function setContact(id) {
  form.value.contactId = id || 0
  const c = contacts.value.find((x) => x.id === id)
  if (c) form.value.supplier = c.name
}

// príjem
const receiving = ref(false)
const rec = ref({ date: '', warehouse: '', qty: [] })
function openReceive() {
  rec.value = { date: today(), warehouse: warehouses.value[0]?.code ?? '', qty: receiveDraft(order.value) }
  errors.value = {}
  receiving.value = true
}
async function receive() {
  if (dirty.value && !(await save())) return
  busy.value = true
  try {
    const r = await api.receivePorder(session.companyId, order.value.id, receivePayload(rec.value.date, rec.value.warehouse, rec.value.qty))
    setOrder(r.order)
    receiving.value = false
    toast.add({ severity: 'success', summary: 'Príjem zapísaný', detail: r.stockNumber ? `Príjemka ${r.stockNumber} na sklad` : 'Položky mimo skladu', life: 4000 })
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Príjem sa nezapísal', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
const recErr = (i) => errors.value[`lines[${i}].receive`]
const date = (k) => ({
  get: () => fromISODate(form.value[k]),
  set: (d) => (form.value[k] = toISODate(d) ?? ''),
})
</script>

<template>
  <div class="page">
    <header class="head">
      <Button icon="pi pi-arrow-left" text rounded size="small" aria-label="Späť na objednávky" @click="router.push('/objednavky')" />
      <h1>{{ order ? `Objednávka ${order.number}` : 'Nová objednávka' }}</h1>
      <span v-if="status" class="tag" :data-tone="status.tone"><span class="tag-text">{{ status.label }}</span></span>
      <span class="spacer" />
      <Button v-if="order" label="Tlač / PDF" icon="pi pi-print" text size="small" @click="print" />
      <Button v-if="canWrite && order && order.status !== 'cancelled'" label="E-mail" icon="pi pi-envelope" text size="small" @click="openMail" />
      <template v-if="canWrite && order">
        <Button v-if="order.status === 'draft'" label="Zmazať" severity="danger" text size="small" @click="remove" />
        <Button v-else-if="editable" label="Zrušiť" severity="danger" text size="small" @click="cancel" />
      </template>
      <Button v-if="editable" label="Uložiť" icon="pi pi-check" size="small" :severity="order ? 'secondary' : undefined" :outlined="!!order"
        :loading="busy" :disabled="order && !dirty" @click="save" />
      <Button v-if="canWrite && order?.status === 'draft'" label="Odoslaná" icon="pi pi-send" size="small" @click="send" />
      <Button v-if="canWrite && (order?.status === 'sent' || order?.status === 'partial')" label="Prijať" icon="pi pi-download" size="small" @click="openReceive" />
    </header>

    <form class="form-grid fields" @submit.prevent="save">
      <FormField label="Dodávateľ" :span="2" :error="errors.supplier || errors.contactId">
        <div class="supplier">
          <Select :model-value="form.contactId || null" :options="contactOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
            :disabled="!editable" placeholder="Z adresára…" @update:model-value="setContact" />
          <InputText v-if="!form.contactId" v-model="form.supplier" size="small" :disabled="!editable" placeholder="alebo názov dodávateľa" />
        </div>
      </FormField>
      <FormField label="Dátum objednávky" :error="errors.date">
        <DatePicker :model-value="date('date').get()" date-format="d. m. yy" size="small" fluid :disabled="!editable" @update:model-value="date('date').set($event)" />
      </FormField>
      <FormField label="Očakávané dodanie" :error="errors.expected" :hint="order?.sentDate ? `odoslaná ${formatDate(order.sentDate)}` : ''">
        <DatePicker :model-value="date('expected').get()" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="!editable" @update:model-value="date('expected').set($event)" />
      </FormField>
      <FormField label="Poznámka pre dodávateľa" :span="4">
        <Textarea v-model="form.note" rows="2" auto-resize :disabled="!editable" placeholder="Napr. dodať do dielne, kontakt pri prevzatí…" />
      </FormField>
    </form>

    <section class="lines" aria-label="Položky objednávky">
      <header class="ehead">
        <h2>Položky</h2>
        <span class="spacer" />
        <Button v-if="editable" label="Položka" icon="pi pi-plus" size="small" text @click="addLine" />
      </header>
      <p v-if="!form.lines.length" class="muted small">Pridajte položky z cenníka (doplní sa kód dodávateľa a posledná nákupná cena) alebo voľným textom.</p>
      <div v-else class="grid" role="table">
        <div class="row thead" role="row">
          <span>Položka</span><span>Kód dodávateľa</span><span class="r">Množstvo</span><span>MJ</span><span class="r">Cena €/MJ</span><span class="r">Spolu</span><span class="r">Prijaté</span><span />
        </div>
        <div v-for="(l, i) in form.lines" :key="i" class="row" role="row">
          <span class="cell wide item" data-label="Položka">
            <Select :model-value="l.itemId || null" :options="itemOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
              :disabled="locked(l)" placeholder="Z cenníka…" @update:model-value="setItem(i, $event)" />
            <InputText v-if="!l.itemId" v-model="l.name" size="small" :disabled="locked(l)" :invalid="!!err(i, 'name')" placeholder="alebo názov" />
          </span>
          <span class="cell" data-label="Kód dodávateľa"><InputText v-model="l.supplierCode" size="small" :disabled="!editable" /></span>
          <span class="cell" data-label="Množstvo"><InputText v-model="l.quantity" size="small" inputmode="decimal" class="r" :disabled="!editable" :invalid="!!err(i, 'quantity')" /></span>
          <span class="cell" data-label="MJ"><InputText v-model="l.unit" size="small" :disabled="!editable || !!l.itemId" /></span>
          <span class="cell" data-label="Cena €/MJ"><InputText v-model="l.unitPrice" size="small" inputmode="decimal" class="r" :disabled="!editable" :invalid="!!err(i, 'unitPrice')" /></span>
          <span class="cell sum" data-label="Spolu">{{ l.total && !dirty ? formatAmount(l.total) : '—' }}</span>
          <span class="cell r rec" data-label="Prijaté" :class="{ done: order && Number(l.received) && !remaining(l) }">{{ Number(l.received) ? formatAmount(l.received) : '' }}</span>
          <span class="cell act">
            <Button v-if="!locked(l)" icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Odstrániť riadok" @click="form.lines.splice(i, 1)" />
          </span>
          <small v-for="(m, k) in [err(i, 'name'), err(i, 'quantity'), err(i, 'unitPrice'), err(i, 'itemId')].filter(Boolean)" :key="k" class="rerr">{{ m }}</small>
        </div>
        <div v-if="order && !dirty" class="row tfoot" role="row">
          <span class="foot-label">Spolu bez DPH</span>
          <span class="cell sum total">{{ formatMoney(order.total) }}</span>
          <span />
        </div>
      </div>
      <small v-if="errors.lines" class="rerr">{{ errors.lines }}</small>
    </section>

    <RelatedDocs v-if="order" kind="porder" :id="Number(order.id)" :can-write="canWrite" />

    <Dialog v-model:visible="receiving" modal header="Príjem z objednávky" :style="{ width: '620px', maxWidth: '96vw' }">
      <div class="form-grid rec-head">
        <FormField label="Dátum príjmu">
          <DatePicker :model-value="fromISODate(rec.date)" date-format="d. m. yy" size="small" fluid @update:model-value="rec.date = toISODate($event) ?? ''" />
        </FormField>
        <FormField v-if="warehouses.length" label="Na sklad" hint="skladové položky dostanú príjemku">
          <Select v-model="rec.warehouse" :options="warehouses" option-label="name" option-value="code" size="small" fluid />
        </FormField>
      </div>
      <div class="rec-lines">
        <div v-for="(l, i) in order.lines" :key="i" class="rec-row">
          <span class="rec-name">{{ l.name }}<small class="muted"> objednané {{ formatAmount(l.quantity) }} {{ l.unit }}, prijaté {{ formatAmount(l.received) }}</small></span>
          <InputText v-model="rec.qty[i]" size="small" inputmode="decimal" class="r" :disabled="!remaining(l)" :invalid="!!recErr(i)" :aria-label="`Prijaté množstvo ${l.name}`" />
          <span class="muted">{{ l.unit }}</span>
          <small v-if="recErr(i)" class="rerr">{{ recErr(i) }}</small>
        </div>
      </div>
      <small v-if="errors.lines" class="rerr">{{ errors.lines }}</small>
      <template #footer>
        <Button label="Zavrieť" severity="secondary" text size="small" @click="receiving = false" />
        <Button label="Zapísať príjem" icon="pi pi-check" size="small" :loading="busy" @click="receive" />
      </template>
    </Dialog>
    <SendMailDialog v-if="order" v-model:visible="mailOpen" kind="purchase-orders" :id="order.id" @sent="mailed" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px 24px;
  max-width: 1100px;
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.spacer {
  flex: 1;
}
.supplier {
  display: flex;
  flex-direction: column;
  gap: 4px;
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
.lines {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.muted {
  color: var(--ink-soft);
}
.small {
  margin: 0;
  font-size: var(--fs-sm);
}
.grid {
  display: flex;
  flex-direction: column;
  font-variant-numeric: tabular-nums;
}
.row {
  display: grid;
  grid-template-columns: minmax(200px, 1fr) 120px 84px 56px 92px 96px 80px 40px;
  gap: 6px;
  align-items: center;
  padding: 3px 0;
  border-bottom: 1px solid var(--rule);
}
.row.tfoot {
  grid-template-columns: 1fr 140px 40px;
  border-bottom: 0;
}
.thead {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.cell {
  min-width: 0;
}
.cell :deep(input) {
  width: 100%;
}
.item {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.r,
.cell :deep(input.r),
.rec-row :deep(input.r) {
  text-align: right;
}
.sum {
  text-align: right;
  font-weight: 600;
}
.total {
  border-bottom: 3px double var(--ink);
}
.rec {
  color: var(--ink-soft);
}
.rec.done {
  color: var(--tone-paid);
  font-weight: 600;
}
.act {
  display: flex;
  justify-content: flex-end;
}
.rerr {
  grid-column: 1 / -1;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.foot-label {
  text-align: right;
  color: var(--ink-soft);
}
.rec-head {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin-bottom: 10px;
}
.rec-lines {
  display: flex;
  flex-direction: column;
}
.rec-row {
  display: grid;
  grid-template-columns: 1fr 100px 36px;
  gap: 8px;
  align-items: center;
  padding: 5px 0;
  border-bottom: 1px solid var(--rule);
}
.rec-name small {
  display: block;
  font-size: var(--fs-xs);
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 20px;
  }
  .rec-head {
    grid-template-columns: minmax(0, 1fr);
  }
  .thead {
    display: none;
  }
  .row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    padding: 8px 0;
  }
  .row.tfoot {
    grid-template-columns: 1fr auto;
  }
  .cell[data-label]::before {
    content: attr(data-label);
    display: block;
    font-size: var(--fs-xs);
    color: var(--ink-faint);
  }
  .cell.wide {
    grid-column: 1 / -1;
  }
  .sum,
  .rec {
    text-align: left;
  }
}
</style>
