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
import BarList, { type BarItem } from './BarList.vue'
import DonutChart from './DonutChart.vue'
import Icon from './Icon.vue'
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

// Gráficos: os 10 primeiros do ranking (mais ou menos vendidos, só os que venderam), por R$ ou quantidade.
const by = ref<'valor' | 'qtd'>('valor')
const units = computed(() => (report.value?.products ?? []).reduce((a, p) => a + p.qty, 0))
const soldTotal = computed(() => (report.value?.products ?? []).reduce((a, p) => a + p.totalCents, 0))
const topBars = computed<BarItem[]>(() => {
  const sold = [...(report.value?.products ?? [])].sort((a, b) => (by.value === 'valor' ? b.totalCents - a.totalCents : b.qty - a.qty))
  const list = ranking.value === 'mais' ? sold.slice(0, 10) : sold.slice(-10).reverse()
  return list.map((p) => ({
    key: p.productId, label: p.name, value: by.value === 'valor' ? p.totalCents : p.qty,
    text: by.value === 'valor' ? money(p.totalCents) : `${p.qty} un.`, sub: by.value === 'valor' ? `${p.qty} un.` : money(p.totalCents),
  }))
})

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
  <template v-else>
  <!-- Só na tela: indicadores e gráficos; a folha para imprimir vem embaixo, como antes. -->
  <div class="kpi-row no-print">
    <div class="kpi">
      <div class="kpi-head">Produtos vendidos<span class="kpi-ic" style="--c: #5b4cf0"><Icon name="m-produtos" :size="18" /></span></div>
      <div class="kpi-value mono">{{ report.products.length }}</div>
      <div class="small muted">diferentes no período</div>
    </div>
    <div class="kpi">
      <div class="kpi-head">Unidades vendidas<span class="kpi-ic" style="--c: #0e9466"><Icon name="m-vendas" :size="18" /></span></div>
      <div class="kpi-value mono">{{ units }}</div>
      <div class="small muted">somando todos os produtos</div>
    </div>
    <div class="kpi">
      <div class="kpi-head">Faturamento<span class="kpi-ic" style="--c: #d98b06"><Icon name="m-relatorios" :size="18" /></span></div>
      <div class="kpi-value mono">{{ money(soldTotal) }}</div>
      <div class="small muted">dos produtos vendidos</div>
    </div>
    <div class="kpi">
      <div class="kpi-head">Sem nenhuma venda<span class="kpi-ic" style="--c: #d63c31"><Icon name="cancelar" :size="18" /></span></div>
      <div class="kpi-value mono">{{ report.notSold?.length ?? 0 }}</div>
      <div class="small muted">produtos ativos parados</div>
    </div>
  </div>
  <div v-if="report.products.length" class="charts no-print">
    <section class="card">
      <div class="card-head"><strong>{{ ranking === 'mais' ? '10 mais vendidos' : '10 menos vendidos (que venderam)' }}</strong>
        <span class="seg-mini">
          <button type="button" :aria-pressed="by === 'valor'" @click="by = 'valor'">por R$</button>
          <button type="button" :aria-pressed="by === 'qtd'" @click="by = 'qtd'">por quantidade</button>
        </span></div>
      <BarList :items="topBars" :color="ranking === 'mais' ? 'var(--primary)' : '#f59e0b'" />
    </section>
    <section class="card">
      <div class="card-head"><strong>Vendas por categoria</strong></div>
      <DonutChart :items="(report.byCategory ?? []).map((c) => ({ name: c.categoryName || 'Sem categoria', value: c.totalCents, text: money(c.totalCents) }))"
        center-label="categorias" :center="String(report.byCategory?.length ?? 0)" />
    </section>
  </div>
  <section ref="sheet" class="card" :class="{ 'print-area': printing }">
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
</template>

<style scoped>
.charts { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(300px, 1fr); gap: 18px; margin-bottom: 18px; }
@media (max-width: 1100px) { .charts { grid-template-columns: minmax(0, 1fr); } }
.charts .card { margin-top: 0; }
.seg-mini { display: inline-flex; border: 1px solid var(--border); border-radius: 999px; padding: 2px; }
.seg-mini button { min-height: 0; padding: 4px 12px; border: 0; border-radius: 999px; background: transparent; font-size: 12.5px; font-weight: 600; color: var(--muted); box-shadow: none; }
.seg-mini button[aria-pressed='true'] { background: var(--text); color: var(--surface); }
.seg { margin: 0 0 8px; border-bottom: 0; }
table th, table td { text-align: left; }
table th.right, table td.right { text-align: right; }
</style>
