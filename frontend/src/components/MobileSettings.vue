<script setup>
// Mobil v sieti (desktop): zapnutie, spárovanie telefónu QR kódom, zoznam telefónov.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import QRCode from 'qrcode'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { OTHER, addressOptions, diagnose, remaining } from '../lib/mobile.js'
import { formatDate } from '../lib/format.js'

const toast = useToast()
const confirm = useConfirm()
const st = ref(null)
const enabled = ref(false)
const port = ref(48081)
const busy = ref('')
const pairing = ref(null)
const urlIdx = ref(0)
const qr = ref('')
const left = ref('')
let timer = null
// adresa do QR kódu: '' = automaticky, OTHER = zadať ručne
const addrChoice = ref('')
const addrOther = ref('')
const addrError = ref('')

// diagnostika „telefón sa iba načítava“: posledná požiadavka zo siete a brána firewall
const fw = ref(null)
const checks = computed(() => (st.value?.enabled ? diagnose(st.value, fw.value, nowTick.value) : []))
const nowTick = ref(Date.now())
async function loadFirewall() {
  if (!st.value?.firewallManaged) return
  try {
    fw.value = await api.mobileFirewall()
  } catch {
    fw.value = null
  }
}
async function allowFirewall() {
  busy.value = 'fw'
  try {
    fw.value = await api.allowMobileFirewall()
    toast.add({ severity: 'success', summary: 'Fiškus je povolený v bráne firewall', detail: 'Skúste telefón pripojiť znova.', life: 5000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Povolenie sa nepodarilo', detail: e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message, life: 8000 })
  } finally {
    busy.value = ''
  }
}
// kým je otvorené párovanie, stav sa obnovuje — pokus z telefónu sa ukáže hneď
let poll = null
function startPoll() {
  clearInterval(poll)
  poll = setInterval(async () => {
    nowTick.value = Date.now()
    try {
      const s = await api.mobile()
      st.value = { ...st.value, lastRequest: s.lastRequest, devices: s.devices }
    } catch {
      /* ďalší pokus o chvíľu */
    }
  }, 3000)
}

async function load() {
  try {
    st.value = await api.mobile()
    loadFirewall()
    enabled.value = st.value.enabled
    port.value = st.value.port
    addrChoice.value = st.value.address ?? ''

  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nastavenie sa nenačítalo', detail: e.message, life: 6000 })
  }
}
onMounted(load)
onBeforeUnmount(() => {
  clearInterval(timer)
  clearInterval(poll)
})

async function save() {
  busy.value = 'save'
  try {
    st.value = await api.setMobile(enabled.value, port.value)
    if (!st.value.enabled) pairing.value = null
    toast.add({ severity: st.value.error ? 'warn' : 'success', summary: st.value.enabled ? 'Mobil v sieti zapnutý' : 'Mobil v sieti vypnutý', detail: st.value.error, life: 5000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message, life: 6000 })
  } finally {
    busy.value = ''
  }
}

async function saveAddress() {
  const address = addrChoice.value === OTHER ? addrOther.value.trim() : addrChoice.value
  if (addrChoice.value === OTHER && !address) {
    addrError.value = 'Zadajte adresu počítača'
    return
  }
  busy.value = 'addr'
  addrError.value = ''
  try {
    st.value = await api.setMobileAddress(address)
    addrChoice.value = st.value.address ?? ''
    pairing.value = null // nový QR kód už s vybranou adresou
    toast.add({ severity: 'success', summary: 'Adresa uložená', detail: 'Spárujte telefón znova — QR kód už obsahuje novú adresu.', life: 4000 })
  } catch (e) {
    addrError.value = e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message
  } finally {
    busy.value = ''
  }
}

async function showQR() {
  const url = pairing.value?.urls[urlIdx.value]
  qr.value = url ? await QRCode.toDataURL(url, { width: 260, margin: 2, color: { dark: '#1f3a5c' } }) : ''
}

async function pair() {
  busy.value = 'pair'
  try {
    pairing.value = await api.newPairing()
    urlIdx.value = 0
    await showQR()
    clearInterval(timer)
    const tick = () => {
      left.value = remaining(pairing.value?.expires)
      if (!left.value) {
        clearInterval(timer)
        pairing.value = null
        load() // spárovaný telefón sa ukáže v zozname
      }
    }
    tick()
    timer = setInterval(tick, 1000)
    startPoll()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Kód sa nevytvoril', detail: e instanceof ApiError && e.fields.length ? e.fields[0].message : e.message, life: 6000 })
  } finally {
    busy.value = ''
  }
}

function remove(d) {
  confirm.require({
    header: 'Odobrať telefón',
    message: `Telefón „${d.name}“ stratí prístup k Fiškusu. Znova ho pripojíte novým spárovaním.`,
    acceptLabel: 'Odobrať',
    rejectLabel: 'Zrušiť',
    accept: async () => {
      try {
        st.value = await api.removeDevice(d.id)
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Neodobral sa', detail: e.message, life: 6000 })
      }
    },
  })
}
</script>

<template>
  <fieldset v-if="st" class="form-section">
    <legend>Mobil v sieti</legend>
    <p class="lead">
      Telefón v tej istej sieti (Wi-Fi) sa pripojí k Fiškusu na tomto počítači — napríklad na odfotenie bločkov. Dnu sa dostanú iba
      spárované telefóny; kým Fiškus beží, sú pripojené natrvalo.
    </p>
    <div class="row">
      <label class="switch">
        <ToggleSwitch v-model="enabled" />
        <span><strong>Povoliť telefóny v sieti</strong><small>Windows sa môže opýtať na povolenie v bráne firewall — povoľte ho; ak telefón nenačíta stránku, pozrite nižšie.</small></span>
      </label>
      <FormField label="Port">
        <InputNumber v-model="port" :min="1024" :max="65535" :use-grouping="false" size="small" />
      </FormField>
      <Button label="Uložiť" icon="pi pi-check" size="small" :loading="busy === 'save'" @click="save" />
    </div>
    <p v-if="st.error" class="err"><i class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ st.error }}</p>
    <p v-if="st.enabled && !st.urls.length" class="err">Počítač nie je pripojený do siete — telefón sa nemá kam pripojiť.</p>

    <template v-if="st.enabled">
      <div class="addr">
        <FormField label="Adresa počítača v sieti (do QR kódu)" :span="2" :error="addrError"
          hint="Telefón musí byť v tej istej Wi-Fi. Adresu Wi-Fi karty zistíte vo Windows príkazom ipconfig (IPv4 Address), na Macu v Nastaveniach → Sieť.">
          <Select v-model="addrChoice" :options="addressOptions(st)" option-label="label" option-value="value" size="small" :placeholder="addressOptions(st)[0].label" />
        </FormField>
        <FormField v-if="addrChoice === OTHER" label="Adresa" :error="addrError">
          <InputText v-model="addrOther" size="small" placeholder="192.168.1.20" inputmode="decimal" />
        </FormField>
        <Button label="Použiť" icon="pi pi-check" size="small" severity="secondary" outlined :loading="busy === 'addr'" @click="saveAddress" />
      </div>
    </template>

    <template v-if="st.enabled && st.urls.length">
      <div class="pairing">
        <div v-if="pairing" class="qr">
          <img :src="qr" alt="QR kód na spárovanie telefónu" width="260" height="260" />
          <div>
            <p><strong>Naskenujte QR kód fotoaparátom telefónu</strong> a potvrďte spárovanie.</p>
            <p class="code">{{ pairing.code }}</p>
            <p class="muted">Kód platí ešte {{ left }} a dá sa použiť raz.</p>
            <p v-if="pairing.urls.length > 1" class="muted">
              Počítač má viac sietí:
              <button v-for="(u, i) in pairing.urls" :key="u" type="button" class="link" :class="{ on: i === urlIdx }" @click="(urlIdx = i), showQR()">
                {{ u.split('/')[2] }}
              </button>
            </p>
            <p class="muted">Bez QR kódu: v telefóne otvorte <code>{{ pairing.urls[urlIdx].split('/parovanie')[0] }}/parovanie</code> a zadajte kód.</p>
          </div>
        </div>
        <Button v-else label="Spárovať telefón" icon="pi pi-qrcode" size="small" :loading="busy === 'pair'" @click="pair" />
      </div>

      <section class="diag" aria-label="Telefón sa nepripojí">
        <h3>Telefón sa iba načítava?</h3>
        <ul class="checks">
          <li v-for="(c, i) in checks" :key="i" :data-tone="c.tone">
            <i class="pi" :class="c.tone === 'ok' ? 'pi-check-circle' : c.tone === 'bad' ? 'pi-times-circle' : 'pi-exclamation-circle'" aria-hidden="true" />
            <span>{{ c.text }}</span>
            <Button v-if="c.fix" label="Povoliť vo firewalle" icon="pi pi-shield" size="small" severity="secondary" outlined :loading="busy === 'fw'" @click="allowFirewall" />
          </li>
        </ul>
        <ol class="steps">
          <li>Telefón musí byť pripojený na <strong>tú istú Wi-Fi</strong> ako počítač — nie na hosťovskú sieť a nie cez mobilné dáta. Vypnite v telefóne VPN.</li>
          <li v-if="st.firewallManaged">Windows pripojenia z telefónu potichu zahodí, ak Fiškus nemá povolenie v bráne firewall. <strong>Povoliť vo firewalle</strong> pridá pravidlo iba pre port {{ st.port }} — Windows sa opýta na povolenie správcu.</li>
          <li>Ak má počítač viac sietí, vyberte vyššie adresu Wi-Fi karty a spárujte znova.</li>
          <li>Niektoré routery zakazujú spojenie medzi zariadeniami v Wi-Fi (izolácia klientov, „AP isolation“) — v routeri ju vypnite.</li>
        </ol>
        <p class="muted">Kým je QR kód otvorený, Fiškus každé 3 sekundy kontroluje, či z telefónu niečo prišlo.</p>
      </section>

      <h3>Spárované telefóny</h3>
      <p v-if="!st.devices.length" class="muted">Zatiaľ žiadny.</p>
      <ul v-else class="devices">
        <li v-for="d in st.devices" :key="d.id">
          <i class="pi pi-mobile" aria-hidden="true" />
          <span class="dname">{{ d.name }}<small>spárovaný {{ formatDate(d.created.slice(0, 10)) }}, naposledy {{ formatDate(d.lastSeen.slice(0, 10)) }}</small></span>
          <Button icon="pi pi-trash" text severity="secondary" size="small" :aria-label="`Odobrať ${d.name}`" @click="remove(d)" />
        </li>
      </ul>
      <p class="muted">Na telefóne si stránku Bločky pridajte na plochu (ponuka prehliadača → Pridať na plochu).</p>
    </template>
  </fieldset>
</template>

<style scoped>
.diag {
  margin: 14px 0 4px;
  padding: 10px 12px;
  background: var(--paper);
  border-left: 3px solid var(--blue);
}
.diag h3 {
  margin: 0 0 6px;
}
.checks {
  list-style: none;
  margin: 0 0 8px;
  padding: 0;
  display: grid;
  gap: 6px;
  font-size: var(--fs-sm);
}
.checks li {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.checks li span {
  flex: 1 1 18rem;
}
.checks [data-tone='ok'] .pi {
  color: var(--tone-paid);
}
.checks [data-tone='bad'] {
  color: var(--tone-overdue);
}
.checks [data-tone='warn'] .pi {
  color: var(--tone-draft);
}
.steps {
  margin: 0 0 6px;
  padding-left: 20px;
  font-size: var(--fs-sm);
  display: grid;
  gap: 3px;
  max-width: 75ch;
}
.lead {
  max-width: 70ch;
  margin-bottom: 10px;
}
.row {
  display: flex;
  align-items: flex-end;
  gap: 16px;
  flex-wrap: wrap;
}
.switch {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  max-width: 420px;
  cursor: pointer;
}
.switch small {
  display: block;
  color: var(--ink-faint);
}
.err {
  margin-top: 10px;
  color: var(--tone-overdue);
}
.pairing {
  margin: 16px 0;
}
.addr {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  flex-wrap: wrap;
  margin: 12px 0;
}
.addr > .p-button {
  margin-top: 21px; /* zarovnané s poľom pod nadpisom */
}
.addr > :first-child {
  flex: 1 1 340px;
  min-width: 0;
}
.qr {
  display: flex;
  gap: 18px;
  align-items: flex-start;
  flex-wrap: wrap;
}
.qr img {
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  background: #fff;
}
.qr p {
  margin-bottom: 6px;
  max-width: 44ch;
}
.code {
  font-size: 1.8rem;
  letter-spacing: 0.3em;
  font-weight: 600;
  color: var(--blue-deep);
  font-variant-numeric: tabular-nums;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.link {
  margin-left: 6px;
  background: none;
  border: 0;
  color: var(--blue);
  text-decoration: underline;
  cursor: pointer;
  font: inherit;
}
.link.on {
  font-weight: 600;
}
h3 {
  margin: 14px 0 6px;
  font-size: var(--fs-md);
}
.devices {
  list-style: none;
  padding: 0;
  margin: 0;
  max-width: 560px;
}
.devices li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.dname {
  flex: 1;
}
.dname small {
  display: block;
  color: var(--ink-faint);
}
</style>
