<!-- Caminho: pdv/frontend/src/views/Clientes.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { DeleteCustomer, ListCustomers, SetCustomerActive } from '../../wailsjs/go/main/App'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CustomerFormModal from '../components/CustomerFormModal.vue'
import PurgeCustomerDialog from '../components/PurgeCustomerDialog.vue'
import { call, errorMessage } from '../api'
import { exportTable } from '../composables/exportTable'
import { can } from '../composables/session'
import { formatDocument } from '../composables/format'
import { birthdayInfo, sendMessage } from '../composables/messages'
import { useToast } from '../composables/toast'
import { type Customer } from '../types'

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
const loading = ref(true)
const loadError = ref('')
let timer: number | undefined

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    customers.value = await call(ListCustomers(term.value))
  } catch (e) {
    loadError.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

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

</script>

<template>
  <section class="card stack">
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
      <div v-else class="table-wrap">
        <table>
          <thead><tr><th>Cliente</th><th>Aniversário</th><th class="right">Faz</th><th>Telefone</th><th class="right">Ações</th></tr></thead>
          <tbody>
            <tr v-for="b in birthdays" :key="b.c.id">
              <td>{{ b.c.name }}</td>
              <td>{{ ddmm(b.c.birthDate!) }} <span v-if="b.days === 0" class="badge ok">hoje</span><span v-else-if="b.days < 7" class="small muted">em {{ b.days }} dia(s)</span></td>
              <td class="right mono">{{ b.age }} anos</td>
              <td>{{ b.c.phone || '—' }}</td>
              <td class="right actions-cell">
                <button class="link icon-btn" style="color: var(--success)" title="Mandar parabéns pelo WhatsApp" aria-label="Parabéns no WhatsApp" @click="sendMessage('msgBirthday', b.c.phone, { nome: b.c.name })"><Icon name="whatsapp" /><span>Parabéns</span></button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
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
    <div v-else class="table-wrap">
      <table>
        <thead>
          <tr><th>Cliente</th><th>Telefone</th><th>CPF/CNPJ</th><th>Cidade</th><th>Nascimento</th><th>Situação</th><th class="right">Ações</th></tr>
        </thead>
        <tbody>
          <tr v-for="c in customers" :key="c.id" :class="{ inactive: !c.active }">
            <td class="left">{{ c.name }}</td>
            <td>{{ c.phone || '—' }}</td>
            <td class="mono small">{{ formatDocument(c.document) || '—' }}</td>
            <td>{{ c.city || '—' }}<span v-if="c.district" class="small muted"> · {{ c.district }}</span></td>
            <td class="nowrap">{{ c.birthDate ? c.birthDate.split('-').reverse().join('/') : '—' }}</td>
            <td><span class="badge" :class="c.active ? 'ok' : ''">{{ c.active ? 'Ativo' : 'Inativo' }}</span></td>
            <td class="right actions-cell">
              <button class="link icon-btn" @click="openForm(c)" title="Editar" aria-label="Editar"><Icon name="edit" /><span>Editar</span></button>
              <button class="link icon-btn" :style="c.active ? '' : 'color: var(--success)'" :title="c.active ? 'Inativar: não compra a prazo e a catraca nega' : 'Ativar o cliente'"
                :aria-label="c.active ? 'Inativar' : 'Ativar'" @click="toggleActive(c)"><Icon :name="c.active ? 'desativar' : 'ativar'" /><span>{{ c.active ? 'Inativar' : 'Ativar' }}</span></button>
              <button class="link icon-btn" style="color: var(--danger)" :title="canPurge ? 'Excluir de vez, com todo o histórico' : 'Excluir de vez (só cliente sem histórico)'" aria-label="Excluir" @click="canPurge ? (purgeAsk = c) : (deleteAsk = c)"><Icon name="remover" /><span>Excluir</span></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="small muted">{{ canPurge ? 'Excluir apaga o cliente com todo o histórico (vendas, fiado, mensalidades): confira antes.' : 'Clientes com histórico não podem ser excluídos: use Inativar.' }} Compras a prazo, cobrança e vale-troca ficam em Financeiro › A prazo.</p>
    </template>
  </section>

  <ConfirmDialog v-if="deleteAsk" title="Excluir cliente" danger
    :message="`Excluir ${deleteAsk.name} de vez? Só é possível para quem não tem vendas, fiado, assinaturas nem passagens na catraca; para os outros, use Inativar.`"
    confirm-label="Excluir" cancel-label="Voltar" @confirm="remove" @cancel="deleteAsk = null" />

  <PurgeCustomerDialog v-if="purgeAsk" :customer-id="purgeAsk.id" :name="purgeAsk.name" @done="purged" @close="purgeAsk = null" />

  <CustomerFormModal v-if="editing !== undefined" :customer="editing" @close="editing = undefined" @saved="editing = undefined; load()" />
</template>
