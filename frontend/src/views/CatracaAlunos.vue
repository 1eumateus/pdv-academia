<!-- Caminho: catraca/frontend/src/views/CatracaAlunos.vue -->
<script setup lang="ts">
// Alunos na catraca: o aluno é o cliente do PDV e a situação vem da
// assinatura, que se cria e se mexe na ficha (clique na linha). Aqui se
// cadastra e se vê o rosto no iDFace e, com a permissão da catraca, se apaga o
// aluno. A outra aba, Mensalidades e cobrança, é a lista de assinaturas (a
// vencer, vencidas, canceladas…) com os lembretes pelo WhatsApp.
// Sem busca aparecem quem tem plano ativo ou cadastro na catraca; a busca
// procura em todos os clientes ativos.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Assinaturas from './Assinaturas.vue'
import { EnrollFace, EnrollFacePhoto, GateCheckAndRelease, GetCustomer, ImportFromIDFace, ListGateMembers, RemoveFace, RemoveGateMember } from '../../wailsjs/go/main/App'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CustomerFormModal from '../components/CustomerFormModal.vue'
import FacePhoto from '../components/FacePhoto.vue'
import PurgeCustomerDialog from '../components/PurgeCustomerDialog.vue'
import FacePhotoModal from '../components/FacePhotoModal.vue'
import Icon from '../components/Icon.vue'
import SubscriptionPanel from '../components/SubscriptionPanel.vue'
import { formatDateTime } from '../composables/format'
import { permanentExemptDue } from '../composables/subscriptions'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { useRowMenu } from '../composables/rowMenu'
import { type Customer } from '../types'

type Member = models.GateMember

const toast = useToast()
// Aba da tela: alunos (rostos e catraca) ou mensalidades e cobrança (?aba=mensalidades).
const route = useRoute()
const router = useRouter()
type Mode = 'alunos' | 'mensalidades'
const mode = ref<Mode>(route.query.aba === 'mensalidades' ? 'mensalidades' : 'alunos')
watch(mode, (m) => {
  router.replace({ query: m === 'mensalidades' ? { aba: 'mensalidades' } : {} })
  if (m === 'alunos') load()
})
watch(() => route.query.aba, (a) => (mode.value = a === 'mensalidades' ? 'mensalidades' : 'alunos'))
const members = ref<Member[]>([])
const term = ref('')
const loading = ref(true)
const error = ref('')
// Ativo = tem assinatura ativa (pausada também conta); inativo = nenhuma
// (ex-aluno ou só com cadastro na catraca).
type View = 'todos' | 'ativos' | 'inativos' | 'semrosto'
const view = ref<View>('todos')
const views: { key: View; label: string; test: (m: Member) => boolean }[] = [
  { key: 'todos', label: 'Todos', test: () => true },
  { key: 'ativos', label: 'Ativos', test: (m) => m.plans.length > 0 },
  { key: 'inativos', label: 'Inativos', test: (m) => m.plans.length === 0 },
  { key: 'semrosto', label: 'Sem rosto', test: (m) => !m.deviceUserId },
]
const filtered = computed(() => members.value.filter(views.find((v) => v.key === view.value)!.test))
const count = (v: (typeof views)[number]) => members.value.filter(v.test).length

const brDate = (d: string) => (d.length === 10 ? `${d.slice(8, 10)}/${d.slice(5, 7)}/${d.slice(0, 4)}` : d)
const plansLabel = (m: Member) =>
  m.plans.map((p) => (p.paused ? `${p.name} (pausado)` : p.dueDate ? (p.dueDate === permanentExemptDue ? `${p.name} · isento` : `${p.name} · vence ${brDate(p.dueDate)}`) : p.name)).join(', ')

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
  toast.info(`Agora cadastre o rosto de ${c.name} e, na ficha dele (clique na linha), crie a assinatura.`)
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

// Rosto por foto: escolhe a imagem, reduz para no máximo 1280 px (JPEG) e envia ao leitor.
const photoInput = ref<HTMLInputElement | null>(null)
const photoTarget = ref<Member | null>(null)
function pickPhoto(m: Member) {
  photoTarget.value = m
  photoInput.value!.value = ''
  photoInput.value!.click()
}
function shrink(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      const k = Math.min(1, 1280 / Math.max(img.width, img.height))
      const canvas = document.createElement('canvas')
      canvas.width = Math.round(img.width * k)
      canvas.height = Math.round(img.height * k)
      canvas.getContext('2d')!.drawImage(img, 0, 0, canvas.width, canvas.height)
      URL.revokeObjectURL(url)
      resolve(canvas.toDataURL('image/jpeg', 0.92))
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('Não foi possível abrir a imagem. Use uma foto JPG ou PNG.'))
    }
    img.src = url
  })
}
async function sendPhoto() {
  const m = photoTarget.value
  const file = photoInput.value?.files?.[0]
  if (!m || !file) return
  enrolling.value = m.customerId
  try {
    replace(await call(EnrollFacePhoto(m.customerId, await shrink(file))))
    toast.success(`Rosto de ${m.name} cadastrado pela foto.`)
    photoOf.value = m
  } catch (e) {
    toast.error(`${m.name}: ${errorMessage(e)}`)
  } finally {
    enrolling.value = 0
    photoTarget.value = null
  }
}

// Menu "Mais" da linha (ações menos usadas).
const menu = useRowMenu()

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

// Apagar de vez, com todo o histórico (permissão de risco).
const purgeAsk = ref<Member | null>(null)
function purged() {
  const m = purgeAsk.value!
  purgeAsk.value = null
  members.value = members.value.filter((x) => x.customerId !== m.customerId)
}

// Apagar: tira da academia (rosto, assinaturas ativas, cadastro ativo); o histórico fica.
const deleteAsk = ref<Member | null>(null)
async function removeMember() {
  const m = deleteAsk.value!
  deleteAsk.value = null
  try {
    await call(RemoveGateMember(m.customerId))
    members.value = members.value.filter((x) => x.customerId !== m.customerId)
    toast.success(`${m.name} saiu da academia. Para voltar, use Reativar em Mensalidades e cobrança › Canceladas.`)
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
  <div class="tabs seg mode-tabs" role="tablist" aria-label="Alunos e mensalidades">
    <button role="tab" :aria-selected="mode === 'alunos'" @click="mode = 'alunos'">Alunos</button>
    <button role="tab" :aria-selected="mode === 'mensalidades'" @click="mode = 'mensalidades'">Mensalidades e cobrança</button>
  </div>
  <Assinaturas v-if="mode === 'mensalidades'" />
  <section v-else class="card stack">
    <div class="row">
      <input v-model="term" type="search" placeholder="Buscar cliente por nome, telefone ou CPF" aria-label="Buscar aluno" style="max-width: 420px" />
      <span class="spacer" />
      <button class="primary" @click="adding = true">+ Adicionar aluno</button>
      <button v-if="can('catraca_configurar')" :disabled="importing" title="Liga ao cadastro de clientes as pessoas que já estão no iDFace" @click="importDevice">
        {{ importing ? 'Importando…' : 'Importar do iDFace' }}
      </button>
    </div>
    <div class="tabs seg" role="tablist" aria-label="Alunos">
      <button v-for="v in views" :key="v.key" role="tab" :aria-selected="view === v.key" @click="view = v.key">
        {{ v.label }} <span class="chip-count">{{ count(v) }}</span>
      </button>
    </div>
    <p class="small muted" style="margin: 0">
      {{ term ? 'Procurando em todos os clientes ativos.' : 'Alunos (com assinatura ativa ou que já tiveram) e quem tem cadastro na catraca. Busque o nome para cadastrar o rosto de outro cliente.' }}
      O plano e a mensalidade ficam na ficha do aluno (clique na linha).
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
            <td class="left small clip" :title="plansLabel(m)">{{ plansLabel(m) || '—' }}<div v-if="m.freeUntil" class="small muted">Cortesia até {{ brDate(m.freeUntil) }}</div></td>
            <td class="left gate-cell" :title="m.reason">
              <span class="badge" :class="m.allowed ? 'ok' : 'bad'">{{ m.allowed ? 'Liberado' : m.blocked ? 'Bloqueado' : 'Negado' }}</span>
              <span class="small muted gate-reason">{{ m.reason }}</span>
            </td>
            <td class="nowrap" @click.stop>
              <button v-if="m.deviceUserId" type="button" class="badge ok" style="border: 0; cursor: pointer" :title="`Ver a foto (usuário ${m.deviceUserId} no iDFace)`" @click="photoOf = m">Cadastrado</button>
              <span v-else class="badge">Sem rosto</span>
            </td>
            <td class="small nowrap">{{ m.lastAccess ? formatDateTime(m.lastAccess) : '—' }}</td>
            <td class="right actions-cell" @click.stop>
              <button class="link icon-btn" :disabled="enrolling !== 0 || !m.active" :title="m.deviceUserId ? 'Cadastrar o rosto de novo' : 'Cadastrar o rosto no leitor'"
                :aria-label="m.deviceUserId ? 'Refazer rosto' : 'Cadastrar rosto'" @click="enroll(m)">
                <Icon name="t-rosto" /><span>{{ enrolling === m.customerId ? (photoTarget ? 'Enviando foto…' : 'Olhe o leitor…') : m.deviceUserId ? 'Refazer rosto' : 'Cadastrar rosto' }}</span>
              </button>
              <button class="link icon-btn" style="color: var(--success)" title="Confere a assinatura e abre a catraca se estiver em dia" aria-label="Liberar" @click="release(m)"><Icon name="liberar" /><span>Liberar</span></button>
              <span class="more">
                <button class="link icon-btn" title="Mais ações" aria-label="Mais ações" :aria-expanded="menu.openId.value === m.customerId" @click.stop="menu.toggle(m.customerId, $event)"><Icon name="mais" /><span>Mais</span></button>
                <div v-if="menu.openId.value === m.customerId" class="more-menu" :style="menu.style.value" role="menu" @click="menu.close">
                  <button role="menuitem" :disabled="enrolling !== 0 || !m.active" title="De frente, rosto inteiro e bem iluminado" @click="pickPhoto(m)"><Icon name="t-rosto" /> Rosto por foto</button>
                  <button v-if="m.deviceUserId && can('catraca_remover_rosto')" role="menuitem" :disabled="enrolling !== 0" @click="removeAsk = m"><Icon name="remover" /> Remover rosto</button>
                  <button v-if="can('catraca_apagar_aluno')" role="menuitem" class="danger-text" title="Apaga o rosto, cancela as assinaturas e desativa o cliente; o histórico fica" @click="deleteAsk = m"><Icon name="desativar" /> Apagar aluno</button>
                  <button v-if="can('apagar_pessoa_historico')" role="menuitem" class="danger-text" title="Com todo o histórico (vendas, mensalidades, passagens)" @click="purgeAsk = m"><Icon name="remover" /> Apagar de vez</button>
                </div>
              </span>
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
  <input ref="photoInput" type="file" accept="image/jpeg,image/png" style="display: none" @change="sendPhoto" />
  <PurgeCustomerDialog v-if="purgeAsk" :customer-id="purgeAsk.customerId" :name="purgeAsk.name" @done="purged" @close="purgeAsk = null" />
  <ConfirmDialog v-if="deleteAsk" title="Apagar aluno" danger
    :message="`Tirar ${deleteAsk.name} da academia? O rosto sai do leitor, as assinaturas ativas são canceladas e o cliente fica inativo. Vendas, mensalidades e passagens continuam guardadas.`"
    confirm-label="Apagar aluno" cancel-label="Voltar" @confirm="removeMember" @cancel="deleteAsk = null" />
  <ConfirmDialog v-if="removeAsk" title="Remover rosto"
    :message="`Apagar ${removeAsk.name} do leitor facial? O cliente e a assinatura continuam; para voltar a passar pelo leitor, cadastre o rosto de novo.`"
    confirm-label="Remover" danger @confirm="removeFace" @cancel="removeAsk = null" />
</template>

<style scoped>
/* Linhas baixas: plano e motivo da catraca numa linha só (o texto inteiro fica no title). */
td.clip { max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
td.gate-cell { max-width: 280px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.gate-reason { margin-left: 6px; }
.mode-tabs { margin-bottom: 12px; }
</style>
