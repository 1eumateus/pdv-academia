<!-- Caminho: pdv/frontend/src/components/ProductsSold.vue -->
<script setup lang="ts">
// Mais e menos vendidos no período (os menos vendidos começam pelos ativos que
// não venderam nada), com o estoque atual, na folha padrão dos relatórios.
import { computed, onMounted, ref } from 'vue'
import { GetSalesReport } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { exportTable, savePdf, usePrint } from '../composables/exportTable'
import { money, stockLevel } from '../composables/format'
import type { ReportFilters } from '../composables/reportFilters'
import type { models, SalesReport } from '../types'
import ReportSheet from './ReportSheet.vue'

const props = defineProps<{ filters: ReportFilters; title: string }>()
const report = ref<SalesReport | null>(null)
const error = ref('')
const { printing, print } = usePrint()
const sheet = ref<HTMLElement | null>(null)
const ranking = ref<'mais' | 'menos'>('mais')

const br = (d: string) => d.split('-').reverse().join('/')
const period = computed(() => (report.value ? br(report.value.startDate) + (report.value.endDate !== report.value.startDate ? ` a ${br(report.value.endDate)}` : '') : ''))
const products = computed(() => {
  const sold = report.value?.products ?? []
  return ranking.value === 'mais' ? sold : [...(report.value?.notSold ?? []), ...[...sold].reverse()]
})
const level = (p: models.ProductSold) => stockLevel(p.stockQty, p.minStock, report.value?.lowStockDefault ?? 0)

async function load() {
  error.value = ''
  try {
    report.value = await call(GetSalesReport(props.filters.start, props.filters.end, 0))
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)

function exportXlsx() {
  exportTable(ranking.value === 'mais' ? 'Produtos mais vendidos' : 'Produtos menos vendidos', `Período: ${period.value}`,
    [['Produto', 'text'], ['Qtd.', 'int'], ['Total', 'money'], ['Estoque atual', 'int']],
    products.value.map((p) => [p.name, p.qty, p.totalCents, p.stockQty]))
}

defineExpose({ exportXlsx, print, pdf: () => savePdf(sheet.value, ranking.value === 'mais' ? 'Mais vendidos' : 'Menos vendidos') })
</script>

<template>
  <div v-if="error" class="alert error">{{ error }}</div>
  <div v-else-if="!report" class="state">Carregando…</div>
  <section v-else ref="sheet" class="card" :class="{ 'print-area': printing }">
    <ReportSheet :title="ranking === 'mais' ? 'Mais vendidos' : 'Menos vendidos'" :period="period">
      <div class="tabs seg no-print" role="tablist" aria-label="Ordem dos produtos">
        <button role="tab" :aria-selected="ranking === 'mais'" @click="ranking = 'mais'">Mais vendidos</button>
        <button role="tab" :aria-selected="ranking === 'menos'" @click="ranking = 'menos'">Menos vendidos<template v-if="report.notSold?.length"> ({{ report.notSold.length }} sem venda)</template></button>
      </div>
      <div v-if="!products.length" class="state">Nenhum produto no período.</div>
      <table v-else>
        <thead><tr><th>#</th><th>Produto</th><th class="right">Qtd. vendida</th><th class="right">Total</th><th class="right">Estoque atual</th></tr></thead>
        <tbody>
          <tr v-for="(p, i) in products" :key="p.productId" :class="`stock-${level(p)}`">
            <td class="muted">{{ i + 1 }}</td><td class="left">{{ p.name }}</td>
            <td class="right mono">{{ p.qty || '—' }}</td><td class="right mono">{{ p.qty ? money(p.totalCents) : '—' }}</td>
            <td class="right"><span v-if="level(p) === 'zerado'" class="badge bad">Sem estoque</span><span v-else-if="level(p) === 'baixo'" class="badge warn">{{ p.stockQty }} · baixo</span><span v-else class="mono">{{ p.stockQty }}</span></td>
          </tr>
        </tbody>
      </table>
    </ReportSheet>
  </section>
</template>

<style scoped>
.seg { margin: 0 0 8px; border-bottom: 0; }
table th, table td { text-align: left; }
table th.right, table td.right { text-align: right; }
</style>
