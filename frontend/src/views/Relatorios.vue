<!-- Caminho: pdv/frontend/src/views/Relatorios.vue -->
<script setup lang="ts">
// Relatórios › Vendas: um relatório só com o resumo em uma linha, os totais por
// forma de pagamento e por operador lado a lado e a lista de vendas do período.
// Sem a permissão de ver todos, o backend devolve só as vendas do próprio usuário.
import { computed, onMounted, ref, watch } from 'vue'
import { GenerateSalesReport, GetProfit, GetSalesReport, ListSellers } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import Icon from '../components/Icon.vue'
import ReportSheet from '../components/ReportSheet.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import SellerFilter from '../components/SellerFilter.vue'
import { exportTable, savePdf, usePrint } from '../composables/exportTable'
import { formatDateTime, money, paymentLabels, saleMethodLabel, today } from '../composables/format'
import { directPrint, printReceipt } from '../composables/receipt'
import { can, currentUser } from '../composables/session'
import { useToast } from '../composables/toast'
import type { SalesReport, User } from '../types'

const toast = useToast()
const methodKeys = ['dinheiro', 'debito', 'credito', 'pix']
const start = ref(today())
const end = ref(today())
const userId = ref(0)
const methods = ref([...methodKeys])
const report = ref<SalesReport | null>(null)
const shownUser = ref(0) // operador do relatório na tela (o filtro pode ter mudado depois)
const error = ref('')
const loading = ref(false)
const saleId = ref<number | null>(null)
const { printing, print } = usePrint()
const sheet = ref<HTMLElement | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    report.value = await call(GetSalesReport(start.value, end.value, userId.value))
    shownUser.value = userId.value
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
// Atalho de período; clicar no mesmo período de novo serve para atualizar.
function preset(days: number) {
  if (start.value === today(-days) && end.value === today()) return load()
  start.value = today(-days)
  end.value = today()
}
// Mudou período ou operador: recarrega sozinho (espera um pouco enquanto digita a data).
let timer = 0
watch([start, end, userId], () => {
  window.clearTimeout(timer)
  if (start.value && end.value) timer = window.setTimeout(load, 400)
})

const sellers = ref<User[]>([])
onMounted(async () => {
  load()
  sellers.value = await call(ListSellers()).catch(() => [])
})
// Quem não vê todos só vê as próprias vendas.
const operator = computed(() => {
  if (!sellers.value.length) return currentUser.value?.name ?? null
  return shownUser.value ? sellers.value.find((u) => u.id === shownUser.value)?.name ?? null : null
})

const br = (d: string) => d.split('-').reverse().join('/')
const period = computed(() => (report.value ? br(report.value.startDate) + (report.value.endDate !== report.value.startDate ? ` a ${br(report.value.endDate)}` : '') : ''))
// Formas marcadas (todas = sem filtro). Venda em várias formas ("misto") só aparece sem filtro.
const filtering = computed(() => methods.value.length !== methodKeys.length)
const byPayment = computed(() => (report.value?.byPaymentMethod ?? []).filter((t) => !filtering.value || methods.value.includes(t.paymentMethod)))
const sales = computed(() => (report.value?.sales ?? []).filter((s) => !filtering.value || methods.value.includes(s.paymentMethod)))
// Retiradas (sangrias) saem do dinheiro: somem junto com o filtro de Dinheiro.
const withdrawals = computed(() => (methods.value.includes('dinheiro') ? report.value?.withdrawals ?? [] : []))
// Com formas desmarcadas, os totais saem das vendas listadas (só as das formas marcadas).
const totals = computed(() => {
  const r = report.value
  if (!r) return { total: 0, count: 0, discount: 0, cancelled: 0 }
  if (!filtering.value) return { total: r.totalCents, count: r.saleCount, discount: r.discountCents, cancelled: r.cancelledCents }
  return sales.value.reduce((a, s) => s.status === 'cancelada'
    ? { ...a, cancelled: a.cancelled + s.totalCents }
    : { ...a, total: a.total + s.totalCents, count: a.count + 1, discount: a.discount + s.discountCents },
  { total: 0, count: 0, discount: 0, cancelled: 0 })
})
const ticket = computed(() => (totals.value.count ? Math.round(totals.value.total / totals.value.count) : 0))
const barColors: Record<string, string> = { pix: '#5b4cf0', dinheiro: '#10b981', cartao: '#06b6d4', debito: '#06b6d4', credito: '#f59e0b', fiado: '#ef4444' }
const barColor = (m: string) => barColors[m] ?? '#8b5cf6'

// ---------- gráficos (só na tela) ----------
// Faturamento por dia (ou por hora, se o período for um dia só), das vendas concluídas listadas.
const pad2 = (n: number) => String(n).padStart(2, '0')
const dayKey = (d: Date) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
const series = computed(() => {
  const r = report.value
  if (!r) return { byHour: false, points: [] as { label: string; cents: number; count: number }[] }
  const done = sales.value.filter((s) => s.status !== 'cancelada')
  if (r.startDate === r.endDate) {
    const hours = Array.from({ length: 24 }, (_, h) => ({ label: `${h}h`, cents: 0, count: 0 }))
    for (const s of done) {
      const h = hours[new Date(s.createdAt).getHours()]
      h.cents += s.totalCents
      h.count++
    }
    // Só o horário com movimento (do primeiro ao último com venda), no mínimo 8h às 20h.
    const used = hours.map((h, i) => (h.count ? i : -1)).filter((i) => i >= 0)
    const from = Math.min(8, ...used)
    const to = Math.max(20, ...used)
    return { byHour: true, points: hours.slice(from, to + 1) }
  }
  const days = new Map<string, { label: string; cents: number; count: number }>()
  const d = new Date(`${r.startDate}T12:00:00`)
  const last = new Date(`${r.endDate}T12:00:00`)
  while (d <= last && days.size < 370) {
    days.set(dayKey(d), { label: `${pad2(d.getDate())}/${pad2(d.getMonth() + 1)}`, cents: 0, count: 0 })
    d.setDate(d.getDate() + 1)
  }
  for (const s of done) {
    const p = days.get(dayKey(new Date(s.createdAt)))
    if (p) {
      p.cents += s.totalCents
      p.count++
    }
  }
  return { byHour: false, points: [...days.values()] }
})
const CW = 900
const CH = 240
const chart = computed(() => {
  const pts = series.value.points
  const max = Math.max(1, ...pts.map((p) => p.cents))
  // Teto "redondo" para as linhas de grade (4 faixas).
  const step = 10 ** Math.floor(Math.log10(max / 4))
  const top = Math.ceil(max / 4 / step) * step * 4
  const x = (i: number) => (pts.length <= 1 ? CW / 2 : (i / (pts.length - 1)) * CW)
  const y = (c: number) => CH - (c / top) * CH
  const coords = pts.map((p, i) => [x(i), y(p.cents)] as const)
  const line = coords.map(([px, py], i) => `${i ? 'L' : 'M'}${px.toFixed(1)},${py.toFixed(1)}`).join(' ')
  const area = coords.length ? `${line} L${coords[coords.length - 1][0].toFixed(1)},${CH} L${coords[0][0].toFixed(1)},${CH} Z` : ''
  // Rótulos do eixo X: no máximo ~10, sempre o primeiro e o último.
  const every = Math.max(1, Math.ceil(pts.length / 10))
  const ticks = pts.map((p, i) => ({ i, x: x(i), label: p.label })).filter((t) => t.i % every === 0 || t.i === pts.length - 1)
  const grid = [0, 1, 2, 3, 4].map((k) => ({ y: CH - (k / 4) * CH, value: (top * k) / 4 }))
  const best = pts.reduce((a, p, i) => (p.cents > (pts[a]?.cents ?? -1) ? i : a), 0)
  return { line, area, ticks, grid, coords, best: pts[best] && pts[best].cents ? { ...pts[best], x: x(best), y: y(pts[best].cents) } : null }
})
const shortMoney = (cents: number) => {
  const v = cents / 100
  return v >= 1000 ? `${(v / 1000).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} mil` : v.toLocaleString('pt-BR', { maximumFractionDigits: 0 })
}

// Vendas por categoria: as 5 maiores e o resto junto em "Outras".
const donutColors = ['#5b4cf0', '#06b6d4', '#10b981', '#f59e0b', '#ec4899', '#94a3b8']
const categories = computed(() => {
  const list = [...(report.value?.byCategory ?? [])].filter((c) => c.totalCents > 0).sort((a, b) => b.totalCents - a.totalCents)
  const head = list.slice(0, 5).map((c) => ({ name: c.categoryName || 'Sem categoria', cents: c.totalCents }))
  const rest = list.slice(5).reduce((a, c) => a + c.totalCents, 0)
  if (rest) head.push({ name: 'Outras', cents: rest })
  const sum = head.reduce((a, c) => a + c.cents, 0) || 1
  let acc = 0
  return head.map((c, i) => {
    const from = acc / sum
    acc += c.cents
    return { ...c, pct: Math.round((c.cents / sum) * 100), color: donutColors[i], path: arc(from, acc / sum) }
  })
})
// Arco do gráfico de rosca (raio 70, centro 90,90), de uma fração a outra da volta.
function arc(from: number, to: number) {
  if (to - from >= 0.9999) to = from + 0.9999
  const p = (f: number) => [90 + 70 * Math.sin(f * 2 * Math.PI), 90 - 70 * Math.cos(f * 2 * Math.PI)].map((n) => n.toFixed(2)).join(',')
  return `M${p(from)} A70,70 0 ${to - from > 0.5 ? 1 : 0},1 ${p(to)}`
}
const operators = computed(() => {
  const list = report.value?.byOperator ?? []
  const max = Math.max(1, ...list.map((o) => o.totalCents))
  return list.map((o) => ({ ...o, pct: (o.totalCents / max) * 100 }))
})

// Lucro bruto do período (precisa ver custo; vale para todos os operadores).
const profit = ref<number | null>(null)
watch(report, async (r) => {
  profit.value = null
  if (!r || !can('ver_custo') || shownUser.value) return
  try {
    profit.value = (await call(GetProfit(r.startDate, r.endDate))).profitCents
  } catch {
    // sem o lucro, o card não aparece
  }
})

function toggleMethod(m: string) {
  methods.value = methods.value.includes(m) ? methods.value.filter((x) => x !== m) : [...methods.value, m]
}

function exportXlsx() {
  if (!report.value) return
  exportTable('Vendas', `Período: ${period.value}${operator.value ? ` - ${operator.value}` : ''}`,
    [['Nº', 'int'], ['Data / Hora', 'text'], ['Operador', 'text'], ['Cliente', 'text'], ['Itens', 'text'], ['Forma', 'text'], ['Total', 'money'], ['Situação', 'text']],
    [...sales.value.map((s) => [s.id, formatDateTime(s.createdAt), s.userName ?? '', s.customerName ?? '', s.itemsLabel,
      saleMethodLabel(s), s.totalCents, s.status === 'cancelada' ? 'Cancelada' : 'Concluída']),
    ...withdrawals.value.map((w) => [null, formatDateTime(w.at), w.userName ?? '', '', w.description || 'Retirada',
      paymentLabels.dinheiro, -w.amountCents, 'Retirada'])])
}

async function printThermal() {
  if (!directPrint.value) return toast.error('Nenhuma impressora térmica configurada. Escolha a impressora em Configurações.')
  try {
    const r = await call(GenerateSalesReport(start.value, end.value, shownUser.value, 'geral'))
    if (r.id) await printReceipt('relatorio', r.id)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <section class="card stack no-print">
    <form class="row filters" @submit.prevent>
      <label class="field">De<input v-model="start" type="date" required /></label>
      <label class="field">Até<input v-model="end" type="date" required /></label>
      <SellerFilter v-model="userId" />
      <button type="button" @click="preset(0)">Hoje</button>
      <button type="button" @click="preset(6)">7 dias</button>
      <button type="button" @click="preset(29)">30 dias</button>
      <span class="spacer" />
      <button type="button" :disabled="!report" @click="print">Imprimir A4</button>
      <button type="button" :disabled="!report" @click="savePdf(sheet, 'Vendas')">Salvar PDF</button>
      <button type="button" :disabled="!report" @click="printThermal">Térmica</button>
      <button type="button" class="ghost" :disabled="!report" @click="exportXlsx">Excel</button>
    </form>
    <div class="row chips" role="group" aria-label="Formas de pagamento">
      <span class="small muted">Forma:</span>
      <div class="chip-track">
        <button v-for="m in methodKeys" :key="m" type="button" class="chip" :aria-pressed="methods.includes(m)" @click="toggleMethod(m)">{{ paymentLabels[m] }}</button>
      </div>
    </div>
    <div v-if="error" class="alert error">{{ error }}</div>
  </section>

  <!-- Só na tela: indicadores e barras por forma de pagamento; a folha impressa continua abaixo. -->
  <template v-if="report">
    <div class="kpi-row no-print" style="margin-top: 18px">
      <div class="kpi">
        <div class="kpi-head">Total vendido<span class="kpi-ic" style="--c: #5b4cf0"><Icon name="m-relatorios" :size="18" /></span></div>
        <div class="kpi-value mono">{{ money(totals.total) }}</div>
        <div class="small muted">{{ totals.count }} venda(s)</div>
      </div>
      <div class="kpi">
        <div class="kpi-head">Ticket médio<span class="kpi-ic" style="--c: #0e9466"><Icon name="recibo" :size="18" /></span></div>
        <div class="kpi-value mono">{{ money(ticket) }}</div>
        <div class="small muted">por venda concluída</div>
      </div>
      <div class="kpi">
        <div class="kpi-head">Descontos<span class="kpi-ic" style="--c: #d98b06"><Icon name="t-promocao" :size="18" /></span></div>
        <div class="kpi-value mono">{{ money(totals.discount) }}</div>
        <div class="small muted">concedidos no período</div>
      </div>
      <div class="kpi">
        <div class="kpi-head">Cancelado<span class="kpi-ic" style="--c: #d63c31"><Icon name="cancelar" :size="18" /></span></div>
        <div class="kpi-value mono">{{ money(totals.cancelled) }}</div>
        <div class="small muted">fora do total</div>
      </div>
      <div v-if="profit !== null" class="kpi">
        <div class="kpi-head">Lucro bruto<span class="kpi-ic" style="--c: #0891b2"><Icon name="receber" :size="18" /></span></div>
        <div class="kpi-value mono" :style="{ color: profit < 0 ? 'var(--danger)' : '' }">{{ money(profit) }}</div>
        <div class="small muted">margem {{ totals.total ? Math.round((profit / totals.total) * 100) : 0 }}% · todos os operadores</div>
      </div>
    </div>

    <div class="charts no-print">
      <section class="card">
        <div class="card-head"><strong>Faturamento {{ series.byHour ? 'por hora' : 'por dia' }}</strong>
          <span class="small muted"><template v-if="chart.best">melhor {{ series.byHour ? 'horário' : 'dia' }}: {{ chart.best.label }} · {{ money(chart.best.cents) }}</template></span></div>
        <div v-if="!totals.count" class="state">Nenhuma venda no período.</div>
        <svg v-else :viewBox="`-58 -12 ${CW + 84} ${CH + 40}`" class="line-chart" role="img" :aria-label="`Faturamento ${series.byHour ? 'por hora' : 'por dia'}`">
          <defs><linearGradient id="fat-area" x1="0" x2="0" y1="0" y2="1"><stop offset="0" stop-color="#5b4cf0" stop-opacity=".28" /><stop offset="1" stop-color="#5b4cf0" stop-opacity="0" /></linearGradient></defs>
          <g v-for="g in chart.grid" :key="g.y">
            <line x1="0" :x2="CW" :y1="g.y" :y2="g.y" class="grid" :class="{ base: g.value === 0 }" />
            <text x="-10" :y="g.y + 4" class="tick" text-anchor="end">{{ shortMoney(g.value) }}</text>
          </g>
          <path :d="chart.area" fill="url(#fat-area)" />
          <path :d="chart.line" class="line" />
          <circle v-for="(c, i) in chart.coords" :key="i" :cx="c[0]" :cy="c[1]" r="4" class="dot">
            <title>{{ series.points[i].label }}: {{ money(series.points[i].cents) }} em {{ series.points[i].count }} venda(s)</title>
          </circle>
          <text v-for="t in chart.ticks" :key="t.i" :x="t.x" :y="CH + 22" class="tick" text-anchor="middle">{{ t.label }}</text>
        </svg>
      </section>
      <section class="card">
        <div class="card-head"><strong>Vendas por categoria</strong></div>
        <div v-if="!categories.length" class="state">Nenhuma venda no período.</div>
        <div v-else class="donut-wrap">
          <svg viewBox="0 0 180 180" class="donut" role="img" aria-label="Vendas por categoria">
            <path v-for="c in categories" :key="c.name" :d="c.path" :stroke="c.color" stroke-width="26" fill="none"><title>{{ c.name }}: {{ money(c.cents) }}</title></path>
            <text x="90" y="85" text-anchor="middle" class="donut-sub">categorias</text>
            <text x="90" y="106" text-anchor="middle" class="donut-num">{{ report.byCategory?.length ?? 0 }}</text>
          </svg>
          <ul class="legend">
            <li v-for="c in categories" :key="c.name"><i :style="{ background: c.color }" /><span>{{ c.name }}</span><strong>{{ c.pct }}%</strong></li>
          </ul>
        </div>
      </section>
    </div>
    <section v-if="!shownUser && operators.length" class="card no-print op-card">
      <div class="card-head"><strong>Vendas por operador</strong></div>
      <div v-for="o in operators" :key="o.userName" class="op-row">
        <span class="op-name">{{ o.userName }}</span>
        <span class="muted small op-count">{{ o.count }} venda(s)</span>
        <div class="bar op-bar"><i :style="{ width: `${o.pct}%`, background: 'var(--primary)' }" /></div>
        <strong class="mono">{{ money(o.totalCents) }}</strong>
      </div>
    </section>
    <section v-if="byPayment.length" class="card no-print pay-bars">
      <div class="card-head"><strong>Formas de pagamento</strong><span class="small muted">{{ period }}</span></div>
      <div v-for="t in byPayment" :key="t.paymentMethod" class="pay-bar">
        <div class="pay-bar-label"><span>{{ paymentLabels[t.paymentMethod] ?? t.paymentMethod }} <span class="muted small">{{ t.count }} venda(s)</span></span>
          <strong class="mono">{{ money(t.totalCents) }} · {{ totals.total ? Math.round((t.totalCents / totals.total) * 100) : 0 }}%</strong></div>
        <div class="bar"><i :style="{ width: `${totals.total ? (t.totalCents / totals.total) * 100 : 0}%`, background: barColor(t.paymentMethod) }" /></div>
      </div>
    </section>
  </template>

  <section v-if="report" ref="sheet" class="card" :class="{ 'print-area': printing }" style="margin-top: 18px">
    <ReportSheet title="Vendas" :period="period" :operator="operator">
      <p class="summary">
        <span>Total vendido <strong>{{ money(totals.total) }}</strong></span>
        <span><strong>{{ totals.count }}</strong> venda(s)</span>
        <span>Ticket médio <strong>{{ money(ticket) }}</strong></span>
        <span>Descontos <strong>{{ money(totals.discount) }}</strong></span>
        <span>Cancelado <strong>{{ money(totals.cancelled) }}</strong></span>
      </p>

      <div class="side">
        <div>
          <h4>Por forma de pagamento</h4>
          <div v-if="!byPayment.length && !withdrawals.length" class="state">Nenhuma venda.</div>
          <table v-else>
            <tbody>
              <tr v-for="t in byPayment" :key="t.paymentMethod"><td>{{ paymentLabels[t.paymentMethod] ?? t.paymentMethod }}</td><td class="right muted">{{ t.count }}</td><td class="right mono">{{ money(t.totalCents) }}</td></tr>
              <template v-if="withdrawals.length">
                <tr><td>Retiradas (sangria)</td><td class="right muted">{{ withdrawals.length }}</td><td class="right mono out">-{{ money(report.withdrawalsCents) }}</td></tr>
                <tr><td><strong>Dinheiro após retiradas</strong></td><td /><td class="right mono"><strong>{{ money(report.cashAfterWithdrawals) }}</strong></td></tr>
              </template>
            </tbody>
          </table>
        </div>
        <div v-if="!shownUser">
          <h4>Por operador</h4>
          <div v-if="!report.byOperator?.length" class="state">Nenhuma venda.</div>
          <table v-else>
            <tbody><tr v-for="t in report.byOperator" :key="t.userName"><td class="left">{{ t.userName }}</td><td class="right muted">{{ t.count }}</td><td class="right mono">{{ money(t.totalCents) }}</td></tr></tbody>
          </table>
        </div>
      </div>

      <h4>Vendas do período</h4>
      <div v-if="!sales.length" class="state">Nenhuma venda no período.</div>
      <table v-else class="list">
        <colgroup><col style="width: 7%" /><col style="width: 15%" /><col style="width: 14%" /><col /><col style="width: 10%" /><col style="width: 14%" /></colgroup>
        <thead><tr><th>Nº</th><th>Data / Hora</th><th>Operador</th><th>Itens</th><th>Forma</th><th class="right">Total</th></tr></thead>
        <tbody>
          <tr v-for="s in sales" :key="s.id" :class="{ inactive: s.status === 'cancelada' }">
            <td><button class="link no-print" @click="saleId = s.id">{{ s.id }}</button><span class="print-only">{{ s.id }}</span></td>
            <td class="nowrap">{{ formatDateTime(s.createdAt) }}</td>
            <td class="left">{{ s.userName ?? '—' }}</td>
            <td class="ellipsis" :title="s.itemsLabel">{{ s.itemsLabel || '—' }}<span v-if="s.customerName" class="muted"> · {{ s.customerName }}</span></td>
            <td>{{ saleMethodLabel(s) }}</td>
            <td class="right mono">{{ money(s.totalCents) }}<span v-if="s.status === 'cancelada'" class="badge bad" style="margin-left: 4px">Cancelada</span></td>
          </tr>
        </tbody>
      </table>

      <template v-if="withdrawals.length">
        <h4>Retiradas do período</h4>
        <table class="list">
          <colgroup><col style="width: 15%" /><col style="width: 14%" /><col /><col style="width: 14%" /></colgroup>
          <thead><tr><th>Data / Hora</th><th>Operador</th><th>Motivo</th><th class="right">Valor</th></tr></thead>
          <tbody>
            <tr v-for="(w, i) in withdrawals" :key="i">
              <td class="nowrap">{{ formatDateTime(w.at) }}</td>
              <td class="left">{{ w.userName ?? '—' }}</td>
              <td class="ellipsis" :title="w.description">{{ w.description || '—' }}</td>
              <td class="right mono out">-{{ money(w.amountCents) }}</td>
            </tr>
          </tbody>
        </table>
      </template>
    </ReportSheet>
  </section>
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" @close="saleId = null" />
</template>

<style scoped>
.filters { align-items: end; }
.filters button { align-self: end; }
.summary { display: flex; flex-wrap: wrap; gap: 6px 22px; margin: 0 0 12px; font-size: 14px; color: var(--muted); }
.summary strong { color: var(--text); }
.side { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
h4 { margin: 12px 0 4px; font-size: 13px; }
table th, table td { text-align: left; padding: 3px 8px; }
table th.right, table td.right { text-align: right; }
.list { table-layout: fixed; }
.ellipsis { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.print-only { display: none; }
.out { color: var(--danger); }
.pay-bars { margin-top: 18px; }
/* Gráficos: faturamento (linha) e categorias (rosca) lado a lado; operadores embaixo. */
.charts { display: grid; grid-template-columns: minmax(0, 1.7fr) minmax(300px, 1fr); gap: 18px; margin-top: 18px; }
@media (max-width: 1100px) { .charts { grid-template-columns: minmax(0, 1fr); } }
.charts .card { margin-top: 0; }
.line-chart { width: 100%; height: auto; display: block; }
.line-chart .grid { stroke: var(--border); stroke-dasharray: 4 4; }
.line-chart .grid.base { stroke-dasharray: none; }
.line-chart .tick { font-size: 12px; fill: var(--muted); }
.line-chart .line { fill: none; stroke: var(--primary); stroke-width: 3; stroke-linejoin: round; stroke-linecap: round; }
.line-chart .dot { fill: var(--surface); stroke: var(--primary); stroke-width: 2.5; }
.line-chart .dot:hover { fill: var(--primary); }
.donut-wrap { display: flex; align-items: center; gap: 20px; }
.donut { width: 180px; flex-shrink: 0; }
.donut path { transition: opacity 0.15s; }
.donut path:hover { opacity: 0.8; }
.donut-sub { font-size: 12px; fill: var(--muted); }
.donut-num { font-size: 22px; font-weight: 700; fill: var(--text); }
.legend { list-style: none; margin: 0; padding: 0; flex: 1; min-width: 0; }
.legend li { display: flex; align-items: center; gap: 10px; padding: 6px 0; font-size: 14px; }
.legend i { width: 11px; height: 11px; border-radius: 3px; flex-shrink: 0; }
.legend span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.op-card { margin-top: 18px; }
.op-row { display: grid; grid-template-columns: minmax(120px, 200px) 100px minmax(0, 1fr) 130px; align-items: center; gap: 14px; padding: 8px 0; }
.op-row + .op-row { border-top: 1px solid var(--border); }
.op-name { font-weight: 600; }
.op-row strong { text-align: right; }
.pay-bar + .pay-bar { margin-top: 12px; }
.pay-bar-label { display: flex; justify-content: space-between; gap: 8px; font-size: 14px; margin-bottom: 6px; }
.bar { height: 8px; border-radius: 8px; background: var(--surface-2); overflow: hidden; }
.bar i { display: block; height: 100%; border-radius: 8px; }
@media print {
  .print-only { display: inline; }
  .out { color: #000; }
  .ellipsis { white-space: normal; }
}
</style>
