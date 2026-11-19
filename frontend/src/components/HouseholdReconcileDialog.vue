<script setup>
// Odsúhlasenie zostatku účtu domácnosti s výpisom banky: zostatok z výpisu
// k dátumu, porovnanie so zostatkom vo Fiškuse (počíta server) a vyrovnanie
// rozdielu pohybom.
import { computed, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatDate, formatMoney, toISODate } from '../lib/format.js'
import { amountInput } from '../lib/household.js'

const props = defineProps({
  account: { type: Object, default: null }, // účet zo zoznamu (null = zatvorené)
  categories: { type: Array, default: () => [] },
  canWrite: { type: Boolean, default: false },
})
const emit = defineEmits(['close', 'done'])
const toast = useToast()

const date = ref(new Date())
const balance = ref('')
const category = ref('')
const result = ref(null)
const errors = ref({})
const busy = ref(false)

watch(
  () => props.account,
  (a) => {
    if (!a) return
    date.value = new Date()
    balance.value = ''
    category.value = ''
    result.value = null
    errors.value = {}
  },
)

// náhľad počíta server (zostatok k dátumu a rozdiel)
let timer = 0
let seq = 0
watch([date, balance, category], () => {
  clearTimeout(timer)
  timer = setTimeout(preview, 250)
})
async function preview() {
  const a = props.account
  const bal = amountInput(balance.value)
  if (!a || !date.value || !bal) {
    result.value = null
    return
  }
  const n = ++seq
  try {
    const r = await api.hhReconcilePreview(session.companyId, a.id, toISODate(date.value), bal, category.value)
    if (n !== seq) return
    result.value = r
    errors.value = {}
    if (!category.value) category.value = r.category
  } catch (e) {
    if (n !== seq) return
    result.value = null
    errors.value = e instanceof ApiError ? e.fieldMap() : { balance: e.message }
  }
}

const differs = computed(() => result.value && Number(result.value.difference) !== 0)
const income = computed(() => result.value && Number(result.value.difference) > 0)
const choices = computed(() => props.categories.filter((c) => c.income === !!income.value).map((c) => c.name))

async function commit() {
  busy.value = true
  try {
    const r = await api.hhReconcile(session.companyId, props.account.id, {
      date: toISODate(date.value), balance: amountInput(balance.value), adjust: differs.value, category: differs.value ? category.value : '',
    })
    toast.add({
      severity: 'success', life: 3000, summary: 'Účet odsúhlasený',
      detail: r.entry ? `Rozdiel ${formatMoney(r.entry.amount)} je zapísaný ako pohyb „Vyrovnanie podľa výpisu“` : '',
    })
    emit('done')
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Účet sa neodsúhlasil', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog :visible="!!account" :header="`Odsúhlasiť s výpisom — ${account?.name ?? ''}`" modal :style="{ width: '30rem' }" :breakpoints="{ '560px': '96vw' }"
    @update:visible="(v) => !v && emit('close')">
    <form v-if="account" class="form-grid" @submit.prevent="commit">
      <p class="lead">
        Opíšte zostatok z výpisu banky (alebo spočítajte peňaženku). Fiškus ho porovná so svojím zostatkom k tomu istému dňu.
      </p>
      <p v-if="account.reconciled" class="last">
        Naposledy odsúhlasené k {{ formatDate(account.reconciled.date) }}: {{ formatMoney(account.reconciled.balance) }}
      </p>
      <FormField label="Dátum zostatku" :error="errors.date">
        <DatePicker v-model="date" date-format="d. m. yy" size="small" fluid :max-date="new Date()" />
      </FormField>
      <FormField label="Zostatok na výpise" :error="errors.balance" :hint="account.kind === 'card' ? 'dlh so znamienkom −' : ''">
        <InputText v-model="balance" size="small" inputmode="decimal" placeholder="0,00" class="num" autofocus />
      </FormField>

      <table v-if="result" class="cmp">
        <tbody>
          <tr><th>Vo Fiškuse k {{ formatDate(result.date) }}</th><td class="num">{{ formatMoney(result.book) }}</td></tr>
          <tr><th>Na výpise</th><td class="num">{{ formatMoney(result.statement) }}</td></tr>
          <tr class="diff" :class="{ off: differs }"><th>Rozdiel</th><td class="num">{{ formatMoney(result.difference) }}</td></tr>
        </tbody>
      </table>
      <p v-if="result && !differs" class="ok"><i class="pi pi-check-circle" aria-hidden="true" /> Zostatok sedí.</p>
      <p v-if="differs" class="hint">
        Skúste najprv dohľadať chýbajúci alebo dvakrát zapísaný pohyb. Ak rozdiel nedohľadáte (bankový poplatok, úrok, drobné), zapíše sa ako pohyb
        „Vyrovnanie podľa výpisu“ k {{ formatDate(result.date) }}.
      </p>
      <FormField v-if="differs" label="Kategória vyrovnania" :span="2" :error="errors.category">
        <Select v-model="category" :options="choices" size="small" fluid />
      </FormField>
      <p v-if="result?.later" class="note">Pohyby po {{ formatDate(result.date) }} ({{ result.later }}) do porovnania nevstupujú.</p>

      <div class="actions">
        <span class="spacer" />
        <Button label="Zrušiť" text size="small" @click="emit('close')" />
        <Button v-if="canWrite" type="submit" :label="differs ? 'Zapísať rozdiel a odsúhlasiť' : 'Odsúhlasiť'" icon="pi pi-check" size="small"
          :loading="busy" :disabled="!result" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.lead,
.last,
.hint,
.note,
.ok {
  grid-column: 1 / -1;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  margin: 0;
}
.last {
  color: var(--ink-faint);
}
.ok {
  color: var(--blue);
  font-weight: 600;
}
.cmp {
  grid-column: 1 / -1;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.cmp th {
  text-align: left;
  font-weight: 400;
  color: var(--ink-soft);
  padding: 3px 0;
}
.cmp td {
  padding: 3px 0;
  font-variant-numeric: tabular-nums;
}
.cmp .diff th,
.cmp .diff td {
  border-top: 1px solid var(--rule);
  font-weight: 600;
  color: var(--ink);
}
.cmp .diff.off td {
  color: var(--tone-overdue);
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  gap: 6px;
  align-items: center;
}
.spacer {
  flex: 1;
}
</style>
