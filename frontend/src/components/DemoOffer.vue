<script setup>
// Ponuka ukážkovej firmy v prázdnom Fiškuse (aj v prehliadači): jedným
// kliknutím sa načíta firma s dokladmi a domácnosť Novákovci na vyskúšanie.
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { loadSession, selectCompany, session } from '../lib/session.js'

const router = useRouter()
const toast = useToast()
const busy = ref(false)

async function load() {
  busy.value = true
  try {
    const c = await api.loadDemo()
    await loadSession()
    selectCompany(c.id)
    toast.add({ severity: 'success', summary: 'Ukážková firma načítaná', detail: c.name, life: 3000 })
    router.push('/faktury')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Ukážku sa nepodarilo načítať', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="session.caps.demo" class="demo-offer">
    <div>
      <strong>Najprv si to vyskúšajte</strong>
      <p>
        Načíta sa ukážková firma s faktúrami, DPH, bankou, skladom a mzdami a domácnosť Novákovci s bločkami a rozpočtom. Keď budete
        chcieť začať naostro, ukážku zmažete v Nastaveniach a založíte svoju firmu.
      </p>
    </div>
    <Button label="Vyskúšať na ukážkovej firme" icon="pi pi-play" size="small" :loading="busy" @click="load" />
  </div>
</template>

<style scoped>
.demo-offer {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px 14px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 3px solid var(--blue-deep);
  text-align: left;
  max-width: 640px;
}
.demo-offer strong {
  color: var(--blue-deep);
}
.demo-offer p {
  margin: 4px 0 0;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.demo-offer :deep(.p-button) {
  flex: none;
}
@media (max-width: 560px) {
  .demo-offer {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
