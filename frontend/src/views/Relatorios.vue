<!-- Caminho: pdv/frontend/src/views/Relatorios.vue -->
<script setup lang="ts">
// Relatórios › Vendas: um relatório só com o resumo em uma linha, os totais por
// forma de pagamento e por operador lado a lado e a lista de vendas do período.
// Sem a permissão de ver todos, o backend devolve só as vendas do próprio usuário.
import { computed, onMounted, ref, watch } from 'vue'
import { GenerateSalesReport, GetSalesReport, ListSellers } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import ReportSheet from '../components/ReportSheet.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import SellerFilter from '../components/SellerFilter.vue'
import { exportTable, savePdf, usePrint } from '../composables/exportTable'
import { formatDateTime, money, paymentLabels, today } from '../composables/format'
import { directPrint, printReceipt } from '../composables/receipt'
import { currentUser } from '../composables/session'
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

function toggleMethod(m: string) {
  methods.value = methods.value.includes(m) ? methods.value.filter((x) => x !== m) : [...methods.value, m]
}

function exportXlsx() {
  if (!report.value) return
  exportTable('Vendas', `Período: ${period.value}${operator.value ? ` - ${operator.value}` : ''}`,
    [['Nº', 'int'], ['Data / Hora', 'text'], ['Operador', 'text'], ['Cliente', 'text'], ['Itens', 'text'], ['Forma', 'text'], ['Total', 'money'], ['Situação', 'text']],
    sales.value.map((s) => [s.id, formatDateTime(s.createdAt), s.userName ?? '', s.customerName ?? '', s.itemsLabel,
      paymentLabels[s.paymentMethod] ?? s.paymentMethod, s.totalCents, s.status === 'cancelada' ? 'Cancelada' : 'Concluída']))
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
      <button v-for="m in methodKeys" :key="m" type="button" class="chip" :aria-pressed="methods.includes(m)" @click="toggleMethod(m)">{{ paymentLabels[m] }}</button>
    </div>
    <div v-if="error" class="alert error">{{ error }}</div>
  </section>

  <section v-if="report" ref="sheet" class="card" :class="{ 'print-area': printing }" style="margin-top: 12px">
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
          <div v-if="!byPayment.length" class="state">Nenhuma venda.</div>
          <table v-else>
            <tbody><tr v-for="t in byPayment" :key="t.paymentMethod"><td>{{ paymentLabels[t.paymentMethod] ?? t.paymentMethod }}</td><td class="right muted">{{ t.count }}</td><td class="right mono">{{ money(t.totalCents) }}</td></tr></tbody>
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
            <td>{{ paymentLabels[s.paymentMethod] ?? s.paymentMethod }}</td>
            <td class="right mono">{{ money(s.totalCents) }}<span v-if="s.status === 'cancelada'" class="badge bad" style="margin-left: 4px">Cancelada</span></td>
          </tr>
        </tbody>
      </table>
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
@media print {
  .print-only { display: inline; }
  .ellipsis { white-space: normal; }
}
</style>
