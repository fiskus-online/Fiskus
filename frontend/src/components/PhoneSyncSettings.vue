<script setup>
// Telefón mimo domu: v počítači spárovanie telefónu QR kódom a zoznam
// telefónov, vo Fiškuse v prehliadači stav kópie a čakajúce záznamy.
import { computed, onMounted, ref } from 'vue'
import QRCode from 'qrcode'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { itemsText, phoneProblem, syncTime } from '../lib/sync.js'

const toast = useToast()
const confirm = useConfirm()
const st = ref(null)
const busy = ref('')
const name = ref('')
const pairing = ref(null)
const qr = ref('')
const relay = ref('')
const relayError = ref('')
const showRelay = ref(false)
const link = ref('')

const msg = (e) => (e instanceof ApiError && e.fields?.length ? e.fields[0].message : e.message)

async function load() {
  try {
    st.value = await api.syncStatus()
    relay.value = st.value.relay ?? ''
    showRelay.value = !!st.value.relay
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Stav sa nenačítal', detail: msg(e), life: 6000 })
  }
}
onMounted(load)

const phones = computed(() => st.value?.phones ?? [])
const phone = computed(() => st.value?.phone ?? null)

async function pair() {
  busy.value = 'pair'
  try {
    pairing.value = await api.syncPair(name.value)
    qr.value = await QRCode.toDataURL(pairing.value.link, { width: 280, margin: 2, errorCorrectionLevel: 'M', color: { dark: '#1f3a5c' } })
    name.value = ''
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Telefón sa nespároval', detail: msg(e), life: 6000 })
  } finally {
    busy.value = ''
  }
}

async function copyLink() {
  try {
    await navigator.clipboard.writeText(pairing.value.link)
    toast.add({ severity: 'success', summary: 'Odkaz skopírovaný', detail: 'Pošlite si ho do telefónu iba sami sebe — kto ho má, vidí údaje.', life: 5000 })
  } catch {
    toast.add({ severity: 'warn', summary: 'Odkaz sa nedá skopírovať', life: 3000 })
  }
}

function remove(p) {
  confirm.require({
    header: 'Odobrať telefón',
    message: `Telefón „${p.name}“ prestane dostávať údaje a jeho schránka sa zmaže. Záznamy, ktoré ešte neposlal, sa stratia.`,
    acceptLabel: 'Odobrať',
    rejectLabel: 'Zrušiť',
    accept: async () => {
      try {
        st.value = await api.syncRemovePhone(p.id)
        if (pairing.value?.phone.id === p.id) pairing.value = null
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Neodobral sa', detail: msg(e), life: 6000 })
      }
    },
  })
}

async function saveRelay() {
  busy.value = 'relay'
  relayError.value = ''
  try {
    st.value = await api.syncSetRelay(relay.value.trim())
    toast.add({ severity: 'success', summary: 'Schránka uložená', detail: phones.value.length ? 'Spárované telefóny spárujte znova.' : '', life: 4000 })
  } catch (e) {
    relayError.value = msg(e)
  } finally {
    busy.value = ''
  }
}

const reloadPage = () => location.reload()

async function run() {
  busy.value = 'run'
  try {
    st.value = await api.syncRun()
    if (st.value.phone?.staged) location.reload()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Synchronizácia zlyhala', detail: msg(e), life: 6000 })
  } finally {
    busy.value = ''
  }
}

async function join() {
  busy.value = 'join'
  try {
    st.value = await api.syncJoin(link.value.trim())
    link.value = ''
    await run()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepripojené', detail: msg(e), life: 6000 })
  } finally {
    busy.value = ''
  }
}

function leave() {
  confirm.require({
    header: 'Odpojiť od počítača',
    message: phone.value?.pending
      ? `${itemsText(phone.value.pending)} ešte nie je v počítači a stratí sa. Údaje v telefóne ostanú, ale už sa nebudú obnovovať.`
      : 'Údaje v telefóne ostanú, ale už sa nebudú obnovovať z počítača.',
    acceptLabel: 'Odpojiť',
    rejectLabel: 'Zrušiť',
    accept: async () => {
      try {
        await api.syncLeave()
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Neodpojené', detail: msg(e), life: 6000 })
      }
    },
  })
}

async function dismiss(r) {
  try {
    st.value = await api.syncDismiss(r.id)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Neskryté', detail: msg(e), life: 6000 })
  }
}
</script>

<template>
  <fieldset v-if="st" class="form-section">
    <legend>Telefón mimo domu</legend>

    <!-- počítač -->
    <template v-if="st.role === 'desktop'">
      <p class="lead">
        Telefón dostane kópiu údajov z tohto počítača a môžete v ňom zapisovať nové veci aj mimo domu — pohyb v domácnosti, úhradu
        záväzku, kontakt, pokladničný doklad, koncept faktúry, jazdu či udalosť. Keď je počítač zapnutý, záznamy si vyzdvihne
        a telefónu pošle čerstvú kópiu. Úpravy a mazanie robíte v počítači.
      </p>
      <p class="note">
        <i class="pi pi-lock" aria-hidden="true" /> Údaje idú cez schránku na internete zašifrované kľúčom, ktorý majú iba tento počítač
        a telefón (je v QR kóde). Schránka ich nevie prečítať.
      </p>

      <div v-if="pairing" class="qr">
        <img :src="qr" alt="QR kód na pripojenie telefónu" width="280" height="280" />
        <div>
          <p><strong>Naskenujte QR kód fotoaparátom telefónu.</strong> Otvorí sa Fiškus v prehliadači a stiahne si kópiu údajov.</p>
          <p class="muted">
            <strong>iPhone a Fiškus na ploche:</strong> aplikácia z plochy má vlastné údaje. Otvorte v telefóne app.fiskus.online, zvoľte
            Zdieľať → <strong>Pridať na plochu</strong>, spustite Fiškus z plochy, ťuknite na <strong>Pripojiť k počítaču</strong>
            a odfoťte tento QR kód. V Androide stačí naskenovať a potom ponuka prehliadača → Pridať na plochu.
          </p>
          <p class="muted">QR kód nikomu neukazujte ani neposielajte — kto ho naskenuje, vidí vaše údaje.</p>
          <div class="row">
            <Button label="Kopírovať odkaz" icon="pi pi-copy" size="small" severity="secondary" outlined @click="copyLink" />
            <Button label="Hotovo" size="small" text @click="pairing = null" />
          </div>
        </div>
      </div>
      <div v-else class="row pair">
        <FormField label="Názov telefónu" hint="napr. Janin iPhone">
          <InputText v-model="name" size="small" maxlength="40" class="name" />
        </FormField>
        <Button label="Pripojiť telefón" icon="pi pi-qrcode" size="small" :loading="busy === 'pair'" @click="pair" />
      </div>

      <h3>Pripojené telefóny</h3>
      <p v-if="!phones.length" class="muted">Zatiaľ žiadny.</p>
      <ul v-else class="list">
        <li v-for="p in phones" :key="p.id">
          <i class="pi pi-mobile" aria-hidden="true" />
          <span class="pname">
            {{ p.name }}
            <small>
              pripojený {{ syncTime(p.created) }}<template v-if="p.lastUpload"> · kópia poslaná {{ syncTime(p.lastUpload) }}</template>
              <template v-if="p.received"> · prijaté {{ itemsText(p.received) }}</template>
            </small>
            <small v-if="p.lastError" class="err">{{ p.lastError }}</small>
          </span>
          <Button icon="pi pi-trash" text severity="secondary" size="small" :aria-label="`Odobrať ${p.name}`" @click="remove(p)" />
        </li>
      </ul>
      <div class="row">
        <Button v-if="phones.length" label="Synchronizovať teraz" icon="pi pi-sync" size="small" severity="secondary" outlined :loading="busy === 'run'" @click="run" />
        <Button :label="showRelay ? 'Skryť schránku' : 'Vlastná schránka…'" size="small" text @click="showRelay = !showRelay" />
      </div>

      <div v-if="showRelay" class="relay">
        <FormField label="Adresa schránky" :error="relayError" :hint="`Prázdne = schránka Fiškusa (${st.defaultRelay}). Vlastnú si nasadíte na bezplatný Cloudflare podľa návodu.`">
          <InputText v-model="relay" size="small" placeholder="https://moja-schranka.example.workers.dev" fluid />
        </FormField>
        <Button label="Uložiť" icon="pi pi-check" size="small" severity="secondary" outlined :loading="busy === 'relay'" @click="saveRelay" />
      </div>
    </template>

    <!-- telefón (Fiškus v prehliadači) -->
    <template v-else-if="st.role === 'phone'">
      <template v-if="phone">
        <p class="lead">
          Tento prehliadač má kópiu údajov z počítača <strong>„{{ phone.desktop }}“</strong><template v-if="phone.version">, stav k {{ syncTime(phone.version) }}</template>.
          Nové záznamy sa prenesú do počítača, keď bude zapnutý; úpravy a mazanie robte v počítači.
        </p>
        <p v-if="phoneProblem(phone)" class="err">{{ phoneProblem(phone) }}</p>
        <p v-if="phone.lastCheck" class="muted">Posledné spojenie {{ syncTime(phone.lastCheck) }}.</p>
        <div class="row">
          <Button :label="phone.staged ? 'Načítať novú kópiu' : 'Synchronizovať teraz'" icon="pi pi-sync" size="small" :loading="busy === 'run'" @click="phone.staged ? reloadPage() : run()" />
          <Button label="Odpojiť od počítača" size="small" text severity="secondary" @click="leave" />
        </div>

        <h3>Čaká na prenos do počítača</h3>
        <p v-if="!phone.items.length" class="muted">Nič — všetko je v počítači.</p>
        <ul v-else class="list">
          <li v-for="it in phone.items" :key="it.id">
            <i class="pi" :class="it.status === 'sent' ? 'pi-cloud-upload' : 'pi-clock'" aria-hidden="true" />
            <span class="pname">{{ it.title }}<small>{{ syncTime(it.at) }} · {{ it.status === 'sent' ? 'v schránke, čaká na počítač' : 'v telefóne, čaká na internet' }}</small></span>
          </li>
        </ul>

        <template v-if="phone.failed.length">
          <h3>Počítač neprijal</h3>
          <ul class="list">
            <li v-for="r in phone.failed" :key="r.id">
              <i class="pi pi-times-circle bad" aria-hidden="true" />
              <span class="pname">{{ r.title }}<small class="err">{{ r.error }}</small></span>
              <Button icon="pi pi-times" text severity="secondary" size="small" :aria-label="`Skryť ${r.title}`" @click="dismiss(r)" />
            </li>
          </ul>
          <p class="muted">Tieto záznamy zapíšte znova — v počítači alebo v telefóne po oprave.</p>
        </template>
      </template>
      <template v-else>
        <p class="lead">
          Fiškus v prehliadači môže byť kópiou údajov z nainštalovaného Fiškusa v počítači. V počítači otvorte Nastavenia → Telefón mimo
          domu a naskenujte QR kód — alebo sem vložte odkaz.
        </p>
        <div class="row pair">
          <FormField label="Odkaz z počítača">
            <InputText v-model="link" size="small" placeholder="https://app.fiskus.online/#pripojit&…" class="name" />
          </FormField>
          <Button label="Pripojiť" icon="pi pi-link" size="small" :disabled="!link.trim()" :loading="busy === 'join'" @click="join" />
        </div>
      </template>
    </template>

    <p v-else class="lead">Telefón mimo domu je v nainštalovanom Fiškuse. Na serveri používajte Fiškus v telefóne priamo cez jeho adresu.</p>
  </fieldset>
</template>

<style scoped>
.lead {
  max-width: 75ch;
  margin-bottom: 8px;
}
.note {
  max-width: 75ch;
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  display: flex;
  gap: 6px;
  align-items: baseline;
  margin-bottom: 12px;
}
.row {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  flex-wrap: wrap;
  margin: 8px 0;
}
.pair > .p-button {
  margin-top: 21px; /* zarovnané s poľom pod nadpisom */
}
.name {
  width: 18rem;
  max-width: 100%;
}
.qr {
  display: flex;
  gap: 18px;
  align-items: flex-start;
  flex-wrap: wrap;
  margin: 12px 0;
  padding: 12px;
  border: 1px solid var(--rule);
  background: var(--sheet);
  max-width: 760px;
}
.qr > div {
  flex: 1 1 260px;
  display: grid;
  gap: 6px;
}
.qr img {
  image-rendering: pixelated;
  max-width: 100%;
  height: auto;
}
h3 {
  margin: 16px 0 6px;
  font-size: var(--fs-md);
  color: var(--blue-deep);
}
.list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-width: 640px;
  border-top: 1px solid var(--rule);
}
.list li {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 6px 2px;
  border-bottom: 1px solid var(--rule);
}
.list .pi {
  color: var(--blue);
}
.list .pi.bad {
  color: var(--tone-overdue);
}
.pname {
  flex: 1;
  min-width: 0;
}
.pname small {
  display: block;
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.muted {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  max-width: 75ch;
}
.err,
.pname small.err {
  color: var(--tone-overdue);
}
.relay {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  flex-wrap: wrap;
  max-width: 640px;
}
.relay > :first-child {
  flex: 1 1 320px;
}
.relay > .p-button {
  margin-top: 21px;
}
</style>
