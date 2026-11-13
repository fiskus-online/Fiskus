<script setup>
// Tlačová zostava zo zoznamu: názov, stĺpce a ich poradie, orientácia strany,
// zoskupenie so súčtami; výstup do tlače/PDF alebo Excelu. Nastavenie si
// pamätá každá stránka zvlášť (v prehliadači).
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Checkbox from 'primevue/checkbox'
import { useToast } from 'primevue/usetoast'
import FormField from './FormField.vue'
import { session } from '../lib/session.js'
import { formatDate } from '../lib/format.js'
import { exportFileName, saveBlob, xlsxCell } from '../lib/export.js'
import { defaultSettings, loadSettings, reportHTML, reportModel, reportSheet, saveSettings } from '../lib/report.js'
import { printReport } from '../lib/reportPrint.js'

const props = defineProps({
  name: { type: String, required: true },
  columns: { type: Array, required: true },
  rows: { type: [Array, Function], required: true },
  subtitle: { type: String, default: '' },
})
const open = defineModel('open', { type: Boolean, default: false })
const toast = useToast()
const s = ref(loadSettings(props.name, props.columns))
const busy = ref(false)
watch(open, (v) => v && (s.value = loadSettings(props.name, props.columns)))

const orientations = [
  { label: 'Na výšku', value: 'portrait', icon: 'pi pi-file' },
  { label: 'Na šírku', value: 'landscape', icon: 'pi pi-file pi-rotate' },
]
const formats = [
  { label: 'Tlač / PDF', value: 'pdf' },
  { label: 'Excel', value: 'xlsx' },
]
// zoznam stĺpcov: vybrané v poradí, potom ostatné
const list = computed(() => {
  const chosen = s.value.columns.map((k) => props.columns.find((c) => c.header === k)).filter(Boolean)
  return [...chosen, ...props.columns.filter((c) => !s.value.columns.includes(c.header))]
})
const groupOptions = computed(() => [{ label: 'bez zoskupenia', value: '' }, ...props.columns.filter((c) => c.type !== 'money' && c.type !== 'number').map((c) => ({ label: c.header, value: c.header }))])
function toggle(c, on) {
  const cols = s.value.columns.filter((k) => k !== c.header)
  s.value.columns = on ? [...cols, c.header] : cols
}
function move(c, dir) {
  const cols = [...s.value.columns]
  const i = cols.indexOf(c.header)
  const j = i + dir
  if (i < 0 || j < 0 || j >= cols.length) return
  ;[cols[i], cols[j]] = [cols[j], cols[i]]
  s.value.columns = cols
}
function reset() {
  s.value = defaultSettings(props.name, props.columns)
}

async function run() {
  if (!s.value.columns.length) {
    toast.add({ severity: 'warn', summary: 'Vyberte aspoň jeden stĺpec', life: 3000 })
    return
  }
  busy.value = true
  try {
    const rows = typeof props.rows === 'function' ? await props.rows() : props.rows
    if (!rows.length) {
      toast.add({ severity: 'warn', summary: 'Nie je čo tlačiť', detail: 'Zoznam je po filtri prázdny.', life: 3000 })
      return
    }
    saveSettings(props.name, s.value)
    const model = reportModel(props.columns, rows, s.value)
    const meta = { company: session.company?.name ?? '', subtitle: props.subtitle, printed: `vytlačené ${formatDate(new Date().toISOString().slice(0, 10))}` }
    if (s.value.format === 'xlsx') {
      const { default: writeExcelFile } = await import('write-excel-file/browser')
      const blob = await writeExcelFile(reportSheet(model, meta, xlsxCell), {
        columns: model.cols.map((c) => ({ width: c.width ?? (c.type === 'money' ? 14 : c.type === 'date' ? 12 : 20) })),
        sheet: s.value.title.replace(/[\\/?*[\]:]/g, ' ').slice(0, 31) || 'Zostava',
        orientation: s.value.orientation,
        fontFamily: 'Calibri',
        fontSize: 11,
      }).toBlob()
      saveBlob(blob, exportFileName(s.value.title, 'xlsx'))
      open.value = false
    } else {
      open.value = false
      // dialóg sa zavrie, potom tlač (inak by bol v náhľade tlače)
      setTimeout(() => printReport(reportHTML(model, meta), s.value.orientation), 50)
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Zostava sa nevytvorila', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="open" header="Tlačová zostava" modal :style="{ width: '40rem' }" :breakpoints="{ '640px': '96vw' }">
    <div class="form-grid rep">
      <FormField label="Názov zostavy" :span="4">
        <InputText v-model="s.title" size="small" maxlength="120" />
      </FormField>
      <FormField label="Výstup" :span="2">
        <SelectButton v-model="s.format" :options="formats" option-label="label" option-value="value" :allow-empty="false" size="small" />
      </FormField>
      <FormField label="Strana A4" :span="2">
        <SelectButton v-model="s.orientation" :options="orientations" option-label="label" option-value="value" :allow-empty="false" size="small" />
      </FormField>
      <FormField label="Zoskupiť podľa" :span="2">
        <Select v-model="s.groupBy" :options="groupOptions" option-label="label" option-value="value" size="small" fluid />
      </FormField>
      <FormField label="Súčty" :span="2">
        <label class="check"><Checkbox v-model="s.totals" binary /> <span>súčty peňažných stĺpcov (aj za skupiny)</span></label>
      </FormField>
    </div>
    <fieldset class="cols">
      <legend>Stĺpce <small>{{ s.columns.length }} z {{ columns.length }} · poradie šípkami</small></legend>
      <ul>
        <li v-for="c in list" :key="c.header" :class="{ off: !s.columns.includes(c.header) }">
          <label class="check"><Checkbox :model-value="s.columns.includes(c.header)" binary @update:model-value="(on) => toggle(c, on)" /> <span>{{ c.header }}</span></label>
          <span v-if="s.columns.includes(c.header)" class="mv">
            <Button icon="pi pi-arrow-up" text rounded size="small" :aria-label="`Posunúť ${c.header} vyššie`" :disabled="s.columns.indexOf(c.header) === 0" @click="move(c, -1)" />
            <Button icon="pi pi-arrow-down" text rounded size="small" :aria-label="`Posunúť ${c.header} nižšie`" :disabled="s.columns.indexOf(c.header) === s.columns.length - 1" @click="move(c, 1)" />
          </span>
        </li>
      </ul>
    </fieldset>
    <template #footer>
      <Button label="Predvolené" text size="small" severity="secondary" @click="reset" />
      <div class="spacer" />
      <Button label="Zrušiť" text size="small" @click="open = false" />
      <Button :label="s.format === 'xlsx' ? 'Stiahnuť Excel' : 'Tlačiť / PDF'" :icon="s.format === 'xlsx' ? 'pi pi-file-excel' : 'pi pi-print'" size="small" :loading="busy" @click="run" />
    </template>
  </Dialog>
</template>

<style scoped>
.rep {
  margin-bottom: 10px;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  cursor: pointer;
}
.cols {
  border: 1px solid var(--rule);
  margin: 0;
  padding: 6px 10px;
}
.cols legend {
  font-weight: 600;
  padding: 0 4px;
}
.cols legend small {
  font-weight: 400;
  color: var(--ink-soft);
}
.cols ul {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 16rem;
  overflow-y: auto;
  columns: 2;
  column-gap: 16px;
}
.cols li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  break-inside: avoid;
}
.cols li.off span {
  color: var(--ink-soft);
}
.mv {
  display: flex;
  flex: none;
}
.spacer {
  flex: 1;
}
@media (max-width: 640px) {
  .cols ul {
    columns: 1;
  }
}
</style>
