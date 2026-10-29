<!-- Caminho: pdv/frontend/src/views/Fiado.vue -->
<script setup lang="ts">
// Fiado: quem deve (à esquerda) e as notas do cliente escolhido (cada compra
// no fiado, com o que já foi pago), recebimentos e estornos (à direita).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { GetCustomerHistory, GetFiadoAging, ReverseCreditPayment } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Icon from '../components/Icon.vue'
import ReceiptModal from '../components/ReceiptModal.vue'
import ReceiveCreditModal from '../components/ReceiveCreditModal.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import { autoReceipt } from '../composables/cashState'
import { exportTable } from '../composables/exportTable'
import { chargeByWhatsapp } from '../composables/charge'
import { formatDateTime, money, paymentLabels } from '../composables/format'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import type { CreditPayment, CustomerHistory, models } from '../types'

const toast = useToast()
const route = useRoute()
const debtors = ref<models.AgingCustomer[]>([])
const loading = ref(true)
const error = ref('')
const term = ref('')

const selectedId = ref<number | null>(null)
const history = ref<CustomerHistory | null>(null)
const showPaid = ref(false)
const paying = ref(false)
// Notas marcadas para pagar (vazio = "Receber" quita as mais antigas).
const selected = ref<number[]>([])
function toggle(n: models.FiadoNote) {
  if (!n.saleId || !n.openCents) return
  selected.value = selected.value.includes(n.saleId) ? selected.value.filter((x) => x !== n.saleId) : [...selected.value, n.saleId]
}
const selectedOpen = computed(() => (history.value?.notes ?? []).filter((n) => selected.value.includes(n.saleId)).reduce((a, n) => a + n.openCents, 0))
const saleId = ref<number | null>(null)
const receiptId = ref<number | null>(null)

const statusLabels: Record<string, string> = { aberta: 'Em aberto', parcial: 'Parcial', paga: 'Paga' }
const statusClass: Record<string, string> = { aberta: 'bad', parcial: 'warn', paga: 'ok' }

async function loadDebtors() {
  loading.value = true
  error.value = ''
  try {
    debtors.value = await call(GetFiadoAging())
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

async function select(id: number) {
  if (selectedId.value !== id) selected.value = []
  selectedId.value = id
  try {
    history.value = await call(GetCustomerHistory(id))
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

onMounted(async () => {
  await loadDebtors()
  // Vindo de Clientes ("Fiado" de um cliente) abre direto nele; senão, no primeiro que deve.
  const fromRoute = Number(route.query.cliente)
  if (fromRoute) select(fromRoute)
  else if (debtors.value.length) select(debtors.value[0].customerId)
})
watch(() => route.query.cliente, (v) => v && select(Number(v)))

const fold = (t: string) => t.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
const shownDebtors = computed(() => {
  const t = fold(term.value.trim())
  return t ? debtors.value.filter((d) => fold(`${d.name} ${d.phone ?? ''}`).includes(t)) : debtors.value
})
const totals = computed(() => debtors.value.reduce((a, d) => ({ balance: a.balance + d.balanceCents, over: a.over + d.over90Cents }), { balance: 0, over: 0 }))

const notes = computed(() => {
  const all = [...(history.value?.notes ?? [])].reverse() // mais recentes primeiro
  return showPaid.value ? all : all.filter((n) => n.status !== 'paga')
})
const openCount = computed(() => (history.value?.notes ?? []).filter((n) => n.status !== 'paga').length)
const paidCount = computed(() => (history.value?.notes ?? []).length - openCount.value)

async function refresh() {
  await loadDebtors()
  if (selectedId.value) await select(selectedId.value)
}

function onPaid(p: CreditPayment) {
  paying.value = false
  selected.value = []
  toast.success(`Recebimento de ${money(p.amountCents)} registrado para ${p.customerName}.`)
  if (autoReceipt.value) receiptId.value = p.id
  refresh()
}

// ---------- estorno de recebimento ----------
const reversing = ref<CreditPayment | null>(null)
const reverseBusy = ref(false)
async function reverse(reason: string) {
  if (!reversing.value) return
  reverseBusy.value = true
  try {
    await call(ReverseCreditPayment(reversing.value.id, reason))
    toast.success('Estorno registrado. A dívida do cliente foi restaurada.')
    reversing.value = null
    refresh()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    reverseBusy.value = false
  }
}

function exportNotes() {
  if (!history.value) return
  exportTable(`A prazo — ${history.value.customer.name}`, `Dívida atual: ${money(history.value.customer.creditBalanceCents)}`,
    [['Venda', 'int'], ['Data', 'text'], ['Itens', 'text'], ['Valor', 'money'], ['Pago', 'money'], ['Em aberto', 'money'], ['Situação', 'text']],
    [...history.value.notes].reverse().map((n) => [n.saleId || '', n.saleId ? formatDateTime(n.createdAt) : '', n.description, n.amountCents, n.paidCents, n.openCents, statusLabels[n.status]]))
}
</script>

<template>
  <div class="grid grid-4" style="margin-bottom: 16px">
    <div class="stat"><div class="label">A receber</div><div class="value mono">{{ money(totals.balance) }}</div></div>
    <div class="stat"><div class="label">Clientes devendo</div><div class="value mono">{{ debtors.length }}</div></div>
    <div class="stat"><div class="label">Há mais de 90 dias</div><div class="value mono" :style="{ color: totals.over ? 'var(--danger)' : '' }">{{ money(totals.over) }}</div></div>
  </div>

  <div class="fiado">
    <section class="card stack">
      <strong>Quem deve</strong>
      <input v-model="term" type="search" placeholder="Buscar cliente ou telefone" aria-label="Buscar cliente devedor" />
      <div v-if="error" class="alert error">{{ error }}</div>
      <div v-else-if="loading" class="state">Carregando…</div>
      <div v-else-if="!shownDebtors.length" class="state">{{ term ? 'Nenhum cliente encontrado.' : 'Ninguém devendo.' }}</div>
      <div v-else class="debtors">
        <button v-for="d in shownDebtors" :key="d.customerId" type="button" class="debtor" :class="{ active: d.customerId === selectedId }" @click="select(d.customerId)">
          <span class="name">{{ d.name }}<small v-if="d.phone" class="muted"> · {{ d.phone }}</small></span>
          <strong class="mono">{{ money(d.balanceCents) }}</strong>
          <span class="small" :style="{ color: d.oldestDays > 90 ? 'var(--danger)' : d.oldestDays > 30 ? 'var(--warning)' : '' }">
            nota mais antiga em aberto: {{ d.oldestDays }} dia(s)
          </span>
        </button>
      </div>
    </section>

    <section class="card stack">
      <div v-if="!history" class="state">Escolha um cliente à esquerda para ver as notas a prazo.</div>
      <template v-else>
        <div class="row between">
          <div>
            <h2 style="margin: 0">{{ history.customer.name }}</h2>
            <span class="small muted">{{ history.customer.phone || 'sem telefone' }} · limite {{ money(history.customer.creditLimitCents) }}</span>
          </div>
          <div class="row">
            <span>Dívida: <strong class="mono" style="font-size: 18px">{{ money(history.customer.creditBalanceCents) }}</strong></span>
            <!-- Cobrar em destaque: botão verde com o ícone do WhatsApp. -->
            <button v-if="history.customer.creditBalanceCents > 0" class="success charge-btn" title="Abre o WhatsApp com a mensagem de cobrança (Configurações)"
              @click="chargeByWhatsapp(history.customer.id).then((ok) => { if (ok) select(history!.customer.id) })"><Icon name="whatsapp" /> Cobrar no WhatsApp</button>
            <span v-if="history.customer.lastChargeAt" class="small muted">cobrado em {{ formatDateTime(history.customer.lastChargeAt) }}</span>
          </div>
        </div>

        <div v-if="selected.length" class="alert info row between">
          <span><strong>{{ selected.length }}</strong> nota(s) marcada(s): <strong class="mono">{{ money(selectedOpen) }}</strong> em aberto</span>
          <span class="row">
            <button class="link" @click="selected = []">Desmarcar</button>
            <button class="primary" @click="paying = true">Receber</button>
          </span>
        </div>

        <div class="row between">
          <strong>Notas a prazo <span class="small muted">({{ openCount }} em aberto<template v-if="paidCount">, {{ paidCount }} paga(s)</template>)</span></strong>
          <div class="row">
            <label v-if="paidCount" class="check small"><input v-model="showPaid" type="checkbox" /> Mostrar pagas</label>
            <button type="button" :disabled="!history.notes.length" @click="exportNotes">Exportar Excel</button>
          </div>
        </div>
        <div v-if="!notes.length" class="state">{{ history.notes.length ? 'Todas as notas estão pagas.' : 'Nenhuma compra a prazo.' }}</div>
        <div v-else class="table-wrap">
          <table>
            <thead><tr><th style="width: 32px"></th><th>Nota</th><th>Data</th><th>Itens</th><th class="right">Valor</th><th class="right">Pago</th><th class="right">Em aberto</th><th>Situação</th></tr></thead>
            <tbody>
              <tr v-for="(n, i) in notes" :key="n.saleId || `saldo-${i}`" :class="{ inactive: n.status === 'paga', clickable: n.saleId && n.openCents, picked: selected.includes(n.saleId) }"
                :title="n.saleId && n.openCents ? 'Clique para marcar esta nota para pagar' : ''" @click="toggle(n)">
                <td><input v-if="n.saleId && n.openCents" type="checkbox" :checked="selected.includes(n.saleId)" :aria-label="`Marcar a nota nº ${n.saleId} para pagar`" @click.stop="toggle(n)" /></td>
                <td><button v-if="n.saleId" class="link" title="Ver a venda" @click.stop="saleId = n.saleId">nº {{ n.saleId }}</button><span v-else class="muted">—</span></td>
                <td class="nowrap">{{ n.saleId ? formatDateTime(n.createdAt) : '—' }}</td>
                <td class="small">{{ n.description }}</td>
                <td class="right mono">{{ money(n.amountCents) }}</td>
                <td class="right mono">{{ n.paidCents ? money(n.paidCents) : '—' }}</td>
                <td class="right mono"><strong v-if="n.openCents">{{ money(n.openCents) }}</strong><span v-else class="muted">—</span></td>
                <td class="nowrap">
                  <span class="badge" :class="statusClass[n.status]">{{ statusLabels[n.status] }}</span>
                  <div v-if="n.openCents && n.saleId" class="small muted">há {{ n.daysOpen }} dia(s)</div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="small muted" style="margin: 0">Marque as notas que o cliente vai pagar para aparecer o botão Receber.</p>

        <strong>Recebimentos</strong>
        <div v-if="!history.payments.length" class="state">Nenhum recebimento.</div>
        <div v-else class="table-wrap">
          <table>
            <thead><tr><th>Data</th><th>Forma</th><th class="right">Valor</th><th></th></tr></thead>
            <tbody>
              <tr v-for="p in history.payments" :key="p.id" :class="{ inactive: p.reversed }">
                <td class="nowrap">{{ formatDateTime(p.createdAt) }}</td>
                <td>{{ paymentLabels[p.paymentMethod] }}<div v-if="p.notes" class="small muted">{{ p.notes }}</div></td>
                <td class="right mono">{{ p.kind === 'estorno' ? '-' : '' }}{{ money(p.amountCents) }}</td>
                <td class="right nowrap">
                  <button class="link icon-btn" title="Recibo" aria-label="Recibo" @click="receiptId = p.id"><Icon name="recibo" /><span>Recibo</span></button>
                  <span v-if="p.kind === 'estorno'" class="badge warn">Estorno</span>
                  <span v-else-if="p.reversed" class="badge">Estornado</span>
                  <button v-else-if="can('estornar_fiado')" class="link icon-btn" title="Estornar" aria-label="Estornar" style="color: var(--danger)" @click="reversing = p"><Icon name="devolucao" /><span>Estornar</span></button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </section>
  </div>

  <ReceiveCreditModal v-if="paying && history" :customer="history.customer" :sale-ids="selected"
    :amount-cents="selected.length ? selectedOpen : undefined" @close="paying = false" @paid="onPaid" />
  <ConfirmDialog
    v-if="reversing"
    title="Estornar recebimento"
    :message="`O recebimento de ${money(reversing.amountCents)} continuará no histórico e um estorno será registrado. A dívida do cliente volta a aumentar e a saída é lançada no caixa aberto.`"
    reason-label="Motivo do estorno"
    confirm-label="Registrar estorno"
    danger
    :busy="reverseBusy"
    @confirm="reverse"
    @cancel="reversing = null"
  />
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" @close="saleId = null" />
  <ReceiptModal v-if="receiptId" kind="recebimento" :id="receiptId" @close="receiptId = null" />
</template>

<style scoped>
.charge-btn { display: inline-flex; align-items: center; gap: 6px; }
.fiado { display: grid; grid-template-columns: minmax(260px, 0.75fr) minmax(0, 1.6fr); gap: 16px; align-items: start; }
.fiado > .card { margin-top: 0; }
.debtors { display: flex; flex-direction: column; border: 1px solid var(--border); border-radius: var(--radius); max-height: 60vh; overflow-y: auto; }
.debtor { display: grid; grid-template-columns: 1fr auto; gap: 2px 8px; text-align: left; padding: 9px 12px; border: 0; border-bottom: 1px solid var(--border);
  border-radius: 0; background: var(--surface); cursor: pointer; }
.debtor:hover { background: var(--primary-soft); }
.debtor.active { background: var(--primary-soft); box-shadow: inset 4px 0 0 var(--primary); }
.debtor .name { font-weight: 600; }
.debtor .small { grid-column: 1 / -1; }
tr.picked td { background: var(--primary-soft); }
@media (max-width: 1000px) { .fiado { grid-template-columns: minmax(0, 1fr); } }
</style>
