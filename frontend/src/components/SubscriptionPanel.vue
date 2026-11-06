<!-- Caminho: catraca/frontend/src/components/SubscriptionPanel.vue -->
<script setup lang="ts">
// Assinaturas de um cliente com as mensalidades de cada uma e as ações (receber,
// alterar, pausar, cancelar, reativar, liberar). Fica na ficha do aluno e no
// histórico da tela Assinaturas. Todos os botões são type="button": a ficha é um formulário.
import { computed, onMounted, ref } from 'vue'
import { ListPlanRules, ListSubscriptionInvoices, ListSubscriptions } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import Icon from './Icon.vue'
import SubscriptionActions from './SubscriptionActions.vue'
import { describeRules } from '../composables/accessRules'
import { formatDateTime, money } from '../composables/format'
import { brDate, exemptLabel, planLabel } from '../composables/subscriptions'
import { models } from '../types'

type Sub = models.Subscription
const props = defineProps<{ customerId: number }>()
const emit = defineEmits<{ changed: [] }>()

const subs = ref<Sub[]>([])
const invoices = ref<Record<number, models.SubscriptionInvoice[]>>({})
const planRules = ref<Record<number, models.AccessRules>>({})
const loading = ref(true)
const error = ref('')
const actions = ref<InstanceType<typeof SubscriptionActions> | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    // Ativas primeiro, depois as canceladas mais recentes.
    subs.value = (await call(ListSubscriptions())).filter((s) => s.customerId === props.customerId)
      .sort((a, b) => Number(a.status !== 'ativa') - Number(b.status !== 'ativa') || b.id - a.id)
    planRules.value = await call(ListPlanRules())
    const inv: Record<number, models.SubscriptionInvoice[]> = {}
    for (const s of subs.value) inv[s.id] = await call(ListSubscriptionInvoices(s.id))
    invoices.value = inv
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
function changed() {
  load()
  emit('changed')
}

const paid = computed(() => Object.values(invoices.value).flat().filter((i) => i.status === 'paga'))
const rulesText = (s: Sub) => {
  const d = describeRules(s.rules ?? planRules.value[s.productId])
  return s.rules ? `${d || 'sem restrição'} (exceção)` : d
}
function badge(s: Sub): { label: string; cls: string } {
  if (s.status === 'cancelada') return { label: 'Cancelada', cls: '' }
  if (s.exempt) return { label: exemptLabel(s), cls: 'ok' }
  if (s.pausedAt) return { label: 'Pausada', cls: 'paused' }
  if (s.overdue) return { label: 'Vencida', cls: 'bad' }
  return { label: 'Ativa', cls: 'ok' }
}
</script>

<template>
  <div class="stack">
    <div class="row between">
      <p class="hist-sum">
        <strong>{{ subs.length }}</strong> assinatura(s) · <strong>{{ paid.length }}</strong> mensalidade(s) paga(s) · total pago
        <strong>{{ money(paid.reduce((a, i) => a + i.amountCents, 0)) }}</strong>
      </p>
      <button type="button" class="primary" @click="actions?.openCreate(customerId)">+ Nova assinatura</button>
    </div>
    <div v-if="loading" class="state">Carregando assinaturas…</div>
    <div v-else-if="error" class="alert error">{{ error }} <button type="button" class="link" @click="load">Tentar novamente</button></div>
    <div v-else-if="!subs.length" class="state">Nenhuma assinatura. Use "+ Nova assinatura".</div>
    <div v-for="x in subs" :key="x.id" class="hist-card">
      <div class="row between">
        <strong>{{ x.planName }} <span class="small muted">· {{ planLabel(x.planDays) }}</span></strong>
        <span class="badge" :class="badge(x).cls">{{ badge(x).label }}</span>
      </div>
      <div class="small muted">
        Desde {{ formatDateTime(x.createdAt) }}
        <template v-if="x.exempt"> · isenção: {{ x.exemptReason }}</template>
        <template v-else-if="x.openInvoice"> · mensalidade {{ money(x.openInvoice.amountCents) }}, vence {{ brDate(x.openInvoice.dueDate) }}</template>
        <template v-if="x.cancelledAt"> · cancelada em {{ formatDateTime(x.cancelledAt) }}</template>
        <template v-if="x.pausedAt"> · pausada desde {{ formatDateTime(x.pausedAt) }}<template v-if="x.pauseReason"> ({{ x.pauseReason }})</template></template>
      </div>
      <div v-if="rulesText(x)" class="small muted">Acesso: {{ rulesText(x) }}</div>
      <div class="row actions-cell wrap" style="justify-content: flex-start">
        <button v-if="x.openInvoice && !x.pausedAt && !x.exempt" type="button" class="link icon-btn" title="Receber no caixa a mensalidade em aberto" @click="actions?.openPay(x)"><Icon name="receber" /><span>Receber</span></button>
        <button v-if="x.exempt && actions?.canExempt" type="button" class="link icon-btn" title="Encerrar a isenção: a mensalidade passa a vencer hoje" @click="actions?.askEndExempt(x)"><Icon name="cancelar" /><span>Encerrar isenção</span></button>
        <button v-if="x.status === 'ativa'" type="button" class="link icon-btn" style="color: var(--success)" title="Confere a assinatura e abre a catraca se estiver em dia" @click="actions?.release(x)"><Icon name="liberar" /><span>Liberar</span></button>
        <button v-if="x.status === 'ativa' && !x.pausedAt" type="button" class="link icon-btn" title="Trocar o plano, o valor ou as regras de acesso" @click="actions?.openChange(x)"><Icon name="edit" /><span>Alterar</span></button>
        <button v-if="x.status === 'ativa' && !x.pausedAt && !x.exempt" type="button" class="link icon-btn" :disabled="!actions?.canPause" :title="actions?.canPause ? 'Pausar: os dias pausados não contam' : actions?.noPauseTip" @click="actions?.openPause(x)"><Icon name="t-historico" /><span>Pausar</span></button>
        <button v-if="x.pausedAt" type="button" class="link icon-btn" style="color: var(--success)" :disabled="!actions?.canPause" :title="actions?.canPause ? 'Retomar: o vencimento anda os dias pausados' : actions?.noPauseTip" @click="actions?.resume(x)"><Icon name="ativar" /><span>Retomar</span></button>
        <button v-if="x.status === 'cancelada'" type="button" class="link icon-btn" style="color: var(--success)" :disabled="actions?.reactivating !== 0" title="Volta a assinatura no mesmo plano e valor; a mensalidade vence hoje" @click="actions?.reactivate(x)"><Icon name="ativar" /><span>{{ actions?.reactivating === x.id ? 'Reativando…' : 'Reativar' }}</span></button>
        <button v-if="x.status === 'ativa'" type="button" class="link icon-btn" title="Cancelar a assinatura" @click="actions?.askCancel(x)"><Icon name="cancelar" /><span>Cancelar</span></button>
      </div>
      <table>
        <thead><tr><th>Vencimento</th><th class="right">Valor</th><th>Situação</th><th>Pago em</th><th></th></tr></thead>
        <tbody>
          <tr v-for="i in invoices[x.id] ?? []" :key="i.id">
            <td>{{ brDate(i.dueDate) }}</td>
            <td class="right mono">{{ money(i.amountCents) }}</td>
            <td><!-- "Não utilizou": mês sem pagamento no sistema anterior; não é dívida. -->
              <span class="badge" :class="i.status === 'paga' ? 'ok' : i.status === 'nao_paga' ? '' : 'warn'" :title="i.status === 'nao_paga' ? 'Mês sem pagamento: o aluno não usou. Não é cobrado.' : ''">{{ i.status === 'paga' ? 'Paga' : i.status === 'nao_paga' ? 'Não utilizou' : 'Em aberto' }}</span>
              <span v-if="i.outsideCash" class="small muted" title="Lançada como paga fora do caixa"> · fora do caixa</span>
              <span v-else-if="i.imported" class="small muted" title="Veio do sistema anterior"> · sist. anterior</span></td>
            <td class="small muted">{{ i.paidAt ? formatDateTime(i.paidAt) : '—' }}</td>
            <td class="right actions-cell">
              <button v-if="i.outsideCash && actions?.canOutside" type="button" class="link icon-btn" style="color: var(--danger)" title="Desfazer o lançamento fora do caixa: a mensalidade volta a ficar em aberto" @click="actions?.undoOutside(i.id)"><Icon name="devolucao" /><span>Desfazer</span></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <SubscriptionActions ref="actions" @changed="changed" />
  </div>
</template>

<style scoped>
.badge.paused { background: #e0e7ff; color: #3730a3; }
.hist-sum { margin: 0; color: var(--muted); }
.hist-sum strong { color: var(--text); }
.hist-card { display: flex; flex-direction: column; gap: 6px; padding: 12px; border: 1px solid var(--border); border-radius: var(--radius); }
.hist-card th, .hist-card td { text-align: left; }
.hist-card .right { text-align: right; }
</style>
