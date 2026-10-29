<!-- Caminho: pdv/frontend/src/views/Assinaturas.vue -->
<script setup lang="ts">
// Assinaturas (planos de academia): cada uma com no máximo uma fatura aberta.
// Receber a fatura grava a venda do plano no caixa e gera a próxima
// (data do pagamento + duração do plano). Sub-abas: ativas, a vencer, vencidas,
// inativos (vencida há muito tempo) e canceladas, com filtro por plano e
// lembretes prontos pelo WhatsApp (textos em Configurações). As janelas das
// ações ficam em SubscriptionActions (também usadas na ficha do aluno).
import { computed, onMounted, ref } from 'vue'
import { ListPlanRules, ListProducts, ListSubscriptions } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { describeRules } from '../composables/accessRules'
import Icon from '../components/Icon.vue'
import Modal from '../components/Modal.vue'
import SubscriptionActions from '../components/SubscriptionActions.vue'
import SubscriptionImport from '../components/SubscriptionImport.vue'
import SubscriptionPanel from '../components/SubscriptionPanel.vue'
import { settings } from '../composables/cashState'
import { formatDateTime, money, today } from '../composables/format'
import { sendMessage, type MessageKind } from '../composables/messages'
import { can } from '../composables/session'
import { brDate, planLabel } from '../composables/subscriptions'
import { models, type Product } from '../types'

type Sub = models.Subscription

const subs = ref<Sub[]>([])
const plans = ref<Product[]>([])
const loading = ref(true)
const importOpen = ref(false)
const error = ref('')
const term = ref('')
type View = 'ativas' | 'avencer' | 'vencidas' | 'inativos' | 'pausadas' | 'canceladas'
const view = ref<View>('ativas')
const actions = ref<InstanceType<typeof SubscriptionActions> | null>(null)

// Regras de acesso de cada plano; a assinatura com regras próprias é exceção.
const planRules = ref<Record<number, models.AccessRules>>({})
const rulesLabel = (s: Sub) => {
  const d = describeRules(s.rules ?? planRules.value[s.productId])
  return s.rules ? `${d || 'sem restrição'} (exceção)` : d
}

// "A vencer": vence nos próximos N dias; "inativo": vencida há mais de N dias (Configurações).
const dueLimit = computed(() => today(settings.value?.dueDays ?? 3))
const missingLimit = computed(() => today(-(settings.value?.missingDays ?? 15)))
const due = (s: Sub) => s.openInvoice?.dueDate ?? ''
const views: { key: View; label: string; test: (s: Sub) => boolean; msg?: MessageKind; msgLabel?: string }[] = [
  { key: 'ativas', label: 'Ativas', test: (s) => s.status === 'ativa' && !s.pausedAt },
  { key: 'avencer', label: 'A vencer', test: (s) => s.status === 'ativa' && !s.pausedAt && !s.overdue && !!due(s) && due(s) <= dueLimit.value, msg: 'msgDue', msgLabel: 'Lembrar' },
  { key: 'vencidas', label: 'Vencidas', test: (s) => s.status === 'ativa' && s.overdue, msg: 'msgOverdue', msgLabel: 'Cobrar' },
  { key: 'inativos', label: 'Inativos', test: (s) => s.status === 'ativa' && s.overdue && due(s) < missingLimit.value, msg: 'msgMissing', msgLabel: 'Chamar' },
  { key: 'pausadas', label: 'Pausadas', test: (s) => s.status === 'ativa' && !!s.pausedAt },
  { key: 'canceladas', label: 'Canceladas', test: (s) => s.status === 'cancelada', msg: 'msgWinback', msgLabel: 'Chamar de volta' },
]
const current = computed(() => views.find((v) => v.key === view.value))
// Filtro por plano (0 = todos); vale também para as contagens das abas.
const planFilter = ref(0)
const planOptions = computed(() => [...new Map(subs.value.map((s) => [s.productId, s.planName])).entries()].sort((a, b) => a[1].localeCompare(b[1], 'pt-BR')))
const byPlan = computed(() => subs.value.filter((s) => !planFilter.value || s.productId === planFilter.value))
const count = (v: (typeof views)[number]) => byPlan.value.filter(v.test).length
const filtered = computed(() => {
  const t = term.value.trim().toLowerCase()
  const test = current.value?.test ?? (() => false)
  return byPlan.value.filter((s) => test(s) && (!t || s.customerName.toLowerCase().includes(t) || s.planName.toLowerCase().includes(t)))
})
// Situação na lista: assinatura nova (mensalidade vencendo hoje) = pendente;
// período atual já pago (próxima vence depois de hoje) = paga.
function situation(s: Sub): { label: string; cls: string } {
  if (s.status === 'cancelada') return { label: 'Cancelada', cls: '' }
  if (s.pausedAt) return { label: 'Pausada', cls: 'paused' }
  if (s.overdue) return { label: 'Vencida', cls: 'bad' }
  if (s.openInvoice && s.openInvoice.dueDate <= today()) return { label: 'Pendente de pagamento', cls: 'warn' }
  // Vence nos próximos dias (os mesmos da aba "A vencer"): ainda não foi paga.
  if (s.openInvoice && s.openInvoice.dueDate <= dueLimit.value) return { label: 'A vencer', cls: 'soon' }
  return { label: 'Paga', cls: 'ok' }
}

function message(s: Sub) {
  const kind = current.value?.msg
  if (!kind) return
  sendMessage(kind, s.customerPhone, {
    nome: s.customerName, plano: s.planName, vencimento: s.openInvoice ? brDate(s.openInvoice.dueDate) : '',
    valor: s.openInvoice ? money(s.openInvoice.amountCents) : '',
  })
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    subs.value = await call(ListSubscriptions())
    planRules.value = await call(ListPlanRules())
    plans.value = (await call(ListProducts())).filter((p) => p.planDays && p.active && !p.pricePending)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// Histórico do cliente (clique na linha): assinaturas, mensalidades e ações.
const customerHist = ref<Sub | null>(null)
</script>

<template>
  <section class="card stack">
    <div class="row">
      <input v-model="term" type="search" placeholder="Buscar cliente" aria-label="Buscar cliente" style="max-width: 320px" />
      <select v-model.number="planFilter" aria-label="Filtrar por plano" style="max-width: 240px">
        <option :value="0">Todos os planos</option>
        <option v-for="[id, name] in planOptions" :key="id" :value="id">{{ name }}</option>
      </select>
      <span class="spacer" />
      <button v-if="can('alterar_configuracoes')" @click="importOpen = true">Importar planilha</button>
      <button class="primary" @click="actions?.openCreate()">+ Nova assinatura</button>
    </div>
    <div class="tabs seg" role="tablist" aria-label="Assinaturas">
      <button v-for="v in views" :key="v.key" role="tab" :aria-selected="view === v.key" @click="view = v.key">
        {{ v.label }} <span class="chip-count">{{ count(v) }}</span>
      </button>
    </div>
    <p v-if="view === 'avencer'" class="small muted" style="margin: 0">Mensalidades que vencem nos próximos {{ settings?.dueDays ?? 3 }} dias.</p>
    <p v-else-if="view === 'inativos'" class="small muted" style="margin: 0">Alunos com a mensalidade vencida há mais de {{ settings?.missingDays ?? 15 }} dias.</p>
    <p v-if="current?.msg" class="small muted" style="margin: 0">A mensagem do WhatsApp é editada em Administração › Configurações.</p>


    <div v-if="!loading && !plans.length" class="alert info">
      Nenhum plano cadastrado. Em <strong>Produtos › Serviços</strong>, cadastre o plano (ex.: "Mensal 5x por semana") com a duração.
    </div>
    <div v-if="loading" class="state">Carregando assinaturas…</div>
    <div v-else-if="error" class="alert error">{{ error }} <button class="link" @click="load">Tentar novamente</button></div>
    <div v-else-if="filtered.length === 0" class="state">Nenhuma assinatura aqui.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Cliente</th><th>Plano</th><th>Situação</th><th>Vencimento</th><th class="right">Valor</th><th>Último pagamento</th><th></th></tr></thead>
        <tbody>
          <!-- Clique em qualquer lugar da linha abre o histórico do cliente (os botões de ação não). -->
          <tr v-for="s in filtered" :key="s.id" class="clickable" :class="{ inactive: s.status === 'cancelada' }" title="Ver o histórico do cliente" @click="customerHist = s">
            <td class="left"><strong class="customer-name">{{ s.customerName }}</strong></td>
            <td>{{ s.planName }} <span class="small muted">· {{ planLabel(s.planDays) }}</span><div v-if="rulesLabel(s)" class="small muted">{{ rulesLabel(s) }}</div></td>
            <td><span class="badge" :class="situation(s).cls">{{ situation(s).label }}</span></td>
            <td class="nowrap">{{ s.openInvoice ? brDate(s.openInvoice.dueDate) : '—' }}</td>
            <td class="right mono">{{ s.openInvoice ? money(s.openInvoice.amountCents) : '—' }}</td>
            <td class="nowrap small muted">{{ s.lastPaidAt ? formatDateTime(s.lastPaidAt) : '—' }}</td>
            <td class="right actions-cell wrap" @click.stop>
              <button v-if="current?.msg" class="link icon-btn" style="color: var(--success)" :title="`${current.msgLabel} pelo WhatsApp`" :aria-label="`${current.msgLabel} pelo WhatsApp`" @click="message(s)"><Icon name="whatsapp" /><span>{{ current.msgLabel }}</span></button>
              <button v-if="s.status === 'ativa'" class="link icon-btn" style="color: var(--success)" title="Confere a assinatura e abre a catraca se estiver em dia" aria-label="Liberar catraca" @click="actions?.release(s)"><Icon name="liberar" /><span>Liberar</span></button>
              <button v-if="s.status === 'cancelada'" class="link icon-btn" style="color: var(--success)" :disabled="actions?.reactivating !== 0" title="Volta a assinatura no mesmo plano e valor (ativa o cliente); a mensalidade vence hoje"
                aria-label="Reativar" @click="actions?.reactivate(s)"><Icon name="ativar" /><span>{{ actions?.reactivating === s.id ? 'Reativando…' : 'Reativar' }}</span></button>
              <button v-if="s.openInvoice && !s.pausedAt" class="link icon-btn" title="Receber no caixa a mensalidade em aberto" aria-label="Receber fatura atual" @click="actions?.openPay(s)"><Icon name="receber" /><span>Receber fatura</span></button>
              <button v-if="s.status === 'ativa' && !s.pausedAt" class="link icon-btn" title="Trocar o plano da assinatura" aria-label="Alterar" @click="actions?.openChange(s)"><Icon name="edit" /><span>Alterar</span></button>
              <button v-if="s.status === 'ativa' && !s.pausedAt" class="link icon-btn" :disabled="!actions?.canPause" :title="actions?.canPause ? 'Pausar: os dias pausados não contam' : actions?.noPauseTip"
                aria-label="Pausar" @click="actions?.openPause(s)"><Icon name="t-historico" /><span>Pausar</span></button>
              <button v-if="s.pausedAt" class="link icon-btn" :disabled="!actions?.canPause" :title="actions?.canPause ? 'Retomar: o vencimento anda os dias pausados' : actions?.noPauseTip"
                aria-label="Retomar" style="color: var(--success)" @click="actions?.resume(s)"><Icon name="ativar" /><span>Retomar</span></button>
              <button v-if="s.status === 'ativa'" class="link icon-btn" title="Cancelar a assinatura" aria-label="Cancelar" @click="actions?.askCancel(s)"><Icon name="cancelar" /><span>Cancelar</span></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <Modal v-if="importOpen" title="Importar assinaturas de planilha" wide @close="importOpen = false">
    <SubscriptionImport @done="load" />
  </Modal>
  <SubscriptionActions ref="actions" @changed="load" />
  <Modal v-if="customerHist" :title="`Histórico · ${customerHist.customerName}`" wide @close="customerHist = null">
    <SubscriptionPanel :customer-id="customerHist.customerId" @changed="load" />
    <template #footer><button @click="customerHist = null">Fechar</button></template>
  </Modal>
</template>

<style scoped>
tr.clickable { cursor: pointer; }
.badge.paused { background: #e0e7ff; color: #3730a3; }
.badge.soon { background: #e0f2fe; color: #075985; }
tr.clickable:hover td { background: var(--primary-soft); }
.customer-name { color: var(--primary); font-weight: 600; }
</style>
