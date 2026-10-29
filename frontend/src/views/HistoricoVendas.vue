<!-- Caminho: pdv/frontend/src/views/HistoricoVendas.vue -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ListSalesByPeriod } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { exportTable } from '../composables/exportTable'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import SellerFilter from '../components/SellerFilter.vue'
import { formatDateTime, money, paymentLabels, today } from '../composables/format'
import type { Sale } from '../types'

const start = ref(today())
const end = ref(today())
const sales = ref<Sale[]>([])
function exportSales() {
  exportTable('Histórico de vendas', `${filtered.value.length} venda(s)`, [['Venda', 'int'], ['Data', 'text'], ['Itens', 'text'], ['Cliente', 'text'], ['Operador', 'text'], ['Forma', 'text'], ['Desconto', 'money'], ['Total', 'money'], ['Situação', 'text']],
    filtered.value.map((s) => [s.id, formatDateTime(s.createdAt), s.itemsLabel, s.customerName ?? '', s.userName ?? '', paymentLabels[s.paymentMethod] ?? s.paymentMethod, s.discountCents, s.totalCents, s.status === 'cancelada' ? 'Cancelada' : 'Concluída']))
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
      <p class="muted">{{ filtered.length }} venda(s) · total concluído: <strong class="mono">{{ money(total) }}</strong></p>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Venda</th><th>Data</th><th>Itens</th><th>Cliente</th><th>Operador</th><th>Forma</th><th class="right">Desconto</th><th class="right">Total</th><th>Situação</th></tr></thead>
          <tbody>
            <tr v-for="s in filtered" :key="s.id" :class="{ inactive: s.status === 'cancelada' }">
              <td><button class="link" @click="saleId = s.id">nº {{ s.id }}</button></td>
              <td class="nowrap">{{ formatDateTime(s.createdAt) }}</td>
              <td>{{ s.itemsLabel || '—' }}</td>
              <td class="left">{{ s.customerName ?? '—' }}</td>
              <td class="small">{{ s.userName ?? '—' }}</td>
              <td>{{ paymentLabels[s.paymentMethod] }}</td>
              <td class="right mono">{{ s.discountCents ? money(s.discountCents) : '—' }}</td>
              <td class="right mono">{{ money(s.totalCents) }}</td>
              <td><span class="badge" :class="s.status === 'cancelada' ? 'bad' : 'ok'">{{ s.status === 'cancelada' ? 'Cancelada' : 'Concluída' }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" allow-cancel @close="saleId = null" @changed="load" />
</template>
