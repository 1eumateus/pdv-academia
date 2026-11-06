<!-- Caminho: pdv/frontend/src/views/Clientes.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { DeleteCustomer, GetCustomerHistory, ListCustomers, SetCustomerActive } from '../../wailsjs/go/main/App'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CustomerFormModal from '../components/CustomerFormModal.vue'
import PurgeCustomerDialog from '../components/PurgeCustomerDialog.vue'
import CustomerAvatar from '../components/CustomerAvatar.vue'
import { call, errorMessage } from '../api'
import { exportTable } from '../composables/exportTable'
import { can } from '../composables/session'
import { formatDate, formatDateTime, formatDocument, money, paymentLabels } from '../composables/format'
import { birthdayInfo, sendMessage } from '../composables/messages'
import { openWhatsapp } from '../composables/charge'
import { storeName } from '../composables/cashState'
import { useToast } from '../composables/toast'
import { type Customer, type models } from '../types'
import LoadMore from '../components/LoadMore.vue'
import { usePaged } from '../composables/paged'

const toast = useToast()
const customers = ref<Customer[]>([])
function exportCustomers() {
  exportTable('Clientes', `${customers.value.length} cliente(s)`, [['Cliente', 'text'], ['Telefone', 'text'], ['CPF/CNPJ', 'text'], ['E-mail', 'text'], ['Bairro', 'text'], ['Cidade', 'text'], ['Nascimento', 'text'], ['Situação', 'text']],
    customers.value.map((c) => [c.name, c.phone ?? '', c.document ?? '', c.email ?? '', c.district ?? '', c.city ?? '', c.birthDate ? c.birthDate.split('-').reverse().join('/') : '', c.active ? 'Ativo' : 'Inativo']))
}
const term = ref('')
// Sub-abas: todos os clientes ou os aniversariantes (hoje, esta semana, este mês).
const sub = ref<'todos' | 'aniversariantes'>('todos')
const period = ref<'hoje' | 'semana' | 'mes'>('hoje')
const birthdays = computed(() => {
  const now = new Date()
  return customers.value
    .filter((c) => c.active && c.birthDate)
    .map((c) => ({ c, ...birthdayInfo(c.birthDate!, now), month: Number(c.birthDate!.slice(5, 7)) }))
    .filter((b) => (period.value === 'hoje' ? b.days === 0 : period.value === 'semana' ? b.days < 7 : b.month === now.getMonth() + 1))
    .sort((a, b) => (period.value === 'mes' ? Number(a.c.birthDate!.slice(8)) - Number(b.c.birthDate!.slice(8)) : a.days - b.days))
})
const ddmm = (d: string) => `${d.slice(8, 10)}/${d.slice(5, 7)}`
// A lista (com a ficha ao lado) aparece quando a aba tem alguém para mostrar.
const showList = computed(() => (sub.value === 'aniversariantes' ? birthdays.value.length > 0 : !loadError.value && customers.value.length > 0))
const loading = ref(true)
const loadError = ref('')
let timer: number | undefined

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    customers.value = await call(ListCustomers(term.value))
    // Veio da busca universal (?id=): abre o cliente escolhido.
    const want = customers.value.find((c) => c.id === pendingId.value)
    if (want) {
      pendingId.value = null
      select(want)
    }
  } catch (e) {
    loadError.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
// Busca universal: /clientes?busca=<nome>&id=<id> filtra e abre o cliente.
const route = useRoute()
const pendingId = ref<number | null>(null)
watch(() => route.query, (q) => {
  if (typeof q.busca !== 'string') return
  pendingId.value = Number(q.id) || null
  sub.value = 'todos'
  if (term.value === q.busca) load()
  else term.value = q.busca
}, { immediate: true })

// Ativar/inativar direto na lista; excluir só quem não tem histórico (o sistema recusa os outros).
async function toggleActive(c: Customer) {
  try {
    const r = await call(SetCustomerActive(c.id, !c.active))
    customers.value = customers.value.map((x) => (x.id === r.id ? r : x))
    toast.success(`${r.name} ${r.active ? 'ativado' : 'inativado'}.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
const deleteAsk = ref<Customer | null>(null)
// Com a permissão, Excluir apaga também todo o histórico (mostra antes o que some).
const purgeAsk = ref<Customer | null>(null)
const canPurge = computed(() => can('apagar_pessoa_historico'))
function purged() {
  const c = purgeAsk.value!
  purgeAsk.value = null
  customers.value = customers.value.filter((x) => x.id !== c.id)
  if (selectedId.value === c.id) selectedId.value = null
}
async function remove() {
  const c = deleteAsk.value!
  deleteAsk.value = null
  try {
    await call(DeleteCustomer(c.id))
    customers.value = customers.value.filter((x) => x.id !== c.id)
    toast.success(`${c.name} excluído.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
watch(term, () => {
  window.clearTimeout(timer)
  timer = window.setTimeout(load, 250)
})

// ---------- cadastro ----------
// editing: cliente aberto no formulário (null = novo); fechado quando undefined.
const editing = ref<Customer | null | undefined>(undefined)
function openForm(c?: Customer) {
  editing.value = c ?? null
}

// WhatsApp: abre a conversa com uma saudação já escrita (o operador completa e envia).
function talk(c: Customer) {
  const first = c.name.trim().split(/\s+/)[0] ?? ''
  openWhatsapp(c.phone ?? '', `Olá, ${first}! Aqui é da ${storeName.value || 'loja'}.`)
}

// ---------- ficha ao lado da lista ----------
// selected: cliente clicado na lista; history: as compras e o fiado dele.
const selectedId = ref<number | null>(null)
const selected = computed(() => customers.value.find((c) => c.id === selectedId.value) ?? null)
const history = ref<models.CustomerHistory | null>(null)
const historyLoading = ref(false)
async function select(c: Customer) {
  selectedId.value = c.id
  history.value = null
  historyLoading.value = true
  try {
    const h = await call(GetCustomerHistory(c.id))
    if (selectedId.value === c.id) history.value = h
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    historyLoading.value = false
  }
}
const done = computed(() => (history.value?.sales ?? []).filter((s) => s.status !== 'cancelada'))
const totalBought = computed(() => done.value.reduce((a, s) => a + s.totalCents, 0))
const lastSale = computed(() => done.value.reduce<models.Sale | null>((a, s) => (!a || s.createdAt > a.createdAt ? s : a), null))
const lateNotes = computed(() => (history.value?.notes ?? []).filter((n) => n.openCents > 0 && n.daysOpen > 30).length)
function address(c: Customer) {
  const street = [c.street, c.number].filter(Boolean).join(', ')
  return [street, c.complement, c.district, [c.city, c.state].filter(Boolean).join('/')].filter(Boolean).join(' · ')
}

// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgClientes = usePaged(() => customers.value, [term, sub])
</script>

<template>
  <section class="card stack cust-page">
    <div class="tabs seg" role="tablist" aria-label="Clientes">
      <button role="tab" :aria-selected="sub === 'todos'" @click="sub = 'todos'">Todos</button>
      <button role="tab" :aria-selected="sub === 'aniversariantes'" @click="sub = 'aniversariantes'">Aniversariantes</button>
    </div>
    <template v-if="sub === 'aniversariantes'">
      <div class="row chips" role="group" aria-label="Período">
        <div class="chip-track">
          <button class="chip" :aria-pressed="period === 'hoje'" @click="period = 'hoje'">Hoje</button>
          <button class="chip" :aria-pressed="period === 'semana'" @click="period = 'semana'">Próximos 7 dias</button>
          <button class="chip" :aria-pressed="period === 'mes'" @click="period = 'mes'">Este mês</button>
        </div>
      </div>
      <div v-if="!birthdays.length" class="state">Nenhum aniversariante {{ period === 'hoje' ? 'hoje' : period === 'semana' ? 'nos próximos 7 dias' : 'este mês' }}. Cadastre a data de nascimento no cliente.</div>
    </template>
    <template v-else>
    <div class="row">
      <input v-model="term" type="search" placeholder="Buscar por nome, telefone, CPF/CNPJ, e-mail, rua, bairro ou cidade" aria-label="Buscar clientes" style="max-width: 460px" />
      <span class="spacer" />
      <button @click="exportCustomers">Exportar Excel</button>
      <button class="primary" @click="openForm()">Novo cliente</button>
    </div>
    <div v-if="loading && customers.length === 0" class="state">Carregando clientes…</div>
    <div v-else-if="loadError" class="alert error">{{ loadError }} <button class="link" @click="load">Tentar novamente</button></div>
    <div v-else-if="customers.length === 0" class="state">{{ term ? 'Nenhum cliente encontrado.' : 'Nenhum cliente cadastrado.' }}</div>
    </template>
    <!-- Lista (todos ou aniversariantes) e a ficha do cliente clicado ao lado. -->
    <div v-if="showList" class="cust-split" :class="{ solo: sub === 'aniversariantes' }">
    <div class="table-wrap cust-list">
      <table v-if="sub === 'aniversariantes'" class="left-table">
        <thead><tr><th>Cliente</th><th>Aniversário</th><th>Telefone</th><th class="right">Ações</th></tr></thead>
        <tbody>
          <tr v-for="b in birthdays" :key="b.c.id">
            <td><div class="cust-cell"><CustomerAvatar :id="b.c.id" :name="b.c.name" />
              <span>{{ b.c.name }}<small class="muted">faz {{ b.age }} anos</small></span></div></td>
            <td class="nowrap">{{ ddmm(b.c.birthDate!) }} <span v-if="b.days === 0" class="badge ok">hoje</span><span v-else-if="b.days < 7" class="small muted">em {{ b.days }} dia(s)</span></td>
            <td class="nowrap">{{ b.c.phone || '—' }}</td>
            <td class="right actions-cell">
              <button class="link icon-btn whats" :disabled="!b.c.phone" :title="b.c.phone ? 'Mandar parabéns pelo WhatsApp' : 'Cadastre o celular do cliente para chamar no WhatsApp'"
                aria-label="Parabéns no WhatsApp" @click="sendMessage('msgBirthday', b.c.phone, { nome: b.c.name })"><Icon name="whatsapp" /><span>Parabéns</span></button>
              <button class="link icon-btn" @click="openForm(b.c)" title="Editar" aria-label="Editar"><Icon name="edit" /><span>Editar</span></button>
            </td>
          </tr>
        </tbody>
      </table>
      <table v-else class="left-table">
        <thead>
          <tr><th>Cliente</th><th>Telefone</th><th>Situação</th><th class="right">Ações</th></tr>
        </thead>
        <tbody>
          <tr v-for="c in pgClientes.visible" :key="c.id" class="clickable" :class="{ inactive: !c.active, selected: c.id === selectedId }" @click="select(c)">
            <td><div class="cust-cell"><CustomerAvatar :id="c.id" :name="c.name" />
              <span>{{ c.name }}<small v-if="c.city" class="muted">{{ c.city }}<template v-if="c.district"> · {{ c.district }}</template></small></span></div></td>
            <td class="nowrap">{{ c.phone || '—' }}</td>
            <td><span class="badge" :class="c.active ? 'ok' : ''">{{ c.active ? 'Ativo' : 'Inativo' }}</span></td>
            <td class="right actions-cell" @click.stop>
              <button class="link icon-btn whats" :disabled="!c.phone" :title="c.phone ? `Abrir conversa com ${c.name} no WhatsApp` : 'Cadastre o celular do cliente para chamar no WhatsApp'"
                aria-label="Chamar no WhatsApp" @click="talk(c)"><Icon name="whatsapp" /><span>WhatsApp</span></button>
              <button class="link icon-btn" @click="openForm(c)" title="Editar" aria-label="Editar"><Icon name="edit" /><span>Editar</span></button>
              <button class="link icon-btn" :style="c.active ? '' : 'color: var(--success)'" :title="c.active ? 'Inativar: não compra a prazo' : 'Ativar o cliente'"
                :aria-label="c.active ? 'Inativar' : 'Ativar'" @click="toggleActive(c)"><Icon :name="c.active ? 'desativar' : 'ativar'" /><span>{{ c.active ? 'Inativar' : 'Ativar' }}</span></button>
              <button class="link icon-btn" style="color: var(--danger)" :title="canPurge ? 'Excluir de vez, com todo o histórico' : 'Excluir de vez (só cliente sem histórico)'" aria-label="Excluir" @click="canPurge ? (purgeAsk = c) : (deleteAsk = c)"><Icon name="remover" /><span>Excluir</span></button>
            </td>
          </tr>
        </tbody>
      </table>
          <LoadMore :paged="pgClientes" />
    </div>
    <aside v-if="sub === 'todos'" class="cust-detail">
      <div v-if="!selected" class="state">Clique num cliente para ver a ficha, o saldo a prazo e as compras.</div>
      <template v-else>
        <div class="detail-head">
          <CustomerAvatar :id="selected.id" :name="selected.name" big />
          <div class="detail-id">
            <strong>{{ selected.name }}</strong>
            <span class="small muted">
              <template v-if="selected.document">{{ formatDocument(selected.document) }} · </template>{{ selected.phone || 'sem telefone' }}<template v-if="selected.email"> · {{ selected.email }}</template>
            </span>
            <span v-if="address(selected)" class="small muted">{{ address(selected) }}</span>
            <span v-if="selected.birthDate" class="small muted">Aniversário {{ ddmm(selected.birthDate) }}</span>
          </div>
          <button @click="openForm(selected)"><Icon name="edit" /> Editar</button>
        </div>
        <div class="detail-kpis">
          <div class="kpi">
            <div class="kpi-head">Em aberto</div>
            <div class="kpi-value mono" :style="{ color: selected.creditBalanceCents > 0 ? 'var(--danger)' : '' }">{{ money(selected.creditBalanceCents) }}</div>
            <div class="small muted">{{ lateNotes ? `${lateNotes} nota(s) há mais de 30 dias` : 'a prazo' }}</div>
          </div>
          <div class="kpi">
            <div class="kpi-head">Limite de crédito</div>
            <div class="kpi-value mono">{{ selected.creditLimitCents ? money(selected.creditLimitCents) : '—' }}</div>
            <div v-if="selected.creditLimitCents" class="bar"><i :style="{ width: `${Math.min(100, (selected.creditBalanceCents / selected.creditLimitCents) * 100)}%` }" /></div>
            <div v-else class="small muted">sem limite definido</div>
          </div>
          <div class="kpi">
            <div class="kpi-head">Total comprado</div>
            <div class="kpi-value mono">{{ history ? money(totalBought) : '…' }}</div>
            <div class="small muted">{{ done.length }} compra(s)</div>
          </div>
          <div class="kpi">
            <div class="kpi-head">Última compra</div>
            <div class="kpi-value">{{ lastSale ? formatDate(lastSale.createdAt) : '—' }}</div>
            <div v-if="selected.storeCreditCents > 0" class="small"><span class="badge ok">vale-troca {{ money(selected.storeCreditCents) }}</span></div>
          </div>
        </div>
        <div class="card-head" style="margin: 4px 0 0"><strong>Histórico de compras</strong>
          <router-link v-if="selected.creditBalanceCents > 0" to="/fiado" class="small">Receber em A prazo</router-link></div>
        <div v-if="historyLoading" class="state">Carregando…</div>
        <div v-else-if="!history?.sales.length" class="state">Nenhuma compra registrada.</div>
        <div v-else class="table-wrap detail-sales">
          <table class="left-table">
            <thead><tr><th>Venda</th><th>Data</th><th>Itens</th><th>Pagamento</th><th class="num">Total</th></tr></thead>
            <tbody>
              <tr v-for="s in history.sales" :key="s.id" :class="{ inactive: s.status === 'cancelada' }">
                <td class="mono">nº {{ s.id }}</td>
                <td class="nowrap">{{ formatDateTime(s.createdAt) }}</td>
                <td class="items">{{ s.itemsLabel || '—' }}</td>
                <td><span class="badge" :class="s.paymentMethod === 'fiado' ? 'bad' : ''">{{ paymentLabels[s.paymentMethod] ?? s.paymentMethod }}</span></td>
                <td class="num mono"><strong>{{ money(s.totalCents) }}</strong></td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </aside>
    </div>
    <p v-if="sub === 'todos'" class="small muted">{{ canPurge ? 'Excluir apaga o cliente com todo o histórico (vendas, fiado): confira antes.' : 'Clientes com histórico não podem ser excluídos: use Inativar.' }} Compras a prazo, cobrança e vale-troca ficam em Financeiro › A prazo.</p>
  </section>

  <ConfirmDialog v-if="deleteAsk" title="Excluir cliente" danger
    :message="`Excluir ${deleteAsk.name} de vez? Só é possível para quem não tem vendas nem fiado; para os outros, use Inativar.`"
    confirm-label="Excluir" cancel-label="Voltar" @confirm="remove" @cancel="deleteAsk = null" />

  <PurgeCustomerDialog v-if="purgeAsk" :customer-id="purgeAsk.id" :name="purgeAsk.name" @done="purged" @close="purgeAsk = null" />
  <CustomerFormModal v-if="editing !== undefined" :customer="editing" @close="editing = undefined" @saved="editing = undefined; load(); selected && select(selected)" />
</template>

<style scoped>
/* Lista à esquerda, ficha do cliente selecionado à direita. */
/* A tela cabe na janela (rota com fill): a página não rola; só a lista (e a
   ficha, se for maior) rolam por dentro. */
.cust-page { flex: 1; min-height: 0; display: flex; flex-direction: column; overflow: hidden; }
.cust-page > .table-wrap { flex: 1; min-height: 0; overflow: auto; }
.cust-split { flex: 1; min-height: 0; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.15fr); gap: 18px; align-items: stretch; }
.cust-list { min-height: 0; overflow: auto; }
.cust-detail { min-height: 0; overflow: auto; }
/* Janela estreita: lista e ficha uma embaixo da outra, e a página volta a rolar. */
@media (max-width: 1200px) {
  .cust-page { overflow: auto; }
  .cust-split { flex: none; grid-template-columns: minmax(0, 1fr); }
  .cust-list { max-height: 60vh; }
}
/* Aniversariantes: só a lista, na largura toda (sem a ficha ao lado). */
.cust-split.solo { grid-template-columns: minmax(0, 1fr); }
.cust-cell { display: flex; align-items: center; gap: 12px; }
.cust-cell small { display: block; font-size: 12px; }
.avatar { width: 36px; height: 36px; border-radius: 11px; display: grid; place-items: center; color: #fff; font-size: 13px; font-weight: 700; flex-shrink: 0; }
.avatar.big { width: 64px; height: 64px; border-radius: 18px; font-size: 22px; }
.cust-detail { display: flex; flex-direction: column; gap: 16px; padding: 20px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface-2); min-height: 320px; }
.detail-head { display: flex; align-items: flex-start; gap: 16px; }
.detail-id { flex: 1; display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.detail-id strong { font-size: 20px; letter-spacing: -0.01em; }
.detail-kpis { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.detail-kpis .kpi { padding: 14px 16px; box-shadow: none; }
.detail-kpis .kpi-value { font-size: 22px; }
.bar { height: 8px; border-radius: 8px; background: var(--surface-2); overflow: hidden; margin-top: 4px; }
.bar i { display: block; height: 100%; border-radius: 8px; background: var(--primary); }
.detail-sales { max-height: 340px; overflow: auto; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); }
.detail-sales .items { max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
/* Botão do WhatsApp em verde, como o do aplicativo. */
.icon-btn.whats { color: #128c4b; }
.icon-btn.whats:disabled { color: var(--muted); }
</style>
