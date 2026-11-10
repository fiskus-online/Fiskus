<script setup>
// Formátovaný text dokladu (text nad položkami a pod nimi): tučné, kurzíva,
// podčiarknuté, odrážky a číslovanie. Vložený text z Wordu, webu či e-mailu
// sa vyčistí na tieto formáty (lib/richtext.js — rovnako ako na serveri).
import { computed, onMounted, ref, watch } from 'vue'
import { fromEditor, sanitize, toHTML } from '../lib/richtext.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  placeholder: { type: String, default: '' },
  label: { type: String, default: '' }, // pre čítačky obrazovky
  inputId: { type: String, default: undefined },
})
const emit = defineEmits(['update:modelValue'])

const el = ref(null)
const empty = ref(true)
let last = null // posledná hodnota z editora (aby sa kurzor neposúval)
const shown = computed(() => toHTML(props.modelValue))

function load(v) {
  if (!el.value) return
  el.value.innerHTML = toHTML(v)
  empty.value = !String(v ?? '').trim()
}

onMounted(() => load(props.modelValue))
watch(
  () => props.modelValue,
  (v) => {
    if (v !== last) load(v)
  },
)
watch(
  () => props.disabled,
  () => setTimeout(() => load(props.modelValue)),
)

function changed() {
  const v = fromEditor(el.value.innerHTML)
  empty.value = !v
  if (v === last || v === (props.modelValue ?? '')) {
    last = v
    return
  }
  last = v
  emit('update:modelValue', v)
}

function cmd(name) {
  el.value?.focus()
  document.execCommand(name, false)
  changed()
}

function focus() {
  // nové riadky ako odseky (nie <div>), všetko ostatné vyčistí sanitize
  document.execCommand('defaultParagraphSeparator', false, 'p')
}

function paste(e) {
  const data = e.clipboardData
  if (!data) return
  e.preventDefault()
  const html = data.getData('text/html')
  const clean = html ? sanitize(html) : toHTML(data.getData('text/plain'))
  if (clean) document.execCommand('insertHTML', false, clean.startsWith('<') ? clean : toHTML(clean))
  changed()
}

const tools = [
  { cmd: 'bold', icon: 'B', title: 'Tučné (Ctrl+B)', cls: 'b' },
  { cmd: 'italic', icon: 'I', title: 'Kurzíva (Ctrl+I)', cls: 'i' },
  { cmd: 'underline', icon: 'U', title: 'Podčiarknuté (Ctrl+U)', cls: 'u' },
  { cmd: 'insertUnorderedList', pi: 'pi-list', title: 'Odrážky' },
  { cmd: 'insertOrderedList', icon: '1.', title: 'Číslovaný zoznam', cls: 'n' },
  { cmd: 'removeFormat', pi: 'pi-eraser', title: 'Zrušiť formátovanie' },
]
</script>

<template>
  <div v-if="disabled" class="rt rt-view rich" v-html="shown" />
  <div v-else class="rt">
    <div class="bar" role="toolbar" :aria-label="label ? `Formát: ${label}` : 'Formát textu'">
      <button v-for="t in tools" :key="t.cmd" type="button" class="tool" :class="t.cls" :title="t.title" :aria-label="t.title"
        @mousedown.prevent @click="cmd(t.cmd)">
        <i v-if="t.pi" class="pi" :class="t.pi" aria-hidden="true" /><span v-else aria-hidden="true">{{ t.icon }}</span>
      </button>
    </div>
    <div :id="inputId" ref="el" class="ed rich" :class="{ empty }" contenteditable="true" role="textbox" aria-multiline="true"
      :aria-label="label || undefined" :data-placeholder="placeholder" spellcheck="true"
      @input="changed" @blur="changed" @focus="focus" @paste="paste" />
  </div>
</template>

<style scoped>
.rt {
  border: 1px solid var(--p-inputtext-border-color, #cbd5e1);
  border-radius: var(--p-inputtext-border-radius, 6px);
  background: var(--p-inputtext-background, #fff);
  min-width: 0;
}
.rt:focus-within {
  border-color: var(--blue);
  box-shadow: 0 0 0 1px var(--blue);
}
.bar {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 3px;
  border-bottom: 1px solid var(--p-inputtext-border-color, #e2e8f0);
}
.tool {
  min-width: 30px;
  height: 28px;
  padding: 0 6px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--ink);
  font: inherit;
  font-size: 0.9rem;
  cursor: pointer;
}
.tool:hover,
.tool:focus-visible {
  background: #e8eef5;
  outline: none;
}
.tool.b {
  font-weight: 700;
}
.tool.i {
  font-style: italic;
  font-family: Georgia, serif;
}
.tool.u {
  text-decoration: underline;
}
.tool.n {
  font-variant-numeric: tabular-nums;
}
.ed {
  min-height: 3.6em;
  max-height: 24em;
  overflow: auto;
  padding: 0.45rem 0.6rem;
  outline: none;
  line-height: 1.45;
  overflow-wrap: anywhere;
}
.ed.empty::before {
  content: attr(data-placeholder);
  color: var(--ink-faint);
  pointer-events: none;
  position: absolute;
}
.rt-view {
  border-style: dashed;
  padding: 0.45rem 0.6rem;
  color: var(--ink-soft);
  background: transparent;
}
.rich :deep(p) {
  margin: 0 0 0.35em;
}
.rich :deep(p:last-child) {
  margin-bottom: 0;
}
.rich :deep(ul),
.rich :deep(ol) {
  margin: 0 0 0.35em;
  padding-left: 1.4em;
}
</style>
