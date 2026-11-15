<script setup>
// Platobné údaje splátky na rýchle prepísanie do internetbankingu: každý
// údaj sa skopíruje jedným klikom, QR (PAY by square) načíta mobilná banka.
import { computed, ref, watchEffect } from 'vue'
import { paymentRows } from '../lib/household.js'
import { obligationPayBySquare, qrSvg } from '../lib/payqr.js'

const props = defineProps({
  obligation: { type: Object, required: true },
  amount: { type: [String, Number], default: '' },
  dueDate: { type: String, default: '' },
})
const rows = computed(() => paymentRows(props.obligation, props.amount))
const qr = ref('')
watchEffect(async () => {
  let code = null
  try {
    code = obligationPayBySquare(props.obligation, props.amount, props.dueDate)
  } catch {
    code = null // napr. neplatný IBAN — QR sa nezobrazí
  }
  qr.value = code ? await qrSvg(code) : ''
})

const copied = ref('')
let timer = null
async function copy(r) {
  try {
    await navigator.clipboard.writeText(r.copy)
  } catch {
    // starší prehliadač bez schránky: označí text na ručné skopírovanie
    const el = document.getElementById(`pay-${r.key}`)
    if (el) window.getSelection()?.selectAllChildren(el)
    return
  }
  copied.value = r.key
  clearTimeout(timer)
  timer = setTimeout(() => (copied.value = ''), 1500)
}
</script>

<template>
  <section v-if="rows.length" class="paydata" aria-label="Platobné údaje">
    <h3>Platba v internetbankingu</h3>
    <div class="body">
      <dl>
        <div v-for="r in rows" :key="r.key" class="row">
          <dt>{{ r.label }}</dt>
          <dd :id="`pay-${r.key}`" :class="{ mono: r.key === 'iban' || r.key === 'amount' }">{{ r.value }}</dd>
          <button type="button" class="cp" :aria-label="`Kopírovať: ${r.label}`" :title="copied === r.key ? 'Skopírované' : 'Kopírovať'" @click="copy(r)">
            <i :class="['pi', copied === r.key ? 'pi-check' : 'pi-copy']" aria-hidden="true" />
          </button>
        </div>
      </dl>
      <figure v-if="qr" class="qr">
        <!-- eslint-disable-next-line vue/no-v-html -- SVG generuje knižnica qrcode -->
        <div class="qr-img" v-html="qr" />
        <figcaption>PAY by square — naskenujte v mobilnej banke</figcaption>
      </figure>
    </div>
  </section>
</template>

<style scoped>
.paydata {
  container-type: inline-size;
  margin: 12px 0;
  border: 1px solid var(--rule);
  background: var(--sheet, #fff);
  padding: 8px 10px;
}
h3 {
  margin: 0 0 6px;
  font-size: var(--fs-md);
  color: var(--blue-deep);
}
.body {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
dl {
  flex: 1;
  min-width: 0;
  margin: 0;
}
.row {
  display: grid;
  grid-template-columns: 8.5rem minmax(0, 1fr) auto;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
  border-bottom: 1px solid var(--rule-soft, #eef1f4);
}
dt {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.mono {
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}
.cp {
  border: 0;
  background: none;
  color: var(--blue);
  cursor: pointer;
  padding: 4px 6px;
}
.cp:hover,
.cp:focus-visible {
  background: var(--paper, #f3f5f8);
}
.qr {
  margin: 0;
  width: 132px;
  flex: none;
  text-align: center;
}
.qr-img :deep(svg) {
  width: 132px;
  height: 132px;
  display: block;
}
figcaption {
  font-size: var(--fs-xs, 0.75rem);
  color: var(--ink-soft);
  margin-top: 4px;
}
@container (max-width: 520px) {
  .body {
    flex-direction: column;
  }
  .row {
    grid-template-columns: 7rem minmax(0, 1fr) auto;
  }
  .qr {
    align-self: center;
  }
}
/* úzky panel: názov nad hodnotou, aby sa hodnota nelámala po slovách */
@container (max-width: 380px) {
  .row {
    grid-template-columns: minmax(0, 1fr) auto;
    grid-template-areas: 'label copy' 'value copy';
    gap: 0 6px;
    padding: 4px 0;
  }
  dt {
    grid-area: label;
    font-size: var(--fs-xs, 0.75rem);
  }
  dd {
    grid-area: value;
  }
  .cp {
    grid-area: copy;
  }
}
</style>
