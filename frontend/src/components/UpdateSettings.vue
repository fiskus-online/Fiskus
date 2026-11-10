<script setup>
// Nastavenia → Aktualizácie: verzia, automatická aktualizácia po spustení,
// ručná kontrola a aktualizácia.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import ToggleSwitch from 'primevue/toggleswitch'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { statusLine, updating } from '../lib/update.js'

const toast = useToast()
const st = ref(null)
const busy = ref('')

const line = computed(() => statusLine(st.value))
const canStart = computed(() => ['available', 'error'].includes(st.value?.state) && st.value?.latest && !st.value?.manual)

async function load() {
  try {
    st.value = await api.updateStatus()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Stav aktualizácie sa nenačítal', detail: e.message, life: 6000 })
  }
}

async function run(what, fn) {
  busy.value = what
  try {
    st.value = await fn()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Aktualizácia', detail: e.message, life: 6000 })
  } finally {
    busy.value = ''
  }
}

const check = () => run('check', api.updateCheck)
const start = () => run('start', api.updateStart)
const setAuto = (on) => run('auto', () => api.updateAuto(on))

// stav sa mení aj mimo tejto obrazovky (kontrola po spustení, „Teraz nie“)
let timer = null
async function poll() {
  if (!busy.value) {
    try {
      st.value = await api.updateStatus()
    } catch {
      /* program sa práve reštartuje */
    }
  }
  timer = setTimeout(poll, 3000)
}
onMounted(async () => {
  await load()
  timer = setTimeout(poll, 3000)
})
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <fieldset v-if="st" class="form-section upd">
    <legend>Automatická aktualizácia</legend>
    <p class="help">
      Po spustení si Fiškus zistí, či je nová verzia. Ak áno, sám ju stiahne, zavrie sa, nainštaluje ju a znova sa otvorí. Údaje
      ostanú, pri zatvorení sa ako vždy uloží záloha.
    </p>

    <label class="switch">
      <ToggleSwitch :model-value="st.auto" :disabled="busy === 'auto'" input-id="upd-auto" @update:model-value="setAuto" />
      <span>Aktualizovať automaticky po spustení</span>
    </label>

    <p class="state" :class="{ fresh: st.state === 'current', bad: st.state === 'error' }" aria-live="polite">{{ line }}</p>
    <p v-if="st.manual && st.state === 'available'" class="help">
      {{ st.manual }} Novú verziu si stiahnite a nainštalujte ručne.
    </p>

    <div class="row">
      <Button size="small" outlined icon="pi pi-refresh" label="Skontrolovať teraz" :loading="busy === 'check'" :disabled="updating(st)" @click="check" />
      <Button v-if="canStart" size="small" icon="pi pi-download" :label="`Aktualizovať na ${st.latest}`" :loading="busy === 'start'" @click="start" />
      <a v-if="st.manual && st.state === 'available'" class="p-button p-button-sm p-button-outlined dl" :href="st.page" target="_blank" rel="noopener">
        <i class="pi pi-external-link" /> Stiahnuť z fiskus.online
      </a>
    </div>

    <details v-if="st.notes && st.latest && st.state !== 'current'" class="notes">
      <summary>Čo je nové vo verzii {{ st.latest }}</summary>
      <pre>{{ st.notes }}</pre>
    </details>
  </fieldset>
</template>

<style scoped>
.upd {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.help {
  margin: 0;
  color: var(--ink-soft);
  max-width: 70ch;
}
.switch {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  cursor: pointer;
}
.state {
  margin: 0;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.state.bad {
  color: var(--tone-overdue);
}
.row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}
.dl {
  text-decoration: none;
  gap: 0.4rem;
}
.notes pre {
  margin: 0.5rem 0 0;
  white-space: pre-wrap;
  font: inherit;
  font-size: 0.88rem;
  color: var(--ink-soft);
  max-height: 16rem;
  overflow: auto;
}
</style>
