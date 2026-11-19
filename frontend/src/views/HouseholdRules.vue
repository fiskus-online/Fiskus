<script setup>
// Domácnosť — pravidlá kategórií: „text obsahuje LIDL → Potraviny“. Server
// ich použije pri návrhu kategórie (nový pohyb, import výpisu, Fio, bločky)
// pred odhadom z histórie; tu sa iba upravujú a dajú použiť na staršie pohyby.
import { computed, onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatDate, formatMoney } from '../lib/format.js'
import { entriesWord, ruleErrors } from '../lib/household.js'
import { matches, searchTerms } from '../lib/search.js'

const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))
const rules = ref([])
const categories = ref([])
const saved = ref('[]')
const errors = ref({})
const busy = ref(false)
const search = ref('')
const changes = ref([])
const dirty = computed(() => JSON.stringify(rules.value) !== saved.value)

const categoryGroups = computed(() => [
  { label: 'Výdavky', items: categories.value.filter((c) => !c.income).map((c) => c.name) },
  { label: 'Príjmy', items: categories.value.filter((c) => c.income).map((c) => c.name) },
])

async function load() {
  try {
    const [r, c] = await Promise.all([api.hhRules(session.companyId), api.hhCategories(session.companyId)])
    rules.value = r.map((x) => ({ text: x.text, category: x.category }))
    categories.value = c
    saved.value = JSON.stringify(rules.value)
    errors.value = ruleErrors(rules.value, c)
    changes.value = await api.hhRuleChanges(session.companyId)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Pravidlá sa nenačítali', detail: e.message, life: 6000 })
  }
}
onMounted(load)

const terms = computed(() => searchTerms(search.value))
const shown = (r) => matches(terms.value, [r.text, r.category])
const rows = computed(() => rules.value.map((r, i) => ({ r, i })).filter((x) => shown(x.r)))

function add() {
  rules.value.push({ text: '', category: null })
  search.value = ''
}
function removeAt(i) {
  rules.value.splice(i, 1)
}
function move(i, d) {
  const j = i + d
  if (j < 0 || j >= rules.value.length) return
  const [x] = rules.value.splice(i, 1)
  rules.value.splice(j, 0, x)
}

async function save() {
  errors.value = ruleErrors(rules.value, categories.value)
  if (Object.keys(errors.value).length) return
  busy.value = true
  try {
    await api.hhSaveRules(session.companyId, rules.value.map((r) => ({ text: r.text.trim(), category: r.category })))
    toast.add({ severity: 'success', summary: 'Pravidlá uložené', life: 2500 })
    await load()
  } catch (e) {
    const f = e instanceof ApiError ? e.fields?.[0] : null
    const m = f?.field?.match(/^rules\.(\d+)$/)
    if (m) errors.value = { [m[1]]: f.message }
    toast.add({ severity: 'error', summary: 'Neuložené', detail: f?.message ?? e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

function applyOld() {
  confirm.require({
    header: 'Zmeniť kategóriu starších pohybov',
    message: `Pravidlá zmenia kategóriu: ${entriesWord(changes.value.length)}. Každá zmena sa zapíše do auditu.`,
    acceptProps: { label: 'Zmeniť', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      busy.value = true
      try {
        const done = await api.hhApplyRules(session.companyId)
        toast.add({ severity: 'success', summary: `Zmenené: ${entriesWord(done.length)}`, life: 3000 })
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Kategórie sa nezmenili', detail: e.message, life: 6000 })
      } finally {
        busy.value = false
      }
    },
  })
}

const exportColumns = [
  { header: 'Text obsahuje', value: (r) => r.text, width: 30 },
  { header: 'Kategória', value: (r) => r.category ?? '', width: 30 },
]
const preview = computed(() => changes.value.slice(0, 50))
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Pravidlá kategórií</h1>
      <span v-if="dirty" class="unsaved">neuložené zmeny</span>
      <div class="spacer" />
      <InputText v-model="search" placeholder="Hľadať pravidlo" size="small" aria-label="Hľadať pravidlo" class="search" />
      <ExportButton name="Pravidlá kategórií" :columns="exportColumns" :rows="rules.filter(shown)" />
      <Button v-if="canWrite" label="Uložiť" icon="pi pi-check" size="small" :disabled="!dirty" :loading="busy" @click="save" />
    </header>
    <p class="help">
      Keď obchod, popis platby alebo položka bločku obsahuje text pravidla (bez ohľadu na diakritiku a veľké písmená, od začiatku slova), pohyb
      dostane jeho kategóriu. Pravidlá platia pri zápise pohybu, importe výpisu, Fio aj bločkoch a majú prednosť pred odhadom podľa starších
      pohybov. Ak sedí viac pravidiel, vyhráva vyššie.
    </p>

    <section class="panel">
      <table class="rules">
        <thead>
          <tr><th>Text obsahuje</th><th>Kategória</th><th aria-label="Akcie" /></tr>
        </thead>
        <tbody>
          <tr v-for="{ r, i } in rows" :key="i">
            <td>
              <InputText v-model="r.text" size="small" fluid placeholder="napr. LIDL" :invalid="!!errors[i]" :disabled="!canWrite"
                :aria-label="`Text pravidla ${i + 1}`" />
              <small v-if="errors[i]" class="err">{{ errors[i] }}</small>
            </td>
            <td class="c">
              <Select v-model="r.category" :options="categoryGroups" option-group-label="label" option-group-children="items" size="small" fluid filter
                placeholder="Kategória" :disabled="!canWrite" :aria-label="`Kategória pravidla ${i + 1}`" />
            </td>
            <td class="act">
              <template v-if="canWrite">
                <Button icon="pi pi-arrow-up" text size="small" aria-label="Vyššie" :disabled="i === 0" @click="move(i, -1)" />
                <Button icon="pi pi-times" text size="small" severity="secondary" aria-label="Odstrániť" @click="removeAt(i)" />
              </template>
            </td>
          </tr>
          <tr v-if="!rules.length">
            <td colspan="3" class="empty">Zatiaľ žiadne pravidlá. Kategóriu navrhuje Fiškus podľa starších pohybov s rovnakým obchodom.</td>
          </tr>
        </tbody>
      </table>
      <Button v-if="canWrite" label="Pridať pravidlo" icon="pi pi-plus" text size="small" @click="add" />
    </section>

    <section v-if="changes.length && !dirty" class="panel">
      <div class="old-head">
        <h2>Staršie pohyby s inou kategóriou</h2>
        <span class="spacer" />
        <Button v-if="canWrite" :label="`Zmeniť kategóriu (${changes.length})`" icon="pi pi-sync" size="small" :loading="busy" @click="applyOld" />
      </div>
      <p class="help">Prevody a bločky rozdelené na položky sa nemenia.</p>
      <table class="old">
        <tbody>
          <tr v-for="c in preview" :key="c.entry.id">
            <td class="d">{{ formatDate(c.entry.date) }}</td>
            <td class="p">{{ c.entry.payee || c.entry.note }}</td>
            <td class="num">{{ formatMoney(c.entry.amount) }}</td>
            <td class="to"><s>{{ c.entry.category }}</s> → {{ c.to }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="changes.length > preview.length" class="help">a ďalšie ({{ changes.length - preview.length }})</p>
    </section>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.head,
.old-head {
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
.panel {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 10px 12px;
  max-width: 900px;
}
.panel h2 {
  font-size: var(--fs-md);
  color: var(--blue-deep);
}
.rules,
.old {
  width: 100%;
  border-collapse: collapse;
}
.rules th {
  text-align: left;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  font-weight: 600;
  padding: 2px 4px 4px;
}
.rules td {
  padding: 2px 4px;
  vertical-align: top;
}
.c {
  width: 45%;
}
.act {
  width: 72px;
  white-space: nowrap;
}
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-xs);
}
.empty {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  padding: 8px 4px;
}
.old td {
  padding: 3px 6px 3px 0;
  border-top: 1px solid var(--rule);
  font-size: var(--fs-sm);
}
.old .d {
  white-space: nowrap;
  color: var(--ink-soft);
}
.old .num {
  text-align: right;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.old .to s {
  color: var(--ink-faint);
}
@media (max-width: 560px) {
  .search {
    width: 100%;
  }
  .act {
    width: 36px;
  }
  .act :deep(.p-button:first-child) {
    display: none;
  }
  .old .d {
    display: none;
  }
}
</style>
