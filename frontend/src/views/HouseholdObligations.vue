<script setup>
// Domácnosť — záväzky a platby: pôžičky, dlhy, exekúcie, pravidelné platby
// a požičané peniaze so splátkovým kalendárom. Stav a splátky počíta server.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Drawer from 'primevue/drawer'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import DatePicker from 'primevue/datepicker'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import ObligationPayDialog from '../components/ObligationPayDialog.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { householdObligationColumns, householdPlanColumns } from '../lib/columns.js'
import {
  categoriesFor, dueStatus, filterObligations, filterPlan, frequencies, ibanGroups, frequencyLabel, fromAmount, monthOptions, monthsLabel, newObligation, obligationIcon,
  obligationKindLabel, obligationKinds, obligationTone, paidPercent, planMonthLabel, toAmount,
} from '../lib/household.js'

const props = defineProps({ id: { type: [String, Number], default: null } })
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))

const list = ref([])
const accounts = ref([])
const categories = ref([])
const loading = ref(true)
const view = ref('open')
const all = computed(() => view.value === 'all')
const search = ref('')
const views = [{ label: 'Otvorené', value: 'open' }, { label: 'Všetky', value: 'all' }, { label: 'Plán po mesiacoch', value: 'plan' }]

async function load() {
  loading.value = true
  try {
    const [os, accs, cats] = await Promise.all([api.hhObligations(session.companyId), api.hhAccounts(session.companyId), api.hhCategories(session.companyId)])
    list.value = os
    accounts.value = accs
    categories.value = cats
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Záväzky sa nenačítali', detail: e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- plán platieb po mesiacoch (sumy počíta server) ----
const plan = ref(null)
const planFrom = ref(new Date(new Date().getFullYear(), new Date().getMonth(), 1))
const planMonths = ref(12)
const planLengths = [{ label: '3 mesiace', value: 3 }, { label: '6 mesiacov', value: 6 }, { label: '12 mesiacov', value: 12 }, { label: '24 mesiacov', value: 24 }]
async function loadPlan() {
  if (view.value !== 'plan') return
  const d = planFrom.value ?? new Date()
  const from = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  try {
    plan.value = await api.hhPlan(session.companyId, from, planMonths.value)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Plán platieb sa nenačítal', detail: e.message, life: 6000 })
  }
}
watch([view, planFrom, planMonths], loadPlan)
const planView = computed(() => filterPlan(plan.value, search.value))
function payItem(it) {
  const o = list.value.find((x) => x.id === it.obligationId)
  if (o) paying.value = { ...o, amount: it.rest }
}

const visible = computed(() => filterObligations(list.value, { all: all.value, search: search.value }))
// súčty zoznamu (už vypočítané zostatky): dlhy, po splatnosti, požičané
const sums = computed(() => {
  const c = (v) => Math.round(Number(v || 0) * 100)
  let owe = 0
  let late = 0
  let lent = 0
  for (const o of visible.value) {
    if (o.closed) continue
    if (o.kind === 'lent') lent += c(o.state?.remaining)
    else {
      owe += c(o.state?.remaining)
      late += c(o.state?.overdueAmount)
    }
  }
  return { owe: (owe / 100).toFixed(2), late: (late / 100).toFixed(2), lent: (lent / 100).toFixed(2) }
})

// ---- detail ----
const detail = ref(null)
async function loadDetail(id) {
  if (!id) {
    detail.value = null
    return
  }
  try {
    detail.value = await api.hhObligation(session.companyId, id)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Záväzok sa nenačítal', detail: e.message, life: 6000 })
    router.replace('/domacnost/zavazky')
  }
}
watch(() => props.id, loadDetail, { immediate: true })
const detailOpen = computed({ get: () => !!props.id, set: (v) => !v && router.push('/domacnost/zavazky') })
const accountName = (id) => accounts.value.find((a) => a.id === id)?.name ?? ''

// ---- zaplatiť ----
const paying = ref(null)
function pay(o) {
  paying.value = { ...o, amount: o.state?.nextAmount }
}
async function afterPay() {
  await load()
  await loadPlan()
  if (props.id) await loadDetail(props.id)
}

// ---- úprava ----
const form = ref(null)
const errors = ref({})
const busy = ref(false)
function edit(o) {
  errors.value = {}
  form.value = o
    ? { ...o, principal: fromAmount(o.principal), paidBefore: fromAmount(o.paidBefore), installment: fromAmount(o.installment), firstDue: fromISODate(o.firstDue), months: [...(o.months ?? [])] }
    : { ...newObligation('loan', accounts.value), firstDue: new Date() }
}
const formCats = computed(() => categoriesFor(categories.value, form.value?.kind === 'lent' ? 'income' : 'expense'))
const kindHint = computed(() => obligationKinds.find((k) => k.value === form.value?.kind)?.hint ?? '')
watch(
  () => form.value?.kind,
  () => {
    if (form.value && form.value.category && !formCats.value.includes(form.value.category)) form.value.category = ''
  },
)
async function save() {
  busy.value = true
  try {
    const f = form.value
    const body = {
      ...f, principal: toAmount(f.principal) || '0', paidBefore: toAmount(f.paidBefore) || '0', installment: toAmount(f.installment) || '0',
      firstDue: toISODate(f.firstDue), count: f.count || 0, accountId: f.accountId || 0, remindDays: f.remindDays || 0,
      months: f.frequency === 'monthly' ? f.months ?? [] : [],
    }
    delete body.state
    const saved = await api.hhSaveObligation(session.companyId, body)
    toast.add({ severity: 'success', summary: f.id ? 'Záväzok uložený' : 'Záväzok pridaný', life: 2000 })
    form.value = null
    await load()
    await loadPlan()
    if (props.id) await loadDetail(props.id)
    else if (!f.id) router.push(`/domacnost/zavazky/${saved.id}`)
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Záväzok sa neuložil', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
function remove(o) {
  confirm.require({
    header: 'Zmazať záväzok',
    message: `Zmazať ${o.name}? Záväzok so zaplatenými splátkami sa nedá zmazať — označte ho ako ukončený.`,
    acceptProps: { label: 'Zmazať', severity: 'danger', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.hhDeleteObligation(session.companyId, o.id)
        form.value = null
        router.push('/domacnost/zavazky')
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Záväzok sa nezmazal', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Záväzky a platby</h1>
      <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <div class="spacer" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať záväzok, veriteľa, spis" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton v-if="view === 'plan'" name="Plán platieb domácnosti" :columns="householdPlanColumns" :rows="planView.rows" />
      <ExportButton v-else name="Záväzky domácnosti" :columns="householdObligationColumns" :rows="visible" />
      <Button v-if="canWrite" label="Pridať záväzok" icon="pi pi-plus" size="small" @click="edit(null)" />
    </header>

    <!-- plán po mesiacoch -->
    <template v-if="view === 'plan'">
      <div class="plan-bar">
        <FormField label="Od mesiaca">
          <DatePicker v-model="planFrom" view="month" date-format="MM yy" size="small" :manual-input="false" />
        </FormField>
        <FormField label="Na">
          <Select v-model="planMonths" :options="planLengths" option-label="label" option-value="value" size="small" />
        </FormField>
      </div>
      <section v-if="plan" class="sums" aria-label="Súčty plánu">
        <div :class="{ late: Number(planView.overdueSum.pay) > 0 }"><span>Treba doplatiť (po splatnosti)</span><strong class="num">{{ formatMoney(planView.overdueSum.pay) }}</strong></div>
        <div><span>Zaplatiť za {{ planMonths }} mes.</span><strong class="num"><span class="ledger">{{ formatMoney(planView.total.pay) }}</span></strong></div>
        <div v-if="Number(planView.total.receive) || Number(planView.overdueSum.receive)"><span>Majú vám vrátiť</span><strong class="num">{{ formatMoney(planView.total.receive) }}</strong></div>
      </section>
      <p v-if="plan && !list.length" class="empty">Pridajte záväzky a pravidelné platby — plán ukáže, čo a kedy treba v ktorom mesiaci zaplatiť.</p>

      <section v-if="planView.overdue.length" class="pm late-m" aria-label="Po splatnosti">
        <h2>
          <span>Po splatnosti</span>
          <small v-if="Number(planView.overdueSum.receive)" class="inc">má vrátiť {{ formatAmount(planView.overdueSum.receive) }}</small>
          <strong v-if="Number(planView.overdueSum.pay)" class="num">doplatiť {{ formatMoney(planView.overdueSum.pay) }}</strong>
        </h2>
        <ul>
          <li v-for="it in planView.overdue" :key="`${it.obligationId}-${it.n}`">
            <span class="dt late">{{ formatDate(it.date) }}</span>
            <i class="pi kind" :class="obligationIcon(it.kind)" aria-hidden="true" />
            <div class="who"><b>{{ it.name }}</b><small>{{ [it.creditor, it.address].filter(Boolean).join(' · ') }}</small></div>
            <div class="bank"><small v-if="it.iban">{{ ibanGroups(it.iban) }}</small><small v-if="it.vs">VS {{ it.vs }}</small></div>
            <b class="num am" :class="{ inc: it.income }">{{ formatAmount(it.rest) }}</b>
            <Button v-if="canWrite" :label="it.income ? 'Prijať' : 'Zaplatiť'" size="small" @click="payItem(it)" />
          </li>
        </ul>
      </section>

      <section v-for="m in planView.months" :key="m.month" class="pm" :aria-label="planMonthLabel(m.month)">
        <h2>
          <span>{{ planMonthLabel(m.month) }}</span>
          <small v-if="Number(m.receive)" class="inc">prijať {{ formatAmount(m.receive) }}</small>
          <strong class="num"><span class="ledger">{{ formatMoney(m.pay) }}</span></strong>
        </h2>
        <p v-if="!m.items.length" class="none">Žiadna platba.</p>
        <ul v-else>
          <li v-for="it in m.items" :key="`${it.obligationId}-${it.n}`">
            <span class="dt">{{ formatDate(it.date) }}</span>
            <i class="pi kind" :class="obligationIcon(it.kind)" aria-hidden="true" />
            <div class="who"><b>{{ it.name }}</b><small>{{ [it.creditor, it.address].filter(Boolean).join(' · ') }}</small></div>
            <div class="bank"><small v-if="it.iban">{{ ibanGroups(it.iban) }}</small><small v-if="it.vs">VS {{ it.vs }}</small></div>
            <b class="num am" :class="{ inc: it.income }">{{ formatAmount(it.rest) }}</b>
            <Button v-if="canWrite" :label="it.income ? 'Prijať' : 'Zaplatiť'" size="small" severity="secondary" outlined @click="payItem(it)" />
          </li>
        </ul>
      </section>
    </template>

    <template v-else>
      <section v-if="list.length" class="sums" aria-label="Súčty">
        <div><span>Zostáva splatiť</span><strong class="num"><span class="ledger">{{ formatMoney(sums.owe) }}</span></strong></div>
        <div :class="{ late: Number(sums.late) > 0 }"><span>Po splatnosti</span><strong class="num">{{ formatMoney(sums.late) }}</strong></div>
        <div v-if="Number(sums.lent) > 0"><span>Majú vám vrátiť</span><strong class="num">{{ formatMoney(sums.lent) }}</strong></div>
      </section>

      <p v-if="!loading && !list.length" class="empty">
        Sem patria pôžičky a úvery, dlhy, exekúcie, ale aj pravidelné platby (nájom, energie, poistky) a peniaze, ktoré ste požičali. Fiškus
        z nich zostaví splátkový kalendár, ukáže splátky v kalendári a pripomenie ich e-mailom pred splatnosťou.
      </p>
      <p v-else-if="!loading && !visible.length" class="empty">Žiadny záväzok nevyhovuje. <button class="lnk" type="button" @click="(view = 'all'), (search = '')">Zobraziť všetky</button></p>

      <ul class="obls">
        <li v-for="o in visible" :key="o.id" :class="{ closed: o.closed || o.state.paidOff }" @click="router.push(`/domacnost/zavazky/${o.id}`)">
          <i class="pi kind" :class="obligationIcon(o.kind)" aria-hidden="true" />
          <div class="main">
            <span class="name">{{ o.name }}</span>
            <small>{{ obligationKindLabel(o.kind) }}<template v-if="o.creditor"> · {{ o.creditor }}</template> · {{ formatMoney(o.installment) }} {{ frequencyLabel(o.frequency).toLowerCase() }}<template v-if="monthsLabel(o.months)"> ({{ monthsLabel(o.months) }})</template></small>
            <span v-if="paidPercent(o) !== null" class="prog" :title="`splatené ${paidPercent(o)} %`"><span :style="{ width: paidPercent(o) + '%' }" /></span>
          </div>
          <div class="rest">
            <template v-if="Number(o.principal)">
              <small>{{ o.kind === 'lent' ? 'má vrátiť' : 'zostáva' }}</small>
              <strong class="num">{{ formatMoney(o.state.remaining) }}</strong>
            </template>
          </div>
          <div class="next">
            <template v-if="o.state.nextDue && !o.closed">
              <small>{{ o.state.overdue ? 'po splatnosti od' : 'najbližšie' }}</small>
              <span :class="{ late: o.state.overdue }">{{ formatDate(o.state.nextDue) }} · <b class="num">{{ formatAmount(o.state.overdue ? o.state.overdueAmount : o.state.nextAmount) }}</b></span>
            </template>
            <span v-else class="tag" :data-tone="obligationTone(o).tone"><span class="tag-text">{{ obligationTone(o).label }}</span></span>
          </div>
          <div class="act" @click.stop>
            <Button v-if="canWrite && !o.closed && !o.state.paidOff" :label="o.kind === 'lent' ? 'Prijať' : 'Zaplatiť'" size="small"
              :severity="o.state.overdue ? undefined : 'secondary'" :outlined="!o.state.overdue" @click="pay(o)" />
          </div>
        </li>
      </ul>
    </template>

    <!-- detail záväzku -->
    <Drawer v-model:visible="detailOpen" position="right" class="obl-panel" :header="detail?.name ?? 'Záväzok'">
      <template v-if="detail">
        <div class="d-head">
          <span class="tag" :data-tone="obligationTone(detail).tone"><span class="tag-text">{{ obligationTone(detail).label }}</span></span>
          <div class="spacer" />
          <Button v-if="canWrite" label="Upraviť" icon="pi pi-pencil" text size="small" @click="edit(detail)" />
          <Button v-if="canWrite && !detail.closed && !detail.state.paidOff" :label="detail.kind === 'lent' ? 'Prijať' : 'Zaplatiť'" icon="pi pi-check" size="small"
            @click="pay(detail)" />
        </div>
        <dl class="facts">
          <dt>Druh</dt><dd>{{ obligationKindLabel(detail.kind) }}</dd>
          <template v-if="detail.creditor"><dt>{{ detail.kind === 'lent' ? 'Kto vracia' : 'Komu' }}</dt><dd>{{ detail.creditor }}</dd></template>
          <template v-if="Number(detail.principal)"><dt>Celkom</dt><dd class="num">{{ formatMoney(detail.principal) }}</dd></template>
          <dt>Zaplatené</dt><dd class="num">{{ formatMoney(detail.state.paid) }}</dd>
          <template v-if="Number(detail.principal)"><dt>Zostáva</dt><dd class="num"><span class="ledger">{{ formatMoney(detail.state.remaining) }}</span></dd></template>
          <dt>Splátka</dt><dd>{{ formatMoney(detail.installment) }} {{ frequencyLabel(detail.frequency).toLowerCase() }}<template v-if="detail.state.left"> · zostáva {{ detail.state.left }}</template></dd>
          <template v-if="monthsLabel(detail.months)"><dt>Platí sa</dt><dd>{{ monthsLabel(detail.months) }}</dd></template>
          <dt>Pripomenúť</dt><dd>{{ detail.remindDays ? `${detail.remindDays} ${detail.remindDays === 1 ? 'deň' : detail.remindDays < 5 ? 'dni' : 'dní'} vopred` : 'podľa nastavenia upozornení' }}</dd>
          <template v-if="detail.accountId"><dt>Z účtu</dt><dd>{{ accountName(detail.accountId) }}</dd></template>
          <template v-if="detail.reference"><dt>Zmluva / spis</dt><dd>{{ detail.reference }}</dd></template>
          <template v-if="detail.iban"><dt>IBAN</dt><dd class="mono">{{ ibanGroups(detail.iban) }}</dd></template>
          <template v-if="detail.vs"><dt>VS</dt><dd>{{ detail.vs }}</dd></template>
          <template v-if="detail.address"><dt>Adresa</dt><dd>{{ detail.address }}</dd></template>
          <template v-if="detail.note"><dt>Poznámka</dt><dd>{{ detail.note }}</dd></template>
        </dl>

        <h3>Splátkový kalendár</h3>
        <p v-if="!detail.schedule.length" class="empty">Bez splátok.</p>
        <table v-else class="sched">
          <thead><tr><th>#</th><th>Splatnosť</th><th class="num">Suma</th><th class="num">Zaplatené</th><th>Stav</th></tr></thead>
          <tbody>
            <tr v-for="d in detail.schedule" :key="d.n" :data-tone="dueStatus(d.status).tone">
              <td class="n">{{ d.n }}</td>
              <td>{{ formatDate(d.date) }}</td>
              <td class="num">{{ formatAmount(d.amount) }}</td>
              <td class="num">{{ Number(d.paid) ? formatAmount(d.paid) : '' }}</td>
              <td><span class="tag" :data-tone="dueStatus(d.status).tone"><span class="tag-text">{{ dueStatus(d.status).label }}</span></span></td>
            </tr>
          </tbody>
        </table>
        <p v-if="Number(detail.paidBefore)" class="note">Pred evidenciou vo Fiškuse splatené {{ formatMoney(detail.paidBefore) }} — kalendár začína prvou splátkou v evidencii.</p>

        <h3>Zaplatené</h3>
        <p v-if="!detail.payments.length" class="empty">Zatiaľ žiadna splátka.</p>
        <table v-else class="sched">
          <tbody>
            <tr v-for="p in detail.payments" :key="p.id">
              <td>{{ formatDate(p.date) }}</td>
              <td>{{ accountName(p.accountId) }}</td>
              <td class="num">{{ formatAmount(p.amount) }}</td>
            </tr>
          </tbody>
        </table>
      </template>
    </Drawer>

    <!-- úprava -->
    <Dialog :visible="!!form" :header="form?.id ? 'Upraviť záväzok' : 'Nový záväzok'" modal :style="{ width: '40rem' }" :breakpoints="{ '640px': '100vw' }"
      @update:visible="(v) => !v && (form = null)">
      <form v-if="form" class="form-grid obl-form" @submit.prevent="save">
        <FormField label="Druh" :span="4" :hint="kindHint" :error="errors.kind">
          <SelectButton v-model="form.kind" :options="obligationKinds" option-label="label" option-value="value" :allow-empty="false" size="small" class="kinds" />
        </FormField>
        <FormField label="Názov" :span="2" :error="errors.name">
          <InputText v-model="form.name" size="small" :placeholder="form.kind === 'execution' ? 'napr. Exekúcia EX 845/2024' : form.kind === 'regular' ? 'napr. Nájom bytu' : form.kind === 'fine' ? 'napr. Pokuta za parkovanie' : 'napr. Hypotéka VÚB'" />
        </FormField>
        <FormField :label="form.kind === 'lent' ? 'Kto vracia' : form.kind === 'execution' ? 'Exekútor' : 'Komu platím'" :span="2" :error="errors.creditor">
          <InputText v-model="form.creditor" size="small" />
        </FormField>
        <FormField v-if="form.kind !== 'regular'" :label="form.kind === 'execution' ? 'Vymáhaná suma vrátane trov' : form.kind === 'lent' ? 'Požičaná suma' : form.kind === 'fine' ? 'Suma spolu' : 'Dlžná suma'" :error="errors.principal">
          <InputText v-model="form.principal" size="small" inputmode="decimal" class="num" placeholder="0,00" />
        </FormField>
        <FormField v-if="form.kind !== 'regular'" label="Splatené skôr" :error="errors.paidBefore" hint="pred evidenciou vo Fiškuse">
          <InputText v-model="form.paidBefore" size="small" inputmode="decimal" class="num" placeholder="0,00" />
        </FormField>
        <FormField label="Splátka" :error="errors.installment">
          <InputText v-model="form.installment" size="small" inputmode="decimal" class="num" placeholder="0,00" />
        </FormField>
        <FormField label="Opakovanie" :error="errors.frequency">
          <Select v-model="form.frequency" :options="frequencies" option-label="label" option-value="value" size="small" fluid />
        </FormField>
        <FormField label="Prvá splátka" :error="errors.firstDue" hint="splatnosť prvej splátky v evidencii">
          <DatePicker v-model="form.firstDue" date-format="d. m. yy" size="small" fluid />
        </FormField>
        <FormField v-if="form.frequency === 'monthly'" label="Platí sa v mesiacoch" :span="4" :error="errors.months" hint="nič nevybraté = každý mesiac; napr. škola september – jún">
          <SelectButton v-model="form.months" :options="monthOptions" option-label="label" option-value="value" multiple size="small" class="months" aria-label="Mesiace" />
        </FormField>
        <FormField v-if="form.frequency !== 'once'" label="Počet splátok" :error="errors.count" :hint="form.kind === 'regular' ? '0 = bez konca' : '0 = kým sa nesplatí'">
          <InputNumber v-model="form.count" :min="0" :max="1200" size="small" fluid />
        </FormField>
        <FormField label="Kategória" :span="2" :error="errors.category" hint="prázdna = podľa druhu">
          <Select v-model="form.category" :options="formCats" size="small" fluid show-clear placeholder="podľa druhu" />
        </FormField>
        <FormField label="Účet" :span="2" :error="errors.accountId">
          <Select v-model="form.accountId" :options="accounts.filter((a) => !a.closed || a.id === form.accountId)" option-label="name" option-value="id" size="small" fluid show-clear placeholder="vybrať pri platbe" />
        </FormField>
        <FormField :label="form.kind === 'execution' ? 'Spisová značka' : 'Číslo zmluvy'" :span="2" :error="errors.reference">
          <InputText v-model="form.reference" size="small" />
        </FormField>
        <FormField label="Variabilný symbol" :error="errors.vs">
          <InputText v-model="form.vs" size="small" inputmode="numeric" />
        </FormField>
        <FormField label="IBAN" :error="errors.iban">
          <InputText v-model="form.iban" size="small" />
        </FormField>
        <FormField label="Adresa príjemcu" :span="3" :error="errors.address" hint="pri platbe poštovou poukážkou alebo šekom">
          <InputText v-model="form.address" size="small" />
        </FormField>
        <FormField label="Pripomenúť (dní vopred)" :error="errors.remindDays" hint="0 = podľa nastavenia">
          <InputNumber v-model="form.remindDays" :min="0" :max="60" size="small" fluid />
        </FormField>
        <FormField label="Poznámka" :span="4" :error="errors.note">
          <InputText v-model="form.note" size="small" />
        </FormField>
        <FormField v-if="form.id" label="Ukončený" :span="2" hint="splatený alebo zrušený — nepripomína sa">
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

    <ObligationPayDialog :target="paying" :accounts="accounts" @close="paying = null" @paid="afterPay" />
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
  gap: 8px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
}
.search {
  width: 240px;
}
.search :deep(input) {
  width: 100%;
}
.empty {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  max-width: 75ch;
}
.lnk {
  border: 0;
  background: none;
  color: var(--blue);
  text-decoration: underline;
  cursor: pointer;
  font: inherit;
  padding: 0;
}
.sums {
  display: flex;
  gap: 0;
  flex-wrap: wrap;
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.sums div {
  flex: 1 1 160px;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--rule-soft);
}
.sums div > span {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.sums strong {
  font-size: var(--fs-lg);
  text-align: left;
}
.sums .late strong {
  color: var(--tone-overdue);
}
.ledger {
  border-bottom: 3px double currentColor;
}
.obls {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.obls li {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr) 130px 190px 96px;
  gap: 10px;
  align-items: center;
  padding: 9px 12px;
  border-bottom: 1px solid var(--rule-soft);
  cursor: pointer;
}
.obls li:last-child {
  border-bottom: 0;
}
.obls li:hover {
  background: var(--blue-wash);
}
.obls li.closed {
  opacity: 0.6;
}
.kind {
  color: var(--blue);
}
.main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.name {
  font-weight: 600;
}
.main small,
.rest small,
.next small {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.prog {
  height: 4px;
  background: var(--rule-soft);
  margin-top: 3px;
  max-width: 320px;
}
.prog span {
  display: block;
  height: 100%;
  background: var(--tone-paid);
}
.rest,
.next {
  display: flex;
  flex-direction: column;
}
.rest {
  text-align: right;
}
.rest strong {
  font-variant-numeric: tabular-nums;
}
.next .late {
  color: var(--tone-overdue);
  font-weight: 600;
}
.act {
  text-align: right;
}
.d-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 10px;
}
.facts {
  display: grid;
  grid-template-columns: 110px 1fr;
  gap: 4px 10px;
  font-size: var(--fs-sm);
  margin: 0 0 14px;
}
.facts dt {
  color: var(--ink-soft);
}
.facts dd {
  margin: 0;
  text-align: left;
}
.mono {
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}
h3 {
  font-size: var(--fs-md);
  color: var(--blue-deep);
  margin: 12px 0 6px;
}
.sched {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
}
.sched th {
  text-align: left;
  color: var(--ink-soft);
  font-weight: 600;
  border-bottom: 1px solid var(--rule);
  padding: 3px 4px;
}
.sched th.num {
  text-align: right;
}
.sched td {
  padding: 4px;
  white-space: nowrap;
  border-bottom: 1px solid var(--rule-soft);
}
.sched .n {
  color: var(--ink-faint);
  width: 26px;
}
.sched tr[data-tone='paid'] td {
  color: var(--ink-faint);
}
.note {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  margin-top: 6px;
}
.obl-form {
  grid-template-columns: repeat(4, minmax(0, 1fr));
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
.plan-bar {
  display: flex;
  gap: 10px;
  align-items: flex-end;
}
.pm {
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.pm h2 {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin: 0;
  padding: 7px 12px;
  font-size: var(--fs-md);
  color: var(--blue-deep);
  border-bottom: 1px solid var(--ink);
  text-transform: capitalize;
}
.pm h2 > span {
  flex: 1;
}
.pm h2 small {
  font-weight: 400;
  font-size: var(--fs-sm);
  text-transform: none;
}
.pm.late-m h2 {
  color: var(--tone-overdue);
  text-transform: none;
}
.pm ul {
  list-style: none;
  margin: 0;
  padding: 0;
}
.pm li {
  display: grid;
  grid-template-columns: 82px 20px minmax(0, 1fr) 230px 90px 92px;
  gap: 10px;
  align-items: center;
  padding: 6px 12px;
  border-bottom: 1px solid var(--rule-soft);
}
.pm li:last-child {
  border-bottom: 0;
}
.pm .dt {
  font-variant-numeric: tabular-nums;
}
.pm .dt.late {
  color: var(--tone-overdue);
  font-weight: 600;
}
.pm .who,
.pm .bank {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.pm .who small,
.pm .bank small {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pm .bank small:first-child {
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}
.pm .am {
  text-align: right;
}
.pm .inc {
  color: var(--tone-paid);
}
.pm .none {
  margin: 0;
  padding: 6px 12px;
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.months {
  flex-wrap: wrap;
}
@media (max-width: 760px) {
  .pm li {
    grid-template-columns: 70px minmax(0, 1fr) auto;
    grid-template-areas: 'd w a' 'd b p';
    row-gap: 2px;
  }
  .pm li .kind {
    display: none;
  }
  .pm .dt {
    grid-area: d;
    align-self: start;
  }
  .pm .who {
    grid-area: w;
  }
  .pm .bank {
    grid-area: b;
  }
  .pm .am {
    grid-area: a;
  }
  .pm li .p-button {
    grid-area: p;
  }
  .search {
    width: 100%;
  }
  .obls li {
    grid-template-columns: 22px minmax(0, 1fr) auto;
    grid-template-areas: 'k m r' 'k n a';
  }
  .kind {
    grid-area: k;
    align-self: start;
    padding-top: 3px;
  }
  .main {
    grid-area: m;
  }
  .rest {
    grid-area: r;
  }
  .next {
    grid-area: n;
  }
  .act {
    grid-area: a;
  }
  .obl-form {
    grid-template-columns: 1fr 1fr;
  }
}
</style>

<style>
.obl-panel.p-drawer {
  width: 34rem;
}
@media (max-width: 600px) {
  .obl-panel.p-drawer {
    width: 100vw !important;
  }
}
</style>
