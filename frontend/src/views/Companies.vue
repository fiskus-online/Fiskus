<script setup>
// Firmy a domácnosti: založenie (aj prvé spustenie) a prepínanie. Fiškus je
// bez obmedzenia počtu firiem.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Dialog from 'primevue/dialog'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { fillFromRegistry } from '../lib/registry.js'
import { session, selectCompany } from '../lib/session.js'
import FormField from '../components/FormField.vue'
import IcoInput from '../components/IcoInput.vue'
import ExportButton from '../components/ExportButton.vue'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { matches, searchTerms } from '../lib/search.js'
import { companyColumns } from '../lib/columns.js'

const router = useRouter()
const toast = useToast()
const busy = ref(false)
const creating = ref(false)
const form = ref(null)
const errors = ref({})

const businesses = computed(() => session.companies.filter((c) => c.bookkeeping !== 'household'))
const count = computed(() => businesses.value.length)
const households = computed(() => session.companies.filter((c) => c.bookkeeping === 'household'))
const search = ref('')
const visible = computed(() => {
  const terms = searchTerms(search.value)
  return session.companies.filter((c) => matches(terms, [c.name, c.ico, c.dic, c.icdph, c.city, c.bookkeeping === 'household' ? 'domácnosť' : '']))
})
function openCreate(household = false) {
  form.value = household
    ? { name: '', street: '', city: '', zip: '', vatPayer: false, bookkeeping: 'household' }
    : { name: '', ico: '', dic: '', icdph: '', street: '', city: '', zip: '', vatPayer: false }
  errors.value = {}
  creating.value = true
}

async function create() {
  busy.value = true
  errors.value = {}
  try {
    const c = await api.createCompany(form.value)
    session.companies.push(c)
    selectCompany(c.id)
    // nová firma: správca je jej členom → obnoviť práva
    session.me = await api.me()
    creating.value = false
    const hh = c.bookkeeping === 'household'
    toast.add({ severity: 'success', summary: hh ? 'Domácnosť založená' : 'Firma založená', detail: c.name, life: 3000 })
    router.push(hh ? '/domacnost/ucty' : '/nastavenia')
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Firmu nemožno založiť', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Firmy a domácnosti</h1>
      <span class="count">{{ count }}</span>
      <div class="spacer" />
      <ExportButton v-if="session.companies.length" name="Firmy" :columns="companyColumns" :rows="visible" />
      <Button size="small" icon="pi pi-home" label="Založiť domácnosť" severity="secondary" outlined @click="openCreate(true)" />
      <Button size="small" icon="pi pi-plus" label="Založiť firmu" @click="openCreate(false)" />
    </header>

    <p v-if="!count" class="welcome">
      Vitajte vo Fiškuse. Začnite založením firmy — IČO stačí zadať a údaje doplníme z registra.
    </p>

    <p v-if="count && !households.length" class="note">
      Domáce účtovníctvo (rodinný rozpočet, účty, výdavky) vediete v samostatnej domácnosti — tlačidlo „Založiť domácnosť“.
    </p>

    <IconField v-if="session.companies.length" class="search">
      <InputIcon class="pi pi-search" />
      <InputText v-model="search" placeholder="Hľadať firmu, IČO, obec" size="small" aria-label="Hľadať firmu" />
    </IconField>

    <table v-if="session.companies.length" class="companies">
      <thead>
        <tr><th>Firma</th><th>IČO</th><th>IČ DPH</th><th aria-label="Akcie" /></tr>
      </thead>
      <tbody>
        <tr v-if="!visible.length"><td colspan="4" class="empty">Žiadna firma nevyhovuje hľadaniu.</td></tr>
        <tr v-for="c in visible" :key="c.id" :class="{ current: c.id === session.companyId }">
          <td class="name" data-label="Firma">{{ c.name }}<small v-if="c.bookkeeping === 'household'" class="hh"> · domácnosť</small></td>
          <td data-label="IČO">{{ c.ico || '—' }}</td>
          <td data-label="IČ DPH">{{ c.icdph || '—' }}</td>
          <td class="act">
            <span v-if="c.id === session.companyId" class="badge">aktuálna</span>
            <Button v-else label="Prepnúť" text size="small" @click="selectCompany(c.id)" />
          </td>
        </tr>
      </tbody>
    </table>

    <Dialog v-model:visible="creating" :header="form?.bookkeeping === 'household' ? 'Založiť domácnosť' : 'Založiť firmu'" modal :style="{ width: '34rem' }" :breakpoints="{ '560px': '94vw' }">
      <form v-if="form && form.bookkeeping === 'household'" class="form-grid create" @submit.prevent="create">
        <FormField label="Názov domácnosti" :error="errors.name" :span="2"><InputText v-model="form.name" fluid placeholder="napr. Novákovci" autofocus /></FormField>
        <FormField label="Ulica" :error="errors.street" :span="2"><InputText v-model="form.street" fluid /></FormField>
        <FormField label="PSČ" :error="errors.zip"><InputText v-model="form.zip" fluid /></FormField>
        <FormField label="Obec" :error="errors.city"><InputText v-model="form.city" fluid /></FormField>
        <p class="hint">Domácnosť má vlastné účty, pohyby, kategórie a rozpočet — oddelene od firmy. Výbery zo živnosti do nej prenesiete v Pohyboch.</p>
        <div class="actions">
          <Button label="Zrušiť" severity="secondary" text size="small" @click="creating = false" />
          <Button type="submit" label="Založiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
      <form v-else-if="form" class="form-grid create" @submit.prevent="create">
        <FormField label="IČO" :error="errors.ico">
          <IcoInput v-model="form.ico" :invalid="!!errors.ico" auto @found="(c) => (form = fillFromRegistry(form, c))" />
        </FormField>
        <FormField label="DIČ" :error="errors.dic"><InputText v-model="form.dic" fluid /></FormField>
        <FormField label="Obchodné meno" :error="errors.name" :span="2"><InputText v-model="form.name" fluid /></FormField>
        <FormField label="Ulica" :error="errors.street" :span="2"><InputText v-model="form.street" fluid /></FormField>
        <FormField label="PSČ" :error="errors.zip"><InputText v-model="form.zip" fluid /></FormField>
        <FormField label="Obec" :error="errors.city"><InputText v-model="form.city" fluid /></FormField>
        <FormField label="IČ DPH" :error="errors.icdph" :span="2" hint="Vyplňte, ak je firma platiteľom DPH">
          <InputText v-model="form.icdph" fluid @update:model-value="(v) => (form.vatPayer = !!v)" />
        </FormField>
        <p class="hint">Ostatné údaje (IBAN, číslovanie, jednotky) doplníte v Nastaveniach.</p>
        <div class="actions">
          <Button label="Zrušiť" severity="secondary" text size="small" @click="creating = false" />
          <Button type="submit" label="Založiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 980px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.hh {
  color: var(--ink-faint);
  font-weight: 400;
}
.count {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.spacer {
  flex: 1;
}
.welcome,
.note {
  margin: 0;
  padding: 8px 12px;
  background: var(--blue-wash);
  border-left: 3px solid var(--blue);
  font-size: var(--fs-md);
}
.note {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
h2 {
  font-size: var(--fs-md);
  margin: 6px 0 6px;
  color: var(--ink-soft);
}
.search {
  max-width: 320px;
}
.search :deep(input) {
  width: 100%;
}
.empty {
  color: var(--ink-faint);
}
.companies {
  width: 100%;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  font-size: var(--fs-md);
}
.companies th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
}
.companies td {
  padding: 4px 10px;
  border-bottom: 1px solid var(--rule-soft);
  font-variant-numeric: tabular-nums;
}
.companies tr.current .name {
  font-weight: 600;
}
.act {
  text-align: right;
  width: 110px;
}
.badge {
  font-size: var(--fs-xs);
  padding: 0 6px;
  border-radius: 8px;
  background: var(--blue-wash);
  color: var(--blue-deep);
}
.create {
  grid-template-columns: 1fr 1fr;
}
.hint {
  grid-column: 1 / -1;
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 640px) {
  .plan-grid {
    grid-template-columns: 1fr;
  }
  .companies thead {
    display: none;
  }
  .companies,
  .companies tbody,
  .companies tr,
  .companies td {
    display: block;
    width: auto;
  }
  .companies tr {
    position: relative;
    padding: 6px 10px;
    border-bottom: 1px solid var(--rule);
  }
  .companies td {
    border: 0;
    padding: 0;
    font-size: var(--fs-sm);
  }
  .companies td.name {
    font-size: var(--fs-md);
  }
  .companies td[data-label='IČO']::before {
    content: 'IČO ';
    color: var(--ink-faint);
  }
  .companies td[data-label='IČ DPH']::before {
    content: 'IČ DPH ';
    color: var(--ink-faint);
  }
  .companies td.act {
    position: absolute;
    top: 6px;
    right: 6px;
    width: auto;
  }
}
</style>
