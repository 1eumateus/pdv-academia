<!-- Caminho: pdv/frontend/src/views/Caixa.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { CloseCash, CloseOtherCash, GetCashDayTotals, GetCashSessionSummary, GetCurrentCash, ListCashSessions, ListOpenCash, OpenCash, RegisterCashMovement } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { can, currentUser } from '../composables/session'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Modal from '../components/Modal.vue'
import ReceiptModal from '../components/ReceiptModal.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import { autoReceipt, refreshCash, settings } from '../composables/cashState'
import type { ReceiptKind } from '../composables/receipt'
import { cashTypeLabels, formatDateTime, money, parseMoney, paymentLabels } from '../composables/format'
import { useToast } from '../composables/toast'
import type { CashSession, CashSummary, models } from '../types'

const toast = useToast()
const summary = ref<CashSummary | null>(null)
const sessions = ref<CashSession[]>([])
const loading = ref(true)
const loadError = ref('')
const busy = ref(false)
const openingText = ref('')
const saleId = ref<number | null>(null)
const pastSummary = ref<CashSummary | null>(null)
const receipt = ref<{ kind: ReceiptKind; id: number } | null>(null)
// Caixa por usuário: caixas abertos dos outros e o total do dia somando todos.
const perUser = computed(() => settings.value?.cashPerUser ?? false)
const othersOpen = ref<CashSession[]>([])
const dayTotals = ref<models.PaymentTotal[] | null>(null)
// Lançamento de venda: mostra o cliente e o que foi vendido ("Suely · Mensal").
function movementSale(m: models.CashMovement): string {
  if (m.referenceType !== 'sale' || !summary.value) return ''
  const sale = summary.value.sales.find((s) => s.id === m.referenceId)
  return sale ? [sale.customerName, sale.itemsLabel].filter(Boolean).join(' · ') : ''
}

function openReceipt(kind: ReceiptKind, id: number) {
  pastSummary.value = null // um modal por vez
  receipt.value = { kind, id }
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    summary.value = await call(GetCurrentCash())
    sessions.value = await call(ListCashSessions())
    othersOpen.value = perUser.value ? (await call(ListOpenCash())).filter((s) => s.ownerUserId !== currentUser.value?.id) : []
    dayTotals.value = perUser.value && can('ver_vendas_de_todos') ? await call(GetCashDayTotals()) : null
  } catch (e) {
    loadError.value = errorMessage(e)
  } finally {
    loading.value = false
    refreshCash()
  }
}
onMounted(load)

async function openCash() {
  const v = openingText.value.trim() === '' ? 0 : parseMoney(openingText.value)
  if (v === null) return toast.error('Saldo inicial inválido.')
  busy.value = true
  try {
    summary.value = await call(OpenCash(v))
    openingText.value = ''
    toast.success('Caixa aberto.')
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}

// ---------- suprimento / sangria ----------
const mov = reactive({ open: false, kind: 'suprimento', amount: '', description: '', busy: false, error: '' })
function openMov(kind: 'suprimento' | 'sangria') {
  Object.assign(mov, { open: true, kind, amount: '', description: '', busy: false, error: '' })
}
async function saveMov() {
  const v = parseMoney(mov.amount)
  if (v === null || v <= 0) return (mov.error = 'Valor inválido.')
  if (!mov.description.trim()) return (mov.error = 'Informe a descrição.')
  mov.busy = true
  mov.error = ''
  try {
    const m = await call( RegisterCashMovement(mov.kind, v, mov.description))
    toast.success(`${cashTypeLabels[mov.kind]} de ${money(v)} registrado.`)
    mov.open = false
    if (autoReceipt.value) openReceipt('movimento', m.id)
    load()
  } catch (e) {
    mov.error = errorMessage(e)
  } finally {
    mov.busy = false
  }
}

// ---------- fechamento ----------
// other: caixa de outro usuário sendo fechado (null = o meu).
const closing = reactive({ open: false, counted: '', notes: '', confirm: false, other: null as CashSummary | null })
const target = computed(() => closing.other ?? summary.value)
const counted = computed(() => parseMoney(closing.counted))
const diff = computed(() => (counted.value === null || !target.value ? null : counted.value - target.value.expectedCashCents))

async function closeOther(id: number) {
  try {
    Object.assign(closing, { open: true, counted: '', notes: '', confirm: false, other: await call(GetCashSessionSummary(id)) })
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function closeCash() {
  if (counted.value === null) return
  busy.value = true
  try {
    const s = await call(closing.other ? CloseOtherCash(closing.other.session.id, counted.value, closing.notes) : CloseCash(counted.value, closing.notes))
    const d = s.session.differenceCents ?? 0
    toast.success(d === 0 ? 'Caixa fechado sem diferença.' : `Caixa fechado com diferença de ${money(d)}.`)
    Object.assign(closing, { open: false, counted: '', notes: '', confirm: false, other: null })
    pastSummary.value = s
    load()
  } catch (e) {
    closing.confirm = false
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}

async function showSession(id: number) {
  try {
    pastSummary.value = await call(GetCashSessionSummary(id))
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div v-if="loading && !summary" class="state">Carregando caixa…</div>
  <div v-else-if="loadError" class="alert error">{{ loadError }} <button class="link" @click="load">Tentar novamente</button></div>

  <section v-else-if="!summary" class="card stack" style="max-width: 460px">
    <h2>{{ perUser ? 'Abrir meu caixa' : 'Abrir caixa' }}</h2>
    <p v-if="perUser" class="muted">Cada usuário tem o próprio caixa: as suas vendas entram só no seu.</p>
    <p class="muted">Informe o dinheiro disponível na gaveta para troco. Vendas, fechamento de comandas e recebimentos exigem caixa aberto.</p>
    <form class="stack" @submit.prevent="openCash">
      <label class="field">Saldo inicial em dinheiro (R$)<input v-model="openingText" inputmode="decimal" placeholder="0,00" /></label>
      <button class="primary big" type="submit" :disabled="busy">{{ busy ? 'Abrindo…' : 'Abrir caixa' }}</button>
    </form>
  </section>

  <template v-else>
    <div class="grid grid-4">
      <div class="stat"><div class="label">Aberto em</div><div class="value" style="font-size: 17px">{{ formatDateTime(summary.session.openedAt) }}</div></div>
      <div class="stat"><div class="label">Saldo inicial</div><div class="value mono">{{ money(summary.session.openingCents) }}</div></div>
      <div class="stat"><div class="label">Vendas na sessão</div><div class="value mono">{{ summary.sales.filter((s) => s.status === 'concluida').length }}</div></div>
      <div class="stat"><div class="label">Dinheiro esperado na gaveta</div><div class="value mono">{{ money(summary.expectedCashCents) }}</div></div>
    </div>
    <section class="card stack" style="margin-top: 16px">
      <div class="row">
        <button @click="openMov('suprimento')">Suprimento (entrada)</button>
        <button v-if="can('sangria')" @click="openMov('sangria')">Sangria (saída)</button>
        <span class="spacer" />
        <button @click="openReceipt('caixa', summary.session.id)">Resumo parcial</button>
        <button class="primary" @click="Object.assign(closing, { open: true, other: null })">Fechar caixa</button>
      </div>
      <div class="grid grid-2">
        <div>
          <h3>Resumo por forma de pagamento</h3>
          <div v-if="summary.byPaymentMethod.length === 0" class="state">Nenhuma venda ou recebimento ainda.</div>
          <table v-else>
            <thead><tr><th>Forma</th><th class="right">Lançamentos</th><th class="right">Total</th></tr></thead>
            <tbody>
              <tr v-for="t in summary.byPaymentMethod" :key="t.paymentMethod">
                <td>{{ paymentLabels[t.paymentMethod] }}</td>
                <td class="right mono">{{ t.count }}</td>
                <td class="right mono">{{ money(t.totalCents) }}</td>
              </tr>
              <tr><td><strong>Total</strong></td><td /><td class="right mono"><strong>{{ money(summary.byPaymentMethod.reduce((a, t) => a + t.totalCents, 0)) }}</strong></td></tr>
            </tbody>
          </table>
          <p class="small muted">Inclui vendas, recebimentos a prazo e estornos. Vendas a prazo não entram no caixa.</p>
        </div>
        <div>
          <h3>Movimentações</h3>
          <div v-if="summary.movements.length === 0" class="state">Nenhuma movimentação.</div>
          <div v-else class="table-wrap" style="max-height: 320px; overflow: auto">
            <table>
              <thead><tr><th>Hora</th><th>Tipo</th><th>Forma</th><th class="right">Valor</th><th></th></tr></thead>
              <tbody>
                <tr v-for="m in summary.movements" :key="m.id">
                  <td class="nowrap">{{ formatDateTime(m.createdAt) }}</td>
                  <td>{{ cashTypeLabels[m.type] }}<div v-if="m.description || movementSale(m)" class="small muted">{{ m.description || movementSale(m) }}</div></td>
                  <td>{{ paymentLabels[m.paymentMethod] }}</td>
                  <td class="right mono" :style="{ color: m.amountCents < 0 ? 'var(--danger)' : '' }">{{ money(m.amountCents) }}</td>
                  <td class="right">
                    <button class="link icon-btn" v-if="m.type === 'sangria' || m.type === 'suprimento'" @click="openReceipt('movimento', m.id)" title="Comprovante" aria-label="Comprovante"><Icon name="recibo" /><span>Comprovante</span></button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
      <h3>Vendas da sessão</h3>
      <div v-if="summary.sales.length === 0" class="state">Nenhuma venda nesta sessão.</div>
      <div v-else class="table-wrap">
        <table>
          <thead><tr><th>Venda</th><th>Hora</th><th>Itens</th><th>Forma</th><th class="right">Total</th><th></th></tr></thead>
          <tbody>
            <tr v-for="s in summary.sales" :key="s.id" :class="{ inactive: s.status === 'cancelada' }">
              <td><button class="link" @click="saleId = s.id">nº {{ s.id }}</button></td>
              <td class="nowrap">{{ formatDateTime(s.createdAt) }}</td>
              <td>{{ s.itemsLabel || '—' }}<span v-if="s.customerName" class="muted"> · {{ s.customerName }}</span></td>
              <td>{{ paymentLabels[s.paymentMethod] }}</td>
              <td class="right mono">{{ money(s.totalCents) }}</td>
              <td><span v-if="s.status === 'cancelada'" class="badge bad">Cancelada</span></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </template>

  <section v-if="dayTotals" class="card stack" style="margin-top: 16px">
    <h2>Hoje, todos os caixas</h2>
    <div v-if="dayTotals.length === 0" class="state">Nada entrou nos caixas hoje.</div>
    <table v-else>
      <thead><tr><th>Forma</th><th class="right">Lançamentos</th><th class="right">Total</th></tr></thead>
      <tbody>
        <tr v-for="t in dayTotals" :key="t.paymentMethod">
          <td>{{ paymentLabels[t.paymentMethod] }}</td>
          <td class="right mono">{{ t.count }}</td>
          <td class="right mono">{{ money(t.totalCents) }}</td>
        </tr>
        <tr><td><strong>Total</strong></td><td /><td class="right mono"><strong>{{ money(dayTotals.reduce((a, t) => a + t.totalCents, 0)) }}</strong></td></tr>
      </tbody>
    </table>
  </section>

  <section v-if="othersOpen.length" class="card stack" style="margin-top: 16px">
    <h2>Caixas abertos de outros usuários</h2>
    <div class="table-wrap">
      <table>
        <thead><tr><th>Usuário</th><th>Aberto em</th><th class="right">Saldo inicial</th><th></th></tr></thead>
        <tbody>
          <tr v-for="s in othersOpen" :key="s.id">
            <td>{{ s.openedByName ?? '—' }}</td>
            <td class="nowrap">{{ formatDateTime(s.openedAt) }}</td>
            <td class="right mono">{{ money(s.openingCents) }}</td>
            <td class="right actions-cell">
              <button class="link icon-btn" @click="showSession(s.id)" title="Detalhes" aria-label="Detalhes"><Icon name="detalhes" /><span>Detalhes</span></button>
              <button v-if="can('fechar_caixa_outro')" class="link icon-btn" @click="closeOther(s.id)" title="Fechar o caixa deste usuário" aria-label="Fechar caixa"><Icon name="cancelar" /><span>Fechar</span></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <section class="card stack" style="margin-top: 16px">
    <h2>Histórico de sessões</h2>
    <div v-if="sessions.length === 0" class="state">Nenhuma sessão registrada.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Operador</th><th>Abertura</th><th>Fechamento</th><th class="right">Esperado</th><th class="right">Contado</th><th class="right">Diferença</th><th></th></tr></thead>
        <tbody>
          <tr v-for="s in sessions" :key="s.id">
            <td>{{ s.openedByName ?? '—' }}</td>
            <td class="nowrap">{{ formatDateTime(s.openedAt) }}</td>
            <td class="nowrap">{{ s.closedAt ? formatDateTime(s.closedAt) : 'Aberta' }}</td>
            <td class="right mono">{{ s.expectedCents != null ? money(s.expectedCents) : '—' }}</td>
            <td class="right mono">{{ s.countedCents != null ? money(s.countedCents) : '—' }}</td>
            <td class="right mono">
              <span v-if="s.differenceCents != null" :class="s.differenceCents === 0 ? 'badge ok' : 'badge bad'">{{ money(s.differenceCents) }}</span>
              <span v-else>—</span>
            </td>
            <td class="right actions-cell"><button class="link icon-btn" @click="showSession(s.id)" title="Detalhes" aria-label="Detalhes"><Icon name="detalhes" /><span>Detalhes</span></button></td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <Modal v-if="mov.open" :title="mov.kind === 'suprimento' ? 'Suprimento de caixa' : 'Sangria de caixa'" :busy="mov.busy" @close="mov.open = false">
    <form class="stack" @submit.prevent="saveMov">
      <label class="field">Valor em dinheiro (R$)<input v-model="mov.amount" inputmode="decimal" required /></label>
      <label class="field">Descrição<input v-model="mov.description" maxlength="500" required /></label>
      <div v-if="mov.error" class="alert error" role="alert">{{ mov.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="mov.busy" @click="mov.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="mov.busy">Registrar</button>
      </footer>
    </form>
  </Modal>

  <Modal v-if="closing.open && target" :title="closing.other ? `Fechar o caixa de ${closing.other.session.openedByName ?? 'outro usuário'}` : 'Fechar caixa'" :busy="busy" @close="closing.open = false">
    <form class="stack" @submit.prevent="counted !== null && (closing.confirm = true)">
      <p>Dinheiro esperado na gaveta: <strong class="mono">{{ money(target.expectedCashCents) }}</strong></p>
      <label class="field">Valor contado (R$)<input v-model="closing.counted" inputmode="decimal" required :aria-invalid="closing.counted !== '' && counted === null" /></label>
      <p v-if="diff !== null" :class="diff === 0 ? 'alert success' : 'alert warn'">
        {{ diff === 0 ? 'Sem diferença.' : diff > 0 ? `Sobra de ${money(diff)}.` : `Falta de ${money(-diff)}.` }}
      </p>
      <label class="field">Observações<textarea v-model="closing.notes" maxlength="500" /></label>
      <footer class="row end">
        <button type="button" @click="closing.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="counted === null">Fechar caixa</button>
      </footer>
    </form>
  </Modal>
  <ConfirmDialog
    v-if="closing.confirm"
    title="Confirmar fechamento"
    :message="`O caixa será fechado com ${money(counted ?? 0)} contados. Depois de fechado, não é possível alterar a sessão.`"
    confirm-label="Fechar caixa"
    :busy="busy"
    @confirm="closeCash"
    @cancel="closing.confirm = false"
  />

  <Modal v-if="pastSummary" title="Sessão de caixa" wide @close="pastSummary = null">
    <div class="grid grid-2">
      <div class="stack small">
        <div>Abertura: {{ formatDateTime(pastSummary.session.openedAt) }}</div>
        <div>Fechamento: {{ pastSummary.session.closedAt ? formatDateTime(pastSummary.session.closedAt) : 'em aberto' }}</div>
        <div>Saldo inicial: {{ money(pastSummary.session.openingCents) }}</div>
        <div>Esperado: {{ money(pastSummary.session.expectedCents ?? pastSummary.expectedCashCents) }}</div>
        <div v-if="pastSummary.session.countedCents != null">Contado: {{ money(pastSummary.session.countedCents) }}</div>
        <div v-if="pastSummary.session.differenceCents != null"><strong>Diferença: {{ money(pastSummary.session.differenceCents) }}</strong></div>
        <div v-if="pastSummary.session.notes">Observações: {{ pastSummary.session.notes }}</div>
      </div>
      <table>
        <thead><tr><th>Forma</th><th class="right">Total</th></tr></thead>
        <tbody>
          <tr v-for="t in pastSummary.byPaymentMethod" :key="t.paymentMethod"><td>{{ paymentLabels[t.paymentMethod] }}</td><td class="right mono">{{ money(t.totalCents) }}</td></tr>
        </tbody>
      </table>
    </div>
    <template #footer>
      <button class="primary" @click="openReceipt('caixa', pastSummary.session.id)">Comprovante</button>
      <button @click="pastSummary = null">Fechar</button>
    </template>
  </Modal>
  <ReceiptModal v-if="receipt" :kind="receipt.kind" :id="receipt.id" @close="receipt = null" />
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" allow-cancel @close="saleId = null" @changed="load" />
</template>
