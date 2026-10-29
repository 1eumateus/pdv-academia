<!-- Caminho: catraca/frontend/src/views/CatracaAlunos.vue -->
<script setup lang="ts">
// Alunos na catraca: o aluno é o cliente do PDV e a situação vem da
// assinatura (Academia › Assinaturas). Aqui se cadastra e se vê o rosto no
// iDFace e, com a permissão da catraca, se apaga o aluno.
// Sem busca aparecem quem tem plano ativo ou cadastro na catraca; a busca
// procura em todos os clientes ativos.
import { computed, onMounted, ref, watch } from 'vue'
import { EnrollFace, GateCheckAndRelease, GetCustomer, ImportFromIDFace, ListGateMembers, RemoveFace, RemoveGateMember } from '../../wailsjs/go/main/App'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CustomerFormModal from '../components/CustomerFormModal.vue'
import FacePhoto from '../components/FacePhoto.vue'
import FacePhotoModal from '../components/FacePhotoModal.vue'
import Icon from '../components/Icon.vue'
import SubscriptionPanel from '../components/SubscriptionPanel.vue'
import { formatDateTime } from '../composables/format'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { type Customer } from '../types'

type Member = models.GateMember

const toast = useToast()
const members = ref<Member[]>([])
const term = ref('')
const loading = ref(true)
const error = ref('')
type View = 'todos' | 'liberados' | 'negados' | 'semrosto'
const view = ref<View>('todos')
const views: { key: View; label: string; test: (m: Member) => boolean }[] = [
  { key: 'todos', label: 'Todos', test: () => true },
  { key: 'liberados', label: 'Liberados', test: (m) => m.allowed },
  { key: 'negados', label: 'Negados', test: (m) => !m.allowed },
  { key: 'semrosto', label: 'Sem rosto', test: (m) => !m.deviceUserId },
]
const filtered = computed(() => members.value.filter(views.find((v) => v.key === view.value)!.test))
const count = (v: (typeof views)[number]) => members.value.filter(v.test).length

const brDate = (d: string) => (d.length === 10 ? `${d.slice(8, 10)}/${d.slice(5, 7)}/${d.slice(0, 4)}` : d)
const plansLabel = (m: Member) =>
  m.plans.map((p) => (p.paused ? `${p.name} (pausado)` : p.dueDate ? `${p.name} · vence ${brDate(p.dueDate)}` : p.name)).join(', ')

async function load() {
  loading.value = true
  try {
    members.value = await call(ListGateMembers(term.value))
    error.value = ''
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
let timer: number | undefined
watch(term, () => {
  window.clearTimeout(timer)
  timer = window.setTimeout(load, 250)
})

function replace(m: Member) {
  members.value = members.value.map((x) => (x.customerId === m.customerId ? m : x))
}

// ---------- novo aluno: vira cliente (aparece em Clientes) e já aparece aqui pela busca ----------
const adding = ref(false)
function added(c: Customer) {
  adding.value = false
  term.value = c.name
  toast.info(`Agora cadastre o rosto de ${c.name} e crie a assinatura em Academia › Assinaturas.`)
}

// ---------- ficha (clique na linha): foto, plano e cadastro editável ----------
const ficha = ref<{ member: Member; customer: Customer } | null>(null)
// Mexeu na assinatura pela ficha: relê a lista e a situação do aluno na ficha.
async function fichaChanged() {
  await load()
  const m = members.value.find((x) => x.customerId === ficha.value?.member.customerId)
  if (m && ficha.value) ficha.value.member = m
}
async function openFicha(m: Member) {
  try {
    ficha.value = { member: m, customer: await call(GetCustomer(m.customerId)) }
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// ---------- rosto no leitor ----------
// photoOf: mostra a foto que o leitor guardou (logo após o cadastro ou ao clicar em "Cadastrado").
const photoOf = ref<Member | null>(null)
// enrolling: aluno olhando para o leitor agora.
const enrolling = ref(0)
async function enroll(m: Member) {
  enrolling.value = m.customerId
  toast.info(`Peça para ${m.name} olhar para o leitor: a foto sai sozinha em 3 segundos…`)
  try {
    replace(await call(EnrollFace(m.customerId)))
    toast.success(`Rosto de ${m.name} cadastrado.`)
    photoOf.value = m
  } catch (e) {
    toast.error(`${m.name}: ${errorMessage(e)}`)
  } finally {
    enrolling.value = 0
  }
}

const removeAsk = ref<Member | null>(null)
async function removeFace() {
  const m = removeAsk.value!
  removeAsk.value = null
  try {
    replace(await call(RemoveFace(m.customerId)))
    toast.success(`Rosto de ${m.name} removido do leitor.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// Apagar: tira da academia (rosto, assinaturas ativas, cadastro ativo); o histórico fica.
const deleteAsk = ref<Member | null>(null)
async function removeMember() {
  const m = deleteAsk.value!
  deleteAsk.value = null
  try {
    await call(RemoveGateMember(m.customerId))
    members.value = members.value.filter((x) => x.customerId !== m.customerId)
    toast.success(`${m.name} saiu da academia. Para voltar, use Reativar em Assinaturas › Canceladas.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

const importing = ref(false)
async function importDevice() {
  importing.value = true
  try {
    const r = await call(ImportFromIDFace())
    toast.success(`iDFace com ${r.deviceUsers} pessoa(s): ${r.linked} ligada(s) a clientes, ${r.created} cliente(s) novo(s), ${r.already} já estavam ligadas.`)
    await load()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    importing.value = false
  }
}

async function release(m: Member) {
  try {
    const r = await call(GateCheckAndRelease(m.customerId))
    if (r.allowed) toast.success(`${m.name}: liberado${r.released ? '' : ' (acionamento desligado: a catraca não foi aberta)'}.`)
    else toast.error(`${m.name}: ${r.reason}`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

</script>

<template>
  <section class="card stack">
    <div class="row">
      <input v-model="term" type="search" placeholder="Buscar cliente por nome, telefone ou CPF" aria-label="Buscar aluno" style="max-width: 420px" />
      <span class="spacer" />
      <button class="primary" @click="adding = true">+ Adicionar aluno</button>
      <button v-if="can('catraca')" :disabled="importing" title="Liga ao cadastro de clientes as pessoas que já estão no iDFace" @click="importDevice">
        {{ importing ? 'Importando…' : 'Importar do iDFace' }}
      </button>
    </div>
    <div class="tabs seg" role="tablist" aria-label="Alunos">
      <button v-for="v in views" :key="v.key" role="tab" :aria-selected="view === v.key" @click="view = v.key">
        {{ v.label }} <span class="chip-count">{{ count(v) }}</span>
      </button>
    </div>
    <p class="small muted" style="margin: 0">
      {{ term ? 'Procurando em todos os clientes ativos.' : 'Alunos com plano ativo ou com cadastro na catraca. Busque o nome para cadastrar o rosto de outro cliente.' }}
      O plano e a mensalidade ficam em Academia › Assinaturas.
    </p>
    <div v-if="loading && members.length === 0" class="state">Carregando…</div>
    <div v-else-if="error" class="alert error">{{ error }} <button class="link" @click="load">Tentar novamente</button></div>
    <div v-else-if="filtered.length === 0" class="state">{{ term ? 'Ninguém encontrado.' : 'Nenhum aluno aqui.' }}</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th class="left">Aluno</th><th class="left">Plano</th><th>Catraca</th><th>Rosto</th><th>Último acesso</th><th></th></tr></thead>
        <tbody>
          <tr v-for="m in filtered" :key="m.customerId" class="clickable" :class="{ inactive: m.blocked || !m.active }" title="Abrir a ficha do aluno" @click="openFicha(m)">
            <td class="left"><strong>{{ m.name }}</strong><div v-if="m.phone" class="small muted">{{ m.phone }}</div></td>
            <td class="left small">{{ plansLabel(m) || '—' }}<div v-if="m.freeUntil" class="small muted">Cortesia até {{ brDate(m.freeUntil) }}</div></td>
            <td :title="m.reason">
              <span class="badge" :class="m.allowed ? 'ok' : 'bad'">{{ m.allowed ? 'Liberado' : m.blocked ? 'Bloqueado' : 'Negado' }}</span>
              <div class="small muted">{{ m.reason }}</div>
            </td>
            <td class="nowrap" @click.stop>
              <button v-if="m.deviceUserId" type="button" class="badge ok" style="border: 0; cursor: pointer" :title="`Ver a foto (usuário ${m.deviceUserId} no iDFace)`" @click="photoOf = m">Cadastrado</button>
              <span v-else class="badge">Sem rosto</span>
            </td>
            <td class="small nowrap">{{ m.lastAccess ? formatDateTime(m.lastAccess) : '—' }}</td>
            <td class="right actions-cell wrap" @click.stop>
              <button v-if="m.deviceUserId" class="link icon-btn" title="Ver a foto do rosto guardada no leitor" aria-label="Ver rosto" @click="photoOf = m"><Icon name="detalhes" /><span>Ver rosto</span></button>
              <button class="link icon-btn" :disabled="enrolling !== 0 || !m.active" :title="m.deviceUserId ? 'Cadastrar o rosto de novo' : 'Cadastrar o rosto no leitor'"
                :aria-label="m.deviceUserId ? 'Refazer rosto' : 'Cadastrar rosto'" @click="enroll(m)">
                <Icon name="t-rosto" /><span>{{ enrolling === m.customerId ? 'Olhe o leitor…' : m.deviceUserId ? 'Refazer rosto' : 'Cadastrar rosto' }}</span>
              </button>
              <button v-if="m.deviceUserId && can('catraca')" class="link icon-btn" :disabled="enrolling !== 0" title="Apagar o rosto do leitor" aria-label="Remover rosto" @click="removeAsk = m"><Icon name="remover" /><span>Remover rosto</span></button>
              <button class="link icon-btn" style="color: var(--success)" title="Confere a assinatura e abre a catraca se estiver em dia" aria-label="Liberar" @click="release(m)"><Icon name="liberar" /><span>Liberar</span></button>
              <button v-if="can('catraca')" class="link icon-btn" style="color: var(--danger)" title="Tirar da academia: apaga o rosto, cancela as assinaturas e desativa o cliente" aria-label="Apagar aluno" @click="deleteAsk = m"><Icon name="desativar" /><span>Apagar</span></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <CustomerFormModal v-if="adding" :customer="null" title="Novo aluno" @close="adding = false" @saved="added" />
  <CustomerFormModal v-if="ficha" :customer="ficha.customer" :title="`Ficha de ${ficha.member.name}`" @close="ficha = null" @saved="ficha = null; load()">
    <template #top>
      <div class="row" style="align-items: flex-start; gap: 16px">
        <div style="width: 180px; flex: none; text-align: center">
          <FacePhoto v-if="ficha.member.deviceUserId" :customer-id="ficha.member.customerId" :name="ficha.member.name" max-height="220px" />
          <div v-else class="state">Sem rosto cadastrado</div>
        </div>
        <div class="stack" style="gap: 6px">
          <div><span class="badge" :class="ficha.member.allowed ? 'ok' : 'bad'">{{ ficha.member.allowed ? 'Liberado' : 'Negado' }}</span> <span class="small muted">{{ ficha.member.reason }}</span></div>
          <div class="small"><strong>Plano:</strong> {{ plansLabel(ficha.member) || 'sem plano ativo' }}</div>
          <div class="small"><strong>Último acesso:</strong> {{ ficha.member.lastAccess ? formatDateTime(ficha.member.lastAccess) : 'nunca' }}</div>
        </div>
      </div>
      <h3 style="margin: 4px 0 0">Assinatura</h3>
      <SubscriptionPanel :customer-id="ficha.member.customerId" @changed="fichaChanged" />
      <h3 style="margin: 4px 0 0">Cadastro</h3>
    </template>
  </CustomerFormModal>
  <FacePhotoModal v-if="photoOf" :customer-id="photoOf.customerId" :name="photoOf.name" @close="photoOf = null" />
  <ConfirmDialog v-if="deleteAsk" title="Apagar aluno" danger
    :message="`Tirar ${deleteAsk.name} da academia? O rosto sai do leitor, as assinaturas ativas são canceladas e o cliente fica inativo. Vendas, mensalidades e passagens continuam guardadas.`"
    confirm-label="Apagar aluno" cancel-label="Voltar" @confirm="removeMember" @cancel="deleteAsk = null" />
  <ConfirmDialog v-if="removeAsk" title="Remover rosto"
    :message="`Apagar ${removeAsk.name} do leitor facial? O cliente e a assinatura continuam; para voltar a passar pelo leitor, cadastre o rosto de novo.`"
    confirm-label="Remover" danger @confirm="removeFace" @cancel="removeAsk = null" />
</template>
