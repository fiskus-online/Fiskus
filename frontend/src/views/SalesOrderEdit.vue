<script setup>
// Prijatá objednávka: odberateľ, termín dodania, položky s cenami. Kým
// čaká na dodanie, rezervuje skladové položky. Dodáva sa po častiach
// dodacím listom alebo faktúrou. Sumy, zvyšky a voľné množstvá počíta server.
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
import Dialog from 'primevue/dialog'
import FormField from '../components/FormField.vue'
import RelatedDocs from '../components/RelatedDocs.vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { ratesOn } from '../lib/codes.js'
import { docPath } from '../lib/links.js'
import { blankSalesOrder, isActive, partyOf, pickDraft, pickPayload, salesOrderForm, salesOrderLine, salesOrderPayload, salesOrderStatusOf, shortage } from '../lib/sorders.js'

const props = defineProps({ id: { type: String, default: '' } })
const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('invoices.write'))
const nonVat = computed(() => !session.company?.vatPayer)
const today = () => toISODate(new Date())

const order = ref(null) // zo servera (číslo, stav, sumy, dodania)
const form = ref(blankSalesOrder(null, today()))
const contacts = ref([])
const items = ref([])
const errors = ref({})
const busy = ref(false)
const saved = ref('')
const snapshot = () => JSON.stringify(salesOrderPayload(form.value))
const dirty = computed(() => snapshot() !== saved.value)

async function load() {
  errors.value = {}
  ;[contacts.value, items.value] = await Promise.all([
    api.contacts(session.companyId).catch(() => []),
    api.items(session.companyId).catch(() => []),
  ])
  if (props.id) {
    setOrder(await api.salesOrder(session.companyId, props.id))
  } else {
    const ct = route.query.kontakt ? contacts.value.find((c) => String(c.id) === String(route.query.kontakt)) : null
    order.value = null
    form.value = blankSalesOrder(ct, today())
    saved.value = snapshot()
  }
}
function setOrder(o) {
  order.value = o
  form.value = salesOrderForm(o)
  saved.value = snapshot()
}
onMounted(load)
watch(() => props.id, load)

const active = computed(() => !order.value || isActive(order.value))
const editable = computed(() => canWrite.value && active.value)
const status = computed(() => (order.value ? salesOrderStatusOf(order.value, today()) : null))
const title = computed(() => (order.value ? `Objednávka ${order.value.number}` : 'Nová objednávka'))
const fulfils = computed(() => (order.value?.fulfils ?? []).map((f) => ({ ...f, path: docPath({ kind: f.kind, id: f.id }), label: f.kind === 'invoice' ? 'faktúra' : 'dodací list' })))

async function save() {
  busy.value = true
  errors.value = {}
  try {
    const wasNew = !order.value
    const o = await api.saveSalesOrder(session.companyId, salesOrderPayload(form.value), order.value?.id)
    setOrder(o)
    toast.add({ severity: 'success', summary: wasNew ? `Objednávka ${o.number} zapísaná` : 'Objednávka uložená', life: 2500 })
    if (wasNew) router.replace(`/prijate-objednavky/${o.id}`)
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
  busy.value = true
  try {
    setOrder(await api.salesOrderAction(session.companyId, order.value.id, name))
    toast.add({ severity: 'success', summary: done, life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
function cancel() {
  confirm.require({
    header: 'Zrušiť objednávku',
    message: `Zrušiť objednávku ${order.value.number}? Rezervovaný tovar sa uvoľní. Už vytvorené dodacie listy a faktúry ostanú.`,
    acceptProps: { label: 'Zrušiť objednávku', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: () => action('cancel', 'Objednávka zrušená'),
  })
}
const reopen = () => action('reopen', 'Objednávka obnovená')
function remove() {
  confirm.require({
    header: 'Zmazať objednávku',
    message: `Zmazať objednávku ${order.value.number}?`,
    acceptProps: { label: 'Zmazať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deleteSalesOrder(session.companyId, order.value.id)
        router.push('/prijate-objednavky')
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Objednávka sa nezmazala', detail: e.message, life: 6000 })
      }
    },
  })
}

// dodanie: výber množstiev a koncept dodacieho listu alebo faktúry
const pick = ref(null) // { to, qty }
const pickErrors = ref({})
async function startPick(to) {
  if (dirty.value && !(await save())) return
  pickErrors.value = {}
  pick.value = { to, qty: pickDraft(order.value) }
}
const pickLines = computed(() => (order.value?.lines ?? []).map((l, i) => ({ ...l, i })).filter((l) => Number(l.remaining) > 0))
async function fulfil() {
  busy.value = true
  pickErrors.value = {}
  try {
    const doc = await api.fulfilSalesOrder(session.companyId, order.value.id, pickPayload(pick.value.to, pick.value.qty))
    const what = doc.kind === 'invoice' ? 'Koncept faktúry' : 'Koncept dodacieho listu'
    toast.add({ severity: 'success', summary: `${what} pripravený`, detail: 'Skontrolujte ho a vystavte.', life: 3000 })
    pick.value = null
    router.push(docPath(doc))
  } catch (e) {
    pickErrors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(pickErrors.value).length) toast.add({ severity: 'error', summary: 'Doklad sa nevytvoril', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
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
const addLine = () => form.value.lines.push(salesOrderLine(null, lineOpts()))
function setItem(i, id) {
  const it = items.value.find((x) => x.id === id) ?? null
  const old = form.value.lines[i]
  form.value.lines[i] = { ...salesOrderLine(it, lineOpts()), key: old.key || 0, quantity: old.quantity || '1' }
}
const err = (i, f) => errors.value[`lines[${i}].${f}`]
const priceModes = [{ value: 'net', label: 'Ceny bez DPH' }, { value: 'gross', label: 'Ceny s DPH' }]
const date = (k) => ({
  get: () => fromISODate(form.value[k]),
  set: (d) => (form.value[k] = toISODate(d) ?? ''),
})
const fresh = computed(() => order.value && !dirty.value)
const served = (i) => order.value?.lines?.find((x) => x.key && x.key === form.value.lines[i]?.key) ?? null
const q = (v) => formatAmount(v).replace(/,00$/, '')
</script>

<template>
  <div class="page">
    <header class="head">
      <Button icon="pi pi-arrow-left" text rounded size="small" aria-label="Späť na prijaté objednávky" @click="router.push('/prijate-objednavky')" />
      <h1>{{ title }}</h1>
      <span v-if="status" class="tag" :data-tone="status.tone"><span class="tag-text">{{ status.label }}</span></span>
      <span class="spacer" />
      <template v-if="canWrite && order">
        <Button v-if="!fulfils.length && order.status !== 'done'" label="Zmazať" severity="danger" text size="small" @click="remove" />
        <Button v-if="active" label="Zrušiť" severity="danger" text size="small" @click="cancel" />
        <Button v-if="order.status === 'cancelled'" label="Obnoviť" icon="pi pi-replay" text size="small" :disabled="busy" @click="reopen" />
      </template>
      <Button v-if="editable" label="Uložiť" icon="pi pi-check" size="small" severity="secondary" outlined :loading="busy" :disabled="order && !dirty" @click="save" />
      <template v-if="canWrite && order && active">
        <Button label="Dodací list" icon="pi pi-truck" size="small" :disabled="busy" @click="startPick('delivery')" />
        <Button label="Faktúra" icon="pi pi-file" size="small" :disabled="busy" @click="startPick('invoice')" />
      </template>
    </header>

    <form class="form-grid fields" @submit.prevent="save">
      <FormField label="Odberateľ" :span="2" :error="errors['customer.name'] || errors.contactId">
        <div class="customer">
          <Select :model-value="form.contactId || null" :options="contactOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
            :disabled="!editable" placeholder="Z adresára…" @update:model-value="setContact" />
          <InputText v-if="!form.contactId" v-model="form.customer.name" size="small" :disabled="!editable" placeholder="alebo názov odberateľa" />
        </div>
      </FormField>
      <FormField label="Prijatá dňa" :error="errors.date">
        <DatePicker :model-value="date('date').get()" date-format="d. m. yy" size="small" fluid :disabled="!editable" @update:model-value="date('date').set($event)" />
      </FormField>
      <FormField label="Dodať do" :error="errors.due" hint="požadovaný termín">
        <DatePicker :model-value="date('due').get()" date-format="d. m. yy" size="small" fluid show-clear :disabled="!editable" @update:model-value="date('due').set($event)" />
      </FormField>
      <FormField label="Číslo u zákazníka" hint="číslo jeho objednávky">
        <InputText v-model="form.ref" size="small" :disabled="!editable" />
      </FormField>
      <FormField label="Miesto dodania" :span="2">
        <InputText v-model="form.place" size="small" :disabled="!editable" placeholder="Napr. prevádzka, stavba" />
      </FormField>
      <FormField v-if="!nonVat" label="Ceny">
        <SelectButton v-model="form.priceMode" :options="priceModes" option-label="label" option-value="value" :allow-empty="false" size="small" :disabled="!editable" />
      </FormField>
      <FormField label="Poznámka" :span="4">
        <Textarea v-model="form.note" rows="1" auto-resize :disabled="!editable" />
      </FormField>
    </form>

    <section class="lines" aria-label="Položky objednávky">
      <header class="ehead">
        <h2>Položky</h2>
        <span class="spacer" />
        <Button v-if="editable" label="Položka" icon="pi pi-plus" size="small" text @click="addLine" />
      </header>
      <p v-if="!form.lines.length" class="muted small">Skladová položka z cenníka sa rezervuje, kým ju nedodáte dodacím listom alebo faktúrou.</p>
      <div v-else class="grid" :class="{ novat: nonVat }" role="table">
        <div class="row thead" role="row">
          <span>Položka</span><span class="r">Množstvo</span><span>MJ</span><span class="r">Cena/MJ</span><span v-if="!nonVat" class="r">DPH</span><span class="r">Spolu</span><span />
        </div>
        <div v-for="(l, i) in form.lines" :key="i" class="row" role="row">
          <span class="cell wide item" data-label="Položka">
            <Select :model-value="l.itemId || null" :options="itemOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
              :disabled="!editable || Number(served(i)?.delivered) > 0" placeholder="Z cenníka…" @update:model-value="setItem(i, $event)" />
            <InputText v-if="!l.itemId" v-model="l.description" size="small" :disabled="!editable" :invalid="!!err(i, 'description')" placeholder="alebo popis" />
            <small v-if="served(i) && (Number(served(i).delivered) > 0 || served(i).available !== undefined)" class="state">
              <template v-if="Number(served(i).delivered) > 0">dodané {{ q(served(i).delivered) }}, zostáva {{ q(served(i).remaining) }} {{ l.unit }}</template>
              <template v-if="served(i).available !== undefined && Number(served(i).remaining) > 0">
                <template v-if="Number(served(i).delivered) > 0"> · </template>
                <b v-if="shortage(served(i))" class="short">na sklade chýba {{ q(served(i).short) }} {{ l.unit }}</b>
                <span v-else>na sklade voľné {{ q(served(i).available) }} {{ l.unit }}</span>
              </template>
            </small>
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
            <Button v-if="editable && !(Number(served(i)?.delivered) > 0)" icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Odstrániť riadok"
              @click="form.lines.splice(i, 1)" />
          </span>
          <small v-for="(m, k) in [err(i, 'description'), err(i, 'quantity'), err(i, 'unitPrice'), err(i, 'vatRate')].filter(Boolean)" :key="k" class="rerr">{{ m }}</small>
        </div>
      </div>
      <small v-if="errors.lines" class="rerr">{{ errors.lines }}</small>
      <dl v-if="fresh && form.lines.length" class="totals">
        <template v-if="!nonVat">
          <div><dt>Základ</dt><dd>{{ formatAmount(order.summary.base) }}</dd></div>
          <div><dt>DPH</dt><dd>{{ formatAmount(order.summary.vat) }}</dd></div>
        </template>
        <div class="grand"><dt>Spolu</dt><dd><span>{{ formatMoney(order.summary.total) }}</span></dd></div>
      </dl>
    </section>

    <section v-if="fulfils.length" class="lines" aria-label="Dodania">
      <header class="ehead"><h2>Dodania</h2></header>
      <ul class="fulfils">
        <li v-for="f in fulfils" :key="f.kind + f.id">
          <RouterLink :to="f.path">{{ f.label }} {{ f.number }}</RouterLink>
          <span class="muted">
            {{ order.lines.filter((l) => f.qty[l.key]).map((l) => `${l.description} ${q(f.qty[l.key])} ${l.unit}`).join(', ') }}
          </span>
        </li>
      </ul>
    </section>

    <RelatedDocs v-if="order" kind="sorder" :id="Number(order.id)" :can-write="canWrite" />

    <Dialog :visible="!!pick" modal :header="pick?.to === 'invoice' ? 'Faktúra z objednávky' : 'Dodací list z objednávky'" :style="{ width: '36rem' }"
      :breakpoints="{ '760px': '96vw' }" @update:visible="pick = null">
      <template v-if="pick">
        <p class="muted small">Koľko teraz dodáte? Vytvorí sa koncept, ktorý ešte skontrolujete. Zvyšok ostane v objednávke na ďalšie dodanie.</p>
        <table class="pick">
          <thead><tr><th>Položka</th><th class="r">Zostáva</th><th class="r">Dodať</th></tr></thead>
          <tbody>
            <tr v-for="l in pickLines" :key="l.key">
              <td>{{ l.description }}<small v-if="shortage(l)" class="short">na sklade chýba {{ q(l.short) }} {{ l.unit }}</small></td>
              <td class="r">{{ q(l.remaining) }} {{ l.unit }}</td>
              <td class="r">
                <InputText v-model="pick.qty[l.key]" size="small" inputmode="decimal" class="r qty" :invalid="!!pickErrors[`lines[${l.i}].pick`]" :aria-label="`Dodať ${l.description}`" />
                <small v-if="pickErrors[`lines[${l.i}].pick`]" class="rerr">{{ pickErrors[`lines[${l.i}].pick`] }}</small>
              </td>
            </tr>
          </tbody>
        </table>
        <small v-if="pickErrors.lines" class="rerr">{{ pickErrors.lines }}</small>
      </template>
      <template #footer>
        <Button label="Zrušiť" severity="secondary" text size="small" @click="pick = null" />
        <Button :label="pick?.to === 'invoice' ? 'Vytvoriť faktúru' : 'Vytvoriť dodací list'" icon="pi pi-check" size="small" :loading="busy" @click="fulfil" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px 24px;
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
.state {
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.short {
  display: block;
  font-size: var(--fs-xs);
  font-weight: 600;
  color: var(--ink);
}
.state .short {
  display: inline;
}
.fulfils {
  list-style: none;
  margin: 0;
  padding: 0;
}
.fulfils li {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  padding: 4px 0;
  border-bottom: 1px solid var(--rule);
  font-size: var(--fs-sm);
}
.fulfils a {
  font-weight: 600;
  color: var(--blue);
  text-decoration: none;
}
.pick {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.pick th {
  font-size: var(--fs-sm);
  font-weight: 400;
  color: var(--ink-soft);
  text-align: left;
  border-bottom: 1px solid var(--rule);
  padding: 4px 6px;
}
.pick td {
  padding: 5px 6px;
  border-bottom: 1px solid var(--rule);
  vertical-align: top;
}
.pick .r {
  text-align: right;
}
.pick .qty {
  width: 90px;
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
