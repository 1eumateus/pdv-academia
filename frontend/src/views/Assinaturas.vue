<!-- Caminho: pdv/frontend/src/views/Assinaturas.vue -->
<script setup lang="ts">
// Mensalidades e cobrança (dentro de Alunos e rostos): só listas para falar
// com o aluno pelo WhatsApp. Vencimentos: mensalidades que vencem (ou
// venceram) no período escolhido; abre em hoje + 3 dias. Pausadas: quem está
// com a assinatura pausada. Aniversariantes: alunos que fazem aniversário
// (parabéns). Mexer na assinatura (receber, alterar, pausar, cancelar…) é na
// ficha do aluno; o clique na linha abre o histórico dele. Textos das
// mensagens em Configurações.
import { computed, onMounted, ref } from 'vue'
import { ListCustomers, ListPlanRules, ListProducts, ListSubscriptions } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { describeRules } from '../composables/accessRules'
import Icon from '../components/Icon.vue'
import Modal from '../components/Modal.vue'
import SubscriptionPanel from '../components/SubscriptionPanel.vue'
import { formatDateTime, money, today } from '../composables/format'
import { birthdayInfo, sendMessage } from '../composables/messages'
import { openWhatsapp } from '../composables/charge'
import { settings } from '../composables/cashState'
import { brDate, planLabel } from '../composables/subscriptions'
import { models, type Customer, type Product } from '../types'
import LoadMore from '../components/LoadMore.vue'
import { usePaged } from '../composables/paged'

type Sub = models.Subscription

const subs = ref<Sub[]>([])
const customers = ref<Customer[]>([])
const plans = ref<Product[]>([])
const loading = ref(true)
const error = ref('')
const term = ref('')
type View = 'vencimentos' | 'pausadas' | 'aniversariantes'
const view = ref<View>('vencimentos')

// Vencimentos: período pelo vencimento da mensalidade em aberto. Padrão: hoje
// até daqui a 3 dias (Configurações › dias do "a vencer"). "De" vazio = desde sempre.
const aheadDays = computed(() => settings.value?.dueDays ?? 3)
const from = ref(today())
const to = ref(today(aheadDays.value))
type Quick = 'proximos' | 'hoje' | 'atraso'
function quick(q: Quick) {
  if (q === 'proximos') [from.value, to.value] = [today(), today(aheadDays.value)]
  else if (q === 'hoje') [from.value, to.value] = [today(), today()]
  else [from.value, to.value] = ['', today(-1)]
}
const isQuick = (q: Quick) =>
  q === 'proximos' ? from.value === today() && to.value === today(aheadDays.value)
  : q === 'hoje' ? from.value === today() && to.value === today()
  : from.value === '' && to.value === today(-1)
const quickOptions = computed<{ key: Quick; label: string }[]>(() => [
  { key: 'proximos', label: `Próximos ${aheadDays.value} dias` },
  { key: 'hoje', label: 'Vencimento hoje' },
  { key: 'atraso', label: 'Em atraso' },
])

// Regras de acesso de cada plano; a assinatura com regras próprias é exceção.
const planRules = ref<Record<number, models.AccessRules>>({})
const rulesLabel = (s: Sub) => {
  const d = describeRules(s.rules ?? planRules.value[s.productId])
  return s.rules ? `${d || 'sem restrição'} (exceção)` : d
}

const due = (s: Sub) => s.openInvoice?.dueDate ?? ''
const subTests: Record<'vencimentos' | 'pausadas', (s: Sub) => boolean> = {
  vencimentos: (s) => s.status === 'ativa' && !s.pausedAt && !s.exempt && !!due(s)
    && (!from.value || due(s) >= from.value) && (!to.value || due(s) <= to.value),
  pausadas: (s) => s.status === 'ativa' && !!s.pausedAt,
}
// Filtro por plano (0 = todos); vale também para as contagens das abas.
const planFilter = ref(0)
const planOptions = computed(() => [...new Map(subs.value.map((s) => [s.productId, s.planName])).entries()].sort((a, b) => a[1].localeCompare(b[1], 'pt-BR')))
const byPlan = computed(() => subs.value.filter((s) => !planFilter.value || s.productId === planFilter.value))
const t = computed(() => term.value.trim().toLowerCase())
const filtered = computed(() => {
  if (view.value === 'aniversariantes') return []
  const test = subTests[view.value]
  const out = byPlan.value.filter((s) => test(s) && (!t.value || s.customerName.toLowerCase().includes(t.value) || s.planName.toLowerCase().includes(t.value)))
  return view.value === 'vencimentos' ? out.sort((a, b) => due(a).localeCompare(due(b)) || a.customerName.localeCompare(b.customerName, 'pt-BR')) : out
})

// Resumo do período (aba Vencimentos): quantidade e total em aberto.
const totalCents = computed(() => filtered.value.reduce((n, s) => n + (s.openInvoice?.amountCents ?? 0), 0))

// Situação do vencimento: em atraso, vence hoje ou vence em N dias.
function dueInfo(s: Sub): { label: string; cls: string } {
  const days = Math.round((new Date(due(s) + 'T12:00').getTime() - new Date(today() + 'T12:00').getTime()) / 86_400_000)
  if (days < 0) return { label: `Em atraso · ${-days} dia(s)`, cls: 'bad' }
  if (days === 0) return { label: 'Vence hoje', cls: 'warn' }
  return { label: `Vence em ${days} dia(s)`, cls: 'soon' }
}

// Lembrete pelo WhatsApp: a mensagem de "vencida" para atrasada, a de "a vencer" para hoje em diante.
function remind(s: Sub) {
  sendMessage(due(s) < today() ? 'msgOverdue' : 'msgDue', s.customerPhone, {
    nome: s.customerName, plano: s.planName, vencimento: s.openInvoice ? brDate(s.openInvoice.dueDate) : '',
    valor: s.openInvoice ? money(s.openInvoice.amountCents) : '',
  })
}
const firstName = (n: string) => n.trim().split(/\s+/)[0] ?? ''

// ---------- aniversariantes: só alunos (quem já teve assinatura) ----------
const period = ref<'hoje' | 'semana' | 'mes'>('hoje')
// Uma assinatura por aluno para mostrar o plano (a ativa, se houver).
const students = computed(() => {
  const out = new Map<number, Sub>()
  for (const s of subs.value) if (!out.has(s.customerId) || s.status === 'ativa') out.set(s.customerId, s)
  return out
})
const birthdays = computed(() => {
  const now = new Date()
  return customers.value
    .filter((c) => c.active && c.birthDate && students.value.has(c.id) && (!t.value || c.name.toLowerCase().includes(t.value)))
    .map((c) => ({ c, sub: students.value.get(c.id)!, ...birthdayInfo(c.birthDate!, now), month: Number(c.birthDate!.slice(5, 7)) }))
    .filter((b) => (period.value === 'hoje' ? b.days === 0 : period.value === 'semana' ? b.days < 7 : b.month === now.getMonth() + 1))
    .sort((a, b) => (period.value === 'mes' ? Number(a.c.birthDate!.slice(8)) - Number(b.c.birthDate!.slice(8)) : a.days - b.days))
})
const ddmm = (d: string) => `${d.slice(8, 10)}/${d.slice(5, 7)}`

const counts = computed(() => ({
  vencimentos: byPlan.value.filter(subTests.vencimentos).length,
  pausadas: byPlan.value.filter(subTests.pausadas).length,
  aniversariantes: birthdays.value.length,
}))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [s, r, p, c] = await Promise.all([call(ListSubscriptions()), call(ListPlanRules()), call(ListProducts()), call(ListCustomers(''))])
    subs.value = s
    planRules.value = r
    plans.value = p.filter((x) => x.planDays && x.active && !x.pricePending)
    customers.value = c
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// Histórico do aluno (clique na linha): assinaturas, mensalidades e ações.
const customerHist = ref<{ id: number; name: string } | null>(null)
// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgMensalidades = usePaged(() => filtered.value, [term, view, from, to, planFilter])
// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgAniversarios = usePaged(() => birthdays.value, [term, period])
</script>

<template>
  <section class="card stack">
    <div class="row">
      <input v-model="term" type="search" placeholder="Buscar aluno" aria-label="Buscar aluno" style="max-width: 320px" />
      <select v-if="view !== 'aniversariantes'" v-model.number="planFilter" aria-label="Filtrar por plano" style="max-width: 240px">
        <option :value="0">Todos os planos</option>
        <option v-for="[id, name] in planOptions" :key="id" :value="id">{{ name }}</option>
      </select>
    </div>
    <div class="tabs seg" role="tablist" aria-label="Mensalidades e cobrança">
      <button role="tab" :aria-selected="view === 'vencimentos'" @click="view = 'vencimentos'">Vencimentos <span class="chip-count">{{ counts.vencimentos }}</span></button>
      <button role="tab" :aria-selected="view === 'pausadas'" @click="view = 'pausadas'">Pausadas <span class="chip-count">{{ counts.pausadas }}</span></button>
      <button role="tab" :aria-selected="view === 'aniversariantes'" @click="view = 'aniversariantes'">Aniversariantes <span class="chip-count">{{ counts.aniversariantes }}</span></button>
    </div>
    <p class="small muted" style="margin: 0">
      <template v-if="view === 'vencimentos'">Mensalidades em aberto pelo vencimento, no período escolhido. </template>
      <template v-else-if="view === 'aniversariantes'">Alunos com data de nascimento no cadastro. </template>
      Para receber, alterar ou cancelar, abra a ficha do aluno. As mensagens do WhatsApp são editadas em Administração › Configurações.
    </p>

    <div v-if="!loading && !plans.length" class="alert info">
      Nenhum plano cadastrado. Em <strong>Produtos › Serviços</strong>, cadastre o plano (ex.: "Mensal 5x por semana") com a duração.
    </div>
    <div v-if="loading" class="state">Carregando…</div>
    <div v-else-if="error" class="alert error">{{ error }} <button class="link" @click="load">Tentar novamente</button></div>

    <!-- Aniversariantes -->
    <template v-else-if="view === 'aniversariantes'">
      <div class="filters">
        <div class="field small">
          <span>Período</span>
          <div class="chip-track" role="group" aria-label="Período">
            <button :aria-pressed="period === 'hoje'" @click="period = 'hoje'">Hoje</button>
            <button :aria-pressed="period === 'semana'" @click="period = 'semana'">Próximos 7 dias</button>
            <button :aria-pressed="period === 'mes'" @click="period = 'mes'">Este mês</button>
          </div>
        </div>
      </div>
      <div v-if="!birthdays.length" class="state">Nenhum aniversariante {{ period === 'hoje' ? 'hoje' : period === 'semana' ? 'nos próximos 7 dias' : 'este mês' }}. A data de nascimento fica no cadastro do aluno.</div>
      <div v-else class="table-wrap">
        <table>
          <thead><tr><th class="left">Aluno</th><th>Aniversário</th><th class="right">Faz</th><th class="left">Plano</th><th></th></tr></thead>
          <tbody>
            <tr v-for="b in pgAniversarios.visible" :key="b.c.id" class="clickable" title="Ver o histórico do aluno" @click="customerHist = { id: b.c.id, name: b.c.name }">
              <td class="left"><strong class="customer-name">{{ b.c.name }}</strong></td>
              <td class="nowrap">{{ ddmm(b.c.birthDate!) }} <span v-if="b.days === 0" class="badge ok">hoje</span><span v-else-if="b.days < 7" class="small muted">em {{ b.days }} dia(s)</span></td>
              <td class="right mono">{{ b.age }} anos</td>
              <td class="left small clip">{{ b.sub.planName }}<span v-if="b.sub.status !== 'ativa'" class="muted"> · cancelada</span></td>
              <td class="right actions-cell" @click.stop>
                <button class="link icon-btn" style="color: var(--success)" title="Mandar parabéns pelo WhatsApp" aria-label="Parabéns no WhatsApp"
                  @click="sendMessage('msgBirthday', b.c.phone, { nome: b.c.name })"><Icon name="whatsapp" /><span>Parabéns</span></button>
              </td>
            </tr>
          </tbody>
        </table>
          <LoadMore :paged="pgAniversarios" />
      </div>
    </template>

    <!-- Vencimentos e Pausadas -->
    <template v-else>
    <div v-if="view === 'vencimentos'" class="filters">
      <div class="field small">
        <span>Período</span>
        <div class="chip-track" role="group" aria-label="Período">
          <button v-for="q in quickOptions" :key="q.key" :aria-pressed="isQuick(q.key)" @click="quick(q.key)">{{ q.label }}</button>
        </div>
      </div>
      <label class="field small">De<input v-model="from" type="date" /></label>
      <label class="field small">Até<input v-model="to" type="date" /></label>
      <div v-if="filtered.length" class="summary">
        <strong>{{ filtered.length }}</strong> mensalidade(s) · <strong class="mono">{{ money(totalCents) }}</strong> em aberto
      </div>
    </div>
    <div v-if="filtered.length === 0" class="state">{{ view === 'vencimentos' ? 'Nenhuma mensalidade vence nesse período.' : 'Nenhuma assinatura pausada.' }}</div>
    <div v-else class="table-wrap">
      <table>
        <thead>
          <tr>
            <th class="left">Aluno</th><th class="left">Plano</th>
            <template v-if="view === 'vencimentos'"><th>Vencimento</th><th>Situação</th><th class="right">Valor</th></template>
            <template v-else><th>Pausada desde</th><th class="left">Motivo</th></template>
            <th>Último pagamento</th><th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in pgMensalidades.visible" :key="s.id" class="clickable" title="Ver o histórico do aluno" @click="customerHist = { id: s.customerId, name: s.customerName }">
            <td class="left"><strong class="customer-name">{{ s.customerName }}</strong></td>
            <td class="left clip" :title="rulesLabel(s) ? `Acesso: ${rulesLabel(s)}` : ''">{{ s.planName }} <span class="small muted">· {{ planLabel(s.planDays) }}</span></td>
            <template v-if="view === 'vencimentos'">
              <td class="nowrap">{{ brDate(due(s)) }}</td>
              <td class="nowrap"><span class="badge" :class="dueInfo(s).cls">{{ dueInfo(s).label }}</span></td>
              <td class="right mono">{{ s.openInvoice ? money(s.openInvoice.amountCents) : '—' }}</td>
            </template>
            <template v-else>
              <td class="nowrap small">{{ s.pausedAt ? formatDateTime(s.pausedAt) : '—' }}</td>
              <td class="left small clip">{{ s.pauseReason || '—' }}</td>
            </template>
            <td class="nowrap small muted">{{ s.lastPaidAt ? formatDateTime(s.lastPaidAt) : '—' }}</td>
            <td class="right actions-cell" @click.stop>
              <button v-if="view === 'vencimentos'" class="link icon-btn" style="color: var(--success)" title="Mandar o lembrete do vencimento pelo WhatsApp" aria-label="Lembrar pelo WhatsApp" @click="remind(s)"><Icon name="whatsapp" /><span>Lembrar</span></button>
              <button v-else class="link icon-btn" style="color: var(--success)" title="Abrir a conversa no WhatsApp" aria-label="WhatsApp" @click="openWhatsapp(s.customerPhone ?? '', `Olá, ${firstName(s.customerName)}!`)"><Icon name="whatsapp" /><span>WhatsApp</span></button>
            </td>
          </tr>
        </tbody>
      </table>
          <LoadMore :paged="pgMensalidades" />
    </div>
    </template>
  </section>

  <Modal v-if="customerHist" :title="`Histórico · ${customerHist.name}`" wide @close="customerHist = null">
    <SubscriptionPanel :customer-id="customerHist.id" @changed="load" />
    <template #footer><button @click="customerHist = null">Fechar</button></template>
  </Modal>
</template>

<style scoped>
tr.clickable { cursor: pointer; }
tr.clickable:hover td { background: var(--primary-soft); }
.customer-name { color: var(--primary); font-weight: 600; }
.badge.soon { background: #e0f2fe; color: #075985; }
/* Linha baixa: textos longos numa linha só. */
td.clip { max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
/* Filtros: os botões de período têm a mesma borda e altura dos campos de data. */
.filters { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 12px; }
.filters .field { display: flex; flex-direction: column; gap: 4px; font-size: 13px; font-weight: 600; color: var(--muted); }
.filters input[type='date'] { width: auto; }
.summary { margin-left: auto; padding-bottom: 9px; font-size: 14px; color: var(--muted); }
.summary strong { color: var(--text); }
</style>
