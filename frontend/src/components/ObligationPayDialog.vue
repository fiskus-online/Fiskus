<script setup>
// Zaplatenie splátky záväzku: zapíše pohyb domácnosti (výdavok, pri
// požičaných príjem) s kategóriou záväzku. Sumu najbližšej splátky dá server.
import { computed, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import FormField from './FormField.vue'
import PaymentDetails from './PaymentDetails.vue'
import { api, ApiError } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatMoney, toISODate } from '../lib/format.js'
import { fromAmount, toAmount } from '../lib/household.js'

const props = defineProps({
  // záväzok alebo splátka { id, name, kind, creditor, accountId, iban, vs, ss, payeeName, message, address, amount, date }; amount = navrhovaná suma
  target: { type: Object, default: null },
  accounts: { type: Array, default: () => [] },
})
const emit = defineEmits(['close', 'paid'])
const toast = useToast()
const form = ref(null)
const errors = ref({})
const busy = ref(false)

watch(
  () => props.target,
  (t) => {
    errors.value = {}
    form.value = t ? { date: new Date(), accountId: t.accountId || props.accounts.find((a) => !a.closed)?.id || null, amount: fromAmount(t.amount), note: '' } : null
  },
  { immediate: true },
)
const open = computed({ get: () => !!form.value, set: (v) => !v && emit('close') })
const income = computed(() => props.target?.kind === 'lent')

async function pay() {
  busy.value = true
  try {
    const f = form.value
    await api.hhPay(session.companyId, props.target.id, { date: toISODate(f.date), accountId: f.accountId, amount: toAmount(f.amount) || '0', note: f.note })
    toast.add({ severity: 'success', summary: income.value ? 'Vrátené peniaze zapísané' : 'Splátka zapísaná', detail: props.target.name, life: 2500 })
    emit('paid')
    emit('close')
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Splátka sa nezapísala', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="open" :header="income ? 'Prijaté peniaze' : 'Zaplatiť splátku'" modal :style="{ width: income ? '26rem' : '40rem' }" :breakpoints="{ '480px': '96vw' }">
    <form v-if="form" class="form-grid pay" @submit.prevent="pay">
      <p class="what">
        <strong>{{ target.name }}</strong>
        <span v-if="target.creditor">{{ target.creditor }}</span>
      </p>
      <PaymentDetails v-if="!income" class="pay-details" :obligation="target" :amount="target.amount" :due-date="target.date || ''" />
      <FormField label="Suma" :error="errors.amount" :hint="target.amount ? `splátka ${formatMoney(target.amount)}` : ''">
        <InputText v-model="form.amount" size="small" inputmode="decimal" class="amt" autofocus />
      </FormField>
      <FormField label="Dátum" :error="errors.date">
        <DatePicker v-model="form.date" date-format="d. m. yy" size="small" fluid />
      </FormField>
      <FormField label="Z účtu" :span="2" :error="errors.accountId">
        <Select v-model="form.accountId" :options="accounts.filter((a) => !a.closed)" option-label="name" option-value="id" size="small" fluid />
      </FormField>
      <FormField label="Poznámka" :span="2">
        <InputText v-model="form.note" size="small" placeholder="nepovinné" />
      </FormField>
      <div class="actions">
        <Button label="Zrušiť" text size="small" @click="open = false" />
        <Button type="submit" :label="income ? 'Zapísať príjem' : 'Zapísať splátku'" icon="pi pi-check" size="small" :loading="busy" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.pay {
  grid-template-columns: 1fr 1fr;
}
.what {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0 0 4px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.what strong {
  font-size: var(--fs-md);
  color: var(--ink);
}
.amt {
  text-align: right;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  justify-content: flex-end;
  gap: 6px;
}
.pay-details {
  grid-column: 1 / -1;
  margin: 0;
}
</style>
