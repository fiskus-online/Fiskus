<script setup>
// Bloček do domácnosti: fotka → QR kód eKasa (položky z Finančnej správy)
// alebo čítanie fotky cez Claude API → položky s kategóriami na kontrolu →
// pohyb rozdelený na položky s fotkou bločku. Kategórie navrhne server.
import { computed, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { categoriesFor, setAllCategory, splitByCategory } from '../lib/household.js'
import { imageReader, loadImage, receiptId, shrinkImage } from '../lib/receipt.js'
import { decodeQR } from '../lib/qrscan.js'

const props = defineProps({
  visible: { type: Boolean, default: false },
  accounts: { type: Array, default: () => [] },
  categories: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:visible', 'saved'])
const toast = useToast()

const step = ref('pick') // pick | busy | review
const busyText = ref('')
const error = ref('')
const code = ref('')
const draft = ref(null) // { entry, from, warnings }
const photo = ref(null) // zmenšená fotka (Blob) na uloženie k pohybu
const photoUrl = ref('')
const all = ref(null)
const saving = ref(false)
const fileInput = ref(null)

watch(
  () => props.visible,
  (v) => {
    if (v) reset()
  },
)
function reset() {
  step.value = 'pick'
  error.value = ''
  code.value = ''
  draft.value = null
  photo.value = null
  if (photoUrl.value) URL.revokeObjectURL(photoUrl.value)
  photoUrl.value = ''
  all.value = null
}
const open = computed({ get: () => props.visible, set: (v) => emit('update:visible', v) })
const cats = computed(() => categoriesFor(props.categories, 'expense'))
const totals = computed(() => splitByCategory(draft.value?.entry.split))
const openAccounts = computed(() => props.accounts.filter((a) => !a.closed))

function showDraft(d) {
  const acc = d.entry.accountId || openAccounts.value.find((a) => a.kind === 'cash')?.id || openAccounts.value[0]?.id || null
  draft.value = { ...d, entry: { ...d.entry, accountId: acc, date: fromISODate(d.entry.date) } }
  step.value = 'review'
}

async function onPhoto(ev) {
  const file = ev.target.files?.[0]
  ev.target.value = ''
  if (!file) return
  error.value = ''
  step.value = 'busy'
  busyText.value = 'Hľadám QR kód…'
  let loaded
  try {
    loaded = await loadImage(file)
    photo.value = await shrinkImage(loaded.img)
    photoUrl.value = URL.createObjectURL(photo.value)
    const text = await decodeQR(loaded.img.naturalWidth, loaded.img.naturalHeight, imageReader(loaded.img))
    if (receiptId(text)) {
      busyText.value = 'Načítavam bloček z eKasa…'
      showDraft(await api.hhReceiptQR(session.companyId, text))
    } else {
      busyText.value = 'Čítam bloček z fotky… (do pol minúty)'
      showDraft(await api.hhReceiptPhoto(session.companyId, photo.value))
    }
  } catch (e) {
    error.value = e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message
    step.value = 'pick'
  } finally {
    if (loaded?.url) URL.revokeObjectURL(loaded.url)
  }
}

async function fromCode() {
  const id = receiptId(code.value)
  if (!id) {
    error.value = 'Kód bločku začína O- a má 32 znakov za pomlčkou (je pod QR kódom).'
    return
  }
  error.value = ''
  step.value = 'busy'
  busyText.value = 'Načítavam bloček z eKasa…'
  try {
    showDraft(await api.hhReceiptQR(session.companyId, id))
  } catch (e) {
    error.value = e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message
    step.value = 'pick'
  }
}

function applyAll(c) {
  if (!c || !draft.value) return
  draft.value.entry.split = setAllCategory(draft.value.entry.split, c)
  all.value = null
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const e = draft.value.entry
    const body = { ...e, id: 0, date: toISODate(e.date) }
    if (!body.split?.length) delete body.split
    const saved = await api.hhSaveEntry(session.companyId, body)
    if (photo.value) {
      try {
        await api.hhSavePhoto(session.companyId, saved.id, photo.value)
      } catch {
        toast.add({ severity: 'warn', summary: 'Fotka bločku sa neuložila', detail: 'Pohyb je zapísaný bez fotky.', life: 5000 })
      }
    }
    toast.add({ severity: 'success', summary: 'Bloček zapísaný', detail: `${e.payee || 'Nákup'} · ${formatMoney(e.amount.replace('-', ''))}`, life: 3000 })
    emit('saved', saved)
    open.value = false
  } catch (e) {
    error.value = e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="open" header="Bloček" modal :style="{ width: step === 'review' ? '46rem' : '30rem' }" :breakpoints="{ '760px': '100vw' }"
    class="rc-dialog">
    <div v-if="step === 'pick'" class="pick">
      <p class="lead">
        Odfoťte bloček celý, aj s QR kódom. Bloček z pokladne eKasa načíta Fiškus priamo z Finančnej správy so všetkými položkami. Iný doklad
        prečíta z fotky (potrebuje kľúč Claude API).
      </p>
      <input ref="fileInput" type="file" accept="image/*" capture="environment" class="visually-hidden" @change="onPhoto" />
      <Button label="Odfotiť alebo vybrať fotku" icon="pi pi-camera" class="shoot" @click="fileInput.click()" />
      <div class="or"><span>alebo kód spod QR kódu</span></div>
      <form class="code" @submit.prevent="fromCode">
        <InputText v-model="code" size="small" placeholder="O-…" aria-label="Kód bločku" spellcheck="false" autocomplete="off" />
        <Button type="submit" label="Načítať" size="small" severity="secondary" outlined />
      </form>
      <p v-if="error" class="err" role="alert">{{ error }}</p>
    </div>

    <div v-else-if="step === 'busy'" class="busy" aria-live="polite">
      <i class="pi pi-spin pi-spinner" aria-hidden="true" />
      <span>{{ busyText }}</span>
    </div>

    <form v-else-if="draft" class="review" @submit.prevent="save">
      <div class="form-grid head">
        <FormField label="Obchod" :span="2"><InputText v-model="draft.entry.payee" size="small" /></FormField>
        <FormField label="Dátum"><DatePicker v-model="draft.entry.date" date-format="d. m. yy" size="small" fluid /></FormField>
        <FormField label="Zaplatené z účtu">
          <Select v-model="draft.entry.accountId" :options="openAccounts" option-label="name" option-value="id" size="small" fluid />
        </FormField>
      </div>
      <ul v-if="draft.warnings?.length" class="warn">
        <li v-for="w in draft.warnings" :key="w">{{ w }}</li>
      </ul>

      <div class="items-head">
        <h3>Položky <small>{{ draft.entry.split?.length || 0 }} · {{ draft.from === 'ekasa' ? 'z eKasa' : 'z fotky' }}</small></h3>
        <Select v-if="draft.entry.split?.length" v-model="all" :options="cats" size="small" placeholder="Všetky do kategórie…" class="all"
          @change="(e) => applyAll(e.value)" />
      </div>
      <div class="items-wrap">
        <table v-if="draft.entry.split?.length" class="items">
          <tbody>
            <tr v-for="(x, i) in draft.entry.split" :key="i">
              <td class="nm">{{ x.note || '—' }}<small v-if="x.quantity"> × {{ x.quantity }}</small></td>
              <td class="ct"><Select v-model="x.category" :options="cats" size="small" fluid :aria-label="`Kategória ${x.note}`" /></td>
              <td class="num am" :class="{ disc: Number(x.amount) > 0 }">{{ formatAmount(x.amount) }}</td>
            </tr>
          </tbody>
        </table>
        <p v-else class="muted">Bloček bez položiek — zapíše sa ako jeden výdavok v kategórii {{ draft.entry.category }}.</p>
      </div>

      <div class="foot">
        <div class="bycat">
          <span v-for="t in totals" :key="t.category" class="chip">{{ t.category }} <b class="num">{{ formatAmount(t.amount.replace('-', '')) }}</b></span>
        </div>
        <div class="total">Spolu <span class="num"><span class="ledger">{{ formatMoney(draft.entry.amount.replace('-', '')) }}</span></span></div>
      </div>
      <a v-if="photoUrl" :href="photoUrl" target="_blank" rel="noopener" class="ph">Zobraziť fotku bločku</a>
      <p v-if="error" class="err" role="alert">{{ error }}</p>
      <div class="actions">
        <Button label="Iný bloček" text size="small" @click="reset" />
        <span class="spacer" />
        <Button label="Zrušiť" text size="small" @click="open = false" />
        <Button type="submit" label="Zapísať bloček" icon="pi pi-check" size="small" :loading="saving" :disabled="!draft.entry.accountId" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.pick {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.lead {
  margin: 0;
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.shoot {
  justify-content: center;
  padding: 14px;
  font-size: var(--fs-lg);
}
.or {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.or::before,
.or::after {
  content: '';
  flex: 1;
  border-top: 1px solid var(--rule);
}
.code {
  display: flex;
  gap: 6px;
}
.code :deep(input) {
  flex: 1;
  font-variant-numeric: tabular-nums;
}
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
  margin: 0;
}
.busy {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 30px 6px;
  color: var(--ink-soft);
}
.busy .pi {
  font-size: 1.4rem;
  color: var(--blue);
}
.review {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.head {
  grid-template-columns: 2fr 1fr 1fr 1fr;
}
.warn {
  margin: 0;
  padding: 6px 10px 6px 26px;
  background: var(--tone-draft-bg);
  color: var(--tone-draft);
  font-size: var(--fs-sm);
}
.items-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.items-head h3 {
  font-size: var(--fs-md);
  color: var(--blue-deep);
  margin: 0;
}
.items-head small {
  color: var(--ink-faint);
  font-weight: 400;
}
.all {
  margin-left: auto;
  min-width: 200px;
}
.items-wrap {
  max-height: 46vh;
  overflow: auto;
  border: 1px solid var(--rule);
}
.items {
  width: 100%;
  table-layout: fixed;
  border-collapse: collapse;
  font-size: var(--fs-sm);
}
.items td {
  padding: 3px 6px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: middle;
}
.nm {
  overflow-wrap: anywhere;
}
.nm small {
  color: var(--ink-faint);
}
.ct {
  width: 220px;
}
.am {
  width: 72px;
  font-weight: 600;
}
.am.disc {
  color: var(--tone-paid);
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  padding: 8px;
  margin: 0;
}
.foot {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.bycat {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  flex: 1;
}
.chip {
  font-size: var(--fs-xs);
  background: var(--blue-wash);
  color: var(--blue-deep);
  padding: 2px 7px;
}
.total {
  white-space: nowrap;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.total .num {
  font-size: var(--fs-lg);
  font-weight: 700;
  color: var(--ink);
  margin-left: 6px;
}
.ledger {
  border-bottom: 3px double currentColor;
}
.ph {
  font-size: var(--fs-sm);
  color: var(--blue);
}
.actions {
  display: flex;
  gap: 6px;
  align-items: center;
}
.spacer {
  flex: 1;
}
@media (max-width: 760px) {
  .head {
    grid-template-columns: 1fr 1fr;
  }
  .items td.ct {
    width: 150px;
  }
  .all {
    min-width: 0;
    flex: 1;
  }
  .foot {
    flex-direction: column;
  }
}
</style>
