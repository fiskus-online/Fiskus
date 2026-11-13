<script setup>
// Domácnosť — účty: hotovosť, bežné účty, karty a sporenie so zostatkom.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import FormField from '../components/FormField.vue'
import IbanInput from '../components/IbanInput.vue'
import ExportButton from '../components/ExportButton.vue'
import FioPanel from '../components/FioPanel.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatMoney } from '../lib/format.js'
import { householdAccountColumns } from '../lib/columns.js'
import { accountBody, accountIcon, accountKindLabel, accountKinds } from '../lib/household.js'
import { matches, searchTerms } from '../lib/search.js'

const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))
const list = ref([])
const loading = ref(true)
const search = ref('')
const visible = computed(() => {
  const t = searchTerms(search.value)
  return list.value.filter((a) => matches(t, [a.name, accountKindLabel(a.kind), a.iban, a.balance]))
})

async function load() {
  loading.value = true
  try {
    list.value = await api.hhAccounts(session.companyId)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Účty sa nenačítali', detail: e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}
onMounted(load)

// napojenie bežného účtu na Fio banku (sťahuje sa v Pohyboch; nie v prehliadači)
const fioOn = computed(() => canWrite.value && !session.caps?.browser)
const fioAccounts = ref([])
const fioOpen = ref(false)
async function loadFio() {
  if (!fioOn.value) return
  fioAccounts.value = await api.fioAccounts(session.companyId).catch(() => [])
}
onMounted(loadFio)
const fioOf = (a) => fioAccounts.value.some((f) => f.householdAccountId === a.id)

const form = ref(null)
const errors = ref({})
const busy = ref(false)
function edit(a) {
  form.value = a ? { ...a, opening: String(a.opening ?? '').replace('.', ',') } : { id: 0, name: '', kind: list.value.length ? 'cash' : 'bank', iban: '', opening: '', closed: false }
  errors.value = {}
}
async function save() {
  busy.value = true
  try {
    const f = accountBody(form.value)
    await api.hhSaveAccount(session.companyId, f)
    toast.add({ severity: 'success', summary: f.id ? 'Účet uložený' : 'Účet pridaný', life: 2000 })
    form.value = null
    await load()
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Účet sa neuložil', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
function remove(a) {
  confirm.require({
    header: 'Zmazať účet',
    message: `Zmazať účet ${a.name}? Účet s pohybmi sa nedá zmazať — označte ho ako zrušený.`,
    acceptProps: { label: 'Zmazať', severity: 'danger', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.hhDeleteAccount(session.companyId, a.id)
        form.value = null
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Účet sa nezmazal', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Účty</h1>
      <span class="count">{{ list.length }}</span>
      <div class="spacer" />
      <ExportButton v-if="list.length" name="Účty domácnosti" :columns="householdAccountColumns" :rows="visible" />
      <Button v-if="fioOn && list.some((a) => a.kind === 'bank' && !a.closed)" :label="fioAccounts.length ? 'Fio banka' : 'Napojiť Fio'"
        :icon="fioAccounts.length ? 'pi pi-cog' : 'pi pi-link'" size="small" severity="secondary" outlined @click="fioOpen = true" />
      <Button v-if="canWrite" label="Pridať účet" icon="pi pi-plus" size="small" @click="edit(null)" />
    </header>
    <p class="help">
      Účet je miesto, kde máte peniaze. Počiatočný stav je zostatok v deň, od ktorého zapisujete pohyby — ďalej ho Fiškus počíta sám.
      Kreditná karta má zostatok záporný (dlh voči banke).
    </p>
    <InputText v-if="list.length" v-model="search" placeholder="Hľadať účet" size="small" class="search" aria-label="Hľadať účet" />

    <table v-if="list.length" class="accounts">
      <thead>
        <tr><th>Účet</th><th>Druh</th><th class="iban">IBAN</th><th class="num">Zostatok</th></tr>
      </thead>
      <tbody>
        <tr v-for="a in visible" :key="a.id" :class="{ closed: a.closed }" @click="canWrite ? edit(a) : router.push({ path: '/domacnost/pohyby', query: { ucet: a.id } })">
          <td data-label="Účet"><i class="pi" :class="accountIcon(a.kind)" aria-hidden="true" /> {{ a.name }}<small v-if="a.closed"> · zrušený</small>
            <span v-if="fioOf(a)" class="fio" title="Pohyby sa sťahujú z Fio banky (Pohyby → Stiahnuť z Fio)">Fio</span></td>
          <td data-label="Druh">{{ accountKindLabel(a.kind) }}</td>
          <td class="iban" data-label="IBAN">{{ a.iban || '—' }}</td>
          <td class="num bal" :class="{ neg: Number(a.balance) < 0 }" data-label="Zostatok">{{ formatMoney(a.balance) }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else-if="!loading" class="empty">Zatiaľ žiadny účet. Začnite bežným účtom a peňaženkou.</p>

    <FioPanel v-if="fioOn" v-model:visible="fioOpen" :cid="session.companyId" :can-write="canWrite" :accounts="fioAccounts" :household-accounts="list"
      @changed="(l) => (fioAccounts = l)" @synced="load" />
    <Dialog :visible="!!form" :header="form?.id ? 'Účet' : 'Nový účet'" modal :style="{ width: '30rem' }" :breakpoints="{ '560px': '96vw' }"
      @update:visible="(v) => !v && (form = null)">
      <form v-if="form" class="form-grid" @submit.prevent="save">
        <FormField label="Druh" :span="2">
          <SelectButton v-model="form.kind" :options="accountKinds" option-label="label" option-value="value" :allow-empty="false" size="small" class="kinds" />
        </FormField>
        <FormField label="Názov" :span="2" :error="errors.name">
          <InputText v-model="form.name" size="small" placeholder="napr. Účet VÚB, Peňaženka" autofocus />
        </FormField>
        <FormField v-if="form.kind !== 'cash'" label="IBAN" :span="2" :error="errors.iban" hint="Na kontrolu, že nahrávaný výpis patrí k účtu">
          <IbanInput v-model="form.iban" :invalid="!!errors.iban" />
        </FormField>
        <FormField label="Počiatočný stav" :error="errors.opening" :hint="form.kind === 'card' ? 'dlh zadajte so znamienkom −' : ''">
          <InputText v-model="form.opening" size="small" inputmode="decimal" placeholder="0,00" class="num" />
        </FormField>
        <FormField v-if="fioOn && form.id && form.kind === 'bank'" label="Fio banka" :span="2"
          :hint="fioOf(form) ? 'Pohyby sťahujete v Pohyboch tlačidlom Stiahnuť z Fio' : 'Pohyby z účtu vo Fio sa stiahnu bez výpisu'">
          <Button :label="fioOf(form) ? 'Napojený – nastavenie' : 'Napojiť na Fio'" :icon="fioOf(form) ? 'pi pi-cog' : 'pi pi-link'" size="small"
            severity="secondary" outlined @click="form = null; fioOpen = true" />
        </FormField>
        <FormField v-if="form.id" label="Zrušený účet" hint="Neponúka sa pri nových pohyboch">
          <ToggleSwitch v-model="form.closed" />
        </FormField>
        <div class="actions">
          <Button v-if="form.id" label="Zmazať" icon="pi pi-trash" severity="danger" text size="small" @click="remove(form)" />
          <span class="spacer" />
          <Button label="Zrušiť" text size="small" @click="form = null" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.fio {
  margin-left: 6px;
  padding: 0 5px;
  border: 1px solid var(--blue);
  border-radius: 3px;
  font-size: var(--fs-xs);
  font-weight: 600;
  color: var(--blue);
}
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.head h1 {
  font-size: var(--fs-xl);
}
.count {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.spacer {
  flex: 1;
}
.help {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  max-width: 75ch;
}
.search {
  max-width: 280px;
}
.empty {
  color: var(--ink-faint);
}
.accounts {
  width: 100%;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  font-size: var(--fs-md);
}
.accounts th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
}
.accounts th.num {
  text-align: right;
}
.accounts td {
  padding: 7px 10px;
  border-bottom: 1px solid var(--rule-soft);
}
.accounts tbody tr {
  cursor: pointer;
}
.accounts tbody tr:hover {
  background: var(--blue-wash);
}
.accounts .pi {
  color: var(--blue);
  font-size: 0.85rem;
  margin-right: 4px;
}
.accounts small {
  color: var(--ink-faint);
}
.closed td {
  color: var(--ink-faint);
}
.bal {
  font-weight: 600;
}
.neg {
  color: var(--tone-overdue);
}
.kinds {
  flex-wrap: wrap;
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  gap: 6px;
  align-items: center;
}
@media (max-width: 560px) {
  .accounts .iban {
    display: none;
  }
}
</style>
