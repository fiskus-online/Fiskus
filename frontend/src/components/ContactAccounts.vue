<script setup>
// Ďalšie bankové účty kontaktu s účelom (škola: stravné, poplatky za ŠKD,
// rodičovský fond). Hlavný účet je pole IBAN vyššie.
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import IbanInput from './IbanInput.vue'

const list = defineModel({ type: Array, default: () => [] })
defineProps({ errors: { type: Object, default: () => ({}) }, readonly: Boolean })

const add = () => (list.value = [...list.value, { iban: '', label: '' }])
const remove = (i) => (list.value = list.value.filter((_, j) => j !== i))
</script>

<template>
  <section class="sub">
    <header>
      <h3>Ďalšie účty <span v-if="list.length" class="count">{{ list.length }}</span></h3>
      <Button v-if="!readonly" label="Účet" icon="pi pi-plus" text size="small" @click="add" />
    </header>
    <p v-if="!list.length" class="empty">Ak má partner viac účtov (škola: stravné, poplatky, rodičovský fond), pri záväzku si vyberiete ten správny.</p>
    <div v-for="(a, i) in list" :key="i" class="row">
      <InputText v-model="a.label" size="small" placeholder="Na čo, napr. stravné" aria-label="Účel účtu" :readonly="readonly" class="c-label" />
      <IbanInput v-model="a.iban" :invalid="!!errors[`accounts[${i}].iban`]" :disabled="readonly" class="c-iban" />
      <Button v-if="!readonly" icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Odstrániť účet" class="c-act" @click="remove(i)" />
      <small v-if="errors[`accounts[${i}].iban`]" class="err">{{ errors[`accounts[${i}].iban`] }}</small>
    </div>
  </section>
</template>

<style scoped>
.sub {
  border-top: 1px solid var(--rule);
  padding-top: 8px;
  margin-top: 4px;
}
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
h3 {
  margin: 0;
  font-size: var(--fs-md);
  font-weight: 600;
  color: var(--blue-deep);
}
.count {
  color: var(--ink-faint);
  font-weight: 400;
}
.empty {
  margin: 2px 0 6px;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.5fr) auto;
  gap: 6px;
  align-items: start;
  padding: 4px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.row :deep(input) {
  min-width: 0;
}
.err {
  grid-column: 1 / -1;
  color: var(--tone-overdue);
  font-size: var(--fs-xs);
}
@container (max-width: 560px) {
  .row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .c-iban {
    grid-column: 1 / -1;
    grid-row: 2;
  }
}
</style>
