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
</style>
