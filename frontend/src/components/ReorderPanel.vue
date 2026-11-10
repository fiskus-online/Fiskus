<script setup>
// Na objednanie: skladové položky, ktorých stav aj s objednaným je pod
// minimálnou zásobou. Vybrané položky sa zoskupia podľa dodávateľa
// do rozpracovaných objednávok. Návrh množstva počíta server.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import ExportButton from './ExportButton.vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount } from '../lib/format.js'
import { reorderColumns } from '../lib/columns.js'
import { matches, searchTerms } from '../lib/search.js'

const router = useRouter()
const toast = useToast()
const rows = ref(null)
const picked = ref({}) // itemId → { on, qty }
const search = ref('')
const busy = ref(false)

async function load() {
  try {
    rows.value = await api.reorder(session.companyId)
    picked.value = Object.fromEntries(rows.value.map((r) => [r.itemId, { on: true, qty: String(Number(r.suggested)) }]))
  } catch (e) {
    rows.value = []
    toast.add({ severity: 'error', summary: 'Návrh sa nenačítal', detail: e.message, life: 6000 })
  }
}
onMounted(load)

const visible = computed(() => {
  const terms = searchTerms(search.value)
  return (rows.value ?? []).filter((r) => matches(terms, [r.code, r.name, r.supplier, r.supplierCode]))
})
const chosen = computed(() => visible.value.filter((r) => picked.value[r.itemId]?.on && Number(String(picked.value[r.itemId].qty).replace(',', '.')) > 0))
const suppliers = computed(() => new Set(chosen.value.map((r) => r.supplier || '—')).size)
const n = (v) => formatAmount(v).replace(/,00$/, '')

async function create() {
  busy.value = true
  try {
    const picks = chosen.value.map((r) => ({ itemId: r.itemId, quantity: String(picked.value[r.itemId].qty).replace(/\s/g, '').replace(',', '.') }))
    const made = await api.ordersFromReorder(session.companyId, picks)
    toast.add({ severity: 'success', summary: made.length === 1 ? `Objednávka ${made[0].number} pripravená` : `${made.length} objednávky pripravené`,
      detail: 'Skontrolujte ich a odošlite dodávateľom.', life: 4000 })
    router.push(made.length === 1 ? `/objednavky/${made[0].id}` : '/objednavky')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Objednávky sa nevytvorili', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="reorder">
    <div class="toolbar">
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať položku, dodávateľa" size="small" aria-label="Hľadať" />
      </IconField>
      <div class="spacer" />
      <ExportButton name="Na objednanie" :columns="reorderColumns" :rows="visible" />
      <Button v-if="can('purchases.write') && rows?.length" :label="chosen.length ? `Vytvoriť objednávky (${suppliers})` : 'Vytvoriť objednávky'"
        icon="pi pi-shopping-cart" size="small" :disabled="!chosen.length" :loading="busy" @click="create" />
    </div>
    <section class="sheet scroll">
      <table class="list">
        <thead>
          <tr>
            <th class="chk" />
            <th>Položka</th>
            <th class="hide-m">Dodávateľ</th>
            <th class="num hide-m">Minimum</th>
            <th class="num">Na sklade</th>
            <th class="num hide-m">Objednané</th>
            <th class="num">Objednať</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="rows && !visible.length">
            <td colspan="7" class="empty">
              {{ rows.length ? 'Nič sa nenašlo.' : 'Všetko je nad minimálnou zásobou. Minimum nastavíte pri skladovej položke v záložke Nastavenie.' }}
            </td>
          </tr>
          <tr v-for="r in visible" :key="r.itemId">
            <td class="chk"><Checkbox v-model="picked[r.itemId].on" binary :aria-label="`Objednať ${r.name}`" /></td>
            <td>
              <span v-if="r.code" class="code">{{ r.code }}</span> {{ r.name }}
              <span class="sub">{{ [r.supplier, r.supplierCode].filter(Boolean).join(' · ') || 'bez dodávateľa na karte produktu' }}</span>
            </td>
            <td class="hide-m">{{ r.supplier || '—' }}</td>
            <td class="num hide-m">{{ n(r.min) }} {{ r.unit }}</td>
            <td class="num neg">{{ n(r.stock) }} {{ r.unit }}</td>
            <td class="num hide-m">{{ Number(r.onOrder) ? `${n(r.onOrder)} ${r.unit}` : '' }}</td>
            <td class="num qty">
              <InputText v-model="picked[r.itemId].qty" size="small" inputmode="decimal" :disabled="!picked[r.itemId].on" :aria-label="`Množstvo ${r.name}`" />
            </td>
          </tr>
        </tbody>
      </table>
    </section>
    <p class="hint">Návrh dopĺňa zásobu na minimum (aj s tým, čo už je objednané). Objednávky vzniknú rozpracované, jedna na dodávateľa z karty produktu.</p>
  </div>
</template>

<style scoped>
.reorder {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.spacer {
  flex: 1;
}
.search {
  width: 280px;
}
.search :deep(input) {
  width: 100%;
}
.sheet {
  background: var(--sheet);
  border: 1px solid var(--rule);
  overflow: auto;
}
.list {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.list th,
.list td {
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
  text-align: left;
  vertical-align: middle;
}
.list th {
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink-soft);
}
.list .num {
  text-align: right;
  white-space: nowrap;
}
.chk {
  width: 36px;
}
.code {
  font-weight: 600;
  color: var(--blue-deep);
}
.sub {
  display: block;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.neg {
  color: var(--tone-overdue);
}
.qty {
  width: 110px;
}
.qty :deep(input) {
  width: 90px;
  text-align: right;
}
.empty {
  color: var(--ink-soft);
  text-align: center;
  padding: 18px;
}
.hint {
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
@media (min-width: 761px) {
  .sub {
    display: none;
  }
}
@media (max-width: 760px) {
  .hide-m {
    display: none;
  }
  .search {
    flex: 1 1 100%;
    width: auto;
  }
  .list th,
  .list td {
    padding: 6px;
  }
  .qty :deep(input) {
    width: 70px;
  }
}
</style>
