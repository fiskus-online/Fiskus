<script setup>
// Potvrdenie o zdaniteľných príjmoch fyzickej osoby zo závislej činnosti
// (§ 39 ods. 5 zákona o dani z príjmov, vzor POT395v26) z mzdového listu;
// ?id= vytlačí jedného zamestnanca, inak všetkých s príjmom v roku.
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import Button from "primevue/button";
import { api } from "../lib/api.js";
import { session } from "../lib/session.js";
import { formatAmount } from "../lib/format.js";
import { addressLines } from "../lib/parties.js";
import { certificateLines, monthGrid } from "../lib/payroll.js";

const route = useRoute();
const router = useRouter();
const year = Number(route.query.rok);
const view = ref(null);
const error = ref("");
const company = computed(() => session.company ?? {});
const rows = computed(() =>
  (view.value?.list ?? []).filter(
    (r) => !route.query.id || r.employee.id === Number(route.query.id),
  ),
);
onMounted(async () => {
  try {
    view.value = await api.settlements(session.companyId, year);
    document.title = `Potvrdenie o príjmoch ${year}`;
  } catch (e) {
    error.value = e.fields?.[0]?.message ?? e.message;
  }
});
const back = () =>
  router.push({ path: "/rocne-zuctovanie", query: { rok: year } });
const print = () => window.print();
const months = Array.from({ length: 12 }, (_, i) =>
  String(i + 1).padStart(2, "0"),
);
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button
        icon="pi pi-arrow-left"
        label="Späť"
        text
        size="small"
        @click="back"
      />
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“</span>
      <Button
        icon="pi pi-print"
        label="Tlačiť / PDF"
        size="small"
        :disabled="!rows.length"
        @click="print"
      />
    </nav>
    <p v-if="error" class="error">{{ error }}</p>
    <p v-else-if="view && !rows.length" class="error">
      V roku {{ year }} nie sú uzavreté mzdy.
    </p>
    <p v-else-if="view?.open.length" class="warn">
      Neuzavreté mesiace: {{ view.open.join(", ") }} — potvrdenie obsahuje iba
      uzavreté mesiace.
    </p>

    <article v-for="r in rows" :key="r.employee.id" class="sheet">
      <header class="head">
        <h1>Potvrdenie</h1>
        <p>
          o zdaniteľných príjmoch fyzickej osoby zo závislej činnosti a ďalších
          údajoch podľa § 39 ods. 5 zákona č. 595/2003 Z. z. o dani z príjmov v
          znení neskorších predpisov za zdaňovacie obdobie
          <strong>{{ year }}</strong>
        </p>
      </header>

      <h2>I. oddiel — údaje o zamestnancovi</h2>
      <dl class="who">
        <div>
          <dt>Rodné číslo</dt>
          <dd>{{ r.employee.rc || "—" }}</dd>
        </div>
        <div>
          <dt>Titul</dt>
          <dd>{{ r.employee.title || "—" }}</dd>
        </div>
        <div>
          <dt>Priezvisko</dt>
          <dd>{{ r.employee.lastName }}</dd>
        </div>
        <div>
          <dt>Meno</dt>
          <dd>{{ r.employee.firstName }}</dd>
        </div>
        <div class="wide">
          <dt>Adresa trvalého pobytu</dt>
          <dd>{{ addressLines(r.employee).join(", ") || "—" }}</dd>
        </div>
      </dl>

      <h2>II. oddiel — údaje o zdaniteľných príjmoch a ďalšie údaje</h2>
      <table class="rows">
        <tbody>
          <tr
            v-for="[code, text] in certificateLines"
            :key="code"
            :class="{ sub: /[a-c]$/.test(code) }"
          >
            <td class="code">{{ code }}</td>
            <td>{{ text }}</td>
            <td class="num">{{ formatAmount(r.certificate.rows[code]) }}</td>
          </tr>
        </tbody>
      </table>
      <p class="label">
        <span class="code">13</span> Kalendárne mesiace, ku ktorým sa viažu sumy
        na r. 01 až 09, 11 a 12
      </p>
      <div class="grid">
        <span
          v-for="(on, i) in monthGrid(r.certificate.months)"
          :key="i"
          :class="{ on }"
          >{{ months[i] }}</span
        >
      </div>

      <p class="label">
        <span class="code">14</span> Vyživované deti, na ktoré zamestnávateľ
        priznal a vyplatil daňový bonus (§ 33 zákona)
      </p>
      <div v-if="r.certificate.children.length" class="kids-wrap">
        <table class="kids">
          <thead>
            <tr>
              <th>Priezvisko a meno</th>
              <th>Rodné číslo</th>
              <th v-for="mo in months" :key="mo" class="m">{{ mo }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="ch in r.certificate.children" :key="ch.rc + ch.name">
              <td>{{ ch.name }}</td>
              <td>{{ ch.rc }}</td>
              <td v-for="(on, i) in monthGrid(ch.months)" :key="i" class="m">
                {{ on ? "×" : "" }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="none">Zamestnanec si daňový bonus neuplatňoval.</p>

      <h2>III. oddiel — údaje o zamestnávateľovi</h2>
      <dl class="who">
        <div class="wide">
          <dt>Názov (meno a priezvisko)</dt>
          <dd>{{ company.name }}</dd>
        </div>
        <div>
          <dt>DIČ</dt>
          <dd>{{ company.dic || "—" }}</dd>
        </div>
        <div>
          <dt>IČO</dt>
          <dd>{{ company.ico || "—" }}</dd>
        </div>
        <div class="wide">
          <dt>Sídlo (adresa)</dt>
          <dd>{{ addressLines(company).join(", ") || "—" }}</dd>
        </div>
      </dl>
      <p class="true">
        Potvrdzujeme, že údaje uvedené v potvrdení sú pravdivé.
      </p>
      <footer class="sign">
        <span>Vypracoval(a): ……………………………</span>
        <span>Dňa: ……………………</span>
        <span>Podpis a pečiatka zamestnávateľa: ……………………………</span>
      </footer>
    </article>
  </div>
</template>

<style scoped>
.print-root {
  min-height: 100%;
  background: #e9ecf0;
  padding: 0 0 40px;
}
.toolbar {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  background: var(--sheet);
  border-bottom: 1px solid var(--rule);
}
.spacer {
  flex: 1;
}
.tip {
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.error,
.warn {
  text-align: center;
  color: var(--tone-overdue);
  padding: 16px;
}
.error {
  padding: 40px;
}
.sheet {
  width: 210mm;
  max-width: 100%;
  min-height: 287mm;
  margin: 24px auto 0;
  padding: 12mm 14mm;
  background: #fff;
  color: #1d2633;
  font-size: 9.5pt;
  line-height: 1.35;
  box-shadow:
    0 1px 3px rgba(18, 24, 32, 0.12),
    0 8px 28px rgba(18, 24, 32, 0.08);
  break-after: page;
}
.head {
  border-bottom: 2px solid #1f3a5c;
  padding-bottom: 6px;
  text-align: center;
}
h1 {
  margin: 0;
  font-size: 15pt;
  letter-spacing: 0.04em;
  color: #1f3a5c;
}
.head p {
  margin: 2px 0 0;
  font-size: 9pt;
}
h2 {
  margin: 12px 0 4px;
  font-size: 9.5pt;
  color: #1f3a5c;
  border-bottom: 1px solid #d5dbe2;
}
.who {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 4px 16px;
  margin: 4px 0;
}
.who .wide {
  grid-column: 1 / -1;
}
.who dt {
  font-size: 7.5pt;
  color: #4a5a6c;
}
.who dd {
  margin: 0;
}
.rows,
.kids {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.rows td,
.kids td,
.kids th {
  padding: 3px 4px;
  border-bottom: 1px solid #d5dbe2;
  vertical-align: top;
}
.rows .sub td {
  color: #4a5a6c;
  padding-left: 14px;
}
.code {
  width: 2.6rem;
  color: #4a5a6c;
  font-weight: 600;
}
.num {
  text-align: right;
  white-space: nowrap;
  width: 7rem;
}
.label {
  margin: 8px 0 4px;
}
.label .code {
  display: inline-block;
}
.grid {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  border: 1px solid #9aa6b4;
}
.grid span {
  text-align: center;
  padding: 2px 0;
  color: #b4bcc6;
  border-right: 1px solid #d5dbe2;
}
.grid span:last-child {
  border-right: none;
}
.grid .on {
  color: #1d2633;
  font-weight: 700;
  background: #eef2f6;
}
.kids th {
  font-size: 7.5pt;
  color: #4a5a6c;
  text-align: left;
  font-weight: 600;
}
.kids .m {
  width: 1.4rem;
  text-align: center;
}
.none {
  color: #4a5a6c;
  margin: 2px 0;
}
.true {
  margin-top: 10px;
}
.sign {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 8px;
  margin-top: 16mm;
  font-size: 9pt;
}
.kids-wrap {
  overflow-x: auto;
}
@media (max-width: 600px) {
  .sheet {
    padding: 6mm 4mm;
  }
  .num {
    width: auto;
  }
}
@media print {
  .print-root {
    background: none;
    padding: 0;
  }
  .toolbar,
  .warn {
    display: none;
  }
  .sheet {
    margin: 0;
    box-shadow: none;
    min-height: auto;
  }
}
</style>
