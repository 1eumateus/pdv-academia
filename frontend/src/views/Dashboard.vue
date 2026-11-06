<!-- Caminho: pdv/frontend/src/views/Dashboard.vue -->
<script setup lang="ts">
// Tela inicial enxuta: hoje, mês, caixa e fiado; gráfico dos últimos 30 dias;
// os 5 mais vendidos e os 5 alertas de estoque. Tudo vem pronto do GetDashboard.
import { computed, onMounted, ref } from 'vue'
import { GetBackupStatus, GetDashboard, RunBackupNow } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import Icon from '../components/Icon.vue'
import { money } from '../composables/format'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import type { models } from '../types'

const d = ref<models.Dashboard | null>(null)
const error = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    d.value = await call(GetDashboard())
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// Aviso de backup: cópia externa atrasada, destino fora do ar ou não configurado.
const toast = useToast()
const backupWarn = ref('')
const backupBusy = ref(false)
const noExternal = ref(false)
async function loadBackup() {
  try {
    const st = await call(GetBackupStatus())
    backupWarn.value = st.warning
    noExternal.value = !st.externalDir
  } catch {
    // o aviso é secundário: sem ele o Painel continua útil
  }
}
onMounted(loadBackup)
async function copyNow() {
  backupBusy.value = true
  try {
    const st = await call(RunBackupNow())
    backupWarn.value = st.warning
    if (st.externalError) toast.error(st.warning)
    else toast.success('Backup feito e copiado para a pasta externa.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    backupBusy.value = false
  }
}

// Variação contra o período anterior: null quando não há base para comparar.
function change(now: number, before: number): number | null {
  return before > 0 ? Math.round(((now - before) / before) * 100) : null
}

// Gráfico de barras (SVG próprio).
const W = 720
const H = 200
const BASE = H - 18
const maxDay = computed(() => Math.max(1, ...(d.value?.days ?? []).map((x) => x.totalCents)))
const bars = computed(() => {
  const days = d.value?.days ?? []
  const slot = W / Math.max(1, days.length)
  return days.map((x, i) => {
    const h = Math.round((x.totalCents / maxDay.value) * (BASE - 6))
    const [, m, dd] = x.date.split('-')
    return { ...x, x: i * slot + 2, w: Math.max(2, slot - 4), y: BASE - h, h, label: `${dd}/${m}`, last: i === days.length - 1 }
  })
})
const daysTotal = computed(() => (d.value?.days ?? []).reduce((a, x) => a + x.totalCents, 0))
const top = computed(() => (d.value?.topProducts ?? []).slice(0, 5))
const alerts = computed(() => (d.value?.stockAlerts ?? []).slice(0, 5))
const alertCount = computed(() => (d.value ? d.value.zeroStockCount + d.value.lowStockCount : 0))

// Quadradinho com as iniciais do produto; a cor segue a posição na lista.
const thumbColors = ['#6366f1', '#10b981', '#f59e0b', '#ef4444', '#06b6d4']
function initials(name: string) {
  return name.trim().slice(0, 2).toUpperCase()
}
const shortcuts = computed(() => [
  { to: '/vendas', label: 'Nova venda', icon: 'm-vendas', color: '#5b4cf0', show: true },
  { to: '/produtos', label: 'Produtos', icon: 'm-produtos', color: '#f59e0b', show: true },
  { to: '/clientes', label: 'Clientes', icon: 'm-clientes', color: '#ec4899', show: true },
  { to: '/compras', label: 'Entrada NF-e', icon: 't-entrada', color: '#06b6d4', show: can('compras') },
  { to: '/etiquetas', label: 'Etiquetas', icon: 't-etiqueta', color: '#10b981', show: true },
  { to: '/orcamentos', label: 'Orçamentos', icon: 't-orcamento', color: '#8b5cf6', show: true },
].filter((s) => s.show))
</script>

<template>
  <div v-if="error" class="alert error">{{ error }} <button class="link" @click="load">Tentar novamente</button></div>
  <div v-else-if="!d" class="state">Carregando o painel…</div>
  <div v-else class="dash">
    <div class="dash-top">
      <div class="notices">
        <div v-if="backupWarn" class="notice warn" role="status">
          <strong>Backup:</strong> {{ backupWarn }}
          <router-link v-if="noExternal && can('alterar_configuracoes')" to="/configuracoes" class="go">Configurar</router-link>
          <button v-else-if="!noExternal" class="link go" :disabled="backupBusy" @click="copyNow">{{ backupBusy ? 'Copiando…' : 'Copiar agora' }}</button>
        </div>
        <router-link v-if="can('contas_a_pagar') && (d.billsDueToday || d.billsOverdue)" to="/financeiro/contas" class="notice" :class="d.billsOverdue ? 'bad' : 'warn'">
          <span v-if="d.billsDueToday">{{ d.billsDueToday }} conta(s) vence(m) hoje</span>
          <span v-if="d.billsDueToday && d.billsOverdue"> e </span>
          <span v-if="d.billsOverdue">{{ d.billsOverdue }} conta(s) vencida(s)</span>
        </router-link>
        <router-link v-if="alertCount" to="/estoque?view=alerta" class="notice" :class="d.zeroStockCount ? 'bad' : 'warn'">
          <span v-if="d.zeroStockCount">{{ d.zeroStockCount }} produto(s) sem estoque</span>
          <span v-if="d.zeroStockCount && d.lowStockCount"> e </span>
          <span v-if="d.lowStockCount">{{ d.lowStockCount }} com estoque baixo</span>
        </router-link>
      </div>
      <span class="small muted">{{ d.allSales ? 'Todos os operadores' : 'Somente as suas vendas' }}</span>
      <button :disabled="loading" @click="load">{{ loading ? 'Atualizando…' : 'Atualizar' }}</button>
    </div>

    <div class="kpis">
      <div class="kpi">
        <div class="kpi-head">Vendas hoje<span class="kpi-ic" style="--c: #5b4cf0"><Icon name="m-vendas" :size="18" /></span></div>
        <div class="kpi-value mono">{{ money(d.today.totalCents) }}</div>
        <div class="kpi-foot small muted">
          <span v-if="change(d.today.totalCents, d.yesterday.totalCents) !== null" class="badge"
            :class="change(d.today.totalCents, d.yesterday.totalCents)! >= 0 ? 'ok' : 'bad'" title="Comparado com ontem até esta hora">
            {{ change(d.today.totalCents, d.yesterday.totalCents)! >= 0 ? '▲' : '▼' }} {{ Math.abs(change(d.today.totalCents, d.yesterday.totalCents)!) }}%
          </span>
          {{ d.today.count }} venda(s)
        </div>
      </div>
      <div class="kpi">
        <div class="kpi-head">Mês até agora<span class="kpi-ic" style="--c: #d98b06"><Icon name="m-relatorios" :size="18" /></span></div>
        <div class="kpi-value mono">{{ money(d.month.totalCents) }}</div>
        <div class="kpi-foot small muted">
          <span v-if="change(d.month.totalCents, d.prevMonth.totalCents) !== null" class="badge"
            :class="change(d.month.totalCents, d.prevMonth.totalCents)! >= 0 ? 'ok' : 'bad'" title="Comparado com o mês passado no mesmo ponto">
            {{ change(d.month.totalCents, d.prevMonth.totalCents)! >= 0 ? '▲' : '▼' }} {{ Math.abs(change(d.month.totalCents, d.prevMonth.totalCents)!) }}%
          </span>
          {{ d.month.count }} venda(s)
        </div>
      </div>
      <router-link to="/caixa" class="kpi link-card">
        <div class="kpi-head">Caixa<span class="kpi-ic" style="--c: #0e9466"><Icon name="m-caixa" :size="18" /></span></div>
        <div class="kpi-value mono">{{ d.cash ? money(d.cashExpectedCents) : 'Fechado' }}</div>
        <div class="kpi-foot small muted">
          <span class="badge" :class="d.cash ? 'ok' : 'warn'">{{ d.cash ? 'Aberto' : 'Fechado' }}</span>
          {{ d.cash ? 'dinheiro na gaveta' : 'clique para abrir' }}
        </div>
      </router-link>
      <router-link to="/fiado" class="kpi link-card">
        <div class="kpi-head">A receber (prazo)<span class="kpi-ic" style="--c: #d63c31"><Icon name="t-aprazo" :size="18" /></span></div>
        <div class="kpi-value mono">{{ money(d.creditOpenCents) }}</div>
        <div class="kpi-foot small muted"><span class="badge">{{ d.creditCustomers }} cliente(s)</span></div>
      </router-link>
    </div>

    <div class="dash-row">
      <section class="card">
        <div class="card-head"><strong>Faturamento diário</strong><span class="small muted">últimos 30 dias · <span class="mono">{{ money(daysTotal) }}</span></span></div>
        <svg :viewBox="`0 0 ${W} ${H}`" class="chart" role="img" aria-label="Gráfico de vendas por dia">
          <line x1="0" :x2="W" :y1="BASE" :y2="BASE" class="axis" />
          <g v-for="(b, i) in bars" :key="b.date">
            <rect :x="b.x" :y="b.y" :width="b.w" :height="Math.max(b.h, b.totalCents ? 1 : 0)" rx="3" :class="['bar', { today: b.last }]">
              <title>{{ b.label }}: {{ money(b.totalCents) }} em {{ b.count }} venda(s)</title>
            </rect>
            <text v-if="b.last || (i % 7 === 0 && i < bars.length - 4)" :x="b.x + b.w / 2" :y="H - 4" class="tick" text-anchor="middle">{{ b.last ? 'hoje' : b.label }}</text>
          </g>
        </svg>
      </section>
      <section class="card">
        <div class="card-head"><strong>Atalhos rápidos</strong></div>
        <div class="shortcuts-grid">
          <router-link v-for="s in shortcuts" :key="s.to" :to="s.to" class="shortcut">
            <span class="kpi-ic solid" :style="{ '--c': s.color }"><Icon :name="s.icon" :size="17" /></span>{{ s.label }}
          </router-link>
        </div>
        <router-link v-if="d.openQuotes" to="/orcamentos" class="quotes-box">
          <span>{{ d.openQuotes }} orçamento(s) em aberto</span><strong class="mono">{{ money(d.openQuotesCents) }}</strong>
        </router-link>
      </section>
    </div>

    <div class="dash-row">
      <section class="card">
        <div class="card-head"><strong>Mais vendidos no mês</strong><router-link to="/relatorios/produtos" class="small">Ver relatório</router-link></div>
        <div v-if="!top.length" class="state">Nenhuma venda neste mês.</div>
        <table v-else class="top-table">
          <thead><tr><th class="left">Produto</th><th class="num">Qtd.</th><th class="num">Total</th></tr></thead>
          <tbody>
            <tr v-for="(p, i) in top" :key="p.productId">
              <td class="left"><div class="prod"><span class="thumb" :style="{ background: thumbColors[i % thumbColors.length] }">{{ initials(p.name) }}</span>{{ p.name }}</div></td>
              <td class="num mono">{{ p.qty }}</td>
              <td class="num mono"><strong>{{ money(p.totalCents) }}</strong></td>
            </tr>
          </tbody>
        </table>
      </section>
      <section class="card">
        <div class="card-head">
          <strong>Estoque baixo</strong>
          <router-link v-if="alertCount" to="/estoque?view=alerta" class="small">Ver todos ({{ alertCount }})</router-link>
        </div>
        <div v-if="!alerts.length" class="state">Tudo certo com o estoque.</div>
        <ul v-else class="list">
          <li v-for="a in alerts" :key="a.productId">
            <span class="name">{{ a.name }}</span>
            <span class="badge" :class="a.stockQty <= 0 ? 'bad' : 'warn'">{{ a.stockQty <= 0 ? 'sem estoque' : `${a.stockQty} un.` }}</span>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
.dash { display: flex; flex-direction: column; gap: 18px; }
.dash-top { display: flex; align-items: center; gap: 12px; }
.notices { display: flex; flex-wrap: wrap; gap: 10px; flex: 1; }
.notice { display: inline-flex; align-items: center; gap: 6px; padding: 8px 14px; border-radius: 999px; font-size: 13px; font-weight: 600; text-decoration: none; }
.notice.warn { background: var(--warning-soft); color: var(--warning-text); }
.notice.bad { background: var(--danger-soft); color: var(--danger); }
.notice .go { font-weight: 700; text-decoration: underline; color: inherit; padding: 0; }
.kpis { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 18px; }
@media (max-width: 1000px) { .kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.dash-row { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr); gap: 18px; }
@media (max-width: 1000px) { .dash-row { grid-template-columns: minmax(0, 1fr); } }
/* main.css dá margin-top a .card + .card; dentro da grade isso desalinha as colunas. */
.dash .card { margin-top: 0; }
.chart { width: 100%; height: auto; display: block; }
.axis { stroke: var(--border); }
.tick { font-size: 10px; fill: var(--muted); }
.bar { fill: color-mix(in srgb, var(--primary) 22%, transparent); }
.bar.today, .bar:hover { fill: var(--primary); }
.shortcuts-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.shortcut { display: flex; align-items: center; gap: 12px; padding: 12px 14px; border: 1px solid var(--border); border-radius: 14px; color: var(--text); font-weight: 600; text-decoration: none; }
.shortcut:hover { border-color: var(--primary); background: var(--primary-soft); }
.quotes-box { display: flex; justify-content: space-between; align-items: center; margin-top: 14px; padding: 12px 16px; border-radius: 14px; background: var(--surface-2); color: var(--text); text-decoration: none; font-size: 14px; }
.top-table .num { text-align: right; }
.prod { display: flex; align-items: center; gap: 12px; }
.thumb { width: 34px; height: 34px; border-radius: 10px; display: grid; place-items: center; color: #fff; font-size: 12px; font-weight: 700; flex-shrink: 0; }
.list { list-style: none; margin: 0; padding: 0; }
.list li { display: flex; justify-content: space-between; align-items: center; gap: 8px; padding: 10px 0; border-bottom: 1px solid var(--border); }
.list li:last-child { border-bottom: 0; }
.list .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
