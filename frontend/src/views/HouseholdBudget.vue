<script setup>
// Domácnosť — kategórie a mesačný rozpočet. Pri výdavkoch je rozpočet
// najvyššia suma na mesiac, pri príjmoch očakávaná (nepovinné).
import { computed, onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { categoryErrors } from '../lib/household.js'
import { matches, searchTerms } from '../lib/search.js'

const toast = useToast()
const canWrite = computed(() => can('ledger.write'))
const cats = ref([])
const saved = ref('[]')
const errors = ref({})
const busy = ref(false)
const search = ref('')
const dirty = computed(() => JSON.stringify(cats.value) !== saved.value)

async function load() {
  try {
    const list = await api.hhCategories(session.companyId)
    cats.value = list.map((c) => ({ ...c, budget: Number(c.budget) ? String(c.budget).replace('.', ',') : '' }))
    saved.value = JSON.stringify(cats.value)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Kategórie sa nenačítali', detail: e.message, life: 6000 })
  }
}
onMounted(load)

const terms = computed(() => searchTerms(search.value))
const shown = (c) => matches(terms.value, [c.name, c.budget])
const income = computed(() => cats.value.map((c, i) => ({ c, i })).filter((x) => x.c.income && shown(x.c)))
const expense = computed(() => cats.value.map((c, i) => ({ c, i })).filter((x) => !x.c.income && shown(x.c)))

function add(isIncome) {
  cats.value.push({ name: '', income: isIncome, budget: '' })
}
function removeAt(i) {
  cats.value.splice(i, 1)
}
function move(i, d) {
  const j = i + d
  if (j < 0 || j >= cats.value.length || cats.value[j].income !== cats.value[i].income) return
  const [x] = cats.value.splice(i, 1)
  cats.value.splice(j, 0, x)
}

async function save() {
  errors.value = categoryErrors(cats.value)
  if (Object.keys(errors.value).length) return
  busy.value = true
  try {
    const body = cats.value.map((c) => ({ name: c.name.trim(), income: c.income, budget: String(c.budget ?? '').replace(/\s/g, '').replace(',', '.') || '0' }))
    await api.hhSaveCategories(session.companyId, body)
    toast.add({ severity: 'success', summary: 'Kategórie a rozpočet uložené', life: 2500 })
    await load()
  } catch (e) {
    const f = e instanceof ApiError ? e.fields?.[0] : null
    const m = f?.field?.match(/^categories\.(\d+)$/)
    if (m) errors.value = { [m[1]]: f.message }
    toast.add({ severity: 'error', summary: 'Neuložené', detail: f?.message ?? e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

const exportColumns = [
  { header: 'Kategória', value: (r) => r.name, width: 30 },
  { header: 'Druh', value: (r) => (r.income ? 'príjem' : 'výdavok'), width: 10 },
  { header: 'Mesačný rozpočet €', value: (r) => r.budget || '', type: 'money', width: 16 },
]
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Kategórie a rozpočet</h1>
      <span v-if="dirty" class="unsaved">neuložené zmeny</span>
      <div class="spacer" />
      <InputText v-model="search" placeholder="Hľadať kategóriu" size="small" aria-label="Hľadať kategóriu" class="search" />
      <ExportButton name="Kategórie domácnosti" :columns="exportColumns" :rows="cats.filter(shown)" />
      <Button v-if="canWrite" label="Uložiť" icon="pi pi-check" size="small" :disabled="!dirty" :loading="busy" @click="save" />
    </header>
    <p class="help">
      Rozpočet je suma na mesiac. Prehľad ukáže, koľko z neho v mesiaci ostáva a kde ste ho prekročili. Kategóriu bez rozpočtu nechajte
      prázdnu. Premenovanie kategórie nezmení staré pohyby — tie ostanú v prehľade pod starým názvom.
    </p>

    <div class="cols">
      <section v-for="grp in [{ t: 'Výdavky', rows: expense, inc: false }, { t: 'Príjmy', rows: income, inc: true }]" :key="grp.t" class="panel">
        <h2>{{ grp.t }}</h2>
        <table class="cats">
          <thead>
            <tr><th>Kategória</th><th class="num">{{ grp.inc ? 'Očakávané / mes.' : 'Rozpočet / mes.' }}</th><th aria-label="Akcie" /></tr>
          </thead>
          <tbody>
            <tr v-for="{ c, i } in grp.rows" :key="i">
              <td>
                <InputText v-model="c.name" size="small" fluid :invalid="!!errors[i]" :disabled="!canWrite" :aria-label="`Názov kategórie ${i + 1}`" />
                <small v-if="errors[i]" class="err">{{ errors[i] }}</small>
              </td>
              <td class="b">
                <InputText v-model="c.budget" size="small" inputmode="decimal" class="num" placeholder="—" :disabled="!canWrite" :aria-label="`Rozpočet ${c.name}`" />
              </td>
              <td class="act">
                <template v-if="canWrite">
                  <Button icon="pi pi-arrow-up" text size="small" aria-label="Vyššie" @click="move(i, -1)" />
                  <Button icon="pi pi-times" text size="small" severity="secondary" aria-label="Odstrániť" @click="removeAt(i)" />
                </template>
              </td>
            </tr>
          </tbody>
        </table>
        <Button v-if="canWrite" :label="grp.inc ? 'Pridať kategóriu príjmov' : 'Pridať kategóriu výdavkov'" icon="pi pi-plus" text size="small" @click="add(grp.inc)" />
      </section>
    </div>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 1100px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
}
.unsaved {
  color: var(--tone-draft);
  font-size: var(--fs-sm);
}
.spacer {
  flex: 1;
}
.search {
  width: 220px;
}
.help {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  max-width: 80ch;
}
.cols {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: 12px;
  align-items: start;
}
.panel {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 10px 12px;
}
.panel h2 {
  font-size: var(--fs-md);
  color: var(--blue-deep);
  margin-bottom: 6px;
}
.cats {
  width: 100%;
  border-collapse: collapse;
}
.cats th {
  text-align: left;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  font-weight: 600;
  padding: 2px 4px 4px;
}
.cats th.num {
  text-align: right;
}
.cats td {
  padding: 2px 4px;
  vertical-align: top;
}
.b {
  width: 130px;
}
.b :deep(input) {
  width: 100%;
  text-align: right;
}
.act {
  width: 72px;
  white-space: nowrap;
}
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-xs);
}
@media (max-width: 860px) {
  .cols {
    grid-template-columns: 1fr;
  }
  .search {
    width: 100%;
  }
  .b {
    width: 100px;
  }
}
</style>
