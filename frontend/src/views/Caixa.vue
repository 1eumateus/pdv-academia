<!-- Caminho: pdv/frontend/src/views/Caixa.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { CloseCash, CloseOtherCash, GetCashDayTotals, GetCashSessionSummary, GetCurrentCash, ListCashSessions, ListOpenCash, OpenCash, RegisterCashMovement } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { can, currentUser } from '../composables/session'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import DonutChart from '../components/DonutChart.vue'
import Modal from '../components/Modal.vue'
import ReceiptModal from '../components/ReceiptModal.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import { autoReceipt, refreshCash, settings } from '../composables/cashState'
import type { ReceiptKind } from '../composables/receipt'
import { cashTypeLabels, formatDateTime, money, parseMoney, paymentLabels, saleMethodLabel } from '../composables/format'
import { useToast } from '../composables/toast'
import type { CashSession, CashSummary, models } from '../types'
import LoadMore from '../components/LoadMore.vue'
import { usePaged } from '../composables/paged'

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

// Cor de cada forma de pagamento no resumo (a mesma em todas as sessões).
const payColors: Record<string, string> = { dinheiro: '#10b981', pix: '#5b4cf0', cartao: '#06b6d4', debito: '#06b6d4', credito: '#f59e0b', fiado: '#ef4444' }
const payColor = (method: string) => payColors[method] ?? '#8b5cf6'

async function showSession(id: number) {
  try {
    pastSummary.value = await call(GetCashSessionSummary(id))
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgSessoes = usePaged(() => sessions.value, [])
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
    <div class="cash-top">
      <section class="cash-hero">
        <div>
          <span class="hero-status"><span class="dot" />Caixa aberto</span>
          <div class="hero-label">Dinheiro esperado na gaveta</div>
          <div class="hero-value mono">{{ money(summary.expectedCashCents) }}</div>
          <div class="hero-sub">{{ summary.session.openedByName ?? currentUser?.name }} · aberto em {{ formatDateTime(summary.session.openedAt) }}</div>
        </div>
        <div class="hero-actions">
          <button @click="openMov('suprimento')"><Icon name="entrada" /> Suprimento (entrada)</button>
          <button v-if="can('sangria')" @click="openMov('sangria')"><Icon name="receber" /> Sangria (saída)</button>
          <button @click="openReceipt('caixa', summary.session.id)"><Icon name="imprimir" /> Resumo parcial</button>
          <button class="close" @click="Object.assign(closing, { open: true, other: null })">Fechar caixa</button>
        </div>
      </section>
      <div class="cash-side">
        <div class="kpi">
          <div class="kpi-head">Vendas na sessão<span class="kpi-ic" style="--c: #5b4cf0"><Icon name="m-vendas" :size="18" /></span></div>
          <div class="kpi-value mono">{{ summary.sales.filter((s) => s.status === 'concluida').length }}</div>
          <div class="small muted">{{ summary.sales.filter((s) => s.status === 'cancelada').length }} cancelada(s)</div>
        </div>
        <div class="kpi">
          <div class="kpi-head">Saldo inicial<span class="kpi-ic" style="--c: #0e9466"><Icon name="m-caixa" :size="18" /></span></div>
          <div class="kpi-value mono">{{ money(summary.session.openingCents) }}</div>
          <div class="small muted">informado na abertura</div>
        </div>
      </div>
    </div>
    <div class="cash-row">
      <section class="card">
        <div class="card-head"><strong>Resumo por forma de pagamento</strong>
          <span class="small muted mono">Total {{ money(summary.byPaymentMethod.reduce((a, t) => a + t.totalCents, 0)) }}</span></div>
        <div v-if="summary.byPaymentMethod.length === 0" class="state">Nenhuma venda ou recebimento ainda.</div>
        <DonutChart v-else :items="summary.byPaymentMethod.map((t) => ({ name: paymentLabels[t.paymentMethod] ?? t.paymentMethod, value: t.totalCents, text: money(t.totalCents) }))"
          center-label="lançamentos" :center="String(summary.byPaymentMethod.reduce((a, t) => a + t.count, 0))" style="margin-bottom: 8px" />
        <ul v-if="summary.byPaymentMethod.length" class="pay-list">
          <li v-for="t in summary.byPaymentMethod" :key="t.paymentMethod">
            <span class="kpi-ic solid" :style="{ '--c': payColor(t.paymentMethod) }"><Icon name="receber" :size="16" /></span>
            <span class="pay-name">{{ paymentLabels[t.paymentMethod] }}<small class="muted">{{ t.count }} lançamento(s)</small></span>
            <strong class="mono">{{ money(t.totalCents) }}</strong>
          </li>
        </ul>
        <p class="small muted" style="margin-top: 12px">Inclui vendas, recebimentos a prazo e estornos. Vendas a prazo não entram no caixa.</p>
      </section>
      <section class="card">
        <div class="card-head"><strong>Movimentações</strong></div>
        <div v-if="summary.movements.length === 0" class="state">Nenhuma movimentação.</div>
        <div v-else class="table-wrap" style="max-height: 340px; overflow: auto">
          <table class="left-table">
            <thead><tr><th>Hora</th><th>Tipo</th><th>Forma</th><th class="num">Valor</th><th></th></tr></thead>
            <tbody>
              <tr v-for="m in summary.movements" :key="m.id">
                <td class="nowrap muted">{{ formatDateTime(m.createdAt) }}</td>
                <td><span class="badge" :class="m.amountCents < 0 ? 'bad' : 'ok'">{{ cashTypeLabels[m.type] }}</span><div v-if="m.description || movementSale(m)" class="small muted">{{ m.description || movementSale(m) }}</div></td>
                <td>{{ paymentLabels[m.paymentMethod] }}</td>
                <td class="num mono" :style="{ color: m.amountCents < 0 ? 'var(--danger)' : '' }"><strong>{{ money(m.amountCents) }}</strong></td>
                <td class="right">
                  <button class="link icon-btn" v-if="m.type === 'sangria' || m.type === 'suprimento'" @click="openReceipt('movimento', m.id)" title="Comprovante" aria-label="Comprovante"><Icon name="recibo" /><span>Comprovante</span></button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
    <section class="card stack" style="margin-top: 18px">
      <div class="card-head" style="margin-bottom: 0"><strong>Vendas da sessão</strong></div>
      <div v-if="summary.sales.length === 0" class="state">Nenhuma venda nesta sessão.</div>
      <div v-else class="table-wrap">
        <table>
          <thead><tr><th>Venda</th><th>Hora</th><th>Itens</th><th>Forma</th><th class="right">Total</th><th></th></tr></thead>
          <tbody>
            <tr v-for="s in summary.sales" :key="s.id" :class="{ inactive: s.status === 'cancelada' }">
              <td><button class="link" @click="saleId = s.id">nº {{ s.id }}</button></td>
              <td class="nowrap">{{ formatDateTime(s.createdAt) }}</td>
              <td>{{ s.itemsLabel || '—' }}<span v-if="s.customerName" class="muted"> · {{ s.customerName }}</span></td>
              <td>{{ saleMethodLabel(s) }}</td>
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
          <tr v-for="s in pgSessoes.visible" :key="s.id">
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
          <LoadMore :paged="pgSessoes" />
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

<style scoped>
/* Topo do caixa aberto: o dinheiro da gaveta em destaque e as ações ao lado. */
.cash-top { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(280px, 1fr); gap: 18px; }
.cash-hero { display: flex; align-items: center; gap: 28px; padding: 28px; border-radius: var(--radius); color: #fff;
  background: linear-gradient(135deg, #14122b, #2d2581); }
.hero-status { display: inline-flex; align-items: center; gap: 8px; padding: 4px 12px; border-radius: 999px; font-size: 12.5px; font-weight: 600;
  background: rgba(34, 211, 166, 0.18); color: #5eead4; }
.dot { width: 8px; height: 8px; border-radius: 50%; background: #22d3a6; }
.hero-label { margin-top: 16px; color: #b9b5e6; font-size: 14px; }
.hero-value { font-size: 50px; font-weight: 800; letter-spacing: -0.03em; line-height: 1.1; }
.hero-sub { color: #b9b5e6; font-size: 13px; margin-top: 4px; }
.hero-actions { margin-left: auto; display: flex; flex-direction: column; gap: 10px; width: 240px; }
.hero-actions button { background: rgba(255, 255, 255, 0.1); border-color: rgba(255, 255, 255, 0.16); color: #fff; box-shadow: none; }
.hero-actions button:hover:not(:disabled) { background: rgba(255, 255, 255, 0.18); }
.hero-actions button.close { background: #fff; color: #14122b; border-color: #fff; }
.hero-actions button.close:hover:not(:disabled) { background: #eeecfe; }
.cash-side { display: flex; flex-direction: column; gap: 18px; }
.cash-side .kpi { flex: 1; }
.cash-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.3fr); gap: 18px; margin-top: 18px; }
.cash-row .card { margin-top: 0; }
@media (max-width: 1100px) {
  .cash-top, .cash-row { grid-template-columns: minmax(0, 1fr); }
  .cash-hero { flex-direction: column; align-items: stretch; }
  .hero-actions { margin-left: 0; width: auto; }
}
.pay-list { list-style: none; margin: 0; padding: 0; }
.pay-list li { display: flex; align-items: center; gap: 12px; padding: 12px 0; border-bottom: 1px solid var(--border); }
.pay-list li:last-child { border-bottom: 0; }
.pay-name { flex: 1; display: flex; flex-direction: column; }
</style>
