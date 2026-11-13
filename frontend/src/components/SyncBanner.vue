<script setup>
// Fiškus v prehliadači ako telefón mimo domu: pruh s tým, odkiaľ je kópia
// údajov, čo čaká na prenos do počítača a tlačidlo na načítanie novej kópie.
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../lib/api.js'
import { phoneLine, phoneProblem, syncState } from '../lib/sync.js'

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
const reloadPage = () => location.reload()
</script>

<template>
  <div v-if="p" class="sb" :class="{ fresh: p.staged }" role="status">
    <i class="pi" :class="p.staged ? 'pi-download' : 'pi-mobile'" aria-hidden="true" />
    <span>
      {{ phoneLine(p) }}<template v-if="p.failed.length"> · <strong class="bad">počítač neprijal {{ p.failed.length }}</strong></template>
      <small v-if="phoneProblem(p)">{{ phoneProblem(p) }}</small>
    </span>
    <button v-if="p.staged" type="button" class="go" @click="reloadPage">Načítať novú kópiu</button>
    <RouterLink v-else to="/nastavenia/telefon">Podrobnosti</RouterLink>
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
