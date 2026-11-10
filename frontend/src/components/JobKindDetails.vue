<script setup>
// Údaje zákazky podľa druhu: vozidlo (autoservis), miesto a tím (montáž),
// zariadenie a záruka (servis). Upravuje priamo formulár zákazky.
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import FormField from './FormField.vue'
import { fromISODate, toISODate } from '../lib/format.js'

const props = defineProps({
  form: { type: Object, required: true },
  errors: { type: Object, default: () => ({}) },
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['plate'])
const fuels = ['rezerva', '1/4', '1/2', '3/4', 'plná']
const e = (k) => props.errors[k]
</script>

<template>
  <section v-if="form.kind === 'auto' && form.vehicle" class="kind" aria-label="Vozidlo">
    <h2>Vozidlo</h2>
    <div class="form-grid fields">
      <FormField label="EČV" :error="e('vehicle.plate')">
        <InputText v-model="form.vehicle.plate" size="small" class="plate" :disabled="readonly" placeholder="BB123AB" autocapitalize="characters"
          @blur="emit('plate')" />
      </FormField>
      <FormField label="VIN" :error="e('vehicle.vin')">
        <InputText v-model="form.vehicle.vin" size="small" class="mono" maxlength="20" :disabled="readonly" autocapitalize="characters" />
      </FormField>
      <FormField label="Značka">
        <InputText v-model="form.vehicle.make" size="small" :disabled="readonly" placeholder="Škoda" />
      </FormField>
      <FormField label="Model">
        <InputText v-model="form.vehicle.model" size="small" :disabled="readonly" placeholder="Octavia 2.0 TDI" />
      </FormField>
      <FormField label="Rok výroby" :error="e('vehicle.year')">
        <InputText v-model="form.vehicle.year" size="small" inputmode="numeric" maxlength="4" :disabled="readonly" />
      </FormField>
      <FormField label="Stav tachometra (km)" :error="e('vehicle.mileage')">
        <InputText v-model="form.vehicle.mileage" size="small" inputmode="numeric" :disabled="readonly" />
      </FormField>
      <FormField label="Palivo pri príjme">
        <Select v-model="form.vehicle.fuel" :options="fuels" size="small" fluid show-clear editable :disabled="readonly" />
      </FormField>
      <span class="gap" />
      <FormField label="Závada / požiadavka zákazníka" :span="2">
        <Textarea v-model="form.vehicle.fault" rows="2" auto-resize :disabled="readonly" placeholder="Čo zákazník reklamuje alebo požaduje" />
      </FormField>
      <FormField label="Poškodenia zistené pri príjme" :span="2">
        <Textarea v-model="form.vehicle.damage" rows="2" auto-resize :disabled="readonly" placeholder="Napr. škrabanec na zadnom nárazníku vpravo" />
      </FormField>
      <FormField label="STK/EK platí do" hint="pripomenie sa v kalendári">
        <DatePicker :model-value="fromISODate(form.vehicle.inspection)" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="readonly"
          @update:model-value="form.vehicle.inspection = toISODate($event) ?? ''" />
      </FormField>
      <FormField label="Ďalší servis" hint="pripomenie sa v kalendári">
        <DatePicker :model-value="fromISODate(form.vehicle.nextService)" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="readonly"
          @update:model-value="form.vehicle.nextService = toISODate($event) ?? ''" />
      </FormField>
    </div>
  </section>

  <section v-else-if="form.kind === 'install' && form.site" class="kind" aria-label="Montáž">
    <h2>Montáž</h2>
    <div class="form-grid fields">
      <FormField label="Miesto montáže" :span="2" hint="predvyplní sa adresa zákazníka">
        <InputText v-model="form.site.address" size="small" :disabled="readonly" placeholder="Ulica, obec, prípadne prevádzka" />
      </FormField>
      <FormField label="Montážnici">
        <InputText v-model="form.site.workers" size="small" :disabled="readonly" placeholder="Mená montážnikov" />
      </FormField>
      <FormField label="Kontakt na mieste">
        <InputText v-model="form.site.contact" size="small" :disabled="readonly" placeholder="Meno, telefón" />
      </FormField>
      <FormField label="Rozsah prác" :span="2">
        <Textarea v-model="form.site.scope" rows="2" auto-resize :disabled="readonly" />
      </FormField>
    </div>
  </section>

  <section v-else-if="form.kind === 'service' && form.device" class="kind" aria-label="Zariadenie">
    <h2>Zariadenie</h2>
    <div class="form-grid fields">
      <FormField label="Zariadenie" :error="e('device.type')">
        <InputText v-model="form.device.type" size="small" :disabled="readonly" placeholder="Napr. práčka, frekvenčný menič" />
      </FormField>
      <FormField label="Výrobca">
        <InputText v-model="form.device.make" size="small" :disabled="readonly" />
      </FormField>
      <FormField label="Typ / model">
        <InputText v-model="form.device.model" size="small" :disabled="readonly" />
      </FormField>
      <FormField label="Výrobné číslo">
        <InputText v-model="form.device.serial" size="small" class="mono" :disabled="readonly" />
      </FormField>
      <FormField label="Záruka">
        <label class="check"><Checkbox v-model="form.device.warranty" binary :disabled="readonly" /> <span>záručná oprava</span></label>
      </FormField>
      <FormField v-if="form.device.warranty" label="Dátum kúpy">
        <DatePicker :model-value="fromISODate(form.device.purchased)" date-format="d. m. yy" size="small" fluid show-button-bar :disabled="readonly"
          @update:model-value="form.device.purchased = toISODate($event) ?? ''" />
      </FormField>
      <span v-else class="gap" />
      <FormField label="Popis závady" :span="2">
        <Textarea v-model="form.device.fault" rows="2" auto-resize :disabled="readonly" />
      </FormField>
      <FormField label="Prevzaté príslušenstvo" :span="2">
        <InputText v-model="form.device.accessories" size="small" :disabled="readonly" placeholder="Napr. napájací kábel, diaľkové ovládanie" />
      </FormField>
    </div>
  </section>
</template>

<style scoped>
.kind {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.kind h2 {
  margin: 0;
  font-size: var(--fs-md);
  border-bottom: 1px solid var(--rule);
  padding-bottom: 2px;
}
.fields {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.plate :deep(input),
.plate {
  text-transform: uppercase;
  font-weight: 600;
  letter-spacing: 0.04em;
}
.mono {
  text-transform: uppercase;
  font-variant-numeric: tabular-nums;
}
.check {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
}
@media (max-width: 640px) {
  .fields {
    grid-template-columns: minmax(0, 1fr);
  }
  .gap {
    display: none;
  }
}
</style>
