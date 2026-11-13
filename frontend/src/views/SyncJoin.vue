<script setup>
// Fiškus v prehliadači: pripojenie k počítaču odkazom z QR kódu (Nastavenia
// → Telefón mimo domu v počítači). Údaje v prehliadači nahradí kópia.
// Odkaz príde z adresy (QR kód naskenovaný fotoaparátom), z fotky QR kódu
// alebo vložený — aplikácia z plochy na iPhone má vlastné úložisko, takže
// sa pripája zvnútra, nie cez prehliadač.
import { computed, onBeforeUnmount, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import { api, ApiError } from '../lib/api.js'
import { session } from '../lib/session.js'
import { decodeQR } from '../lib/qrscan.js'
import { imageReader, loadImage } from '../lib/receipt.js'
import { appleMobile, joinName, linkFragment, standalone, takeJoin } from '../lib/sync.js'

const fragment = ref(takeJoin())
const name = computed(() => joinName(fragment.value))
const pasted = ref('')
const reading = ref(false)
const fileInput = ref(null)
// iPhone v karte prehliadača: aplikácia z plochy by kópiu nevidela
const homeFirst = appleMobile() && !standalone()

function usePasted() {
  error.value = ''
  const f = linkFragment(pasted.value)
  if (!f) {
    error.value = 'Toto nie je odkaz na pripojenie z Fiškusa v počítači.'
    return
  }
  fragment.value = f
}

async function onPhoto(e) {
  const file = e.target.files?.[0]
  e.target.value = ''
  if (!file) return
  error.value = ''
  reading.value = true
  let url = ''
  try {
    const loaded = await loadImage(file)
    url = loaded.url
    const text = await decodeQR(loaded.img.naturalWidth, loaded.img.naturalHeight, imageReader(loaded.img))
    const f = linkFragment(text)
    if (!f) throw new Error(text ? 'QR kód nie je odkaz na pripojenie z Fiškusa v počítači.' : 'Na fotke sa nenašiel QR kód — odfoťte ho zblízka a ostro.')
    fragment.value = f
  } catch (err) {
    error.value = err.message
  } finally {
    if (url) URL.revokeObjectURL(url)
    reading.value = false
  }
}
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
    await api.syncJoin(fragment.value)
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
        <p>
          V počítači otvorte <strong>Nastavenia → Telefón mimo domu → Pripojiť telefón</strong>. Potom tu odfoťte QR kód z obrazovky
          počítača alebo vložte odkaz (tlačidlo Kopírovať odkaz v počítači).
        </p>
        <p v-if="homeFirst" class="warn">
          <i class="pi pi-info-circle" aria-hidden="true" />
          <span>
            Na iPhone si Fiškus najprv pridajte na plochu (Zdieľať → <strong>Pridať na plochu</strong>), spustite ho z plochy a pripojte
            až tam — aplikácia z plochy má vlastné údaje a kópiu z prehliadača by nevidela.
          </span>
        </p>
        <div class="actions">
          <Button label="Odfotiť QR kód" icon="pi pi-camera" :loading="reading" @click="fileInput.click()" />
          <input ref="fileInput" type="file" accept="image/*" capture="environment" class="hidden" @change="onPhoto" />
        </div>
        <div class="paste">
          <InputText v-model="pasted" placeholder="https://app.fiskus.online/#pripojit&…" aria-label="Odkaz na pripojenie" fluid />
          <Button label="Použiť odkaz" severity="secondary" outlined :disabled="!pasted.trim()" @click="usePasted" />
        </div>
        <p v-if="error" class="err">{{ error }}</p>
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
      <p v-if="fragment && homeFirst" class="muted tip">
        Na iPhone má aplikácia z plochy vlastné údaje: ak chcete Fiškus spúšťať z plochy, pridajte si ho tam najprv (Zdieľať → Pridať na
        plochu) a pripojte ho až v ňom. V karte prehliadača Safari údaje po týždni nepoužívania zmaže.
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
.hidden {
  display: none;
}
.paste {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.paste > :first-child {
  flex: 1 1 14rem;
}
</style>
