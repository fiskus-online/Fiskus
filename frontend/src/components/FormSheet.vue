<script setup>
// Tlač vyplneného tlačiva na hárku v skutočnej veľkosti (mm). Sumy sú
// zo servera (computed z /api/forms/preview), tu sa iba zobrazujú.
import { computed } from 'vue'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { amountInWords } from '../lib/words.js'
import { bodyFields, byRole, printValue } from '../lib/forms.js'

const props = defineProps({
  form: { type: Object, required: true },
  values: { type: Object, required: true },
  items: { type: Array, default: () => [] },
  calc: { type: Object, default: null },
})

const role = (r) => byRole(props.form, props.values, r)
const field = (r) => props.form.fields.find((f) => f.role === r)
const cash = computed(() => props.form.layout === 'cash')
const income = computed(() => props.form.cash === 'in')
const total = computed(() => props.calc?.total ?? '0')
const hasAmount = computed(() => Number(total.value) > 0)
const hasVat = computed(() => Number(props.calc?.vat ?? 0) > 0)
const body = computed(() => bodyFields(props.form))
const rows = computed(() => props.items.map((it, i) => ({ ...it, line: props.calc?.lines?.[i] ?? '' })).filter((r) => r.desc || Number(r.line)))
const accounting = computed(() => props.values.accounting ?? '')
const num = (s) => String(s ?? '').replace(/[\s\u00a0]/g, '').replace(',', '.')
const sheetClass = computed(() => [cash.value ? 'cash' : 'doc', props.form.paper === 'A5' ? 'a5' : 'a4'])
</script>

<template>
  <article class="sheet" :class="sheetClass">
    <!-- pokladničný doklad -->
    <template v-if="cash">
      <header class="head">
        <div>
          <p class="kind">{{ form.name }}</p>
          <h1>{{ role('number') || ' ' }}</h1>
        </div>
        <div class="issuer">{{ role('issuer') }}</div>
      </header>
      <dl class="rows">
        <div><dt>Dátum</dt><dd>{{ formatDate(role('date')) }}</dd></div>
        <div><dt>{{ field('partner')?.label }}</dt><dd class="pre">{{ role('partner') }}</dd></div>
        <div class="wide"><dt>Účel platby</dt><dd>{{ role('purpose') }}</dd></div>
        <div v-if="role('note')" class="wide"><dt>Poznámka</dt><dd>{{ role('note') }}</dd></div>
      </dl>
      <table v-if="hasVat" class="vat">
        <thead><tr><th>Sadzba DPH</th><th>Základ</th><th>DPH</th><th>Spolu</th></tr></thead>
        <tbody>
          <tr><td>{{ calc.rate }} %</td><td>{{ formatAmount(calc.base) }}</td><td>{{ formatAmount(calc.vat) }}</td><td>{{ formatAmount(calc.total) }}</td></tr>
        </tbody>
      </table>
      <section class="sum">
        <div>
          <span>Suma</span>
          <strong class="grand">{{ hasAmount ? formatMoney(total) : '' }}</strong>
        </div>
        <p class="words">Slovom: {{ hasAmount ? amountInWords(total) : '' }}</p>
      </section>
      <p v-if="accounting" class="acc">Účtovací predpis: {{ accounting }}</p>
      <footer class="signs four">
        <div><b>{{ role('sign1') }}</b><span>Vyhotovil</span></div>
        <div><b>{{ role('sign2') }}</b><span>Schválil</span></div>
        <div><b /><span>{{ income ? 'Prijal (pokladník)' : 'Vyplatil (pokladník)' }}</span></div>
        <div><b /><span>{{ income ? 'Zaplatil' : 'Prijal' }}</span></div>
      </footer>
    </template>

    <!-- list (dodací list, objednávka, vlastné tlačivo …) -->
    <template v-else>
      <header class="head">
        <div>
          <h1 class="title">{{ form.name }}</h1>
          <p v-if="role('number')" class="docnum">č. {{ role('number') }}</p>
        </div>
        <p v-if="role('date')" class="date">{{ field('date')?.label }}: <b>{{ formatDate(role('date')) }}</b></p>
      </header>
      <section v-if="field('issuer') || field('partner')" class="parties">
        <div v-if="field('issuer')"><span>{{ field('issuer').label }}</span><p class="pre">{{ role('issuer') }}</p></div>
        <div v-if="field('partner')"><span>{{ field('partner').label }}</span><p class="pre">{{ role('partner') }}</p></div>
      </section>
      <dl v-if="body.length" class="rows">
        <div v-for="f in body" :key="f.key" :class="{ wide: f.wide || f.type === 'textarea' }">
          <dt>{{ f.label }}</dt><dd class="pre">{{ printValue(f, values[f.key]) }}</dd>
        </div>
      </dl>
      <table v-if="form.items" class="items">
        <thead><tr><th class="n">#</th><th class="d">Popis</th><th>Množstvo</th><th class="u">MJ</th><th>Cena/MJ €</th><th>Spolu €</th></tr></thead>
        <tbody>
          <tr v-for="(r, i) in rows" :key="i">
            <td class="n">{{ i + 1 }}.</td><td class="d">{{ r.desc }}</td><td>{{ String(r.qty ?? '').replace('.', ',') }}</td><td class="u">{{ r.unit }}</td>
            <td>{{ Number(num(r.price)) ? formatAmount(num(r.price)) : '' }}</td><td>{{ Number(r.line) ? formatAmount(r.line) : '' }}</td>
          </tr>
          <tr v-if="!rows.length"><td colspan="6" class="empty">&nbsp;</td></tr>
        </tbody>
        <tfoot v-if="Number(calc?.itemsTotal)">
          <tr><td colspan="5">Spolu</td><td><span class="grand-sm">{{ formatAmount(calc.itemsTotal) }}</span></td></tr>
        </tfoot>
      </table>
      <section v-if="field('amount') && hasAmount" class="sum">
        <div><span>{{ field('amount').label }}</span><strong class="grand">{{ formatMoney(total) }}</strong></div>
        <p class="words">Slovom: {{ amountInWords(total) }}</p>
      </section>
      <footer v-if="field('sign1') || field('sign2')" class="signs">
        <div v-if="field('sign1')"><b>{{ role('sign1') }}</b><span>{{ field('sign1').label }}</span></div>
        <div v-if="field('sign2')"><b>{{ role('sign2') }}</b><span>{{ field('sign2').label }}</span></div>
      </footer>
    </template>
    <!-- logo Fiškus dole v strede každého tlačiva -->
    <p class="brand" aria-label="Fiškus">
      <svg viewBox="0 0 32 32" aria-hidden="true"><rect width="32" height="32" rx="7" fill="#1f3a5c" /><path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#f6f7f9" /><rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" /></svg>
      <span>Fiškus</span>
    </p>
  </article>
</template>

<style scoped>
.sheet {
  position: relative;
  background: #fff;
  color: #1d2633;
  font-family: 'Source Sans 3', system-ui, sans-serif;
  font-size: 10pt;
  line-height: 1.4;
  box-sizing: border-box;
  overflow: hidden;
}
.sheet.cash {
  width: 210mm;
  height: 148mm;
  padding: 10mm 14mm;
}
.sheet.doc.a4 {
  width: 210mm;
  min-height: 297mm;
  padding: 18mm 18mm;
}
.sheet.doc.a5 {
  width: 148mm;
  min-height: 210mm;
  padding: 12mm 12mm;
  font-size: 9.5pt;
}
.pre {
  white-space: pre-line;
}
.brand {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 3mm;
  margin: 0;
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 1.5mm;
  line-height: 1;
  font-weight: 600;
  color: #1f3a5c;
}
.brand svg {
  display: block;
  flex: none;
  width: 4mm;
  height: 4mm;
}
/* text zarovnaný na stred ikony (veľké písmená bez horného priestoru písma) */
.brand span {
  display: block;
  font-size: 9pt;
  line-height: 4mm;
  height: 4mm;
  transform: translateY(0.1mm);
}
.sheet.doc {
  padding-bottom: 14mm !important;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 10mm;
  border-bottom: 2px solid #1f3a5c;
  padding-bottom: 3mm;
}
.kind {
  margin: 0;
  font-size: 9pt;
  color: #4e5866;
}
h1 {
  margin: 0;
  font-size: 18pt;
  color: #1f3a5c;
  min-height: 1.2em;
}
h1.title {
  font-size: 20pt;
}
.docnum {
  margin: 1mm 0 0;
  color: #4e5866;
}
.date {
  margin: 2mm 0 0;
  white-space: nowrap;
}
.issuer {
  white-space: pre-line;
  text-align: right;
  font-size: 9pt;
  max-width: 85mm;
}
.issuer::first-line {
  font-weight: 700;
  font-size: 10.5pt;
}
.parties {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8mm;
  margin: 6mm 0 2mm;
}
.parties div {
  border: 1px solid #c9d1db;
  border-radius: 1.5mm;
  padding: 3mm 4mm;
  min-height: 22mm;
}
.parties span {
  font-size: 8pt;
  color: #6b7685;
}
.parties p {
  margin: 1mm 0 0;
}
.parties p::first-line {
  font-weight: 700;
}
.rows {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 3mm 8mm;
  margin: 5mm 0;
}
.rows div {
  display: flex;
  flex-direction: column;
  border-bottom: 1px solid #dce1e8;
  padding-bottom: 1.5mm;
  min-height: 9mm;
}
.rows .wide {
  grid-column: 1 / -1;
}
.rows dt {
  font-size: 8pt;
  color: #6b7685;
}
.rows dd {
  margin: 0;
  font-weight: 600;
}
.vat {
  width: 100%;
  border-collapse: collapse;
  font-size: 9pt;
  font-variant-numeric: tabular-nums;
  margin-bottom: 2mm;
}
.vat th,
.vat td {
  border-bottom: 1px solid #d5dbe2;
  padding: 1mm 2mm;
  text-align: right;
}
.vat th:first-child,
.vat td:first-child {
  text-align: left;
}
.vat th {
  font-weight: 500;
  color: #4a5a6c;
}
.items {
  width: 100%;
  border-collapse: collapse;
  margin: 4mm 0;
  font-variant-numeric: tabular-nums;
  font-size: 9.5pt;
}
.items th,
.items td {
  border-bottom: 1px solid #d5dbe2;
  padding: 1.5mm 2mm;
  text-align: right;
  vertical-align: top;
}
.items th {
  font-weight: 600;
  color: #4a5a6c;
  border-bottom: 1.5px solid #1f3a5c;
}
.items .d {
  text-align: left;
  width: 50%;
}
.items .n,
.items .u {
  text-align: left;
  white-space: nowrap;
}
.items .empty {
  height: 30mm;
}
.items tfoot td {
  border: 0;
  font-weight: 700;
  padding-top: 2.5mm;
}
.grand-sm {
  border-bottom: 3px double currentColor;
}
.sum {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 1mm;
  margin-top: 2mm;
}
.sum div {
  display: flex;
  align-items: baseline;
  gap: 6mm;
}
.sum span {
  color: #4e5866;
}
.grand {
  font-size: 16pt;
  font-variant-numeric: tabular-nums;
  border-bottom: 3px double currentColor;
  min-width: 30mm;
  text-align: right;
}
.words {
  margin: 0;
  font-size: 9pt;
  color: #4e5866;
}
.acc {
  margin: 2mm 0 0;
  font-size: 9pt;
}
.signs {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8mm;
  margin-top: 14mm;
}
.signs.four {
  grid-template-columns: repeat(4, 1fr);
  gap: 5mm;
  position: absolute;
  left: 14mm;
  right: 14mm;
  bottom: 11mm;
}
.signs div {
  display: flex;
  flex-direction: column;
  border-top: 1px solid #8a96a3;
  padding-top: 1mm;
  font-size: 8pt;
  color: #6b7685;
}
.signs b {
  order: -1;
  font-weight: 600;
  color: #1d2633;
  font-size: 9pt;
  min-height: 1.2em;
  margin-top: -5.5mm;
  margin-bottom: 1mm;
}
</style>
