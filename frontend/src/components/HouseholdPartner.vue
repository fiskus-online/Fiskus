<script setup>
// Kontakt v adresári domácnosti: platby s partnerom (pohyby a záväzky).
// Súčty počíta server (household.PartnerEntries), tu sa iba zobrazujú.
import { onMounted, ref, watch } from 'vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatDate, formatMoney } from '../lib/format.js'
import { obligationIcon, obligationTone } from '../lib/household.js'
import ActivityPanel from './ActivityPanel.vue'

const props = defineProps({ contactId: { type: Number, required: true } })
const data = ref(null)
const all = ref(false)
const shown = 15

async function load() {
  data.value = null
  all.value = false
  if (!can('ledger.read')) return
  data.value = await api.hhPartner(session.companyId, props.contactId).catch(() => null)
}
onMounted(load)
watch(() => props.contactId, load)
</script>

<template>
  <section v-if="data" class="partner" aria-label="Platby s partnerom">
    <h3>Platby</h3>
    <dl v-if="data.count" class="totals">
      <div>
        <dt>Zaplatené {{ data.year }}</dt>
        <dd class="num">{{ formatMoney(data.yearPaid) }}</dd>
      </div>
      <div>
        <dt>Zaplatené spolu</dt>
        <dd class="num">{{ formatMoney(data.paid) }}</dd>
      </div>
      <div v-if="Number(data.received)">
        <dt>Prijaté spolu</dt>
        <dd class="num">{{ formatMoney(data.received) }}</dd>
      </div>
    </dl>
    <p v-else class="muted">Zatiaľ žiadna platba. Partnera vyberiete pri pohybe alebo záväzku v poli „Obchod / komu“; pohyby z banky sa priradia podľa IBAN alebo mena.</p>

    <template v-if="data.obligations.length">
      <h4>Záväzky a pravidelné platby</h4>
      <ul>
        <li v-for="o in data.obligations" :key="o.id">
          <i :class="['pi', obligationIcon(o.kind)]" aria-hidden="true" />
          <RouterLink :to="`/domacnost/zavazky/${o.id}`">{{ o.name }}</RouterLink>
          <span class="tag" :data-tone="obligationTone(o).tone"><span class="tag-text">{{ obligationTone(o).label }}</span></span>
          <span class="sum num">{{ formatMoney(o.installment) }}</span>
        </li>
      </ul>
    </template>

    <template v-if="data.entries.length">
      <h4>Pohyby <span class="count">{{ data.count }}</span></h4>
      <ul>
        <li v-for="e in all ? data.entries : data.entries.slice(0, shown)" :key="e.id">
          <span class="date muted">{{ formatDate(e.date) }}</span>
          <span class="what">{{ e.note || e.category }}</span>
          <span class="sum num" :class="{ inc: Number(e.amount) > 0 }">{{ formatMoney(e.amount) }}</span>
        </li>
      </ul>
      <button v-if="!all && data.entries.length > shown" type="button" class="more" @click="all = true">
        Zobraziť všetky ({{ data.entries.length }})
      </button>
    </template>
  </section>
  <ActivityPanel class="crm-panel" :contact-id="contactId" :can-write="can('contacts.write')" show-about />
</template>

<style scoped>
.partner {
  margin-top: 12px;
}
.partner h3 {
  margin: 0 0 6px;
  font-size: var(--fs-md, 0.95rem);
  border-bottom: 1px solid var(--rule);
  padding-bottom: 2px;
}
.partner h4 {
  margin: 10px 0 2px;
  font-size: var(--fs-sm, 0.85rem);
  color: var(--ink-soft);
  font-weight: 600;
}
.count {
  font-weight: 400;
}
.totals {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin: 0;
}
.totals dt {
  color: var(--ink-soft);
  font-size: var(--fs-sm, 0.85rem);
}
.totals dd {
  margin: 0;
  font-weight: 600;
  text-align: left;
  font-variant-numeric: tabular-nums;
}
ul {
  list-style: none;
  margin: 0;
  padding: 0;
}
li {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.25rem 0;
  border-bottom: 1px solid var(--rule);
  font-variant-numeric: tabular-nums;
}
li .pi {
  color: var(--blue);
  font-size: 0.85em;
}
li a,
.what {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
li a {
  color: var(--blue);
  text-decoration: none;
}
.date {
  width: 5.5rem;
  flex: none;
}
.sum {
  min-width: 6rem;
  text-align: right;
}
.inc {
  color: var(--tone-paid);
}
.muted {
  color: var(--ink-soft);
}
.more {
  margin-top: 6px;
  background: none;
  border: 0;
  padding: 0;
  color: var(--blue);
  cursor: pointer;
  font: inherit;
}
.crm-panel {
  margin-top: 12px;
}
@media (max-width: 560px) {
  .totals {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
