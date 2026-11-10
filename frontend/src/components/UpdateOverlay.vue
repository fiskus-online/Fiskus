<script setup>
// Automatická aktualizácia: kým sa Fiškus sťahuje, inštaluje a reštartuje,
// celé okno prekryje priebeh (nič rozpracované sa nestratí). Po spustení
// novej verzie sa okno samo znova načíta.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { pending, percent, progressText, restarted, updating } from '../lib/update.js'

const st = ref(null)
const waiting = ref(false) // starý Fiškus skončil, čaká sa na nový
const gaveUp = ref(false)
let before = ''
let since = 0
let timer = null
let alive = true
const opened = Date.now()

const shown = computed(() => waiting.value || updating(st.value))
const text = computed(() =>
  waiting.value && !gaveUp.value ? progressText({ ...st.value, state: 'restarting' }) : progressText(st.value),
)
const pct = computed(() => percent(st.value))

async function tick() {
  if (waiting.value) {
    try {
      if (restarted(before, await api.capabilities())) {
        location.reload()
        return
      }
    } catch {
      /* nový Fiškus ešte nebeží */
    }
    if (Date.now() - since > 120000) gaveUp.value = true
  } else {
    try {
      st.value = await api.updateStatus()
      if (updating(st.value)) before = st.value.current
      if (st.value.state === 'restarting') {
        waiting.value = true
        since = Date.now()
      }
    } catch {
      // program sa ukončil uprostred inštalácie → čaká sa na nový
      if (updating(st.value)) {
        waiting.value = true
        since = Date.now()
      }
    }
  }
  schedule()
}

function schedule() {
  if (!alive) return
  // rýchlo počas aktualizácie a krátko po spustení (kontrola vydania), inak zriedka
  const fast = shown.value || (pending(st.value) && Date.now() - opened < 60000)
  timer = setTimeout(tick, fast ? 1000 : 10000)
}

async function postpone() {
  try {
    st.value = await api.updatePostpone()
  } catch {
    /* ďalší pokus pri ďalšom spustení */
  }
}

const reload = () => location.reload()

onMounted(tick)
onBeforeUnmount(() => {
  alive = false
  clearTimeout(timer)
})
</script>

<template>
  <div v-if="shown" class="update-cover" role="dialog" aria-modal="true" aria-labelledby="update-title">
    <div class="update-box">
      <svg viewBox="0 0 32 32" width="44" height="44" aria-hidden="true">
        <rect width="32" height="32" rx="7" fill="#1f3a5c" />
        <path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#f6f7f9" />
        <rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" />
      </svg>
      <template v-if="!gaveUp">
        <h2 id="update-title" aria-live="polite">{{ text.title }}</h2>
        <div class="bar" :class="{ busy: st?.state !== 'downloading' }">
          <span :style="{ width: (st?.state === 'downloading' ? pct : 100) + '%' }" />
        </div>
        <p class="note">{{ text.note }}</p>
        <Button v-if="st?.state === 'downloading'" text size="small" label="Teraz nie, aktualizovať pri ďalšom spustení" @click="postpone" />
      </template>
      <template v-else>
        <h2 id="update-title">Nový Fiškus sa sám neotvoril</h2>
        <p class="note">Otvorte Fiškus ikonou. Ak sa neotvorí, stiahnite si inštalátor z fiskus.online.</p>
        <Button size="small" label="Skúsiť znova" icon="pi pi-refresh" @click="reload" />
      </template>
    </div>
  </div>
</template>

<style scoped>
.update-cover {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: grid;
  place-items: center;
  padding: 24px 16px;
  background: var(--paper);
}
.update-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.85rem;
  width: min(440px, 100%);
  text-align: center;
}
h2 {
  margin: 0;
  font-size: 1.2rem;
  color: var(--blue-deep);
  font-variant-numeric: tabular-nums;
}
.note {
  margin: 0;
  color: var(--ink-soft);
  font-size: 0.92rem;
}
.bar {
  width: 100%;
  height: 6px;
  border-radius: 3px;
  background: #dde3ea;
  overflow: hidden;
}
.bar span {
  display: block;
  height: 100%;
  background: var(--blue);
  transition: width 0.4s ease;
}
.bar.busy span {
  animation: pulse 1.4s ease-in-out infinite;
}
@keyframes pulse {
  50% {
    opacity: 0.45;
  }
}
@media (prefers-reduced-motion: reduce) {
  .bar span {
    transition: none;
  }
  .bar.busy span {
    animation: none;
  }
}
</style>
