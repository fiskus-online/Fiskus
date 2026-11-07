<script setup>
// Napojenie na Fio banku: token z internetbankingu (iba na čítanie pohybov).
// Token sa po uložení už nikdy nezobrazí — server vracia iba, že je nastavený.
import { ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Drawer from 'primevue/drawer'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import FormField from './FormField.vue'
import { api } from '../lib/api.js'
import { formatDate } from '../lib/format.js'
import { fioError, fioPayload, importSummary, lastSyncLabel, validToken } from '../lib/fio.js'

const props = defineProps({
  cid: { type: Number, required: true },
  canWrite: { type: Boolean, default: false },
  accounts: { type: Array, default: () => [] },
})
const visible = defineModel('visible', { type: Boolean, default: false })
const emit = defineEmits(['changed', 'synced'])
const toast = useToast()
const confirm = useConfirm()

const form = ref(null)
const errs = ref({})
const busy = ref(false)
const syncing = ref(0)

watch(visible, (v) => {
  if (v && !props.accounts.length) openNew()
  if (!v) form.value = null
})

function openNew() {
  form.value = { id: 0, label: 'Fio – bežný účet', token: '' }
  errs.value = {}
}
function openEdit(a) {
  form.value = { id: a.id, label: a.label, token: '' }
  errs.value = {}
}

async function save() {
  errs.value = {}
  if (!form.value.id && !validToken(form.value.token)) {
    errs.value = { token: 'Vložte celý token z internetbankingu Fio (písmená a číslice)' }
    return
  }
  busy.value = true
  try {
    const list = await api.saveFio(props.cid, fioPayload(form.value))
    emit('changed', list)
    form.value = null
    toast.add({ severity: 'success', summary: 'Napojenie uložené', life: 3000 })
  } catch (e) {
    errs.value = fioError(e)
    if (!Object.keys(errs.value).length) toast.add({ severity: 'error', summary: 'Napojenie sa neuložilo', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

function remove(a) {
  confirm.require({
    header: 'Zrušiť napojenie',
    message: `Zrušiť napojenie „${a.label}“? Stiahnuté pohyby v Banke zostanú.`,
    acceptLabel: 'Zrušiť napojenie',
    rejectLabel: 'Ponechať',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        emit('changed', await api.deleteFio(props.cid, a.id))
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Napojenie sa nezrušilo', detail: e.message, life: 6000 })
      }
    },
  })
}

async function sync(a) {
  syncing.value = a.id
  try {
    const r = await api.syncFio(props.cid, a.id)
    toast.add({ severity: 'success', summary: `Stiahnuté z Fio (${a.label})`, detail: importSummary(r), life: 6000 })
    for (const w of r.warnings || []) toast.add({ severity: 'warn', summary: 'Upozornenie', detail: w, life: 8000 })
    emit('synced', r)
  } catch (e) {
    const m = fioError(e)
    toast.add({ severity: 'error', summary: 'Sťahovanie z Fio zlyhalo', detail: m.fio || m.token || e.message, life: 8000 })
  } finally {
    syncing.value = 0
  }
}
</script>

<template>
  <Drawer v-model:visible="visible" position="right" class="fio-panel" header="Fio banka – napojenie">
    <div class="fio">
      <p class="lead">Fiškus si pohyby stiahne priamo z Fio banky, bez výpisu. Stačí token z internetbankingu: <b>Nastavenia → API → Pridať nový token</b>, oprávnenie <b>iba sledovať účet</b>. Token nedovolí zadať platbu.</p>

      <ul v-if="accounts.length" class="accs">
        <li v-for="a in accounts" :key="a.id" class="acc">
          <div class="who">
            <strong>{{ a.label }}</strong>
            <span class="meta">{{ a.account || 'účet sa zistí pri prvom stiahnutí' }}</span>
            <span class="meta">{{ lastSyncLabel(a, formatDate) }}</span>
          </div>
          <div class="acts">
            <Button size="small" icon="pi pi-download" label="Stiahnuť" :loading="syncing === a.id" :disabled="!canWrite || !!syncing" @click="sync(a)" />
            <Button v-if="canWrite" size="small" text icon="pi pi-pencil" aria-label="Upraviť" @click="openEdit(a)" />
            <Button v-if="canWrite" size="small" text severity="danger" icon="pi pi-trash" aria-label="Zrušiť napojenie" @click="remove(a)" />
          </div>
        </li>
      </ul>

      <form v-if="form && canWrite" class="form-grid one" @submit.prevent="save">
        <FormField label="Názov" :error="errs.label">
          <InputText v-model="form.label" size="small" />
        </FormField>
        <FormField label="Token Fio API" :error="errs.token" :hint="form.id ? 'prázdne = ponechať uložený token' : 'uloží sa vo Fiškuse, už sa nikde nezobrazí'">
          <Password v-model="form.token" :feedback="false" toggle-mask size="small" fluid autocomplete="off" input-id="fio-token" />
        </FormField>
        <div class="row">
          <Button type="submit" size="small" icon="pi pi-check" :label="form.id ? 'Uložiť' : 'Napojiť účet'" :loading="busy" />
          <Button v-if="accounts.length" size="small" text label="Zrušiť" @click="form = null" />
        </div>
      </form>
      <Button v-else-if="canWrite" size="small" outlined icon="pi pi-plus" label="Napojiť ďalší účet" @click="openNew" />

      <p class="note">Pri prvom stiahnutí Fiškus načíta pohyby za posledných 90 dní, potom vždy od posledného stiahnutého dňa. Fio dovolí jedno stiahnutie za 30 sekúnd. Už načítané pohyby sa nezdvoja a párujú sa s faktúrami rovnako ako z výpisu.</p>
    </div>
  </Drawer>
</template>

<style scoped>
.fio {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}
.lead,
.note {
  margin: 0;
  color: var(--ink-soft, #4a5866);
  font-size: 0.9rem;
}
.note {
  font-size: 0.82rem;
}
.accs {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--rule, #d8dee6);
}
.acc {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.55rem 0;
  border-bottom: 1px solid var(--rule, #d8dee6);
}
.who {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.meta {
  font-size: 0.82rem;
  color: #4a5a6c;
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}
.acts {
  display: flex;
  align-items: center;
  gap: 0.15rem;
}
.form-grid.one {
  grid-template-columns: minmax(0, 1fr);
}
.row {
  display: flex;
  gap: 0.5rem;
}
</style>

<style>
.p-drawer.p-component.fio-panel {
  width: min(480px, 100vw);
}
</style>
