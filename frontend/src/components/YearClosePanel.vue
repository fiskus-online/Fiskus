<script setup>
// Uzávierka roka v jednoduchom účtovníctve: kontroly, uzavretie (zámok
// roka a prenos stavov účtov do ďalšieho roka), otvorenie s dôvodom.
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatDate, formatMoney } from '../lib/format.js'
import { closeSummary, eventLabel } from '../lib/yearclose.js'

const props = defineProps({ year: { type: Number, required: true } })
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
    message: `Doklady, pohyby peňazí a rozpis denníka s dátumom v roku ${props.year} sa uzamknú. Konečné stavy bankových účtov sa prenesú ako počiatočné stavy roka ${props.year + 1}. Rok sa dá neskôr otvoriť s uvedením dôvodu.`,
    acceptProps: { label: 'Uzavrieť rok', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      busy.value = true
      try {
        info.value = await api.closeYear(session.companyId, props.year)
        toast.add({ severity: 'success', summary: `Rok ${props.year} je uzavretý`, detail: `Stavy účtov sú prenesené do roka ${props.year + 1}.`, life: 5000 })
        emit('changed', true)
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Rok sa neuzavrel', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
      } finally {
        busy.value = false
      }
    },
  })
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
      <template v-if="canWrite">
        <Button v-if="closed" label="Otvoriť rok…" icon="pi pi-lock-open" size="small" severity="secondary" text @click="reopening = true" />
        <Button v-else label="Uzavrieť rok" icon="pi pi-lock" size="small" :disabled="!info.canClose" :loading="busy" @click="close" />
      </template>
    </header>

    <div v-if="closed" class="done">
      <dl>
        <div><dt>Príjmy</dt><dd class="num">{{ formatMoney(info.close.income) }}</dd></div>
        <div><dt>Výdavky</dt><dd class="num">{{ formatMoney(info.close.expenses) }}</dd></div>
        <div><dt>Rozdiel</dt><dd class="num"><span class="ledger">{{ formatMoney(info.close.result) }}</span></dd></div>
      </dl>
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
@media print {
  .yc {
    display: none;
  }
}
</style>
