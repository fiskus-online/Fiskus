<script setup>
// Fiškus v prehliadači: výzva na inštaláciu pri funkcii, ktorú vie iba
// nainštalovaný program. Údaje sa prenesú zálohou.
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import { downloadURL, install } from '../lib/install.js'
import { isSuperAdmin } from '../lib/session.js'

const router = useRouter()
const visible = computed({
  get: () => !!install.feature,
  set: (v) => {
    if (!v) install.feature = ''
  },
})
function toBackup() {
  install.feature = ''
  router.push('/zalohy')
}
</script>

<template>
  <Dialog v-model:visible="visible" header="Funkcia nainštalovaného Fiškusa" modal :style="{ width: '30rem' }" :breakpoints="{ '480px': '96vw' }">
    <div class="inst">
      <i class="pi pi-desktop" aria-hidden="true" />
      <div>
        <p class="lead"><strong>{{ install.feature }}</strong> je iba v nainštalovanom Fiškuse.</p>
        <p>Fiškus v prehliadači pracuje bez inštalácie a údaje má iba v tomto počítači. Nemôže sa však pripájať na iné služby, posielať e-maily ani pracovať na pozadí.</p>
        <p>Údaje do nainštalovaného Fiškusa prenesiete zálohou: tu ju stiahnete a v nainštalovanom Fiškuse ju obnovíte v časti Zálohy.</p>
      </div>
    </div>
    <template #footer>
      <Button label="Zavrieť" text size="small" @click="visible = false" />
      <Button v-if="isSuperAdmin()" label="Stiahnuť zálohu" icon="pi pi-save" severity="secondary" size="small" @click="toBackup" />
      <a class="p-button p-button-sm" :href="downloadURL" target="_blank" rel="noopener">
        <i class="pi pi-download" aria-hidden="true" />&nbsp;Stiahnuť Fiškus
      </a>
    </template>
  </Dialog>
</template>

<style scoped>
.inst {
  display: flex;
  gap: 14px;
  font-size: var(--fs-sm);
}
.inst > .pi {
  font-size: 1.6rem;
  color: var(--blue);
  margin-top: 2px;
}
.inst p {
  margin: 0 0 8px;
  color: var(--ink-soft);
}
.inst .lead {
  color: var(--ink);
  font-size: var(--fs-md);
}
a.p-button {
  text-decoration: none;
}
</style>
