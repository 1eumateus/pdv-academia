<!-- Caminho: catraca/frontend/src/components/AccessRulesFields.vue -->
<script setup lang="ts">
// Regras de acesso na catraca: dias da semana permitidos, vezes por semana e
// entradas por dia. Tudo vazio = sem restrição. Com "plan" (assinatura), as
// regras do plano valem e os campos só aparecem se marcar a exceção (v-model:own).
import { models } from '../../wailsjs/go/models'
import { copyRules, describeRules } from '../composables/accessRules'

const rules = defineModel<models.AccessRules>({ required: true })
const own = defineModel<boolean>('own', { default: true })
const props = defineProps<{ plan?: models.AccessRules | null; withPlan?: boolean }>()
const days = [[1, 'Seg'], [2, 'Ter'], [3, 'Qua'], [4, 'Qui'], [5, 'Sex'], [6, 'Sáb'], [7, 'Dom']] as const

function toggle(d: number) {
  const w = rules.value.weekdays ?? []
  rules.value = { ...rules.value, weekdays: w.includes(d) ? w.filter((x) => x !== d) : [...w, d].sort() }
}
// Campo vazio = 0 (sem limite).
const num = (v: string) => Math.max(0, Math.trunc(Number(v) || 0))
// Ao marcar a exceção, começa das regras do plano.
function setOwn(v: boolean) {
  own.value = v
  if (v) rules.value = copyRules(props.plan)
}
</script>

<template>
  <fieldset class="stack" style="border: 0; padding: 0; margin: 0; gap: 8px">
    <legend class="small muted" style="padding: 0; margin-bottom: 6px">Acesso na catraca (vazio = sem restrição)</legend>
    <template v-if="withPlan">
      <p v-if="!own" class="small" style="margin: 0">Regras do plano: <strong>{{ describeRules(plan) || 'sem restrição' }}</strong></p>
      <label class="check"><input type="checkbox" :checked="own" @change="setOwn(($event.target as HTMLInputElement).checked)" /> Regras próprias para este aluno (exceção ao plano)</label>
    </template>
    <template v-if="!withPlan || own">
      <div class="row" role="group" aria-label="Dias permitidos" style="gap: 4px; flex-wrap: wrap">
        <button v-for="[d, label] in days" :key="d" type="button" :class="{ primary: rules.weekdays?.includes(d) }" :aria-pressed="rules.weekdays?.includes(d) ?? false" @click="toggle(d)">{{ label }}</button>
      </div>
      <div class="row">
        <label class="field">Vezes por semana
          <input type="number" min="1" max="7" :value="rules.weeklyLimit || ''" placeholder="Sem limite" @input="rules = { ...rules, weeklyLimit: num(($event.target as HTMLInputElement).value) }" />
        </label>
        <label class="field">Entradas por dia
          <input type="number" min="1" max="20" :value="rules.dailyLimit || ''" placeholder="Sem limite" @input="rules = { ...rules, dailyLimit: num(($event.target as HTMLInputElement).value) }" />
        </label>
      </div>
    </template>
  </fieldset>
</template>
