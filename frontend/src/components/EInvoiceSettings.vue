<script setup>
// Pripojenie k digitálnemu poštárovi (Peppol): národné rozhranie SAPI-SK,
// priečinky alebo vlastný adaptér.
import { computed, onMounted, ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { session, isSuperAdmin } from '../lib/session.js'
import { postmanModes, sapiProviderOf, sapiProviders } from '../lib/einvoice.js'

const toast = useToast()
const form = ref({ mode: '', outDir: '', inDir: '', url: '', clientId: '', token: '', provider: '' })
const tokenSet = ref(false)
const errors = ref({})
const busy = ref(false)
const loaded = ref(false)
// priečinky servera smie nastaviť iba správca inštalácie
const modes = computed(() => postmanModes(session.caps.mode, isSuperAdmin()))

onMounted(async () => {
  try {
    const s = await api.einvoiceSettings(session.companyId)
    tokenSet.value = s.tokenSet
    form.value = { mode: s.mode ?? '', outDir: s.outDir ?? '', inDir: s.inDir ?? '', url: s.url ?? '', clientId: s.clientId ?? '', token: '', provider: s.provider ?? '' }
    preset.value = form.value.url ? sapiProviderOf(form.value.url) : 'ePošťák'
    savedURL.value = form.value.url
  } finally {
    loaded.value = true
  }
})

// výber známeho poskytovateľa SAPI-SK doplní adresu a názov
const preset = ref('ePošťák')
const savedURL = ref('')
watch(preset, (label) => {
  const p = sapiProviders.find((x) => x.label === label)
  if (p?.url) {
    form.value.url = p.url
    form.value.provider = label.replace(/ – .*/, '')
  }
})
// uložené tajomstvo platí iba pre uloženú adresu (server ho inak nepoužije)
const secretKept = computed(() => tokenSet.value && form.value.url === savedURL.value)
watch(
  () => form.value.mode,
  (m) => {
    if (m === 'sapi' && !form.value.url) {
      const p = sapiProviders.find((x) => x.label === preset.value)
      if (p?.url) form.value.url = p.url
      if (!form.value.provider) form.value.provider = preset.value.replace(/ – .*/, '')
    }
  },
)

const checking = ref(false)
async function check() {
  checking.value = true
  try {
    await api.checkEInvoice(session.companyId)
    toast.add({ severity: 'success', summary: 'Spojenie s poštárom funguje', detail: 'Prihlásenie aj schránka firmy sú v poriadku.', life: 4000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Spojenie zlyhalo', detail: e.message, life: 8000 })
  } finally {
    checking.value = false
  }
}

async function save() {
  busy.value = true
  try {
    const s = await api.saveEInvoiceSettings(session.companyId, form.value)
    tokenSet.value = s.tokenSet
    savedURL.value = s.url ?? ''
    form.value.token = ''
    errors.value = {}
    toast.add({ severity: 'success', summary: 'Pripojenie e-faktúr uložené', life: 2500 })
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    toast.add({ severity: 'error', summary: 'Nastavenie sa neuložilo', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <fieldset v-if="loaded" class="form-section einv">
    <legend>E-faktúry (digitálny poštár)</legend>
    <p class="help">
      Od 1. 1. 2027 si platitelia DPH posielajú faktúry ako e-faktúry cez sieť Peppol. Fiškus e-faktúru vytvorí; odošle a prijme ju
      váš digitálny poštár (certifikovaný poskytovateľ). Vaša adresa v sieti je <strong>0245:{{ session.company?.dic || 'DIČ' }}</strong>.
    </p>
    <SelectButton v-model="form.mode" :options="modes" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Pripojenie" />
    <div v-if="form.mode" class="form-grid">
      <FormField v-if="form.mode !== 'sapi'" label="Poskytovateľ" :span="2" hint="Názov poštára (iba pre vás)">
        <InputText v-model="form.provider" size="small" />
      </FormField>
      <template v-if="form.mode === 'folder'">
        <FormField label="Priečinok na odosielanie" :span="2" :error="errors.outDir" hint="Sem Fiškus uloží e-faktúru; aplikácia poštára ju odošle">
          <InputText v-model="form.outDir" size="small" :invalid="!!errors.outDir" placeholder="napr. C:\Users\…\Postar\Odoslat" />
        </FormField>
        <FormField label="Priečinok prijatých" :span="2" :error="errors.inDir" hint="Odtiaľto Fiškus číta prijaté e-faktúry; prevzaté presunie do „prevzate“">
          <InputText v-model="form.inDir" size="small" :invalid="!!errors.inDir" placeholder="napr. C:\Users\…\Postar\Prijate" />
        </FormField>
      </template>
      <template v-else-if="form.mode === 'sapi'">
        <FormField label="Poskytovateľ SAPI-SK" :span="2" hint="Rozhranie SAPI-SK majú viacerí certifikovaní poštári">
          <Select v-model="preset" :options="sapiProviders" option-label="label" option-value="label" size="small" />
        </FormField>
        <FormField v-if="preset === 'Iný poskytovateľ'" label="Názov poskytovateľa" :span="2" hint="Iba pre vás">
          <InputText v-model="form.provider" size="small" />
        </FormField>
        <FormField label="Adresa API" :span="2" :error="errors.url" hint="Z dokumentácie poskytovateľa, končí /sapi alebo /sapi/v1">
          <InputText v-model="form.url" size="small" :invalid="!!errors.url" placeholder="https://…/sapi/v1" />
        </FormField>
        <FormField label="Client ID" :span="2" :error="errors.clientId">
          <InputText v-model="form.clientId" size="small" :invalid="!!errors.clientId" autocomplete="off" spellcheck="false" />
        </FormField>
        <FormField label="Client secret" :span="2" :error="errors.token" :hint="secretKept ? 'Uložené — nové zadajte iba pri zmene' : tokenSet ? 'Pri zmene adresy treba zadať znova' : ''">
          <Password v-model="form.token" :feedback="false" toggle-mask size="small" fluid :invalid="!!errors.token" autocomplete="off" />
        </FormField>
      </template>
      <template v-else>
        <FormField label="Adresa API" :span="2" :error="errors.url" hint="Adaptér poskytovateľa (POST /documents, GET /inbox)">
          <InputText v-model="form.url" size="small" :invalid="!!errors.url" placeholder="https://…" />
        </FormField>
        <FormField label="Prístupový token" :span="2" :error="errors.token" :hint="secretKept ? 'Uložený — nový zadajte iba pri zmene' : tokenSet ? 'Pri zmene adresy treba zadať znova' : ''">
          <Password v-model="form.token" :feedback="false" toggle-mask size="small" fluid :invalid="!!errors.token" autocomplete="off" />
        </FormField>
      </template>
    </div>
    <p v-if="form.mode === 'sapi'" class="help">
      Client ID a secret vygenerujete v portáli svojho poštára (časť API / integrácia). Poštára si zároveň vyberte na portáli
      finančnej správy (výber digitálneho poštára) — inak vám e-faktúry nebude doručovať.
    </p>
    <div class="actions">
      <Button label="Uložiť pripojenie" icon="pi pi-check" severity="secondary" outlined size="small" :loading="busy" @click="save" />
      <Button v-if="form.mode && form.mode !== 'folder'" label="Overiť spojenie" icon="pi pi-wifi" text size="small" :loading="checking"
        v-tooltip.bottom="'Overí uložené pripojenie — prihlásenie a schránku firmy, nič neodošle'" @click="check" />
    </div>
  </fieldset>
</template>

<style scoped>
.einv {
  display: grid;
  gap: 10px;
  justify-items: start;
}
.einv .form-grid {
  width: 100%;
}
.einv :deep(.p-selectbutton) {
  flex-wrap: wrap;
}
.einv :deep(.p-togglebutton) {
  white-space: nowrap;
}
.actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.help {
  margin: 0;
  max-width: 70ch;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
</style>
