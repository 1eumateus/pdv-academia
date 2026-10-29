<!-- Caminho: pdv/frontend/src/views/RelatoriosAnalise.vue -->
<script setup lang="ts">
// Relatórios › Produtos (seletor: vendidos, parados, curva ABC) e as abas Lucro e
// Descontos do Financeiro (um relatório só, sem seletor). A barra mostra só os
// filtros que o relatório usa.
import { computed, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AnalyticsPanel from '../components/AnalyticsPanel.vue'
import ProductsSold from '../components/ProductsSold.vue'
import { today } from '../composables/format'
import type { AnalyticsKind, ReportFilters } from '../composables/reportFilters'

type Kind = 'vendidos' | AnalyticsKind
const route = useRoute()
const options = computed<{ key: Kind; label: string }[]>(() => route.meta.section === 'lucro'
  ? [{ key: 'lucro', label: 'Lucro bruto' }]
  : route.meta.section === 'descontos'
  ? [{ key: 'descontos', label: 'Descontos por operador' }]
  : [
      { key: 'vendidos', label: 'Mais e menos vendidos' },
      { key: 'parados', label: 'Parados' },
      { key: 'abc', label: 'Curva ABC' },
    ])

const kind = ref<Kind>(options.value[0].key)
const label = computed(() => options.value.find((o) => o.key === kind.value)?.label ?? '')
const usesPeriod = computed(() => !['parados', 'fiado'].includes(kind.value))

const form = reactive({ start: today(-29), end: today(), days: 30 })
const applied = ref<ReportFilters>({ ...form, userId: 0, methods: [] })
const run = ref(0) // muda a chave do relatório: recria e recarrega com os filtros novos
const current = ref<{ exportXlsx: () => void; print: () => void; pdf: () => void } | null>(null)

function generate() {
  applied.value = { ...form, userId: 0, methods: [] }
  run.value++
}
function pick(k: Kind) {
  kind.value = k
  generate()
}
// Produtos e Financeiro usam esta mesma tela: ao trocar de aba, volta ao primeiro relatório.
watch(() => route.meta.section, () => pick(options.value[0].key))

// Atalho de período; clicar no mesmo período de novo serve para atualizar.
function preset(days: number) {
  if (form.start === today(-days) && form.end === today()) return generate()
  form.start = today(-days)
  form.end = today()
}
// Mudou período ou dias: recarrega sozinho (espera um pouco enquanto digita).
let timer = 0
watch(form, () => {
  window.clearTimeout(timer)
  if (form.start && form.end && form.days > 0) timer = window.setTimeout(generate, 400)
})
</script>

<template>
  <section class="card stack no-print">
    <div v-if="options.length > 1" class="tabs seg" role="tablist" aria-label="Relatório">
      <button v-for="o in options" :key="o.key" role="tab" :aria-selected="kind === o.key" @click="pick(o.key)">{{ o.label }}</button>
    </div>
    <form class="row filters" @submit.prevent>
      <template v-if="usesPeriod">
        <label class="field">De<input v-model="form.start" type="date" required /></label>
        <label class="field">Até<input v-model="form.end" type="date" required /></label>
      </template>
      <label v-if="kind === 'parados'" class="field" style="width: 170px">Sem venda há (dias)<input v-model.number="form.days" type="number" min="1" max="3650" required /></label>
      <template v-if="usesPeriod">
        <button type="button" @click="preset(0)">Hoje</button>
        <button type="button" @click="preset(6)">7 dias</button>
        <button type="button" @click="preset(29)">30 dias</button>
      </template>
      <span class="spacer" />
      <button type="button" @click="current?.print()">Imprimir A4</button>
      <button type="button" @click="current?.pdf()">Salvar PDF</button>
      <button type="button" class="ghost" @click="current?.exportXlsx()">Excel</button>
    </form>
  </section>

  <div style="margin-top: 12px">
    <ProductsSold v-if="kind === 'vendidos'" :key="run" ref="current" :filters="applied" :title="label" />
    <AnalyticsPanel v-else :key="`a${run}`" ref="current" :kind="kind" :filters="applied" :title="label" />
  </div>
</template>

<style scoped>
.seg { margin: 0; }
.filters { align-items: end; }
.filters button { align-self: end; }
</style>
