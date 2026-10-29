<!-- Caminho: catraca/frontend/src/views/CatracaFuncionarios.vue -->
<script setup lang="ts">
// Funcionários na catraca: passam sempre, sem plano. Não são os usuários do
// sistema (operadores de caixa): por baixo são clientes marcados como
// funcionário, então o rosto e as passagens funcionam como os dos alunos.
// Desligar volta a pessoa a cliente comum (a catraca passa a negar).
import { onMounted, reactive, ref, watch } from 'vue'
import { EnrollFace, GateCheckAndRelease, ListGateMembers, ListGateStaff, RemoveFace, RemoveGateStaff, SaveGateMember, SaveGateStaff } from '../../wailsjs/go/main/App'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import FacePhotoModal from '../components/FacePhotoModal.vue'
import Icon from '../components/Icon.vue'
import Modal from '../components/Modal.vue'
import { formatDateTime } from '../composables/format'
import { useToast } from '../composables/toast'

type Member = models.GateMember

const toast = useToast()
const staff = ref<Member[]>([])
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  try {
    staff.value = await call(ListGateStaff())
    error.value = ''
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

function replace(m: Member) {
  staff.value = staff.value.map((x) => (x.customerId === m.customerId ? m : x))
}

// ---------- cadastro: pessoa nova ou cliente que já existe ----------
const form = reactive({
  open: false, editing: false, customerId: 0, existing: false, name: '', phone: '', role: '', term: '',
  found: [] as Member[], busy: false, error: '',
})
function openNew() {
  Object.assign(form, { open: true, editing: false, customerId: 0, existing: false, name: '', phone: '', role: '', term: '', found: [], busy: false, error: '' })
}
function editRole(m: Member) {
  Object.assign(form, { open: true, editing: true, customerId: m.customerId, existing: true, name: m.name, phone: '', role: m.role, term: '', found: [], busy: false, error: '' })
}
let timer: number | undefined
watch(() => form.term, (t) => {
  window.clearTimeout(timer)
  if (!t.trim()) {
    form.found = []
    return
  }
  timer = window.setTimeout(async () => {
    try {
      form.found = (await call(ListGateMembers(t))).filter((m) => !m.staff).slice(0, 8)
    } catch (e) {
      form.error = errorMessage(e)
    }
  }, 250)
})
function pick(m: Member) {
  Object.assign(form, { customerId: m.customerId, name: m.name, term: '', found: [] })
}
async function save() {
  form.busy = true
  form.error = ''
  try {
    const m = await call(SaveGateStaff(models.GateStaffInput.createFrom({
      customerId: form.existing ? form.customerId : 0, name: form.name, phone: form.phone, role: form.role,
    })))
    if (staff.value.some((x) => x.customerId === m.customerId)) replace(m)
    else await load()
    form.open = false
    toast.success(`${m.name} salvo como funcionário.`)
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}

// ---------- rosto, liberação, bloqueio e desligamento ----------
// photoOf: mostra a foto que o leitor guardou (logo após o cadastro ou ao clicar em "Cadastrado").
const photoOf = ref<Member | null>(null)
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

async function release(m: Member) {
  try {
    const r = await call(GateCheckAndRelease(m.customerId))
    if (r.allowed) toast.success(`${m.name}: liberado${r.released ? '' : ' (acionamento desligado: a catraca não foi aberta)'}.`)
    else toast.error(`${m.name}: ${r.reason}`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function toggleBlock(m: Member) {
  try {
    const r = await call(SaveGateMember(models.GateMemberInput.createFrom({ customerId: m.customerId, blocked: !m.blocked, freeUntil: m.freeUntil, notes: m.notes })))
    replace(r)
    toast.success(`${m.name}: ${r.blocked ? 'acesso bloqueado' : 'acesso desbloqueado'}.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

const dismissAsk = ref<Member | null>(null)
async function dismiss() {
  const m = dismissAsk.value!
  dismissAsk.value = null
  try {
    await call(RemoveGateStaff(m.customerId))
    staff.value = staff.value.filter((x) => x.customerId !== m.customerId)
    toast.success(`${m.name} desligado: a catraca passa a negar a entrada.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <section class="card stack">
    <div class="row">
      <p class="small muted" style="margin: 0">
        Funcionários passam na catraca a qualquer hora, sem plano. Não são os usuários do sistema (operadores de caixa).
      </p>
      <span class="spacer" />
      <button class="primary" @click="openNew">Novo funcionário</button>
    </div>
    <div v-if="loading && staff.length === 0" class="state">Carregando…</div>
    <div v-else-if="error" class="alert error">{{ error }} <button class="link" @click="load">Tentar novamente</button></div>
    <div v-else-if="staff.length === 0" class="state">Nenhum funcionário cadastrado.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th class="left">Funcionário</th><th class="left">Função</th><th>Catraca</th><th>Rosto</th><th>Último acesso</th><th></th></tr></thead>
        <tbody>
          <tr v-for="m in staff" :key="m.customerId" :class="{ inactive: m.blocked || !m.active }">
            <td class="left"><strong>{{ m.name }}</strong><div v-if="m.phone" class="small muted">{{ m.phone }}</div></td>
            <td class="left">{{ m.role || '—' }}</td>
            <td :title="m.reason">
              <span class="badge" :class="m.allowed ? 'ok' : 'bad'">{{ m.allowed ? 'Liberado' : m.blocked ? 'Bloqueado' : 'Negado' }}</span>
              <div v-if="!m.allowed" class="small muted">{{ m.reason }}</div>
            </td>
            <td class="nowrap">
              <button v-if="m.deviceUserId" type="button" class="badge ok" style="border: 0; cursor: pointer" :title="`Ver a foto (usuário ${m.deviceUserId} no iDFace)`" @click="photoOf = m">Cadastrado</button>
              <span v-else class="badge">Sem rosto</span>
            </td>
            <td class="small nowrap">{{ m.lastAccess ? formatDateTime(m.lastAccess) : '—' }}</td>
            <td class="right actions-cell wrap">
              <button class="link icon-btn" :disabled="enrolling !== 0 || !m.active" :title="m.deviceUserId ? 'Cadastrar o rosto de novo' : 'Cadastrar o rosto no leitor'"
                :aria-label="m.deviceUserId ? 'Refazer rosto' : 'Cadastrar rosto'" @click="enroll(m)">
                <Icon name="t-rosto" /><span>{{ enrolling === m.customerId ? 'Olhe o leitor…' : m.deviceUserId ? 'Refazer rosto' : 'Cadastrar rosto' }}</span>
              </button>
              <button v-if="m.deviceUserId" class="link icon-btn" :disabled="enrolling !== 0" title="Apagar o rosto do leitor" aria-label="Remover rosto" @click="removeAsk = m"><Icon name="remover" /><span>Remover rosto</span></button>
              <button class="link icon-btn" style="color: var(--success)" title="Abrir a catraca agora" aria-label="Liberar" @click="release(m)"><Icon name="liberar" /><span>Liberar</span></button>
              <button class="link icon-btn" title="Trocar a função" aria-label="Editar" @click="editRole(m)"><Icon name="edit" /><span>Editar</span></button>
              <button class="link icon-btn" :title="m.blocked ? 'Desbloquear o acesso' : 'Bloquear o acesso (a catraca nega)'" :aria-label="m.blocked ? 'Desbloquear' : 'Bloquear'" @click="toggleBlock(m)">
                <Icon :name="m.blocked ? 'ativar' : 'desativar'" /><span>{{ m.blocked ? 'Desbloquear' : 'Bloquear' }}</span>
              </button>
              <button class="link icon-btn" style="color: var(--danger)" title="Desligar: deixa de ser funcionário" aria-label="Desligar" @click="dismissAsk = m"><Icon name="cancelar" /><span>Desligar</span></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <Modal v-if="form.open" :title="form.editing ? `Funcionário · ${form.name}` : 'Novo funcionário'" :busy="form.busy" @close="form.open = false">
    <form class="stack" @submit.prevent="save">
      <template v-if="!form.editing">
        <div class="tabs seg" role="tablist" aria-label="Quem é">
          <button type="button" role="tab" :aria-selected="!form.existing" @click="Object.assign(form, { existing: false, customerId: 0, name: '' })">Pessoa nova</button>
          <button type="button" role="tab" :aria-selected="form.existing" @click="Object.assign(form, { existing: true, customerId: 0, name: '' })">Já é cliente</button>
        </div>
        <template v-if="!form.existing">
          <label class="field">Nome<input v-model="form.name" required maxlength="120" /></label>
          <label class="field">Telefone<input v-model="form.phone" maxlength="20" /></label>
        </template>
        <template v-else>
          <div v-if="form.customerId" class="row">
            <strong>{{ form.name }}</strong>
            <button type="button" class="link" @click="Object.assign(form, { customerId: 0, name: '' })">Trocar</button>
          </div>
          <template v-else>
            <label class="field">Buscar cliente<input v-model="form.term" type="search" placeholder="Nome, telefone ou CPF" /></label>
            <div v-if="form.found.length" class="stack" style="gap: 4px">
              <button v-for="m in form.found" :key="m.customerId" type="button" class="link" style="text-align: left" @click="pick(m)">
                {{ m.name }}<span v-if="m.phone" class="small muted"> · {{ m.phone }}</span>
              </button>
            </div>
            <p class="small muted" style="margin: 0">Cliente inativo é ativado ao virar funcionário.</p>
          </template>
        </template>
      </template>
      <label class="field">Função (opcional)<input v-model="form.role" maxlength="60" placeholder="Professor, Recepção, Limpeza…" /></label>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy || (form.existing && !form.customerId)">Salvar</button>
      </footer>
    </form>
  </Modal>

  <FacePhotoModal v-if="photoOf" :customer-id="photoOf.customerId" :name="photoOf.name" @close="photoOf = null" />
  <ConfirmDialog v-if="removeAsk" title="Remover rosto"
    :message="`Apagar ${removeAsk.name} do leitor facial? Para voltar a passar pelo leitor, cadastre o rosto de novo.`"
    confirm-label="Remover" danger @confirm="removeFace" @cancel="removeAsk = null" />
  <ConfirmDialog v-if="dismissAsk" title="Desligar funcionário" danger
    :message="`${dismissAsk.name} deixa de ser funcionário e a catraca passa a negar a entrada. O cadastro e as passagens continuam guardados.`"
    confirm-label="Desligar" cancel-label="Voltar" @confirm="dismiss" @cancel="dismissAsk = null" />
</template>
