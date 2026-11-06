<script setup>
// Fiškus v prehliadači: pripomienka zálohy, keď chýba alebo je stará.
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../lib/api.js'
import { reminderText } from '../lib/browserbackup.js'
import { isSuperAdmin } from '../lib/session.js'

const route = useRoute()
const text = ref('')
const hidden = ref(false)
async function load() {
  if (!isSuperAdmin()) return
  try {
    text.value = reminderText(await api.browserBackup())
  } catch {
    text.value = ''
  }
}
// po návrate zo záloh (alebo po zmene obrazovky) sa stav obnoví
watch(() => route.path, load, { immediate: true })
</script>

<template>
  <div v-if="text && !hidden && route.path !== '/zalohy'" class="bb" role="status">
    <i class="pi pi-exclamation-circle" aria-hidden="true" />
    <span>{{ text }}</span>
    <RouterLink to="/zalohy">Stiahnuť zálohu</RouterLink>
    <button type="button" aria-label="Skryť pripomienku" @click="hidden = true"><i class="pi pi-times" /></button>
  </div>
</template>

<style scoped>
.bb {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 6px 16px;
  background: var(--tone-draft-bg, #fdf3dc);
  border-bottom: 1px solid var(--rule);
  font-size: var(--fs-sm);
  color: var(--ink);
}
.bb > .pi {
  color: var(--tone-draft);
}
.bb span {
  flex: 1 1 16rem;
}
.bb a {
  color: var(--blue);
  font-weight: 600;
}
.bb button {
  border: 0;
  background: none;
  color: var(--ink-soft);
  cursor: pointer;
  padding: 2px 4px;
}
</style>
