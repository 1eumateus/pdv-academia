<!-- Caminho: pdv/frontend/src/views/Dashboard.vue -->
<script setup lang="ts">
// Tela inicial enxuta: hoje, mês, caixa e fiado; gráfico dos últimos 30 dias;
// os 5 mais vendidos e os 5 alertas de estoque. Tudo vem pronto do GetDashboard.
import { computed, onMounted, ref } from 'vue'
import { GetBackupStatus, GetDashboard, RunBackupNow } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
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
const H = 160
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
</script>

<template>
  <div v-if="error" class="alert error">{{ error }} <button class="link" @click="load">Tentar novamente</button></div>
  <div v-else-if="!d" class="state">Carregando o painel…</div>
  <div v-else class="stack dash">
    <div class="row between">
      <span class="small muted">{{ d.allSales ? 'Todos os operadores' : 'Somente as suas vendas' }}</span>
      <button :disabled="loading" @click="load">{{ loading ? 'Atualizando…' : 'Atualizar' }}</button>
    </div>

    <div v-if="backupWarn" class="stock-alert warn" role="status">
      <strong>Backup:</strong> <span>{{ backupWarn }}</span>
      <span class="spacer" />
      <router-link v-if="noExternal && can('alterar_configuracoes')" to="/configuracoes" class="go">Configurar →</router-link>
      <button v-else-if="!noExternal" :disabled="backupBusy" @click="copyNow">{{ backupBusy ? 'Copiando…' : 'Copiar agora' }}</button>
    </div>
    <router-link v-if="can('contas_a_pagar') && (d.billsDueToday || d.billsOverdue)" to="/financeiro/contas" class="stock-alert" :class="d.billsOverdue ? 'bad' : 'warn'">
      <strong>Contas a pagar:</strong>
      <span v-if="d.billsDueToday">{{ d.billsDueToday }} vence(m) hoje</span>
      <span v-if="d.billsDueToday && d.billsOverdue"> e </span>
      <span v-if="d.billsOverdue">{{ d.billsOverdue }} vencida(s)</span>
      <span class="spacer" />
      <span>Ver contas →</span>
    </router-link>
    <router-link v-if="alertCount" to="/estoque?view=alerta" class="stock-alert" :class="d.zeroStockCount ? 'bad' : 'warn'">
      <strong>Atenção ao estoque:</strong>
      <span v-if="d.zeroStockCount">{{ d.zeroStockCount }} produto(s) sem estoque</span>
      <span v-if="d.zeroStockCount && d.lowStockCount"> e </span>
      <span v-if="d.lowStockCount">{{ d.lowStockCount }} com estoque baixo</span>
      <span class="spacer" />
      <span class="go">Ver produtos →</span>
    </router-link>

    <div class="grid grid-4 kpis">
      <div class="stat">
        <div class="label">Hoje</div>
        <div class="value mono">{{ money(d.today.totalCents) }}</div>
        <div class="small muted">
          {{ d.today.count }} venda(s)
          <span v-if="change(d.today.totalCents, d.yesterday.totalCents) !== null" class="badge"
            :class="change(d.today.totalCents, d.yesterday.totalCents)! >= 0 ? 'ok' : 'bad'"
            title="Comparado com ontem até esta hora">
            {{ change(d.today.totalCents, d.yesterday.totalCents)! > 0 ? '+' : '' }}{{ change(d.today.totalCents, d.yesterday.totalCents) }}%
          </span>
        </div>
      </div>
      <div class="stat">
        <div class="label">Mês</div>
        <div class="value mono">{{ money(d.month.totalCents) }}</div>
        <div class="small muted">
          {{ d.month.count }} venda(s)
          <span v-if="change(d.month.totalCents, d.prevMonth.totalCents) !== null" class="badge"
            :class="change(d.month.totalCents, d.prevMonth.totalCents)! >= 0 ? 'ok' : 'bad'"
            title="Comparado com o mês passado no mesmo ponto">
            {{ change(d.month.totalCents, d.prevMonth.totalCents)! > 0 ? '+' : '' }}{{ change(d.month.totalCents, d.prevMonth.totalCents) }}%
          </span>
        </div>
      </div>
      <router-link to="/caixa" class="stat link-card">
        <div class="label">Caixa</div>
        <div class="value">
          <span class="badge" :class="d.cash ? 'ok' : 'warn'">{{ d.cash ? 'Aberto' : 'Fechado' }}</span>
        </div>
        <div class="small muted">{{ d.cash ? `${money(d.cashExpectedCents)} em dinheiro` : 'Clique para abrir' }}</div>
      </router-link>
      <router-link to="/clientes" class="stat link-card">
        <div class="label">A prazo a receber</div>
        <div class="value mono">{{ money(d.creditOpenCents) }}</div>
        <div class="small muted">{{ d.creditCustomers }} cliente(s)</div>
      </router-link>
    </div>

    <section class="card stack">
      <div class="row between">
        <strong>Últimos 30 dias</strong>
        <span class="small muted mono">{{ money(daysTotal) }}</span>
      </div>
      <svg :viewBox="`0 0 ${W} ${H}`" class="chart" role="img" aria-label="Gráfico de vendas por dia">
        <line x1="0" :x2="W" :y1="BASE" :y2="BASE" class="axis" />
        <g v-for="(b, i) in bars" :key="b.date">
          <rect :x="b.x" :y="b.y" :width="b.w" :height="Math.max(b.h, b.totalCents ? 1 : 0)" rx="2" :class="['bar', { today: b.last }]">
            <title>{{ b.label }}: {{ money(b.totalCents) }} em {{ b.count }} venda(s)</title>
          </rect>
          <text v-if="b.last || (i % 7 === 0 && i < bars.length - 4)":x="b.x + b.w / 2" :y="H - 4" class="tick" text-anchor="middle">{{ b.last ? 'hoje' : b.label }}</text>
        </g>
      </svg>
    </section>

    <div class="grid grid-2">
      <section class="card stack">
        <strong>Mais vendidos no mês</strong>
        <div v-if="!top.length" class="state">Nenhuma venda neste mês.</div>
        <ul v-else class="list">
          <li v-for="p in top" :key="p.productId">
            <span class="name">{{ p.name }}</span>
            <span class="mono muted">{{ p.qty }} un.</span>
          </li>
        </ul>
      </section>

      <section class="card stack">
        <div class="row between">
          <strong>Estoque baixo</strong>
          <router-link v-if="alertCount" to="/estoque?view=alerta" class="small">Ver todos ({{ alertCount }})</router-link>
        </div>
        <div v-if="!alerts.length" class="state">Tudo certo com o estoque.</div>
        <ul v-else class="list">
          <li v-for="a in alerts" :key="a.productId">
            <span class="name">{{ a.name }}</span>
            <span class="badge" :class="a.stockQty <= 0 ? 'bad' : 'warn'">{{ a.stockQty <= 0 ? 'zerado' : `${a.stockQty} un.` }}</span>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
.kpis { grid-template-columns: repeat(4, minmax(0, 1fr)); }
@media (max-width: 900px) { .kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
/* main.css dá margin-top a .card + .card; dentro da grade isso desalinha as colunas. */
.dash .grid > .card { margin-top: 0; }
.stock-alert { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; padding: 12px 16px; border-radius: var(--radius); border: 1px solid; border-left-width: 5px; text-decoration: none; font-size: 15px; }
.stock-alert.bad { background: var(--danger-soft); border-color: var(--danger); color: var(--danger); }
.stock-alert.warn { background: var(--warning-soft); border-color: var(--warning); color: var(--warning-text); }
.stock-alert .go { font-weight: 600; text-decoration: underline; }
.link-card { color: inherit; text-decoration: none; }
.link-card:hover { border-color: var(--primary); }
.chart { width: 100%; height: auto; display: block; }
.axis { stroke: var(--border, #ccc); }
.tick { font-size: 10px; fill: var(--muted, #666); }
.bar { fill: var(--primary); opacity: 0.75; }
.bar.today { opacity: 1; }
.bar:hover { opacity: 1; }
.list { list-style: none; margin: 0; padding: 0; }
.list li { display: flex; justify-content: space-between; align-items: center; gap: 8px; padding: 8px 0; border-bottom: 1px solid var(--border); }
.list li:last-child { border-bottom: 0; }
.list .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
