<script setup>
// Fiškus v prehliadači: údaje sú v úložisku prehliadača na tomto počítači.
// Záloha = stiahnutý šifrovaný súbor .fbk (rovnaký ako v nainštalovanom
// Fiškuse), obnova = nahratie súboru zálohy.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Password from 'primevue/password'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { api } from '../lib/api.js'
import { saveBlob } from '../lib/export.js'
import { lastBackupText, passwordErrors } from '../lib/browserbackup.js'
import { downloadURL } from '../lib/install.js'

const toast = useToast()
const confirm = useConfirm()
const st = ref(null)
const storage = ref({ persisted: null, usage: 0 })
const error = ref('')

async function load() {
  try {
    st.value = await api.browserBackup()
    error.value = ''
  } catch (e) {
    error.value = e.message
  }
  try {
    const [persisted, est] = await Promise.all([navigator.storage.persisted(), navigator.storage.estimate()])
    storage.value = { persisted, usage: est.usage ?? 0 }
  } catch {
    /* prehliadač stav úložiska neprezradí */
  }
}
onMounted(load)

const mb = computed(() => (storage.value.usage / 1048576).toFixed(1).replace('.', ','))
async function persist() {
  const ok = await navigator.storage.persist().catch(() => false)
  storage.value.persisted = ok
  toast.add(ok
    ? { severity: 'success', summary: 'Údaje sú uložené natrvalo', detail: 'Prehliadač ich sám nezmaže.', life: 4000 }
    : { severity: 'warn', summary: 'Prehliadač nepovolil trvalé uloženie', detail: 'Zálohujte pravidelne. V Chrome pomôže pridať Fiškus na plochu (Inštalovať aplikáciu).', life: 7000 })
}

// záloha
const password = ref('')
const again = ref('')
const errs = ref({})
const busy = ref(false)
async function backup() {
  errs.value = passwordErrors(password.value, again.value)
  if (Object.keys(errs.value).length) return
  busy.value = true
  try {
    const { blob, name } = await api.downloadBrowserBackup(password.value)
    saveBlob(blob, name)
    password.value = ''
    again.value = ''
    toast.add({ severity: 'success', summary: 'Záloha je stiahnutá', detail: `${name} — uložte ju mimo tohto počítača a zapamätajte si heslo.`, life: 6000 })
    await load()
  } catch (e) {
    errs.value = { password: e.fields?.[0]?.message ?? e.message }
  } finally {
    busy.value = false
  }
}

// obnova
const file = ref(null)
const rpass = ref('')
const rerr = ref('')
const rbusy = ref(false)
function pick(e) {
  file.value = e.target.files?.[0] ?? null
  rerr.value = ''
}
function restore() {
  if (!file.value) {
    rerr.value = 'Vyberte súbor zálohy (.fbk)'
    return
  }
  confirm.require({
    header: 'Obnoviť zo zálohy',
    message: `Všetky údaje Fiškusa v tomto prehliadači sa nahradia údajmi zo zálohy ${file.value.name}. Fiškus sa potom znova načíta.`,
    acceptProps: { label: 'Obnoviť', severity: 'danger', size: 'small' },
    rejectProps: { label: 'Zrušiť', text: true, size: 'small' },
    accept: async () => {
      rbusy.value = true
      try {
        await api.restoreBrowserBackup(file.value, rpass.value)
        location.reload()
      } catch (e) {
        rerr.value = e.fields?.[0]?.message ?? e.message
        rbusy.value = false
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Zálohy</h1>
    </header>
    <p v-if="error" class="error">{{ error }}</p>

    <section class="card where">
      <h2><i class="pi pi-desktop" aria-hidden="true" /> Údaje sú iba v tomto prehliadači</h2>
      <p>Fiškus v prehliadači ukladá údaje do úložiska prehliadača na tomto počítači — na internet neodchádzajú. Ak vymažete údaje stránok, preinštalujete počítač alebo otvoríte Fiškus v inom prehliadači, údaje tam nebudú. Preto si pravidelne sťahujte zálohu.</p>
      <p class="state">
        <span v-if="storage.persisted === true"><i class="pi pi-check" aria-hidden="true" /> Uložené natrvalo — prehliadač ich sám nezmaže.</span>
        <span v-else-if="storage.persisted === false" class="warn"><i class="pi pi-exclamation-circle" aria-hidden="true" /> Prehliadač môže údaje pri nedostatku miesta zmazať.</span>
        <span v-if="storage.usage" class="muted">Zaberajú {{ mb }} MB.</span>
        <Button v-if="storage.persisted === false" label="Uložiť natrvalo" size="small" text @click="persist" />
      </p>
    </section>

    <section class="card">
      <h2>Stiahnuť zálohu</h2>
      <p class="muted">{{ lastBackupText(st) }} Záloha je šifrovaná heslom; bez neho sa z nej nedá obnoviť.</p>
      <form class="grid" @submit.prevent="backup">
        <label>Heslo zálohy
          <Password v-model="password" :feedback="false" toggle-mask size="small" autocomplete="new-password" :invalid="!!errs.password" fluid />
          <small v-if="errs.password" class="err">{{ errs.password }}</small>
        </label>
        <label>Heslo znova
          <Password v-model="again" :feedback="false" toggle-mask size="small" autocomplete="new-password" :invalid="!!errs.confirm" fluid />
          <small v-if="errs.confirm" class="err">{{ errs.confirm }}</small>
        </label>
        <div class="act">
          <Button type="submit" label="Stiahnuť zálohu" icon="pi pi-download" size="small" :loading="busy" />
        </div>
      </form>
    </section>

    <section class="card">
      <h2>Obnoviť zo zálohy</h2>
      <p class="muted">Súbor .fbk zo zálohy v prehliadači alebo z nainštalovaného Fiškusa (priečinok záloh).</p>
      <form class="grid" @submit.prevent="restore">
        <div class="field">
          <span>Súbor zálohy</span>
          <label class="file">
            <input type="file" accept=".fbk" @change="pick" />
            <span class="p-button p-button-secondary p-button-outlined p-button-sm"><i class="pi pi-folder-open" aria-hidden="true" />&nbsp;Vybrať súbor</span>
            <span class="name">{{ file?.name ?? 'nevybraný' }}</span>
          </label>
        </div>
        <label>Heslo zálohy
          <Password v-model="rpass" :feedback="false" toggle-mask size="small" autocomplete="current-password" :invalid="!!rerr" fluid />
        </label>
        <div class="act">
          <Button type="submit" label="Obnoviť…" icon="pi pi-history" severity="secondary" size="small" :loading="rbusy" />
        </div>
      </form>
      <small v-if="rerr" class="err">{{ rerr }}</small>
    </section>

    <section class="card move">
      <h2>Prechod na nainštalovaný Fiškus</h2>
      <p>Nainštalovaný Fiškus sa zálohuje sám do zvoleného priečinka a vie aj funkcie, ktoré v prehliadači nie sú (e-maily, banka, e-faktúry, mobil v sieti). Stiahnite zálohu, <a :href="downloadURL" target="_blank" rel="noopener">nainštalujte Fiškus</a>, súbor zálohy skopírujte do jeho priečinka záloh a v časti Zálohy ho obnovte.</p>
    </section>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.error,
.err {
  color: var(--tone-overdue);
  margin: 0;
}
.card {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 12px 14px;
}
.card h2 {
  font-size: var(--fs-md);
  margin: 0 0 6px;
}
.card p {
  margin: 0 0 6px;
  font-size: var(--fs-sm);
}
.where {
  border-left: 3px solid var(--blue);
}
.where h2 .pi {
  color: var(--blue);
}
.state {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 14px;
}
.state .pi-check {
  color: var(--tone-paid);
}
.warn {
  color: var(--tone-draft);
}
.muted {
  color: var(--ink-soft);
}
.grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 16rem)) auto;
  gap: 8px 12px;
  align-items: end;
}
.grid label {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: var(--fs-sm);
}
.field {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: var(--fs-sm);
  min-width: 0;
}
.grid label.file {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  min-width: 0;
}
.file input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
}
.file input:focus-visible + .p-button {
  outline: 2px solid var(--blue);
  outline-offset: 2px;
}
.file .name {
  color: var(--ink-soft);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.move p {
  color: var(--ink-soft);
}
@media (max-width: 640px) {
  .grid {
    grid-template-columns: 1fr;
  }
}
</style>
