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
import { session } from '../lib/session.js'
import { fromISODate, toISODate } from '../lib/format.js'
import { categoriesFor, entryTypes, fromForm, toForm } from '../lib/household.js'
import { fold } from '../lib/search.js'

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
const open = computed({ get: () => !!form.value, set: (v) => !v && emit('close') })
const cats = computed(() => categoriesFor(props.categories, form.value?.type))
const openAccounts = computed(() => props.accounts.filter((a) => !a.closed || a.id === form.value?.accountId || a.id === form.value?.toAccountId))
// záväzky pre druh pohybu: výdavok — dlhy a platby, príjem — požičané
const obligationOptions = computed(() =>
  props.obligations.filter((o) => (o.kind === 'lent') === (form.value?.type === 'income') && (!o.closed || o.id === form.value?.obligationId)),
)
function pickObligation(id) {
  const o = props.obligations.find((x) => x.id === id)
  if (!o || !form.value) return
  if (o.category) form.value.category = o.category
  if (!form.value.payee && o.creditor) form.value.payee = o.creditor
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

const payeeHits = ref([])
function searchPayee(q) {
  const f = fold(q)
  payeeHits.value = props.payees.filter((p) => fold(p).includes(f)).slice(0, 12)
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
      form.value = { ...f, id: 0, amount: '', payee: '', note: '', category: '', obligationId: 0 }
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
  <Drawer v-model:visible="open" position="right" class="hh-panel" :header="form?.id ? 'Pohyb' : 'Nový pohyb'">
    <form v-if="form" class="form-grid hh-form" @submit.prevent="save(false)">
      <FormField label="Druh" :span="2">
        <SelectButton v-model="form.type" :options="entryTypes" option-label="label" option-value="value" :allow-empty="false" size="small" :disabled="!canWrite" />
      </FormField>
      <FormField label="Suma" :error="errors.amount">
        <InputText v-model="form.amount" size="small" inputmode="decimal" class="amount" :class="form.type" placeholder="0,00" autofocus :disabled="!canWrite" />
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
        <FormField :label="form.type === 'income' ? 'Od koho' : 'Obchod / komu'" :span="2" :error="errors.payee">
          <AutoComplete v-model="form.payee" :suggestions="payeeHits" size="small" fluid :disabled="!canWrite"
            :placeholder="form.type === 'income' ? 'napr. zamestnávateľ' : 'napr. Lidl, ZSE, nájomca'" @complete="(e) => searchPayee(e.query)" @blur="suggest"
            @option-select="suggest" />
        </FormField>
        <FormField v-if="obligationOptions.length" label="Splátka záväzku" :span="2" :error="errors.obligationId" hint="nepovinné — započíta sa do splátkového kalendára">
          <Select v-model="form.obligationId" :options="obligationOptions" option-label="name" option-value="id" size="small" fluid show-clear
            placeholder="nie je splátka" :disabled="!canWrite" @change="(e) => pickObligation(e.value)" />
        </FormField>
        <FormField label="Kategória" :span="2" :error="errors.category" :hint="suggested ? 'navrhnutá podľa obchodu' : ''">
          <Select v-model="form.category" :options="cats" size="small" fluid filter :disabled="!canWrite" placeholder="Vyberte kategóriu"
            @change="suggested = false" />
        </FormField>
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
</style>

<style>
.hh-panel.p-drawer {
  width: 26rem;
}
@media (max-width: 560px) {
  .hh-panel.p-drawer {
    width: 100vw;
  }
}
</style>
