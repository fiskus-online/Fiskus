<script setup>
// Fiškus v prehliadači: pripojenie k počítaču odkazom z QR kódu (Nastavenia
// → Telefón mimo domu v počítači). Údaje v prehliadači nahradí kópia.
import { computed, onBeforeUnmount, ref } from 'vue'
import Button from 'primevue/button'
import { api, ApiError } from '../lib/api.js'
import { session } from '../lib/session.js'
import { joinName, takeJoin } from '../lib/sync.js'

const fragment = takeJoin()
const name = computed(() => joinName(fragment))
const state = ref('ask') // ask | wait | error
const error = ref('')
const note = ref('')
let timer = null
let tries = 0
onBeforeUnmount(() => clearTimeout(timer))

const msg = (e) => (e instanceof ApiError && e.fields?.length ? e.fields[0].message : e.message)

async function check() {
  try {
    const st = await api.syncRun()
    if (st.phone?.staged) {
      location.replace('/') // načítať stránku znova už s kópiou z počítača
      return
    }
    tries++
    note.value =
      st.phone?.lastError ||
      (tries > 3 ? 'Počítač ešte neposlal kópiu. Zapnite Fiškus v počítači — stránka sa načíta sama, keď kópia príde.' : '')
  } catch (e) {
    note.value = msg(e)
  }
  timer = setTimeout(check, tries < 10 ? 3000 : 15000)
}

async function join() {
  error.value = ''
  try {
    await api.syncJoin(fragment)
    state.value = 'wait'
    check()
  } catch (e) {
    state.value = 'error'
    error.value = msg(e)
  }
}
</script>

<template>
  <div class="page">
    <div class="box">
      <h1><i class="pi pi-mobile" aria-hidden="true" /> Pripojiť k počítaču</h1>
      <template v-if="!fragment">
        <p>Odkaz na pripojenie chýba. V počítači otvorte Nastavenia → Telefón mimo domu a naskenujte QR kód znova.</p>
      </template>
      <template v-else-if="state === 'ask'">
        <p>
          Tento prehliadač dostane kópiu údajov z počítača <strong>„{{ name }}“</strong>. Môžete v ňom zapisovať nové pohyby, úhrady,
          kontakty a ďalšie záznamy aj mimo domu — prenesú sa do počítača, keď bude zapnutý.
        </p>
        <p v-if="session.companies.length" class="warn">
          <i class="pi pi-exclamation-triangle" aria-hidden="true" />
          <span>
            Údaje, ktoré sú teraz v tomto prehliadači ({{ session.companies.map((c) => c.name).join(', ') }}), nahradí kópia z počítača.
            Ak ich chcete mať, najprv si <RouterLink to="/zalohy">stiahnite zálohu</RouterLink>.
          </span>
        </p>
        <div class="actions">
          <Button label="Pripojiť" icon="pi pi-link" @click="join" />
          <RouterLink to="/" class="cancel">Zrušiť</RouterLink>
        </div>
      </template>
      <template v-else-if="state === 'wait'">
        <p class="wait"><i class="pi pi-spin pi-spinner" aria-hidden="true" /> Sťahujem kópiu údajov z počítača…</p>
        <p v-if="note" class="muted">{{ note }}</p>
      </template>
      <template v-else>
        <p class="err">{{ error }}</p>
        <Button label="Skúsiť znova" severity="secondary" outlined @click="join" />
      </template>
      <p class="muted tip">
        Tip: na iPhone v Safari zvoľte <strong>Zdieľať → Pridať na plochu</strong> — inak Safari údaje stránky po týždni
        nepoužívania zmaže. V Androide ponuka prehliadača → <strong>Pridať na plochu</strong>.
      </p>
    </div>
  </div>
</template>

<style scoped>
.page {
  padding: 24px 16px;
  display: flex;
  justify-content: center;
}
.box {
  max-width: 34rem;
  display: grid;
  gap: 12px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 3px solid var(--blue-deep);
  padding: 18px 20px;
}
h1 {
  font-size: var(--fs-xl);
  color: var(--blue-deep);
  display: flex;
  gap: 8px;
  align-items: center;
}
.warn {
  display: flex;
  gap: 8px;
  align-items: baseline;
  background: var(--tone-draft-bg, #fdf3dc);
  padding: 8px 10px;
}
.warn .pi {
  color: var(--tone-draft);
}
.actions {
  display: flex;
  gap: 14px;
  align-items: center;
}
.cancel {
  color: var(--ink-soft);
}
.wait {
  display: flex;
  gap: 8px;
  align-items: center;
  font-weight: 600;
}
.muted {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.tip {
  border-top: 1px solid var(--rule);
  padding-top: 10px;
}
.err {
  color: var(--tone-overdue);
}
</style>
