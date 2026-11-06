<!-- Caminho: pdv/frontend/src/views/HistoricoVendas.vue -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ListSalesByPeriod } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { exportTable } from '../composables/exportTable'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import SellerFilter from '../components/SellerFilter.vue'
import { formatDateTime, money, paymentLabels, saleMethodLabel, today } from '../composables/format'
import type { Sale } from '../types'

const start = ref(today())
const end = ref(today())
const sales = ref<Sale[]>([])
function exportSales() {
  exportTable('Histórico de vendas', `${filtered.value.length} venda(s)`, [['Venda', 'int'], ['Data', 'text'], ['Itens', 'text'], ['Cliente', 'text'], ['Operador', 'text'], ['Forma', 'text'], ['Desconto', 'money'], ['Total', 'money'], ['Situação', 'text']],
    filtered.value.map((s) => [s.id, formatDateTime(s.createdAt), s.itemsLabel, s.customerName ?? '', s.userName ?? '', saleMethodLabel(s), s.discountCents, s.totalCents, s.status === 'cancelada' ? 'Cancelada' : 'Concluída']))
}
const loading = ref(false)
const error = ref('')
const statusFilter = ref<'todas' | 'concluida' | 'cancelada'>('todas')
const saleId = ref<number | null>(null)
const userId = ref(0)
const term = ref('')

// Busca local por nº da venda, produto, cliente ou operador.
const filtered = computed(() => {
  const t = term.value.trim().toLowerCase().replace(/^n[ºo°]?\s*/, '')
  return sales.value.filter((s) => (statusFilter.value === 'todas' || s.status === statusFilter.value) &&
    (!t || String(s.id) === t || s.itemsLabel.toLowerCase().includes(t) || (s.customerName ?? '').toLowerCase().includes(t) || (s.userName ?? '').toLowerCase().includes(t)))
})
const total = computed(() => filtered.value.filter((s) => s.status === 'concluida').reduce((a, s) => a + s.totalCents, 0))
const doneCount = computed(() => filtered.value.filter((s) => s.status === 'concluida').length)
const cancelled = computed(() => filtered.value.filter((s) => s.status === 'cancelada'))
const discounts = computed(() => filtered.value.filter((s) => s.status === 'concluida').reduce((a, s) => a + s.discountCents, 0))
// Cor da etiqueta de cada forma de pagamento.
const payClass: Record<string, string> = { pix: 'acc', dinheiro: 'ok', credito: 'warn', fiado: 'bad' }
// Gráfico: vendas concluídas por hora do dia (somando o período), para ver o movimento.
const hours = computed(() => {
  const h = Array.from({ length: 24 }, (_, i) => ({ hour: i, cents: 0, count: 0 }))
  for (const s of filtered.value) {
    if (s.status !== 'concluida') continue
    const x = h[new Date(s.createdAt).getHours()]
    x.cents += s.totalCents
    x.count++
  }
  const used = h.filter((x) => x.count).map((x) => x.hour)
  const from = Math.min(8, ...used)
  const to = Math.max(20, ...used)
  return h.slice(from, to + 1)
})
const hoursMax = computed(() => Math.max(1, ...hours.value.map((h) => h.cents)))
const peak = computed(() => hours.value.reduce((a, h) => (h.cents > a.cents ? h : a), hours.value[0]))

async function load() {
  loading.value = true
  error.value = ''
  try {
    sales.value = await call(ListSalesByPeriod(start.value, end.value, userId.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <section class="card stack">
    <form class="row" @submit.prevent="load">
      <label class="field">De<input v-model="start" type="date" required /></label>
      <label class="field">Até<input v-model="end" type="date" required /></label>
      <label class="field">
        Situação
        <select v-model="statusFilter">
          <option value="todas">Todas</option>
          <option value="concluida">Concluídas</option>
          <option value="cancelada">Canceladas</option>
        </select>
      </label>
      <SellerFilter v-model="userId" />
      <button class="primary" style="align-self: end" type="submit" :disabled="loading">{{ loading ? 'Buscando…' : 'Buscar' }}</button>
    </form>
    <div class="row">
      <input v-model="term" type="search" placeholder="Filtrar por nº da venda, produto, cliente ou operador" aria-label="Filtrar vendas" style="max-width: 420px" />
      <span class="spacer" />
      <button type="button" :disabled="!filtered.length" @click="exportSales">Exportar Excel</button>
    </div>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="loading && sales.length === 0" class="state">Carregando…</div>
    <div v-else-if="filtered.length === 0" class="state">Nenhuma venda no período.</div>
    <template v-else>
      <div class="sum-row">
        <div><span class="muted small">Vendas no período</span><strong class="mono">{{ doneCount }}</strong></div>
        <div><span class="muted small">Faturamento</span><strong class="mono">{{ money(total) }}</strong></div>
        <div><span class="muted small">Ticket médio</span><strong class="mono">{{ money(doneCount ? Math.round(total / doneCount) : 0) }}</strong></div>
        <div><span class="muted small">Descontos</span><strong class="mono">{{ money(discounts) }}</strong></div>
        <div><span class="muted small">Canceladas</span><strong class="mono">{{ cancelled.length }} · {{ money(cancelled.reduce((a, s) => a + s.totalCents, 0)) }}</strong></div>
      </div>
      <div v-if="doneCount" class="hours">
        <div class="card-head" style="margin-bottom: 8px"><strong>Vendas por hora</strong>
          <span class="small muted">pico às {{ peak.hour }}h · {{ money(peak.cents) }} em {{ peak.count }} venda(s)</span></div>
        <div class="h-cols">
          <div v-for="h in hours" :key="h.hour" class="h-col" :title="`${h.hour}h: ${money(h.cents)} em ${h.count} venda(s)`">
            <span class="h-bar"><i :class="{ peak: h.hour === peak.hour }" :style="{ height: `${(h.cents / hoursMax) * 100}%` }" /></span>
            <span class="h-label">{{ h.hour }}h</span>
          </div>
        </div>
      </div>
      <div class="table-wrap">
        <table class="left-table">
          <thead><tr><th>Venda</th><th>Data</th><th>Itens</th><th>Cliente</th><th>Operador</th><th>Forma</th><th class="num">Desconto</th><th class="num">Total</th><th>Situação</th></tr></thead>
          <tbody>
            <tr v-for="s in filtered" :key="s.id" :class="{ inactive: s.status === 'cancelada' }">
              <td><button class="link mono" @click="saleId = s.id"><strong>nº {{ s.id }}</strong></button></td>
              <td class="nowrap muted">{{ formatDateTime(s.createdAt) }}</td>
              <td class="items">{{ s.itemsLabel || '—' }}</td>
              <td>{{ s.customerName ?? '—' }}</td>
              <td class="small">{{ s.userName ?? '—' }}</td>
              <td><span class="badge" :class="payClass[s.paymentMethod]">{{ saleMethodLabel(s) }}</span></td>
              <td class="num mono">{{ s.discountCents ? money(s.discountCents) : '—' }}</td>
              <td class="num mono"><strong>{{ money(s.totalCents) }}</strong></td>
              <td><span class="badge" :class="s.status === 'cancelada' ? 'bad' : 'ok'">{{ s.status === 'cancelada' ? 'Cancelada' : 'Concluída' }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" allow-cancel @close="saleId = null" @changed="load" />
</template>

<style scoped>
.sum-row { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; }
.sum-row > div { display: flex; flex-direction: column; gap: 2px; padding: 12px 16px; border-radius: 14px; background: var(--surface-2); }
.sum-row strong { font-size: 19px; }
@media (max-width: 1000px) { .sum-row { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.hours { padding: 14px 16px; border: 1px solid var(--border); border-radius: 14px; }
.h-cols { display: flex; gap: 6px; align-items: flex-end; }
.h-col { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 4px; }
.h-bar { width: 100%; height: 90px; display: flex; align-items: flex-end; }
.h-bar i { display: block; width: 100%; min-height: 2px; border-radius: 5px 5px 0 0; background: color-mix(in srgb, var(--primary) 30%, transparent); }
.h-bar i.peak { background: var(--primary); }
.h-label { font-size: 11px; color: var(--muted); }
.items { max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
