<script setup>
// Zmazanie firmy so všetkými dátami (iba správca inštalácie). Server pred
// zmazaním vytvorí zálohu; na potvrdenie treba opísať názov firmy.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import { api, ApiError } from '../lib/api.js'
import { session, loadSession } from '../lib/session.js'

const router = useRouter()
const toast = useToast()
const open = ref(false)
const name = ref('')
const error = ref('')
const busy = ref(false)
const matches = computed(() => name.value.trim() === session.company?.name)
const hh = computed(() => session.company?.bookkeeping === 'household')

async function remove() {
  busy.value = true
  error.value = ''
  try {
    const r = await api.deleteCompany(session.companyId, name.value)
    open.value = false
    toast.add({
      severity: 'success',
      summary: hh.value ? 'Domácnosť je zmazaná' : 'Firma je zmazaná',
      detail: r.backup ? `Záloha pred zmazaním: ${r.backup}` : 'Zálohy sú vypnuté — záloha sa nevytvorila.',
      life: 10000,
    })
    await loadSession()
    router.push('/')
  } catch (e) {
    error.value = e instanceof ApiError && e.fields.length ? e.fieldMap().name : e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <fieldset class="form-section danger">
    <legend>{{ hh ? 'Zmazať domácnosť' : 'Zmazať firmu' }}</legend>
    <p v-if="hh">
      Zmaže domácnosť <strong>{{ session.company?.name }}</strong> so všetkými účtami, pohybmi, kategóriami a rozpočtom. Záznamy
      v audite ostanú. Pred zmazaním sa vytvorí záloha, z ktorej sa domácnosť dá obnoviť v Zálohách.
    </p>
    <p v-else>
      Zmaže firmu <strong>{{ session.company?.name }}</strong> so všetkými dokladmi, adresárom, cenníkom a nastaveniami; záznamy
      v audite ostanú. Pred zmazaním sa vytvorí záloha, z ktorej sa firma dá obnoviť v Zálohách. Potom môžete firmu založiť nanovo.
    </p>
    <Button :label="hh ? 'Zmazať domácnosť…' : 'Zmazať firmu…'" icon="pi pi-trash" severity="danger" outlined size="small" @click="(open = true), (name = ''), (error = '')" />
  </fieldset>

  <Dialog v-model:visible="open" :header="hh ? 'Zmazať domácnosť' : 'Zmazať firmu'" modal :style="{ width: '30rem' }" :breakpoints="{ '640px': '94vw' }">
    <p class="confirm">Táto akcia sa nedá vrátiť (iba obnovou zo zálohy). Na potvrdenie napíšte názov {{ hh ? 'domácnosti' : 'firmy' }}:</p>
    <p class="name">{{ session.company?.name }}</p>
    <InputText v-model="name" size="small" fluid autofocus :invalid="!!error" aria-label="Názov firmy na potvrdenie" @keyup.enter="matches && remove()" />
    <small v-if="error" class="err">{{ error }}</small>
    <template #footer>
      <Button label="Zrušiť" text size="small" @click="open = false" />
      <Button label="Zmazať natrvalo" icon="pi pi-trash" severity="danger" size="small" :disabled="!matches" :loading="busy" @click="remove" />
    </template>
  </Dialog>
</template>

<style scoped>
.danger {
  border-color: var(--tone-overdue-bg);
}
.danger legend {
  color: var(--tone-overdue);
}
.danger p {
  max-width: 70ch;
  margin-bottom: 10px;
}
.confirm {
  margin-bottom: 6px;
}
.name {
  margin-bottom: 8px;
  font-weight: 600;
  color: var(--tone-overdue);
}
.err {
  display: block;
  margin-top: 6px;
  color: var(--tone-overdue);
}
</style>
