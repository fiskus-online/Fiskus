<script setup>
// Uzávierka roka v jednoduchom aj podvojnom účtovníctve: kontroly,
// uzavretie (zámok roka a prenos stavov do ďalšieho roka), otvorenie
// s dôvodom.
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { closeSummary, closingRows, eventLabel, filterClosing } from '../lib/yearclose.js'
import { closingEntryColumns } from '../lib/columns.js'
import ExportButton from './ExportButton.vue'

const props = defineProps({ year: { type: Number, required: true }, double: { type: Boolean, default: false } })
const emit = defineEmits(['changed'])
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))
const info = ref(null)
const busy = ref(false)

async function load() {
  try {
    info.value = await api.yearClose(session.companyId, props.year)
  } catch {
    info.value = null
  }
}
watch(() => props.year, load, { immediate: true })
defineExpose({ load })

const closed = computed(() => !!info.value?.close?.closed)
const summary = computed(() => closeSummary(info.value))

function close() {
  confirm.require({
    header: `Uzavrieť rok ${props.year}`,
    message: props.double
      ? `Doklady, pohyby peňazí a zápisy hlavnej knihy s dátumom v roku ${props.year} sa uzamknú. Konečné stavy súvahových účtov sa prenesú ako počiatočné stavy roka ${props.year + 1} (výsledok hospodárenia na 431), ak ich rok ${props.year + 1} ešte nemá zadané. Rok sa dá neskôr otvoriť s uvedením dôvodu.`
      : `Doklady, pohyby peňazí a rozpis denníka s dátumom v roku ${props.year} sa uzamknú. Konečné stavy bankových účtov sa prenesú ako počiatočné stavy roka ${props.year + 1}. Rok sa dá neskôr otvoriť s uvedením dôvodu.`,
    acceptProps: { label: 'Uzavrieť rok', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      busy.value = true
      try {
        info.value = await api.closeYear(session.companyId, props.year)
        const carried = !info.value.close?.double || info.value.close?.opening
        toast.add({ severity: 'success', summary: `Rok ${props.year} je uzavretý`,
          detail: carried ? `Stavy účtov sú prenesené do roka ${props.year + 1}.` : `Počiatočné stavy roka ${props.year + 1} ostali tak, ako boli zadané.`, life: 5000 })
        emit('changed', true)
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Rok sa neuzavrel', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
      } finally {
        busy.value = false
      }
    },
  })
}

// uzávierkové zápisy 710/702 a otváracie 701 (podvojné účtovníctvo)
const entries = ref(null)
const entriesOpen = ref(false)
const entriesQ = ref('')
const entryRows = computed(() => filterClosing(closingRows(entries.value), entriesQ.value))
async function showEntries() {
  try {
    entries.value = await api.ledgerClosingEntries(session.companyId, props.year)
    entriesQ.value = ''
    entriesOpen.value = true
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Uzávierkové zápisy', detail: e.message, life: 5000 })
  }
}

const reopening = ref(false)
const reason = ref('')
const reasonErr = ref('')
async function reopen() {
  busy.value = true
  try {
    info.value = await api.reopenYear(session.companyId, props.year, reason.value)
    reopening.value = false
    reason.value = ''
    toast.add({ severity: 'success', summary: `Rok ${props.year} je otvorený`, detail: 'Po oprave ho znova uzavrite.', life: 4000 })
    emit('changed', false)
  } catch (e) {
    reasonErr.value = e.fields?.[0]?.message ?? e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section v-if="info" class="yc" :class="{ closed }" aria-labelledby="yc-title">
    <header>
      <i class="pi" :class="closed ? 'pi-lock' : 'pi-lock-open'" aria-hidden="true" />
      <h2 id="yc-title">Uzávierka roka {{ year }}</h2>
      <span class="state">{{ summary }}</span>
      <div class="spacer" />
      <Button v-if="double" label="Uzávierkové zápisy" icon="pi pi-list" size="small" severity="secondary" text @click="showEntries" />
      <template v-if="canWrite">
        <Button v-if="closed" label="Otvoriť rok…" icon="pi pi-lock-open" size="small" severity="secondary" text @click="reopening = true" />
        <Button v-else label="Uzavrieť rok" icon="pi pi-lock" size="small" :disabled="!info.canClose" :loading="busy" @click="close" />
      </template>
    </header>

    <div v-if="closed" class="done">
      <dl>
        <div><dt>{{ info.close.double ? 'Výnosy' : 'Príjmy' }}</dt><dd class="num">{{ formatMoney(info.close.income) }}</dd></div>
        <div><dt>{{ info.close.double ? 'Náklady' : 'Výdavky' }}</dt><dd class="num">{{ formatMoney(info.close.expenses) }}</dd></div>
        <div><dt>{{ info.close.double ? 'Výsledok hospodárenia' : 'Rozdiel' }}</dt><dd class="num"><span class="ledger">{{ formatMoney(info.close.result) }}</span></dd></div>
      </dl>
      <p v-if="info.close.double" class="carry">
        {{ info.close.opening ? `Konečné stavy súvahových účtov sú prenesené ako počiatočné stavy roka ${year + 1}.` : `Počiatočné stavy roka ${year + 1} ostali tak, ako boli zadané.` }}
      </p>
      <p v-if="Object.keys(info.close.bank || {}).length" class="carry">
        Prenesené do {{ year + 1 }}:
        <span v-for="(v, k) in info.close.bank" :key="k" class="acc">{{ k }} <b class="num">{{ formatMoney(v) }}</b></span>
      </p>
    </div>
    <ul v-else class="checks">
      <li v-for="c in info.checks" :key="c.id" :data-ok="c.ok" :data-blocking="c.blocking">
        <i class="pi" :class="c.ok ? 'pi-check' : c.blocking ? 'pi-times' : 'pi-exclamation-circle'" aria-hidden="true" />
        <span>{{ c.text }}</span>
        <RouterLink v-if="!c.ok && c.link" :to="c.link" class="fix">Opraviť</RouterLink>
      </li>
    </ul>
    <details v-if="info.close?.history?.length" class="hist">
      <summary>História uzávierky</summary>
      <ul>
        <li v-for="(h, i) in info.close.history" :key="i">{{ formatDate(String(h.at).slice(0, 10)) }} · {{ eventLabel(h) }} · {{ h.by }}<template v-if="h.reason"> — {{ h.reason }}</template></li>
      </ul>
    </details>

    <Dialog v-model:visible="entriesOpen" :header="`Uzávierkové zápisy ${year}`" modal :style="{ width: '56rem' }" :breakpoints="{ '760px': '100vw' }">
      <p class="dlg">
        Uzavretie účtovných kníh: náklady a výnosy na 710 – Účet ziskov a strát, výsledok a súvahové účty na 702 – Konečný účet súvahový,
        v ďalšom roku otvorenie cez 701 – Začiatočný účet súvahový. Do obratov sa nezapočítavajú, výkazy sú zo zostatkov pred uzavretím.
        <b v-if="entries && !entries.closed">Rok ešte nie je uzavretý — zápisy sú návrh podľa súčasných zostatkov.</b>
      </p>
      <div class="ent-bar">
        <InputText v-model="entriesQ" size="small" placeholder="Hľadať účet, názov, sumu" aria-label="Hľadať v zápisoch" />
        <ExportButton :name="`uzavierkove-zapisy-${year}`" :columns="closingEntryColumns" :rows="entryRows" />
      </div>
      <div class="ent-wrap">
        <table class="ent">
          <thead><tr><th>Dátum</th><th>Zápis</th><th>Účet</th><th class="num">MD</th><th class="num">D</th></tr></thead>
          <tbody>
            <tr v-for="r in entryRows" :key="r.key" :class="{ first: r.first }">
              <td>{{ r.first ? formatDate(r.date) : '' }}</td>
              <td>{{ r.first ? r.text : '' }}</td>
              <td><b>{{ r.account }}</b> <span class="an">{{ r.name }}</span></td>
              <td class="num">{{ r.debit ? formatAmount(r.debit) : '' }}</td>
              <td class="num">{{ r.credit ? formatAmount(r.credit) : '' }}</td>
            </tr>
            <tr v-if="!entryRows.length"><td colspan="5" class="empty">Žiadne zápisy — účty nemajú zostatky.</td></tr>
          </tbody>
        </table>
      </div>
    </Dialog>

    <Dialog v-model:visible="reopening" :header="`Otvoriť rok ${year}`" modal :style="{ width: '28rem' }" :breakpoints="{ '480px': '96vw' }">
      <p class="dlg">Otvorený rok sa dá znova meniť. Dôvod sa zapíše do auditu; po oprave rok znova uzavrite — stavy účtov sa prenesú nanovo.</p>
      <label class="dlg-l">Dôvod
        <InputText v-model="reason" size="small" fluid autofocus :invalid="!!reasonErr" placeholder="napr. chýbajúca faktúra za december" />
      </label>
      <small v-if="reasonErr" class="err">{{ reasonErr }}</small>
      <template #footer>
        <Button label="Zrušiť" text size="small" @click="reopening = false" />
        <Button label="Otvoriť rok" icon="pi pi-lock-open" size="small" severity="danger" :loading="busy" @click="reopen" />
      </template>
    </Dialog>
  </section>
</template>

<style scoped>
.yc {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-left: 3px solid var(--tone-draft);
  padding: 10px 14px;
  margin-bottom: 14px;
}
.yc.closed {
  border-left-color: var(--blue-deep);
}
.yc > header {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.yc h2 {
  font-size: var(--fs-md);
}
.yc > header .pi {
  color: var(--tone-draft);
}
.yc.closed > header .pi {
  color: var(--blue-deep);
}
.state {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.spacer {
  flex: 1;
}
.checks {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
  display: grid;
  gap: 4px;
  font-size: var(--fs-sm);
}
.checks li {
  display: flex;
  gap: 8px;
  align-items: baseline;
}
.checks .pi {
  font-size: 0.8rem;
  width: 14px;
}
.checks li[data-ok='true'] .pi {
  color: var(--tone-paid);
}
.checks li[data-ok='false'][data-blocking='true'] {
  color: var(--tone-overdue);
}
.checks li[data-ok='false'][data-blocking='false'] .pi {
  color: var(--tone-draft);
}
.fix {
  color: var(--blue);
  font-size: var(--fs-xs);
}
.done dl {
  display: flex;
  gap: 24px;
  margin: 8px 0 4px;
  flex-wrap: wrap;
}
.done dl div {
  display: flex;
  flex-direction: column;
}
.done dt {
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.done dd {
  margin: 0;
  font-weight: 600;
  text-align: left;
}
.ledger {
  border-bottom: 3px double currentColor;
}
.carry {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  margin: 4px 0 0;
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
}
.acc {
  font-variant-numeric: tabular-nums;
}
.hist {
  margin-top: 8px;
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.hist ul {
  margin: 4px 0 0;
  padding-left: 18px;
}
.dlg {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  margin: 0 0 10px;
}
.dlg-l {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: var(--fs-sm);
}
.err {
  color: var(--tone-overdue);
}
.ent-bar {
  display: flex;
  gap: 8px;
  justify-content: space-between;
  margin-bottom: 8px;
}
.ent-bar .p-inputtext {
  flex: 1;
  max-width: 20rem;
  min-width: 0;
}
.ent-wrap {
  overflow-x: auto;
}
.ent {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.ent th {
  text-align: left;
  font-weight: 600;
  color: var(--ink-soft);
  border-bottom: 1px solid var(--rule);
  padding: 4px 6px;
}
.ent td {
  padding: 3px 6px;
  vertical-align: top;
}
.ent tr.first td {
  border-top: 1px solid var(--rule);
}
.ent .num {
  text-align: right;
  white-space: nowrap;
}
.ent .an {
  color: var(--ink-soft);
}
.ent .empty {
  color: var(--ink-soft);
  text-align: center;
}
@media (max-width: 600px) {
  .ent th:first-child,
  .ent td:first-child {
    display: none;
  }
}
@media print {
  .yc {
    display: none;
  }
}
</style>
