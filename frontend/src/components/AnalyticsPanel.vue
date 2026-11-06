<!-- Caminho: pdv/frontend/src/components/AnalyticsPanel.vue -->
<script setup lang="ts">
// Análises da loja: lucro, curva ABC, produtos parados, fiado em atraso,
// compras por fornecedor, estoque valorizado e descontos por operador.
// As contas são do backend; os filtros vêm da barra de Relatórios.
import { computed, onMounted, ref } from 'vue'
import {
  GetABC, GetDiscountsByOperator, GetFiadoAging, GetIdleProducts, GetProfit, GetPurchasesBySupplier, GetStockValue,
} from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { exportTable, savePdf, usePrint, type Col } from '../composables/exportTable'
import Icon from './Icon.vue'
import { chargeByWhatsapp } from '../composables/charge'
import { formatDateTime, money, today } from '../composables/format'
import type { AnalyticsKind, ReportFilters } from '../composables/reportFilters'
import { can } from '../composables/session'
import type { models } from '../types'
import BarList, { type BarItem } from './BarList.vue'
import DonutChart from './DonutChart.vue'
import ReportSheet from './ReportSheet.vue'

const props = defineProps<{ kind: AnalyticsKind; filters: ReportFilters; title: string }>()
const tab = computed(() => props.kind)
const start = computed(() => props.filters.start)
const end = computed(() => props.filters.end)
const days = computed(() => props.filters.days)
const loading = ref(false)
const error = ref('')
const { printing, print } = usePrint()

const profit = ref<models.ProfitReport | null>(null)
const profitBy = ref<'produto' | 'categoria'>('produto')
const abc = ref<models.ABCReport | null>(null)
const idle = ref<models.IdleProduct[]>([])
const aging = ref<models.AgingCustomer[]>([])
const suppliers = ref<models.SupplierPurchases[]>([])
const stock = ref<models.StockValueRow[]>([])
const discounts = ref<models.DiscountByOperator[]>([])

const usesPeriod = computed(() => ['lucro', 'abc', 'compras', 'descontos'].includes(tab.value))
const period = computed(() => (start.value === end.value ? br(start.value) : `${br(start.value)} a ${br(end.value)}`))
const br = (d: string) => d.split('-').reverse().join('/')
const pct = (part: number, whole: number) => (whole ? `${((part / whole) * 100).toFixed(1).replace('.', ',')}%` : '—')

async function load() {
  loading.value = true
  error.value = ''
  try {
    switch (tab.value) {
      case 'lucro': profit.value = await call(GetProfit(start.value, end.value)); break
      case 'abc': abc.value = await call(GetABC(start.value, end.value)); break
      case 'parados': idle.value = await call(GetIdleProducts(days.value)); break
      case 'fiado': aging.value = await call(GetFiadoAging()); break
      case 'compras': suppliers.value = await call(GetPurchasesBySupplier(start.value, end.value)); break
      case 'estoque': stock.value = await call(GetStockValue()); break
      case 'descontos': discounts.value = await call(GetDiscountsByOperator(start.value, end.value)); break
    }
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const profitRows = computed(() => (profitBy.value === 'produto' ? profit.value?.products : profit.value?.categories) ?? [])
const agingTotal = computed(() => aging.value.reduce((a, c) => ({
  balance: a.balance + c.balanceCents, d30: a.d30 + c.upto30Cents, d60: a.d60 + c.upto60Cents, d90: a.d90 + c.upto90Cents, over: a.over + c.over90Cents,
}), { balance: 0, d30: 0, d60: 0, d90: 0, over: 0 }))
const stockTotal = computed(() => stock.value.reduce((a, r) => ({ qty: a.qty + r.qty, cost: a.cost + r.costCents, price: a.price + r.priceCents, noCost: a.noCost + r.noCost }), { qty: 0, cost: 0, price: 0, noCost: 0 }))
const idleCost = computed(() => idle.value.reduce((a, p) => a + p.stockQty * p.costCents, 0))
const abcCount = (c: string) => abc.value?.rows.filter((r) => r.class === c).length ?? 0

// ---------- gráficos (só na tela) ----------
const classColor: Record<string, string> = { A: '#0e9466', B: '#d98b06', C: '#94a3b8' }
// Curva ABC: % de produtos e % da receita de cada classe.
const abcShares = computed(() => {
  const rows = abc.value?.rows ?? []
  const total = abc.value?.totalCents || 1
  return ['A', 'B', 'C'].map((c) => {
    const list = rows.filter((r) => r.class === c)
    return { c, products: rows.length ? (list.length / rows.length) * 100 : 0, revenue: (list.reduce((a, r) => a + r.revenueCents, 0) / total) * 100 }
  })
})
// Curva de Pareto: o acumulado da receita, produto a produto (do que mais vende ao que menos vende).
const PW = 900
const PH = 200
const pareto = computed(() => {
  const rows = abc.value?.rows ?? []
  if (!rows.length) return null
  const n = rows.length
  const x = (i: number) => (n === 1 ? PW : (i / (n - 1)) * PW)
  const y = (pct: number) => PH - (pct / 100) * PH
  const line = rows.map((r, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(r.cumulativePct).toFixed(1)}`).join(' ')
  // Faixas de cada classe no eixo X (do primeiro ao último produto da classe).
  const bands = ['A', 'B', 'C'].map((c) => {
    const idx = rows.map((r, i) => (r.class === c ? i : -1)).filter((i) => i >= 0)
    if (!idx.length) return null
    const from = idx[0] === 0 ? 0 : (x(idx[0] - 1) + x(idx[0])) / 2
    const to = idx[idx.length - 1] === n - 1 ? PW : (x(idx[idx.length - 1]) + x(idx[idx.length - 1] + 1)) / 2
    return { c, from, to, count: idx.length }
  }).filter((b) => b !== null)
  return { line, area: `${line} L${PW},${PH} L0,${PH} Z`, bands, y80: y(80), y95: y(95) }
})
// Parados: os 10 com mais dinheiro parado (ou mais unidades, para quem não vê custo).
const idleBars = computed<BarItem[]>(() => {
  const byCost = can('ver_custo')
  return [...idle.value]
    .sort((a, b) => (byCost ? b.stockQty * b.costCents - a.stockQty * a.costCents : b.stockQty - a.stockQty))
    .slice(0, 10)
    .map((p) => ({
      key: p.productId, label: p.name, value: byCost ? p.stockQty * p.costCents : p.stockQty,
      text: byCost ? money(p.stockQty * p.costCents) : `${p.stockQty} un.`,
      sub: `${p.stockQty} un. · ${p.idleDays < 0 ? 'nunca vendeu' : `parado há ${p.idleDays} dias`}`,
      color: p.idleDays < 0 || p.idleDays > 90 ? '#d63c31' : '#d98b06',
    }))
})
const idleByCategory = computed(() => {
  const m = new Map<string, number>()
  for (const p of idle.value) m.set(p.categoryName || 'Sem categoria', (m.get(p.categoryName || 'Sem categoria') ?? 0) + p.stockQty)
  return [...m].map(([name, value]) => ({ name, value, text: `${value} un.` }))
})
// Descontos: quanto cada operador deu e a soma de desconto e promoção.
const discountBars = computed<BarItem[]>(() => [...discounts.value].sort((a, b) => b.discountCents - a.discountCents).map((d) => ({
  key: d.userName, label: d.userName, value: d.discountCents, text: money(d.discountCents),
  sub: `${pct(d.discountCents, d.subtotalCents)} do vendido · ${d.salesWithDiscount} de ${d.sales} vendas`,
})))
const discountTotals = computed(() => discounts.value.reduce((a, d) => ({ discount: a.discount + d.discountCents, promo: a.promo + d.promoCents }), { discount: 0, promo: 0 }))
const shortMoney = (cents: number) => (cents >= 100000 ? `R$ ${(cents / 100000).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} mil` : money(cents))

// Lucro: os 10 que mais deram lucro no período.
const profitBars = computed<BarItem[]>(() => [...profitRows.value].sort((a, b) => b.profitCents - a.profitCents).slice(0, 10).map((r) => ({
  key: r.productId || r.name, label: r.name, value: r.profitCents, text: money(r.profitCents),
  sub: `margem ${pct(r.profitCents, r.revenueCents)}`, color: r.profitCents < 0 ? '#d63c31' : '#0e9466',
})))

// Exportação: a mesma tabela da tela, com os números crus para o Excel somar.
function exportCurrent() {
  const frac = (part: number, whole: number) => (whole ? part / whole : '')
  switch (tab.value) {
    case 'lucro': {
      const cols: Col[] = [[profitBy.value === 'produto' ? 'Produto' : 'Categoria', 'text'], ['Qtd.', 'int'], ['Receita', 'money'], ['Custo', 'money'], ['Lucro', 'money'], ['Margem', 'pct']]
      return exportTable(`Lucro bruto por ${profitBy.value}`, `Período: ${period.value}`, cols,
        profitRows.value.map((r) => [r.name, r.qty, r.revenueCents, r.costCents, r.profitCents, frac(r.profitCents, r.revenueCents)]))
    }
    case 'abc':
      return exportTable('Curva ABC', `Período: ${period.value}`, [['Classe', 'text'], ['Produto', 'text'], ['Qtd.', 'int'], ['Receita', 'money'], ['Participação', 'pct'], ['Acumulado', 'pct']],
        (abc.value?.rows ?? []).map((r) => [r.class, r.name, r.qty, r.revenueCents, r.sharePct / 100, r.cumulativePct / 100]))
    case 'parados':
      return exportTable('Produtos parados', `Sem venda há ${days.value} dias ou mais`, [['Produto', 'text'], ['Categoria', 'text'], ['Estoque', 'int'], ['Dias parado', 'text'], ['Valor a custo', 'money']],
        idle.value.map((p) => [p.name, p.categoryName, p.stockQty, p.idleDays < 0 ? 'nunca vendeu' : String(p.idleDays), p.stockQty * p.costCents]))
    case 'fiado':
      return exportTable('A prazo em atraso', `Posição em ${br(today())}`, [['Cliente', 'text'], ['Telefone', 'text'], ['Dívida', 'money'], ['Até 30 dias', 'money'], ['31 a 60', 'money'], ['61 a 90', 'money'], ['Mais de 90', 'money']],
        aging.value.map((c) => [c.name, c.phone ?? '', c.balanceCents, c.upto30Cents, c.upto60Cents, c.upto90Cents, c.over90Cents]))
    case 'compras':
      return exportTable('Compras por fornecedor', `Período: ${period.value}`, [['Fornecedor', 'text'], ['Compras', 'int'], ['Total', 'money']],
        suppliers.value.map((s) => [s.name, s.count, s.totalCents]))
    case 'estoque':
      return exportTable('Estoque valorizado', `Posição em ${br(today())}`, [['Categoria', 'text'], ['Produtos', 'int'], ['Unidades', 'int'], ['A custo', 'money'], ['A preço de venda', 'money']],
        stock.value.map((r) => [r.categoryName, r.products, r.qty, r.costCents, r.priceCents]))
    case 'descontos':
      return exportTable('Descontos por operador', `Período: ${period.value}`, [['Operador', 'text'], ['Vendas', 'int'], ['Com desconto', 'int'], ['Subtotal', 'money'], ['Desconto dado', 'money'], ['% do subtotal', 'pct'], ['Promoções', 'money']],
        discounts.value.map((d) => [d.userName, d.sales, d.salesWithDiscount, d.subtotalCents, d.discountCents, frac(d.discountCents, d.subtotalCents), d.promoCents]))
  }
}
const sheet = ref<HTMLElement | null>(null)
defineExpose({ load, exportXlsx: exportCurrent, print, pdf: () => savePdf(sheet.value, props.title) })
</script>

<template>
  <section class="card stack">
    <div v-if="error" class="alert error">{{ error }}</div>

    <div ref="sheet" :class="{ 'print-area': printing }" class="stack">
      <ReportSheet :title="title" :period="usesPeriod ? period : `Posição em ${br(today())}`">

      <!-- Lucro bruto -->
      <template v-if="tab === 'lucro' && profit">
        <div class="grid grid-4">
          <div class="stat"><div class="label">Receita líquida</div><div class="value mono">{{ money(profit.revenueCents) }}</div></div>
          <div class="stat"><div class="label">Custo</div><div class="value mono">{{ money(profit.costCents) }}</div></div>
          <div class="stat"><div class="label">Lucro bruto</div><div class="value mono" :style="{ color: profit.profitCents < 0 ? 'var(--danger)' : 'var(--success)' }">{{ money(profit.profitCents) }}</div></div>
          <div class="stat"><div class="label">Margem</div><div class="value mono">{{ pct(profit.profitCents, profit.revenueCents) }}</div></div>
        </div>
        <section v-if="profitRows.length" class="chart-box no-print">
          <div class="card-head"><strong>10 que mais deram lucro ({{ profitBy === 'produto' ? 'produtos' : 'categorias' }})</strong></div>
          <BarList :items="profitBars" />
        </section>
        <div class="row no-print">
          <div class="tabs seg" role="tablist" aria-label="Agrupar lucro">
            <button role="tab" :aria-selected="profitBy === 'produto'" @click="profitBy = 'produto'">Por produto</button>
            <button role="tab" :aria-selected="profitBy === 'categoria'" @click="profitBy = 'categoria'">Por categoria</button>
          </div>
        </div>
        <div v-if="!profitRows.length" class="state">Nenhuma venda no período.</div>
        <table v-else>
          <thead><tr><th class="left">{{ profitBy === 'produto' ? 'Produto' : 'Categoria' }}</th><th class="right">Qtd.</th><th class="right">Receita</th><th class="right">Custo</th><th class="right">Lucro</th><th class="right">Margem</th></tr></thead>
          <tbody>
            <tr v-for="r in profitRows" :key="r.productId || r.name">
              <td class="left">{{ r.name }}
                <span v-if="r.noCost" class="badge warn" title="Produto sem custo cadastrado: o lucro aparece maior do que é">sem custo</span>
                <span v-else-if="r.estimated" class="badge" title="Venda antiga, sem custo gravado: usado o custo atual">custo estimado</span>
              </td>
              <td class="right mono">{{ r.qty }}</td>
              <td class="right mono">{{ money(r.revenueCents) }}</td>
              <td class="right mono">{{ money(r.costCents) }}</td>
              <td class="right mono" :style="{ color: r.profitCents < 0 ? 'var(--danger)' : '' }">{{ money(r.profitCents) }}</td>
              <td class="right mono">{{ pct(r.profitCents, r.revenueCents) }}</td>
            </tr>
          </tbody>
        </table>
        <p class="small muted" style="margin: 0">Receita = o que o cliente pagou (com descontos e promoções), menos devoluções. Custo = o custo do produto na hora da venda.</p>
      </template>

      <!-- Curva ABC -->
      <template v-else-if="tab === 'abc' && abc">
        <div class="grid grid-4">
          <div class="stat"><div class="label">Receita do período</div><div class="value mono">{{ money(abc.totalCents) }}</div></div>
          <div class="stat"><div class="label">Classe A (80% da receita)</div><div class="value mono">{{ abcCount('A') }} produto(s)</div></div>
          <div class="stat"><div class="label">Classe B (próximos 15%)</div><div class="value mono">{{ abcCount('B') }} produto(s)</div></div>
          <div class="stat"><div class="label">Classe C (últimos 5%)</div><div class="value mono">{{ abcCount('C') }} produto(s)</div></div>
        </div>
        <div v-if="pareto" class="charts no-print">
          <section class="chart-box">
            <div class="card-head"><strong>Curva ABC</strong><span class="small muted">receita acumulada, do produto que mais vende ao que menos vende</span></div>
            <svg :viewBox="`-44 -10 ${PW + 54} ${PH + 40}`" class="pareto" role="img" aria-label="Curva ABC">
              <rect v-for="b in pareto.bands" :key="b.c" :x="b.from" y="0" :width="Math.max(1, b.to - b.from)" :height="PH" :fill="classColor[b.c]" opacity="0.1" />
              <text v-for="b in pareto.bands" :key="`t${b.c}`" :x="(b.from + b.to) / 2" :y="PH + 22" text-anchor="middle" class="tick strong" :fill="classColor[b.c]">
                {{ b.c }} · {{ b.count }}</text>
              <line v-for="g in [0, 25, 50, 75, 100]" :key="g" x1="0" :x2="PW" :y1="PH - (g / 100) * PH" :y2="PH - (g / 100) * PH" class="grid" />
              <text v-for="g in [0, 25, 50, 75, 100]" :key="`g${g}`" x="-8" :y="PH - (g / 100) * PH + 4" text-anchor="end" class="tick">{{ g }}%</text>
              <line x1="0" :x2="PW" :y1="pareto.y80" :y2="pareto.y80" class="limit" />
              <line x1="0" :x2="PW" :y1="pareto.y95" :y2="pareto.y95" class="limit" />
              <path :d="pareto.area" class="area" />
              <path :d="pareto.line" class="line" />
            </svg>
          </section>
          <section class="chart-box">
            <div class="card-head"><strong>Quanto cada classe representa</strong></div>
            <div v-for="s in abcShares" :key="s.c" class="share">
              <div class="share-head"><span class="badge" :style="{ background: classColor[s.c], color: '#fff' }">{{ s.c }}</span>
                <span class="small muted">{{ s.products.toFixed(0) }}% dos produtos → {{ s.revenue.toFixed(0) }}% da receita</span></div>
              <div class="share-bar"><i class="p" :style="{ width: `${s.products}%` }" /></div>
              <div class="share-bar"><i :style="{ width: `${s.revenue}%`, background: classColor[s.c] }" /></div>
            </div>
            <p class="small muted" style="margin: 8px 0 0"><span class="key p" /> produtos &nbsp; <span class="key" /> receita</p>
          </section>
        </div>
        <div v-if="!abc.rows.length" class="state">Nenhuma venda no período.</div>
        <table v-else>
          <thead><tr><th>Classe</th><th class="left">Produto</th><th class="right">Qtd.</th><th class="right">Receita</th><th class="right">Participação</th><th class="right">Acumulado</th></tr></thead>
          <tbody>
            <tr v-for="r in abc.rows" :key="r.productId">
              <td><span class="badge" :class="{ ok: r.class === 'A', warn: r.class === 'B' }">{{ r.class }}</span></td>
              <td class="left">{{ r.name }}</td>
              <td class="right mono">{{ r.qty }}</td>
              <td class="right mono">{{ money(r.revenueCents) }}</td>
              <td class="right mono">{{ r.sharePct.toFixed(1).replace('.', ',') }}%</td>
              <td class="right mono">{{ r.cumulativePct.toFixed(1).replace('.', ',') }}%</td>
            </tr>
          </tbody>
        </table>
        <p class="small muted" style="margin: 0">A: poucos produtos que trazem a maior parte do faturamento (não pode faltar). C: muitos produtos que vendem pouco.</p>
      </template>

      <!-- Produtos parados -->
      <template v-else-if="tab === 'parados'">
        <div class="grid grid-4">
          <div class="stat"><div class="label">Produtos parados</div><div class="value mono">{{ idle.length }}</div></div>
          <div class="stat"><div class="label">Unidades paradas</div><div class="value mono">{{ idle.reduce((a, p) => a + p.stockQty, 0) }}</div></div>
          <div v-if="can('ver_custo')" class="stat"><div class="label">Dinheiro parado (a custo)</div><div class="value mono">{{ money(idleCost) }}</div></div>
        </div>
        <div v-if="idle.length" class="charts no-print">
          <section class="chart-box">
            <div class="card-head"><strong>{{ can('ver_custo') ? 'Mais dinheiro parado' : 'Mais unidades paradas' }}</strong>
              <span class="small muted"><span class="key red" /> nunca vendeu ou +90 dias &nbsp; <span class="key amber" /> até 90 dias</span></div>
            <BarList :items="idleBars" />
          </section>
          <section class="chart-box">
            <div class="card-head"><strong>Unidades paradas por categoria</strong></div>
            <DonutChart :items="idleByCategory" center-label="unidades" :center="String(idle.reduce((a, p) => a + p.stockQty, 0))" />
          </section>
        </div>
        <div v-if="!idle.length" class="state">Nenhum produto com estoque parado há {{ days }} dias ou mais.</div>
        <table v-else>
          <thead><tr><th class="left">Produto</th><th>Categoria</th><th class="right">Estoque</th><th>Última venda</th><th class="right">Parado há</th><th v-if="can('ver_custo')" class="right">Valor a custo</th></tr></thead>
          <tbody>
            <tr v-for="p in idle" :key="p.productId">
              <td class="left">{{ p.name }}</td>
              <td>{{ p.categoryName }}</td>
              <td class="right mono">{{ p.stockQty }}</td>
              <td class="nowrap">{{ p.lastSaleAt ? formatDateTime(p.lastSaleAt) : '—' }}</td>
              <td class="right">
                <span v-if="p.idleDays < 0" class="badge bad">nunca vendeu</span>
                <span v-else class="mono">{{ p.idleDays }} dias</span>
              </td>
              <td v-if="can('ver_custo')" class="right mono">{{ money(p.stockQty * p.costCents) }}</td>
            </tr>
          </tbody>
        </table>
      </template>

      <!-- Fiado em atraso -->
      <template v-else-if="tab === 'fiado'">
        <div class="grid grid-4">
          <div class="stat"><div class="label">A receber</div><div class="value mono">{{ money(agingTotal.balance) }}</div><div class="small muted">{{ aging.length }} cliente(s)</div></div>
          <div class="stat"><div class="label">Até 30 dias</div><div class="value mono">{{ money(agingTotal.d30) }}</div></div>
          <div class="stat"><div class="label">31 a 90 dias</div><div class="value mono">{{ money(agingTotal.d60 + agingTotal.d90) }}</div></div>
          <div class="stat"><div class="label">Mais de 90 dias</div><div class="value mono" :style="{ color: agingTotal.over ? 'var(--danger)' : '' }">{{ money(agingTotal.over) }}</div></div>
        </div>
        <div v-if="!aging.length" class="state">Nenhum cliente devendo.</div>
        <table v-else>
          <thead><tr><th>Cliente</th><th class="right">Dívida</th><th class="right">Até 30 dias</th><th class="right">31 a 60</th><th class="right">61 a 90</th><th class="right">Mais de 90</th><th class="right">Mais antiga</th><th class="no-print"></th></tr></thead>
          <tbody>
            <tr v-for="c in aging" :key="c.customerId">
              <td class="left">{{ c.name }}<div v-if="c.phone" class="small muted">{{ c.phone }}</div></td>
              <td class="right mono"><strong>{{ money(c.balanceCents) }}</strong></td>
              <td class="right mono">{{ c.upto30Cents ? money(c.upto30Cents) : '—' }}</td>
              <td class="right mono">{{ c.upto60Cents ? money(c.upto60Cents) : '—' }}</td>
              <td class="right mono">{{ c.upto90Cents ? money(c.upto90Cents) : '—' }}</td>
              <td class="right mono" :style="{ color: c.over90Cents ? 'var(--danger)' : '' }">{{ c.over90Cents ? money(c.over90Cents) : '—' }}</td>
              <td class="right mono">{{ c.oldestDays }} dias</td>
              <td class="no-print"><button class="link icon-btn" title="Cobrar pelo WhatsApp" aria-label="Cobrar pelo WhatsApp" style="color: var(--success)" @click="chargeByWhatsapp(c.customerId)"><Icon name="whatsapp" /><span>Cobrar</span></button></td>
            </tr>
          </tbody>
        </table>
        <p class="small muted" style="margin: 0">O que o cliente já pagou abate as compras mais antigas; a dívida é das compras mais recentes.</p>
      </template>

      <!-- Compras por fornecedor -->
      <template v-else-if="tab === 'compras'">
        <div class="grid grid-4">
          <div class="stat"><div class="label">Total comprado</div><div class="value mono">{{ money(suppliers.reduce((a, s) => a + s.totalCents, 0)) }}</div></div>
          <div class="stat"><div class="label">Notas de compra</div><div class="value mono">{{ suppliers.reduce((a, s) => a + s.count, 0) }}</div></div>
        </div>
        <div v-if="!suppliers.length" class="state">Nenhuma compra no período.</div>
        <table v-else>
          <thead><tr><th>Fornecedor</th><th class="right">Compras</th><th class="right">Total</th><th class="right">Participação</th><th>Última compra</th></tr></thead>
          <tbody>
            <tr v-for="s in suppliers" :key="s.supplierId">
              <td class="left">{{ s.name }}</td>
              <td class="right mono">{{ s.count }}</td>
              <td class="right mono">{{ money(s.totalCents) }}</td>
              <td class="right mono">{{ pct(s.totalCents, suppliers.reduce((a, x) => a + x.totalCents, 0)) }}</td>
              <td class="nowrap">{{ s.lastAt ? formatDateTime(s.lastAt) : '—' }}</td>
            </tr>
          </tbody>
        </table>
        <p class="small muted" style="margin: 0">Compras estornadas ficam de fora.</p>
      </template>

      <!-- Estoque valorizado -->
      <template v-else-if="tab === 'estoque'">
        <div class="grid grid-4">
          <div class="stat"><div class="label">Unidades em estoque</div><div class="value mono">{{ stockTotal.qty }}</div></div>
          <div v-if="can('ver_custo')" class="stat"><div class="label">Valor a custo</div><div class="value mono">{{ money(stockTotal.cost) }}</div></div>
          <div class="stat"><div class="label">Valor a preço de venda</div><div class="value mono">{{ money(stockTotal.price) }}</div></div>
          <div v-if="can('ver_custo')" class="stat"><div class="label">Lucro potencial</div><div class="value mono">{{ money(stockTotal.price - stockTotal.cost) }}</div></div>
        </div>
        <div v-if="can('ver_custo') && stockTotal.noCost" class="alert warn">{{ stockTotal.noCost }} produto(s) com estoque estão sem custo cadastrado; o valor a custo fica menor do que é.</div>
        <div v-if="!stock.length" class="state">Nenhum produto com estoque.</div>
        <table v-else>
          <thead><tr><th>Categoria</th><th class="right">Produtos</th><th class="right">Unidades</th><th v-if="can('ver_custo')" class="right">A custo</th><th class="right">A preço de venda</th></tr></thead>
          <tbody>
            <tr v-for="r in stock" :key="r.categoryName">
              <td>{{ r.categoryName }}</td>
              <td class="right mono">{{ r.products }}</td>
              <td class="right mono">{{ r.qty }}</td>
              <td v-if="can('ver_custo')" class="right mono">{{ money(r.costCents) }}</td>
              <td class="right mono">{{ money(r.priceCents) }}</td>
            </tr>
          </tbody>
        </table>
      </template>

      <!-- Descontos por operador -->
      <template v-else-if="tab === 'descontos'">
        <div v-if="discounts.length" class="charts no-print">
          <section class="chart-box">
            <div class="card-head"><strong>Desconto dado por operador</strong><span class="small muted">em R$ e % do que vendeu</span></div>
            <BarList :items="discountBars" color="#d98b06" />
          </section>
          <section class="chart-box">
            <div class="card-head"><strong>Desconto × promoção</strong></div>
            <DonutChart :items="[{ name: 'Desconto do operador', value: discountTotals.discount, text: money(discountTotals.discount) },
              { name: 'Promoções automáticas', value: discountTotals.promo, text: money(discountTotals.promo) }]"
              center-label="total" :center="shortMoney(discountTotals.discount + discountTotals.promo)" />
          </section>
        </div>
        <div v-if="!discounts.length" class="state">Nenhuma venda no período.</div>
        <table v-else>
          <thead><tr><th>Operador</th><th class="right">Vendas</th><th class="right">Com desconto</th><th class="right">Subtotal</th><th class="right">Desconto dado</th><th class="right">% do subtotal</th><th class="right">Promoções</th></tr></thead>
          <tbody>
            <tr v-for="d in discounts" :key="d.userName">
              <td class="left">{{ d.userName }}</td>
              <td class="right mono">{{ d.sales }}</td>
              <td class="right mono">{{ d.salesWithDiscount }}</td>
              <td class="right mono">{{ money(d.subtotalCents) }}</td>
              <td class="right mono"><strong>{{ money(d.discountCents) }}</strong></td>
              <td class="right mono">{{ pct(d.discountCents, d.subtotalCents) }}</td>
              <td class="right mono">{{ money(d.promoCents) }}</td>
            </tr>
          </tbody>
        </table>
        <p class="small muted" style="margin: 0">Desconto dado = no item e no total da venda, pelo operador. Promoções são automáticas e ficam à parte.</p>
      </template>
      </ReportSheet>
    </div>
  </section>
</template>

<style scoped>
.seg { margin: 0; border-bottom: 0; }
/* Gráficos (só na tela), acima das tabelas. */
.charts { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(300px, 1fr); gap: 18px; }
@media (max-width: 1100px) { .charts { grid-template-columns: minmax(0, 1fr); } }
.chart-box { padding: 16px 18px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); }
.pareto { width: 100%; height: auto; display: block; }
.pareto .grid { stroke: var(--border); stroke-dasharray: 4 4; }
.pareto .limit { stroke: var(--muted); stroke-dasharray: 2 4; opacity: 0.6; }
.pareto .tick { font-size: 12px; fill: var(--muted); }
.pareto .tick.strong { font-weight: 700; }
.pareto .line { fill: none; stroke: var(--primary); stroke-width: 3; stroke-linejoin: round; }
.pareto .area { fill: var(--primary); opacity: 0.08; }
.share + .share { margin-top: 14px; }
.share-head { display: flex; align-items: center; gap: 10px; margin-bottom: 6px; }
.share-bar { height: 9px; border-radius: 9px; background: var(--surface-2); overflow: hidden; margin-top: 4px; }
.share-bar i { display: block; height: 100%; border-radius: 9px; }
.share-bar i.p, .key.p { background: #c7c2f8; }
.key { display: inline-block; width: 10px; height: 10px; border-radius: 3px; background: var(--muted); vertical-align: middle; }
.key.red { background: #d63c31; }
.key.amber { background: #d98b06; }
</style>
