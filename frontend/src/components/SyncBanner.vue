<script setup>
// Fiškus v prehliadači ako telefón mimo domu: pruh s tým, odkiaľ je kópia
// údajov, čo čaká na prenos do počítača a tlačidlo na načítanie novej kópie.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../lib/api.js'
import { desktopState, phoneLine, phoneProblem, syncState, unseenDone } from '../lib/sync.js'

// čo už používateľ videl spracované (čas posledného výsledku) — iba pohodlie
const SEEN = 'fiskus.sync.seenDone'
const seenAt = ref('')
try {
  seenAt.value = localStorage.getItem(SEEN) ?? ''
} catch {
  /* bez úložiska sa oznam ukáže znova */
}

let timer = null
async function load() {
  try {
    const st = await api.syncStatus()
    syncState.phone = st.role === 'phone' ? (st.phone ?? null) : null
  } catch {
    /* stav sa skúsi znova */
  }
}
onMounted(() => {
  load()
  timer = setInterval(load, 20000)
})
onBeforeUnmount(() => clearInterval(timer))

const p = computed(() => syncState.phone)
const desk = computed(() => desktopState(p.value))
const fresh = computed(() => unseenDone(p.value, seenAt.value))
const reloadPage = () => location.reload()
function hideDone() {
  seenAt.value = fresh.value?.at ?? ''
  try {
    localStorage.setItem(SEEN, seenAt.value)
  } catch {
    /* nevadí */
  }
}
</script>

<template>
  <div v-if="p" class="sb" :class="{ fresh: p.staged }" role="status">
    <i class="pi" :class="p.staged ? 'pi-download' : 'pi-mobile'" aria-hidden="true" />
    <span>
      {{ phoneLine(p) }}<template v-if="p.failed.length"> · <strong class="bad">počítač neprijal {{ p.failed.length }}</strong></template>
      <small class="desk" :class="{ on: desk.online }"><i class="dot" aria-hidden="true" />{{ desk.text }}</small>
      <small v-if="phoneProblem(p)">{{ phoneProblem(p) }}</small>
      <small v-if="fresh" class="done">
        <i class="pi pi-check-circle" aria-hidden="true" /> Spracované v počítači: {{ fresh.detail || fresh.title }}
        <button type="button" class="x" aria-label="Skryť oznam" @click="hideDone"><i class="pi pi-times" aria-hidden="true" /></button>
      </small>
    </span>
    <button v-if="p.staged" type="button" class="go" @click="reloadPage">Načítať novú kópiu</button>
    <template v-else>
      <RouterLink to="/m" class="shot"><i class="pi pi-camera" aria-hidden="true" /> Bloček</RouterLink>
      <RouterLink to="/nastavenia/telefon">Podrobnosti</RouterLink>
    </template>
  </div>
</template>

<style scoped>
.sb {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 5px 16px;
  background: #eef2f6;
  border-bottom: 1px solid var(--rule);
  font-size: var(--fs-sm);
  color: var(--ink);
}
.sb.fresh {
  background: #e7f1e9;
}
.sb > .pi {
  color: var(--blue);
}
.sb span {
  flex: 1 1 16rem;
}
.sb small {
  display: block;
  color: var(--ink-soft);
}
.bad {
  color: var(--tone-overdue);
}
.desk {
  display: flex !important;
  align-items: center;
  gap: 5px;
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #9aa4af;
  flex: none;
}
.desk.on .dot {
  background: var(--tone-paid);
}
.done {
  color: var(--tone-paid) !important;
  font-weight: 600;
}
.x {
  border: 0;
  background: none;
  color: var(--ink-soft);
  cursor: pointer;
  padding: 0 4px;
}
.sb a {
  color: var(--blue);
  font-weight: 600;
}
.go {
  border: 0;
  background: var(--blue-deep);
  color: #fff;
  padding: 4px 10px;
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
</style>
