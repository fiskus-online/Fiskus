<script setup>
// Pole IBAN: zápis po štvoriciach, kontrola pri písaní a banka podľa kódu.
// Chyba zo servera (FormField :error) má prednosť, toto je iba pomoc pri písaní.
import { computed } from 'vue'
import InputText from 'primevue/inputtext'
import { ibanGroups, ibanStatus } from '../lib/iban.js'

const model = defineModel({ type: String, default: '' })
defineProps({ disabled: { type: Boolean, default: false }, invalid: { type: Boolean, default: false } })
const st = computed(() => ibanStatus(model.value))
// pri opustení poľa sa IBAN zapíše po štvoriciach
function tidy() {
  if (model.value) model.value = ibanGroups(model.value)
}
</script>

<template>
  <span class="iban">
    <InputText v-model="model" size="small" class="iban-in" :disabled="disabled" :invalid="invalid || !!st.error" autocapitalize="characters"
      spellcheck="false" placeholder="SK00 0000 0000 0000 0000 0000" maxlength="42" @blur="tidy" />
    <small v-if="st.error" class="bad" role="alert">{{ st.error }}</small>
    <small v-else-if="st.ok" class="ok"><i class="pi pi-check-circle" aria-hidden="true" /> {{ st.bank || 'IBAN je v poriadku' }}</small>
  </span>
</template>

<style scoped>
.iban {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.iban-in {
  width: 100%;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}
.ok {
  color: var(--tone-paid);
  font-size: var(--fs-sm);
}
.bad {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
</style>
