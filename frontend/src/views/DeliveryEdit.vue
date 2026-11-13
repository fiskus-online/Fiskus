<script setup>
// Dodací list: odberateľ, miesto dodania, položky s cenami na neskoršiu
// faktúru. Vydanie pridelí číslo a vydá tovar zo skladu. Sumy počíta server.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import DatePicker from 'primevue/datepicker'
import FormField from '../components/FormField.vue'
import RelatedDocs from '../components/RelatedDocs.vue'
import SendMailDialog from '../components/SendMailDialog.vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { ratesOn } from '../lib/codes.js'
import { blankDelivery, deliveryForm, deliveryLine, deliveryPayload, deliveryStatusOf, partyOf } from '../lib/deliveries.js'

const props = defineProps({ id: { type: String, default: '' } })
const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('invoices.write'))
const nonVat = computed(() => !session.company?.vatPayer)
const today = () => toISODate(new Date())

const note = ref(null) // zo servera (číslo, stav, sumy)
const form = ref(blankDelivery(null, today()))
const contacts = ref([])
const items = ref([])
const errors = ref({})
const busy = ref(false)
const saved = ref('')
const snapshot = () => JSON.stringify(deliveryPayload(form.value))
const dirty = computed(() => snapshot() !== saved.value)

async function load() {
  errors.value = {}
  ;[contacts.value, items.value] = await Promise.all([
    api.contacts(session.companyId).catch(() => []),
    api.items(session.companyId).catch(() => []),
  ])
  if (props.id) {
    setNote(await api.delivery(session.companyId, props.id))
  } else {
    const ct = route.query.kontakt ? contacts.value.find((c) => String(c.id) === String(route.query.kontakt)) : null
    note.value = null
    form.value = blankDelivery(ct, today())
    saved.value = snapshot()
  }
}
function setNote(n) {
  note.value = n
  form.value = deliveryForm(n)
  saved.value = snapshot()
}
onMounted(load)
watch(() => props.id, load)

const editable = computed(() => canWrite.value && (!note.value || note.value.status === 'draft' || note.value.status === 'issued'))
const status = computed(() => (note.value ? deliveryStatusOf(note.value) : null))
const title = computed(() => (note.value?.number ? `Dodací list ${note.value.number}` : note.value ? 'Koncept dodacieho listu' : 'Nový dodací list'))

async function save() {
  busy.value = true
  errors.value = {}
  try {
    const wasNew = !note.value
    const n = await api.saveDelivery(session.companyId, deliveryPayload(form.value), note.value?.id)
    setNote(n)
    if (wasNew && route.query.zakazka) {
      await api.linkDoc(session.companyId, 'delivery', n.id, { kind: 'job', id: Number(route.query.zakazka) }).catch(() => {})
    }
    toast.add({ severity: 'success', summary: wasNew ? 'Koncept dodacieho listu uložený' : 'Dodací list uložený', life: 2500 })
    if (wasNew) router.replace(`/dodacie-listy/${n.id}`)
    return true
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Dodací list sa neuložil', detail: e.message, life: 6000 })
    return false
  } finally {
    busy.value = false
  }
}

async function action(name, done) {
  if ((dirty.value || !note.value) && !(await save())) return
  busy.value = true
  try {
    setNote(await api.deliveryAction(session.companyId, note.value.id, name))
    toast.add({ severity: 'success', summary: done(note.value), life: 3000 })
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
const issue = () => action('issue', (n) => `Dodací list ${n.number} vydaný`)
function cancel() {
  confirm.require({
    header: 'Stornovať dodací list',
    message: `Stornovať dodací list ${note.value.number}? Tovar zo skladu sa vráti na sklad.`,
    acceptProps: { label: 'Stornovať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: () => action('cancel', () => 'Dodací list stornovaný'),
  })
}
function remove() {
  confirm.require({
    header: 'Zmazať koncept',
    message: 'Zmazať koncept dodacieho listu?',
    acceptProps: { label: 'Zmazať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deleteDelivery(session.companyId, note.value.id)
        router.push('/dodacie-listy')
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Koncept sa nezmazal', detail: e.message, life: 6000 })
      }
    },
  })
}
async function invoice() {
  if (dirty.value && !(await save())) return
  busy.value = true
  try {
    const inv = await api.invoiceDeliveries(session.companyId, [note.value.id])
    toast.add({ severity: 'success', summary: 'Koncept faktúry pripravený', detail: 'Skontrolujte ho a vystavte.', life: 3000 })
    router.push(`/faktury/${inv.id}`)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Faktúra sa nevytvorila', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
const mailOpen = ref(false)
async function openMail() {
  if (dirty.value && !(await save())) return
  mailOpen.value = true
}
async function print() {
  if ((dirty.value || !note.value) && !(await save())) return
  router.push(`/dodacie-listy/${note.value.id}/tlac`)
}

// odberateľ
const contactOptions = computed(() => contacts.value.map((c) => ({ value: c.id, label: c.name + (c.ico ? ` (${c.ico})` : '') })))
function setContact(id) {
  form.value.contactId = id || 0
  const c = contacts.value.find((x) => x.id === id)
  form.value.customer = c ? partyOf(c) : { name: '', country: 'SK' }
}

// riadky
const itemOptions = computed(() =>
  items.value.filter((it) => !it.archived).map((it) => ({ value: it.id, label: it.code ? `${it.code} · ${it.name}` : it.name })),
)
const rates = computed(() => ratesOn(form.value.date || today()))
const lineOpts = () => ({ priceMode: form.value.priceMode, nonVat: nonVat.value, rate: rates.value[0] ?? 23 })
const addLine = () => form.value.lines.push(deliveryLine(null, lineOpts()))
function setItem(i, id) {
  const it = items.value.find((x) => x.id === id) ?? null
  const old = form.value.lines[i]
  form.value.lines[i] = { ...deliveryLine(it, lineOpts()), quantity: old.quantity || '1' }
}
const err = (i, f) => errors.value[`lines[${i}].${f}`]
const priceModes = [{ value: 'net', label: 'Ceny bez DPH' }, { value: 'gross', label: 'Ceny s DPH' }]
const date = (k) => ({
  get: () => fromISODate(form.value[k]),
  set: (d) => (form.value[k] = toISODate(d) ?? ''),
})
const fresh = computed(() => note.value && !dirty.value)
</script>

<template>
  <div class="page">
    <header class="head">
      <Button icon="pi pi-arrow-left" text rounded size="small" aria-label="Späť na dodacie listy" @click="router.push('/dodacie-listy')" />
      <h1>{{ title }}</h1>
      <span v-if="status" class="tag" :data-tone="status.tone"><span class="tag-text">{{ status.label }}</span></span>
      <RouterLink v-if="note?.invoiceId" :to="`/faktury/${note.invoiceId}`" class="inv">faktúra {{ note.invoiceNumber }}</RouterLink>
      <span class="spacer" />
      <Button v-if="note" label="Tlač / PDF" icon="pi pi-print" text size="small" @click="print" />
      <Button v-if="canWrite && (note?.status === 'issued' || note?.status === 'invoiced')" label="E-mail" icon="pi pi-envelope" text size="small" @click="openMail" />
      <template v-if="canWrite && note">
        <Button v-if="note.status === 'draft'" label="Zmazať" severity="danger" text size="small" @click="remove" />
        <Button v-else-if="note.status === 'issued'" label="Stornovať" severity="danger" text size="small" @click="cancel" />
      </template>
      <Button v-if="editable" label="Uložiť" icon="pi pi-check" size="small" severity="secondary" outlined :loading="busy" :disabled="note && !dirty" @click="save" />
      <Button v-if="canWrite && (!note || note.status === 'draft')" label="Vydať" icon="pi pi-check-circle" size="small" :disabled="busy" @click="issue" />
      <Button v-if="canWrite && note?.status === 'issued'" label="Vyfakturovať" icon="pi pi-file" size="small" :disabled="busy" @click="invoice" />
    </header>

    <form class="form-grid fields" @submit.prevent="save">
      <FormField label="Odberateľ" :span="2" :error="errors['customer.name'] || errors.contactId">
        <div class="customer">
          <Select :model-value="form.contactId || null" :options="contactOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
            :disabled="!editable" placeholder="Z adresára…" @update:model-value="setContact" />
          <InputText v-if="!form.contactId" v-model="form.customer.name" size="small" :disabled="!editable" placeholder="alebo názov odberateľa" />
        </div>
      </FormField>
      <FormField label="Dátum dodania" :error="errors.date">
        <DatePicker :model-value="date('date').get()" date-format="d. m. yy" size="small" fluid :disabled="!editable" @update:model-value="date('date').set($event)" />
      </FormField>
      <FormField label="Objednávka odberateľa" hint="číslo, ak ste ju dostali">
        <InputText v-model="form.order" size="small" :disabled="!editable" />
      </FormField>
      <FormField label="Miesto dodania" :span="2">
        <InputText v-model="form.place" size="small" :disabled="!editable" placeholder="Napr. prevádzka, stavba, adresa" />
      </FormField>
      <FormField label="Prevzal" hint="meno toho, kto tovar prevzal">
        <InputText v-model="form.receiver" size="small" :disabled="!editable" />
      </FormField>
      <FormField v-if="!nonVat" label="Ceny">
        <SelectButton v-model="form.priceMode" :options="priceModes" option-label="label" option-value="value" :allow-empty="false" size="small" :disabled="!editable" />
      </FormField>
      <FormField label="Poznámka" :span="4">
        <Textarea v-model="form.note" rows="2" auto-resize :disabled="!editable" />
      </FormField>
    </form>

    <section class="lines" aria-label="Položky dodacieho listu">
      <header class="ehead">
        <h2>Položky</h2>
        <span class="spacer" />
        <Button v-if="editable" label="Položka" icon="pi pi-plus" size="small" text @click="addLine" />
      </header>
      <p v-if="!form.lines.length" class="muted small">Položka z cenníka vedená na sklade sa pri vydaní dodacieho listu vydá zo skladu.</p>
      <div v-else class="grid" :class="{ novat: nonVat }" role="table">
        <div class="row thead" role="row">
          <span>Položka</span><span class="r">Množstvo</span><span>MJ</span><span class="r">Cena/MJ</span><span v-if="!nonVat" class="r">DPH</span><span class="r">Spolu</span><span />
        </div>
        <div v-for="(l, i) in form.lines" :key="i" class="row" role="row">
          <span class="cell wide item" data-label="Položka">
            <Select :model-value="l.itemId || null" :options="itemOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
              :disabled="!editable" placeholder="Z cenníka…" @update:model-value="setItem(i, $event)" />
            <InputText v-if="!l.itemId" v-model="l.description" size="small" :disabled="!editable" :invalid="!!err(i, 'description')" placeholder="alebo popis" />
          </span>
          <span class="cell" data-label="Množstvo"><InputText v-model="l.quantity" size="small" inputmode="decimal" class="r" :disabled="!editable" :invalid="!!err(i, 'quantity')" /></span>
          <span class="cell" data-label="MJ"><InputText v-model="l.unit" size="small" :disabled="!editable || !!l.itemId" /></span>
          <span class="cell" data-label="Cena/MJ"><InputText v-model="l.unitPrice" size="small" inputmode="decimal" class="r" :disabled="!editable" :invalid="!!err(i, 'unitPrice')" /></span>
          <span v-if="!nonVat" class="cell" data-label="DPH">
            <Select v-model="l.vatRate" :options="rates" size="small" fluid :disabled="!editable" :invalid="!!err(i, 'vatRate')">
              <template #value="{ value }">{{ value }} %</template>
              <template #option="{ option }">{{ option }} %</template>
            </Select>
          </span>
          <span class="cell sum" data-label="Spolu">{{ fresh && l.amount ? formatAmount(l.amount) : '—' }}</span>
          <span class="cell act">
            <Button v-if="editable" icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Odstrániť riadok" @click="form.lines.splice(i, 1)" />
          </span>
          <small v-for="(m, k) in [err(i, 'description'), err(i, 'quantity'), err(i, 'unitPrice'), err(i, 'vatRate')].filter(Boolean)" :key="k" class="rerr">{{ m }}</small>
        </div>
      </div>
      <small v-if="errors.lines" class="rerr">{{ errors.lines }}</small>
      <dl v-if="fresh && form.lines.length" class="totals">
        <template v-if="!nonVat">
          <div><dt>Základ</dt><dd>{{ formatAmount(note.summary.base) }}</dd></div>
          <div><dt>DPH</dt><dd>{{ formatAmount(note.summary.vat) }}</dd></div>
        </template>
        <div class="grand"><dt>Spolu</dt><dd><span>{{ formatMoney(note.summary.total) }}</span></dd></div>
      </dl>
    </section>

    <RelatedDocs v-if="note" kind="delivery" :id="Number(note.id)" :can-write="canWrite" />
    <SendMailDialog v-if="note" v-model:visible="mailOpen" kind="deliveries" :id="note.id" />
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
.inv {
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--blue);
  text-decoration: none;
}
.spacer {
  flex: 1;
}
.customer {
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
  grid-template-columns: minmax(220px, 1fr) 84px 60px 96px 104px 100px 40px;
  gap: 6px;
  align-items: center;
  padding: 3px 0;
  border-bottom: 1px solid var(--rule);
}
.grid.novat .row {
  grid-template-columns: minmax(220px, 1fr) 84px 60px 96px 100px 40px;
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
.cell :deep(.p-select-label) {
  padding-right: 0;
}
.item {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.r,
.cell :deep(input.r) {
  text-align: right;
}
.sum {
  text-align: right;
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
.totals {
  align-self: flex-end;
  margin: 4px 46px 0 0;
  min-width: 240px;
  font-variant-numeric: tabular-nums;
}
.totals div {
  display: flex;
  justify-content: space-between;
  gap: 20px;
  padding: 2px 0;
}
.totals dt {
  color: var(--ink-soft);
}
.totals dd {
  margin: 0;
}
.totals .grand dd span {
  font-size: var(--fs-lg);
  font-weight: 600;
  border-bottom: 3px double var(--ink);
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 20px;
  }
  .thead {
    display: none;
  }
  .row,
  .grid.novat .row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    padding: 8px 0;
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
  .sum {
    text-align: left;
  }
  .totals {
    margin-right: 0;
    align-self: stretch;
  }
}
</style>
