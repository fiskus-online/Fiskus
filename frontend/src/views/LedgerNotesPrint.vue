<script setup>
// Poznámky k účtovnej závierke (Úč POD 3-01) na tlač alebo do PDF.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount } from '../lib/format.js'
import { notesSections } from '../lib/notes.js'

const route = useRoute()
const router = useRouter()
const year = Number(route.query.rok)
const notes = ref(null)
const error = ref('')
// sumy výkazov sú zo servera už v celých eurách
const euro = (v) => `${formatAmount(v).replace(/,00$/, '')} €`
const sections = computed(() => notesSections(notes.value, euro))
const part = (name) => (notes.value?.rows ?? []).filter((r) => r.statement === name)
const parts = [
  { name: 'aktíva', title: 'Súvaha — strana aktív (netto)' },
  { name: 'pasíva', title: 'Súvaha — strana pasív' },
  { name: 'výkaz', title: 'Výkaz ziskov a strát' },
]

onMounted(async () => {
  try {
    notes.value = await api.ledgerNotes(session.companyId, year)
    document.title = `Poznámky k účtovnej závierke ${year}`
  } catch (e) {
    error.value = e.message
  }
})
const back = () => router.push('/hlavna-kniha')
const print = () => window.print()
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="back" />
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“</span>
      <Button icon="pi pi-print" label="Tlačiť / PDF" size="small" :disabled="!notes" @click="print" />
    </nav>
    <p v-if="error" class="err">{{ error }}</p>
    <article v-if="notes" class="page">
      <header class="head">
        <span>poznámky Úč POD 3-01</span>
        <span>IČO {{ notes.company.ico }}<template v-if="notes.company.dic"> · DIČ {{ notes.company.dic }}</template></span>
      </header>
      <h1>Poznámky k účtovnej závierke {{ year }}</h1>
      <p class="sub">{{ notes.company.name }} — účtovná závierka ku dňu 31. 12. {{ year }}, v celých eurách</p>
      <section v-for="s in sections" :key="s.title">
        <h2>{{ s.title }}</h2>
        <p v-for="(p, i) in s.paragraphs" :key="i">{{ p }}</p>
        <template v-if="s.table">
          <table v-for="pt in parts" :key="pt.name" class="nums">
            <caption>{{ pt.title }}</caption>
            <thead>
              <tr><th>Riadok</th><th>Položka</th><th class="num">{{ year }}</th><th v-if="!notes.previousEmpty" class="num">{{ year - 1 }}</th></tr>
            </thead>
            <tbody>
              <tr v-for="r in part(pt.name)" :key="r.code">
                <td>{{ r.code }}</td>
                <td>{{ r.label }}</td>
                <td class="num">{{ euro(r.current) }}</td>
                <td v-if="!notes.previousEmpty" class="num">{{ euro(r.previous) }}</td>
              </tr>
            </tbody>
          </table>
        </template>
      </section>
    </article>
  </div>
</template>

<style scoped>
.print-root {
  max-width: 820px;
  margin: 0 auto;
  padding: 12px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.spacer {
  flex: 1;
}
.tip {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.page {
  background: #fff;
  padding: 24px 28px;
  border: 1px solid var(--rule);
  font-size: 14px;
  line-height: 1.45;
}
.head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  color: var(--ink-soft);
}
h1 {
  font-size: 20px;
  margin: 12px 0 2px;
}
.sub {
  color: var(--ink-soft);
  margin: 0 0 12px;
}
h2 {
  font-size: 15px;
  margin: 16px 0 6px;
}
p {
  margin: 0 0 6px;
  white-space: pre-line;
}
.nums {
  width: 100%;
  border-collapse: collapse;
  margin: 8px 0 12px;
  font-variant-numeric: tabular-nums;
}
.nums caption {
  text-align: left;
  font-weight: 600;
  padding: 4px 0;
}
.nums th,
.nums td {
  border-bottom: 1px solid var(--rule);
  padding: 3px 6px;
  text-align: left;
  vertical-align: top;
}
.nums .num {
  text-align: right;
  white-space: nowrap;
}
.err {
  color: var(--tone-overdue);
}
@media (max-width: 600px) {
  .page {
    padding: 14px 12px;
  }
  .tip {
    display: none;
  }
}
@media print {
  .toolbar {
    display: none;
  }
  .print-root {
    padding: 0;
    max-width: none;
  }
  .page {
    border: 0;
    padding: 0;
  }
  .nums {
    page-break-inside: avoid;
  }
}
</style>
