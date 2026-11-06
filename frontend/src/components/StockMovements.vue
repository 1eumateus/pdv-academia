<!-- Caminho: pdv/frontend/src/components/StockMovements.vue -->
<script setup lang="ts">
// Estoque › Histórico: as movimentações de todos os produtos no período, com
// filtro por tipo (entradas, saídas, devoluções). Ajuste entra em Entradas ou Saídas pelo sinal; cancelamento de
// venda conta como devolução. O valor de custo só vem com a permissão de ver custo.
import { computed, onMounted, ref, watch } from 'vue'
import { ListStockMovements } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { exportTable, savePdf, usePrint } from '../composables/exportTable'
import { formatDateTime, money, stockReasonLabels, today } from '../composables/format'
import type { StockMovement } from '../types'
import ReportSheet from './ReportSheet.vue'

type Kind = '' | 'entradas' | 'saidas' | 'devolucoes'
const kind = ref<Kind>('')
const titles: Record<Kind, string> = { '': 'Histórico do estoque', entradas: 'Entradas de estoque', saidas: 'Saídas de estoque', devolucoes: 'Devoluções ao estoque' }

const start = ref(today().slice(0, 8) + '01') // começo do mês
const end = ref(today())
const term = ref('')
const items = ref<StockMovement[]>([])
const loading = ref(false)
const error = ref('')
const { printing, print } = usePrint()
const sheet = ref<HTMLElement | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = await call(ListStockMovements(0, start.value, end.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
let timer = 0
watch([start, end], () => {
  window.clearTimeout(timer)
  if (start.value && end.value) timer = window.setTimeout(load, 400)
})
onMounted(load)

const isReturn = (m: StockMovement) => m.reason === 'devolucao' || m.reason === 'cancelamento'
const isKind = (m: StockMovement, k: Kind) =>
  !k || (k === 'devolucoes' ? isReturn(m) : !isReturn(m) && (k === 'entradas' ? m.qtyChange > 0 : m.qtyChange < 0))
// Botões iguais aos do Saldo (Todos, Repor estoque…), com a contagem de cada tipo.
const kinds: { key: Kind; label: string }[] = [
  { key: '', label: 'Todos' }, { key: 'entradas', label: 'Entradas' }, { key: 'saidas', label: 'Saídas' }, { key: 'devolucoes', label: 'Devoluções' },
]
const searched = computed(() => {
  const t = term.value.trim().toLowerCase()
  return items.value.filter((m) => !t || m.productName.toLowerCase().includes(t))
})
const countOf = (k: Kind) => searched.value.filter((m) => isKind(m, k)).length
const rows = computed(() => searched.value.filter((m) => isKind(m, kind.value)))
const value = (m: StockMovement) => (m.unitCents == null ? null : m.unitCents * Math.abs(m.qtyChange))
// O rodapé de total só aparece com um tipo escolhido: em "Todos", entrada e saída se misturam.
const totals = computed(() => rows.value.reduce((a, m) => ({ qty: a.qty + Math.abs(m.qtyChange), value: a.value + (value(m) ?? 0) }), { qty: 0, value: 0 }))
const hiddenCost = computed(() => rows.value.some((m) => m.unitCents == null))

const refLabels: Record<string, string> = { sale: 'Venda', purchase: 'Entrada', return: 'Devolução' }
const otherLabels: Record<string, string> = { manual: 'Manual', saldo_inicial: 'Saldo inicial', reconciliacao: 'Conferência de estoque' }
function origin(m: StockMovement) {
  const r = m.referenceType ?? ''
  if (refLabels[r]) return `${refLabels[r]} nº ${m.referenceId}`
  return otherLabels[r] ?? stockReasonLabels[m.reason] ?? m.reason
}

const br = (d: string) => d.split('-').reverse().join('/')
const period = computed(() => br(start.value) + (end.value !== start.value ? ` a ${br(end.value)}` : ''))
function exportXlsx() {
  exportTable(titles[kind.value], `Período: ${period.value}`,
    [['Data / Hora', 'text'], ['Produto', 'text'], ['Tipo', 'text'], ['Origem', 'text'], ['Qtd.', 'int'], ['Valor', 'money'], ['Operador', 'text'], ['Observação', 'text']],
    rows.value.map((m) => [formatDateTime(m.createdAt), m.productName, stockReasonLabels[m.reason] ?? m.reason, origin(m), kind.value ? Math.abs(m.qtyChange) : m.qtyChange, value(m), m.userName ?? '', m.notes ?? '']))
}
</script>

<template>
  <div class="stack">
    <!-- Mesmo formato do Saldo: barra com busca e datas; embaixo, os botões de tipo. -->
    <div class="row products-toolbar no-print">
      <input v-model="term" class="products-search" type="search" placeholder="Buscar produto" aria-label="Buscar produto" />
      <input v-model="start" type="date" required aria-label="De" title="De" style="width: 150px" />
      <input v-model="end" type="date" required aria-label="Até" title="Até" style="width: 150px" />
      <span class="spacer" />
      <button class="ghost" :disabled="!rows.length" @click="exportXlsx">Exportar Excel</button>
      <button class="ghost" :disabled="!rows.length" @click="print">Imprimir A4</button>
      <button class="ghost" :disabled="!rows.length" @click="savePdf(sheet, titles[kind])">Salvar PDF</button>
    </div>
    <div class="row chips no-print" role="group" aria-label="Mostrar">
      <div class="chip-track">
        <button v-for="k in kinds" :key="k.key" class="chip" :aria-pressed="kind === k.key" @click="kind = k.key">
          {{ k.label }} <span class="chip-count">{{ countOf(k.key) }}</span>
        </button>
      </div>
    </div>
    <div v-if="error" class="alert error no-print">{{ error }}</div>
    <div v-else-if="loading && !items.length" class="state">Carregando…</div>
    <div v-else-if="!rows.length" class="state">Nada no período.</div>
    <div v-else ref="sheet" :class="{ 'print-area': printing }">
      <ReportSheet :title="titles[kind]" :period="period">
        <div class="table-wrap">
          <table>
            <thead><tr><th>Data / Hora</th><th class="left">Produto</th><th>Tipo</th><th>Origem</th><th class="right">Qtd.</th><th class="right">Valor</th><th>Operador</th><th>Observação</th></tr></thead>
            <tbody>
              <tr v-for="m in rows" :key="m.id">
                <td class="nowrap">{{ formatDateTime(m.createdAt) }}</td>
                <td class="left">{{ m.productName }}</td>
                <td>{{ stockReasonLabels[m.reason] ?? m.reason }}</td>
                <td class="small">{{ origin(m) }}</td>
                <td class="right mono">{{ kind ? Math.abs(m.qtyChange) : (m.qtyChange > 0 ? '+' : '') + m.qtyChange }}</td>
                <td class="right mono">{{ value(m) == null ? '—' : money(value(m)!) }}</td>
                <td class="small">{{ m.userName ?? '' }}</td>
                <td class="small">{{ m.notes ?? '' }}</td>
              </tr>
            </tbody>
            <tfoot>
              <tr v-if="kind"><th colspan="4" class="right">Total ({{ rows.length }} movimentações)</th><th class="right mono">{{ totals.qty }}</th><th class="right mono">{{ money(totals.value) }}</th><th colspan="2"></th></tr>
            </tfoot>
          </table>
        </div>
        <p v-if="hiddenCost" class="small muted" style="margin: 6px 0 0">— = valor de custo, visível só para quem pode ver custo (fora do total).</p>
      </ReportSheet>
    </div>
  </div>
</template>
