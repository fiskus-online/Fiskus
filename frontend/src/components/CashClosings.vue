<script setup>
// Uzávierky e-kasy: obrat z dennej (intervalovej) uzávierky sa zapíše do
// pokladne ako PPD na celú tržbu a VPD na platby kartou. Sumy počíta server.
import { computed, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Drawer from 'primevue/drawer'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import DatePicker from 'primevue/datepicker'
import Checkbox from 'primevue/checkbox'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from './FormField.vue'
import ExportButton from './ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { formatAmount, formatDate, formatMoney, toISODate } from '../lib/format.js'
import { ratesOn } from '../lib/codes.js'
import { closingColumns } from '../lib/columns.js'
import { blankClosing, closingPayload, closingTotal, errorMap, filterClosings } from '../lib/closings.js'

const props = defineProps({
  cid: { type: Number, required: true },
  year: { type: Number, required: true },
  registerId: { type: Number, default: 0 },
  canWrite: { type: Boolean, default: false },
  vatPayer: { type: Boolean, default: true },
})
const visible = defineModel('visible', { type: Boolean, default: false })
const emit = defineEmits(['changed'])
const toast = useToast()
const confirm = useConfirm()

const list = ref([])
const q = ref('')
const form = ref(null)
const date = ref(new Date())
const preview = ref(null)
const busy = ref(false)

async function load() {
  try {
    list.value = await api.cashClosings(props.cid, props.year)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Uzávierky sa nenačítali', detail: e.message, life: 5000 })
  }
}
watch(visible, (v) => v && load())
watch(() => props.year, () => visible.value && load())

const shown = computed(() => filterClosings(list.value, q.value))

function openNew() {
  date.value = new Date()
  const iso = toISODate(date.value)
  form.value = blankClosing(iso, props.vatPayer ? ratesOn(iso) : [0], props.registerId)
  if (!props.vatPayer) form.value.rows = [{ vatRate: 0, gross: '' }]
  preview.value = null
}

let timer = null
let seq = 0
watch(
  () => form.value && [form.value, date.value],
  () => {
    clearTimeout(timer)
    if (!form.value) return
    timer = setTimeout(async () => {
      const my = ++seq
      try {
        const p = await api.previewCashClosing(props.cid, closingPayload(form.value, toISODate(date.value)))
        if (my === seq) preview.value = p
      } catch (e) {
        if (my === seq) preview.value = { errors: [{ field: '', message: e.message }] }
      }
    }, 250)
  },
  { deep: true },
)
const errs = computed(() => errorMap(preview.value?.errors))
const showErrs = ref(false)

async function save() {
  showErrs.value = true
  busy.value = true
  try {
    const c = await api.createCashClosing(props.cid, closingPayload(form.value, toISODate(date.value)))
    toast.add({ severity: 'success', summary: 'Uzávierka zapísaná do pokladne', detail: [c.inNumber, c.outNumber].filter(Boolean).join(', '), life: 4000 })
    form.value = null
    showErrs.value = false
    await load()
    emit('changed')
  } catch (e) {
    const detail = e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message
    toast.add({ severity: 'error', summary: 'Uzávierka sa nezapísala', detail, life: 6000 })
  } finally {
    busy.value = false
  }
}

function cancel(c) {
  confirm.require({
    header: 'Stornovať uzávierku',
    message: `Stornovať uzávierku ${c.number || formatDate(c.date)} aj doklady ${[c.inNumber, c.outNumber].filter(Boolean).join(' a ')}?`,
    acceptLabel: 'Stornovať',
    rejectLabel: 'Späť',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await api.cancelCashClosing(props.cid, c.id)
        await load()
        emit('changed')
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Storno zlyhalo', detail: e.message, life: 6000 })
      }
    },
  })
}
const rateLabel = (r) => (props.vatPayer ? `${r} %` : 'Tržba')
</script>

<template>
  <Drawer v-model:visible="visible" position="right" class="closings-panel" :header="`Uzávierky e-kasy ${year}`">
    <div class="cl">
      <template v-if="form">
        <p class="lead">Opíšte údaje z dennej alebo intervalovej uzávierky pokladnice e-kasa klient. Fiškus zapíše do pokladne príjmový doklad na celú tržbu{{ vatPayer ? ' s DPH podľa sadzieb' : '' }} a výdavkový doklad na platby kartou, ktoré prídu na účet.</p>
        <form class="form-grid" @submit.prevent="save">
          <FormField label="Dátum uzávierky" :error="showErrs ? errs.date : ''">
            <DatePicker v-model="date" date-format="d. m. yy" size="small" fluid />
          </FormField>
          <FormField label="Číslo uzávierky" :error="showErrs ? errs.number : ''">
            <InputText v-model="form.number" size="small" />
          </FormField>
          <FormField label="Kód pokladnice (ORP)" hint="nepovinné">
            <InputText v-model="form.orp" size="small" inputmode="numeric" />
          </FormField>
          <FormField label="Počet dokladov" hint="nepovinné">
            <InputNumber v-model="form.receipts" :min="0" size="small" />
          </FormField>

          <FormField :label="vatPayer ? 'Obrat s DPH podľa sadzieb' : 'Obrat (tržba spolu)'" :span="2" :error="showErrs ? errs.rows : ''"
            :hint="vatPayer ? 'po odpočítaní vrátených a stornovaných položiek' : ''">
            <table class="rates">
              <thead v-if="vatPayer"><tr><th>Sadzba</th><th>S DPH</th><th>Základ</th><th>DPH</th></tr></thead>
              <tbody>
                <tr v-for="(r, i) in form.rows" :key="i">
                  <td>{{ rateLabel(r.vatRate) }}</td>
                  <td><InputText v-model="r.gross" size="small" inputmode="decimal" :aria-label="`Obrat ${rateLabel(r.vatRate)}`" :invalid="showErrs && !!errs[`rows.${i}.gross`]" /></td>
                  <template v-if="vatPayer">
                    <td class="num">{{ formatAmount(preview?.rows?.find((x) => x.vatRate === r.vatRate)?.base ?? '') }}</td>
                    <td class="num">{{ formatAmount(preview?.rows?.find((x) => x.vatRate === r.vatRate)?.vat ?? '') }}</td>
                  </template>
                </tr>
              </tbody>
            </table>
          </FormField>
          <FormField label="Z toho kartou a bezhotovostne" :error="showErrs ? errs.card : ''" hint="peniaze na ceste na účet">
            <InputText v-model="form.card" size="small" inputmode="decimal" placeholder="0" />
          </FormField>
          <FormField label="Druh tržby">
            <label class="chk"><Checkbox v-model="form.goods" binary /> predaj tovaru</label>
          </FormField>
          <FormField label="Poznámka" :span="2">
            <InputText v-model="form.note" size="small" />
          </FormField>

          <dl v-if="preview && preview.total" class="sum" aria-live="polite">
            <div><dt>Tržba spolu</dt><dd>{{ formatMoney(preview.total) }}</dd></div>
            <div><dt>Kartou na účet (VPD)</dt><dd>{{ formatMoney(preview.card || '0') }}</dd></div>
            <div class="total"><dt>V hotovosti v pokladni</dt><dd><span class="grand">{{ formatMoney(preview.cashPart) }}</span></dd></div>
          </dl>

          <div class="actions">
            <Button label="Späť" text size="small" severity="secondary" @click="form = null" />
            <span class="spacer" />
            <Button type="submit" label="Zapísať do pokladne" icon="pi pi-book" size="small" :loading="busy" />
          </div>
        </form>
      </template>

      <template v-else>
        <div class="tbar">
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="q" size="small" placeholder="Hľadať číslo, ORP, doklad, sumu" aria-label="Hľadať v uzávierkach" />
          </IconField>
          <ExportButton name="uzavierky-ekasa" :columns="closingColumns" :rows="shown" />
          <Button v-if="canWrite" label="Nová uzávierka" icon="pi pi-plus" size="small" @click="openNew" />
        </div>
        <p v-if="!shown.length" class="muted">{{ q ? 'Nič sa nenašlo.' : 'Zatiaľ žiadne uzávierky e-kasy v tomto roku.' }}</p>
        <ul v-else class="list">
          <li v-for="c in shown" :key="c.id" :class="{ off: c.cancelled }">
            <div class="main">
              <b>{{ formatDate(c.date) }}<template v-if="c.number">, uzávierka {{ c.number }}</template></b>
              <small>{{ [c.orp && `ORP ${c.orp}`, c.receipts && `${c.receipts} dokladov`, c.inNumber, c.outNumber, c.cancelled && 'stornovaná'].filter(Boolean).join(', ') }}</small>
            </div>
            <div class="amt">
              <b>{{ formatMoney(closingTotal(c)) }}</b>
              <small v-if="Number(c.card)">kartou {{ formatMoney(c.card) }}</small>
            </div>
            <Button v-if="canWrite && !c.cancelled" icon="pi pi-ban" text rounded size="small" severity="secondary" :aria-label="`Stornovať uzávierku ${c.number}`"
              v-tooltip.left="'Stornovať uzávierku aj jej doklady'" @click="cancel(c)" />
          </li>
        </ul>
        <p class="note">Platby kartou banka pripíše na účet obvykle o deň-dva. V banke ich Fiškus spozná podľa textu (platby kartou, terminál, Global Payments…) a v peňažnom denníku aj v podvojnom účtovníctve ich zaúčtuje ako peniaze na ceste (261), nie ako ďalšiu tržbu.</p>
      </template>
    </div>
  </Drawer>
</template>

<style scoped>
.cl {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.lead,
.note {
  margin: 0;
  color: var(--ink-soft, #4a5866);
  font-size: 0.9rem;
}
.note {
  font-size: 0.82rem;
  margin-top: 0.5rem;
}
.form-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.rates {
  border-collapse: collapse;
  width: 100%;
  font-variant-numeric: tabular-nums;
}
.rates th {
  font-weight: 500;
  font-size: 0.8rem;
  color: #4a5a6c;
  text-align: right;
  padding: 0 0.4rem 0.2rem;
}
.rates th:nth-child(2) {
  text-align: left;
}
.rates th:first-child {
  text-align: left;
  padding-left: 0;
}
.rates td {
  padding: 0.15rem 0.4rem;
}
.rates td:first-child {
  padding-left: 0;
  white-space: nowrap;
  width: 4rem;
}
.rates td :deep(input) {
  width: 100%;
  max-width: 9rem;
  text-align: right;
}
.rates .num {
  text-align: right;
  color: var(--ink-soft, #4a5866);
  white-space: nowrap;
}
.chk {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  min-height: 2rem;
}
.sum {
  grid-column: 1 / -1;
  margin: 0.25rem 0 0;
  border-top: 1px solid var(--line, #d8dee5);
  padding-top: 0.5rem;
}
.sum div {
  display: flex;
  justify-content: space-between;
  padding: 0.15rem 0;
}
.sum dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
}
.sum .total {
  font-weight: 700;
  padding-top: 0.35rem;
}
.grand {
  border-bottom: 3px double currentColor;
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  gap: 0.5rem;
  align-items: center;
}
.spacer {
  flex: 1;
}
.tbar {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.search {
  flex: 1 1 12rem;
}
.search :deep(input) {
  width: 100%;
}
.list {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--line, #d8dee5);
}
.list li {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.5rem 0;
  border-bottom: 1px solid var(--line, #d8dee5);
}
.list li.off {
  opacity: 0.55;
}
.list li.off b {
  text-decoration: line-through;
}
.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.main small,
.amt small {
  color: var(--ink-faint, #8a96a3);
  font-size: 0.8rem;
}
.amt {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.muted {
  color: var(--ink-faint, #8a96a3);
}
@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .form-grid > :deep(*) {
    grid-column: auto;
  }
}
</style>

<style>
.p-drawer.p-component.closings-panel {
  width: min(560px, 100vw);
}
</style>
