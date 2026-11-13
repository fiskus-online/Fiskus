<script setup>
// Pohyb domácnosti (výdavok, príjem, prevod medzi účtami). Kategóriu podľa
// obchodu navrhne server (história a známe obchody).
import { computed, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Drawer from 'primevue/drawer'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import DatePicker from 'primevue/datepicker'
import AutoComplete from 'primevue/autocomplete'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, fromISODate, toISODate } from '../lib/format.js'
import { categoriesFor, entryTypes, fromForm, payeeOptions, receiptFacts, receiptRows, setPayee, toForm } from '../lib/household.js'

const props = defineProps({
  entry: { type: Object, default: null }, // pohyb z API alebo nový z newEntry()
  accounts: { type: Array, default: () => [] },
  categories: { type: Array, default: () => [] },
  payees: { type: Array, default: () => [] }, // obchody z histórie (návrhy)
  obligations: { type: Array, default: () => [] }, // záväzky (pohyb môže byť splátkou)
  canWrite: { type: Boolean, default: true },
})
const emit = defineEmits(['close', 'saved'])
const toast = useToast()
const confirm = useConfirm()

const form = ref(null)
const errors = ref({})
const busy = ref(false)
const suggested = ref(false) // kategóriu doplnil návrh (prepíše sa pri zmene obchodu)

watch(
  () => props.entry,
  (e) => {
    errors.value = {}
    suggested.value = false
    form.value = e ? { ...(e.type ? e : toForm(e)), date: fromISODate(e.date) } : null
  },
  { immediate: true },
)
// fotka bločku (iba overenie, že je; zobrazí sa v novom okne)
const hasPhoto = ref(false)
watch(
  () => form.value?.id,
  async (id) => {
    hasPhoto.value = false
    if (!id || !form.value?.split?.length) return
    try {
      const r = await fetch(api.hhPhotoUrl(session.companyId, id), { method: 'HEAD', credentials: 'same-origin' })
      hasPhoto.value = r.ok
    } catch {
      hasPhoto.value = false
    }
  },
)
const open = computed({ get: () => !!form.value, set: (v) => !v && emit('close') })
const cats = computed(() => categoriesFor(props.categories, form.value?.type))
// bloček: položky s cenou za kus a DPH, údaje dokladu z eKasy alebo fotky
const rows = computed(() => receiptRows(form.value?.split ?? [], form.value?.receipt))
const facts = computed(() => receiptFacts(form.value?.receipt))
const hasVAT = computed(() => !!form.value?.receipt?.vat?.length)
// množstvo bez zbytočných núl: 2.000 → 2, 0.5 → 0,5
const qty = (v) => (v === '' || v == null ? '' : String(v).replace(/\.?0+$/, '').replace('.', ','))
const openAccounts = computed(() => props.accounts.filter((a) => !a.closed || a.id === form.value?.accountId || a.id === form.value?.toAccountId))
// záväzky pre druh pohybu: výdavok — dlhy a platby, príjem — požičané
const obligationOptions = computed(() =>
  props.obligations.filter((o) => (o.kind === 'lent') === (form.value?.type === 'income') && (!o.closed || o.id === form.value?.obligationId)),
)
function pickObligation(id) {
  const o = props.obligations.find((x) => x.id === id)
  if (!o || !form.value) return
  if (o.category) form.value.category = o.category
  if (!form.value.payee && o.creditor) {
    form.value.payee = o.creditor
    form.value.contactId = o.contactId || 0
  }
  if (!form.value.amount && Number(o.state?.nextAmount)) form.value.amount = String(o.state.nextAmount).replace('.', ',')
  suggested.value = false
}
const targets = computed(() => openAccounts.value.filter((a) => a.id !== form.value?.accountId))

// pri zmene druhu sa kategória iného druhu vymaže
watch(
  () => form.value?.type,
  () => {
    if (form.value && form.value.category && !cats.value.includes(form.value.category)) form.value.category = ''
  },
)

// príjemca: partneri z adresára a obchody z histórie
const contacts = ref([])
watch(
  open,
  async (v) => {
    if (!v || contacts.value.length || !can('contacts.read')) return
    contacts.value = await api.contacts(session.companyId).catch(() => [])
  },
  { immediate: true },
)
const payeeHits = ref([])
function searchPayee(q) {
  payeeHits.value = payeeOptions(q, contacts.value, props.payees)
}
function changePayee(v) {
  if (form.value) setPayee(form.value, v, contacts.value)
}

async function suggest() {
  const f = form.value
  if (!f || f.type === 'transfer' || !f.payee?.trim() || (f.category && !suggested.value)) return
  try {
    const r = await api.hhSuggest(session.companyId, f.payee, f.type === 'income')
    if (r.category && form.value === f) {
      f.category = r.category
      suggested.value = true
    }
  } catch {
    /* návrh je iba pomôcka */
  }
}

async function save(again = false) {
  busy.value = true
  try {
    const f = form.value
    const saved = await api.hhSaveEntry(session.companyId, fromForm({ ...f, date: toISODate(f.date) }))
    toast.add({ severity: 'success', summary: f.id ? 'Pohyb uložený' : 'Pohyb pridaný', life: 2000 })
    emit('saved', saved)
    if (again) {
      // ďalší pohyb: rovnaký dátum, účet a druh
      form.value = { ...f, id: 0, amount: '', payee: '', contactId: 0, note: '', category: '', obligationId: 0 }
      errors.value = {}
      suggested.value = false
    } else emit('close')
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Pohyb sa neuložil', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

function remove() {
  confirm.require({
    header: 'Zmazať pohyb',
    message: 'Pohyb sa zmaže zo zoznamu aj zo zostatku účtu.',
    acceptProps: { label: 'Zmazať', severity: 'danger', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.hhDeleteEntry(session.companyId, form.value.id)
        emit('saved', null)
        emit('close')
        toast.add({ severity: 'success', summary: 'Pohyb zmazaný', life: 2000 })
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Pohyb sa nezmazal', detail: e.message, life: 6000 })
      }
    },
  })
}
</script>

<template>
  <Drawer v-model:visible="open" position="right" class="hh-panel" :class="{ 'hh-wide': form?.split?.length }" :header="form?.id ? 'Pohyb' : 'Nový pohyb'">
    <form v-if="form" class="form-grid hh-form" @submit.prevent="save(false)">
      <FormField label="Druh" :span="2">
        <SelectButton v-model="form.type" :options="entryTypes" option-label="label" option-value="value" :allow-empty="false" size="small" :disabled="!canWrite || !!form.split?.length" />
      </FormField>
      <FormField label="Suma" :error="errors.amount">
        <InputText v-model="form.amount" size="small" inputmode="decimal" class="amount" :class="form.type" placeholder="0,00" autofocus :disabled="!canWrite || !!form.split?.length" />
      </FormField>
      <FormField label="Dátum" :error="errors.date">
        <DatePicker v-model="form.date" date-format="d. m. yy" size="small" fluid :disabled="!canWrite" />
      </FormField>
      <FormField :label="form.type === 'transfer' ? 'Z účtu' : 'Účet'" :span="form.type === 'transfer' ? 1 : 2" :error="errors.accountId">
        <Select v-model="form.accountId" :options="openAccounts" option-label="name" option-value="id" size="small" fluid :disabled="!canWrite"
          placeholder="Vyberte účet" />
      </FormField>
      <FormField v-if="form.type === 'transfer'" label="Na účet" :error="errors.toAccountId">
        <Select v-model="form.toAccountId" :options="targets" option-label="name" option-value="id" size="small" fluid :disabled="!canWrite"
          placeholder="Vyberte účet" />
      </FormField>
      <template v-else>
        <FormField :label="form.type === 'income' ? 'Od koho' : 'Obchod / komu'" :span="2" :error="errors.payee || errors.contactId">
          <AutoComplete :model-value="form.payee" :suggestions="payeeHits" option-label="label" size="small" fluid :disabled="!canWrite"
            :placeholder="form.type === 'income' ? 'napr. zamestnávateľ' : 'napr. Lidl, ZSE, nájomca'" @update:model-value="changePayee"
            @complete="(e) => searchPayee(e.query)" @blur="suggest" @option-select="suggest">
            <template #option="{ option }">
              <span class="payee-opt">
                <i v-if="option.contactId" class="pi pi-address-book" aria-hidden="true" />
                <span>{{ option.label }}</span>
                <small v-if="option.city">{{ option.city }}</small>
              </span>
            </template>
          </AutoComplete>
          <small v-if="form.contactId" class="partner">
            <i class="pi pi-address-book" aria-hidden="true" /> partner z adresára —
            <RouterLink :to="`/adresar/${form.contactId}`" @click="emit('close')">platby a údaje</RouterLink>
          </small>
        </FormField>
        <FormField v-if="obligationOptions.length" label="Splátka záväzku" :span="2" :error="errors.obligationId" hint="nepovinné — započíta sa do splátkového kalendára">
          <Select v-model="form.obligationId" :options="obligationOptions" option-label="name" option-value="id" size="small" fluid show-clear
            placeholder="nie je splátka" :disabled="!canWrite" @change="(e) => pickObligation(e.value)" />
        </FormField>
        <div v-if="form.split?.length" class="split">
          <span class="ff-l">Položky bločku <small v-if="errors.split" class="err">{{ errors.split }}</small></span>
          <div class="tbl-wrap">
            <table>
              <thead>
                <tr><th>Tovar</th><th class="num">Množstvo</th><th class="num">Cena/ks</th><th v-if="hasVAT" class="num">DPH</th><th class="num">Suma</th><th>Kategória</th></tr>
              </thead>
              <tbody>
                <tr v-for="r in rows" :key="r.index">
                  <td class="nm" :title="r.name">{{ r.name }}<small v-if="r.kind" class="kind">{{ r.kind }}</small></td>
                  <td class="num">{{ qty(r.quantity) }}</td>
                  <td class="num">{{ r.unitPrice === '' ? '' : formatAmount(r.unitPrice) }}</td>
                  <td v-if="hasVAT" class="num">{{ r.vatRate }}</td>
                  <td class="num am">{{ formatAmount(r.amount) }}</td>
                  <td class="ct"><Select v-model="form.split[r.index].category" :options="cats" size="small" fluid :disabled="!canWrite" :aria-label="`Kategória ${r.name}`" /></td>
                </tr>
              </tbody>
              <tfoot v-if="form.receipt">
                <tr class="tot">
                  <td :colspan="hasVAT ? 4 : 3">Spolu</td>
                  <td class="num"><strong class="ledger">{{ formatAmount(form.receipt.total) }}</strong></td>
                  <td></td>
                </tr>
              </tfoot>
            </table>
          </div>

          <section v-if="form.receipt" class="rcpt" aria-label="Údaje bločku">
            <h4>Bloček {{ form.source?.startsWith('photo:') ? '(prečítaný z fotky)' : 'z eKasy' }}</h4>
            <p v-if="form.paired" class="paired"><i class="pi pi-link" aria-hidden="true" /> Spárovaný s platbou kartou z banky — je to jeden výdavok.</p>
            <dl>
              <template v-for="[k, v] in facts" :key="k"><dt>{{ k }}</dt><dd>{{ v }}</dd></template>
            </dl>
            <table v-if="form.receipt.vat?.length" class="vat">
              <thead><tr><th>Sadzba DPH</th><th class="num">Základ</th><th class="num">DPH</th></tr></thead>
              <tbody>
                <tr v-for="v in form.receipt.vat" :key="v.rate"><td>{{ v.rate }} %</td><td class="num">{{ formatAmount(v.base) }}</td><td class="num">{{ formatAmount(v.vat) }}</td></tr>
              </tbody>
            </table>
          </section>
          <a v-if="form.id && hasPhoto" :href="api.hhPhotoUrl(session.companyId, form.id)" target="_blank" rel="noopener" class="ph">
            <img :src="api.hhPhotoUrl(session.companyId, form.id)" alt="Fotka bločku" loading="lazy" />
            <span><i class="pi pi-image" aria-hidden="true" /> Fotka bločku</span>
          </a>
        </div>
        <template v-else>
          <FormField label="Kategória" :span="2" :error="errors.category" :hint="suggested ? 'navrhnutá podľa obchodu' : ''">
          <Select v-model="form.category" :options="cats" size="small" fluid filter :disabled="!canWrite" placeholder="Vyberte kategóriu"
            @change="suggested = false" />
        </FormField>
        </template>
      </template>
      <FormField label="Poznámka" :span="2" :error="errors.note">
        <InputText v-model="form.note" size="small" :disabled="!canWrite" />
      </FormField>
      <div v-if="canWrite" class="actions">
        <Button v-if="form.id" label="Zmazať" icon="pi pi-trash" severity="danger" text size="small" @click="remove" />
        <span class="spacer" />
        <Button v-if="!form.id" label="Uložiť a ďalší" severity="secondary" outlined size="small" :loading="busy" @click="save(true)" />
        <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
      </div>
    </form>
  </Drawer>
</template>

<style scoped>
.hh-form {
  grid-template-columns: 1fr 1fr;
}
.kind {
  margin-left: 0.4rem;
}
.amount {
  font-variant-numeric: tabular-nums;
  font-size: var(--fs-lg);
  font-weight: 600;
  text-align: right;
}
.amount.expense {
  color: var(--tone-overdue);
}
.amount.income {
  color: var(--tone-paid);
}
.split {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ff-l {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.tbl-wrap {
  overflow-x: auto;
  border: 1px solid var(--rule);
}
.split table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
}
.split th {
  text-align: left;
  font-weight: 600;
  color: var(--ink-soft);
  padding: 4px 6px;
  border-bottom: 1px solid var(--ink);
  white-space: nowrap;
}
.split td {
  padding: 2px 6px;
  border-bottom: 1px solid var(--rule-soft);
  white-space: nowrap;
}
.split .num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.split .nm small {
  color: var(--ink-faint);
}
.split .ct {
  width: 13rem;
  min-width: 11rem;
}
.split .ct :deep(.p-select) {
  max-width: 100%;
}
.rcpt {
  margin-top: 10px;
  padding: 10px 12px;
  background: var(--paper);
  font-size: var(--fs-sm);
}
.rcpt h4 {
  margin: 0 0 6px;
  font-size: var(--fs-sm);
}
.rcpt dl {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 2px 14px;
  margin: 0 0 8px;
}
.rcpt dt {
  color: var(--ink-soft);
}
.rcpt dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.rcpt .vat {
  width: auto;
  min-width: 18rem;
  margin-bottom: 6px;
  background: var(--sheet);
}
.tot td {
  border-top: 1px solid var(--ink);
  font-weight: 600;
  white-space: nowrap;
}
.ledger {
  border-bottom: 3px double currentColor;
}
.split .err {
  color: var(--tone-overdue);
}
.ph {
  font-size: var(--fs-sm);
  color: var(--blue);
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
}
.ph img {
  width: 72px;
  height: 96px;
  object-fit: cover;
  border: 1px solid var(--rule);
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  gap: 6px;
  align-items: center;
  margin-top: 6px;
}
.spacer {
  flex: 1;
}
.payee-opt {
  display: flex;
  align-items: center;
  gap: 8px;
}
.payee-opt .pi {
  color: var(--blue);
  font-size: 0.85em;
}
.payee-opt small {
  color: var(--ink-soft);
}
.partner {
  color: var(--ink-soft);
  font-size: var(--fs-sm, 0.85em);
}
.partner .pi {
  color: var(--blue);
  font-size: 0.85em;
}
</style>

<style>
.hh-panel.p-drawer {
  width: 26rem;
}
/* pohyb z bločku: širší panel, aby sa názvy tovaru nezalamovali */
.hh-panel.hh-wide.p-drawer {
  width: min(54rem, 100vw);
}
@media (max-width: 560px) {
  .hh-panel.p-drawer {
    width: 100vw !important;
  }
}
.paired {
  margin: 0 0 0.4rem;
  font-size: 0.85rem;
  color: var(--ink-soft);
}
</style>
