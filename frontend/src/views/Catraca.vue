<!-- Caminho: catraca/frontend/src/views/Catraca.vue -->
<script setup lang="ts">
// Monitor da catraca: situação do leitor facial, passagens ao vivo, liberação
// do aluno no balcão (busca o cliente e confere a assinatura) e liberação
// manual com motivo.
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { GateCheckAndRelease, GateManualRelease, GateStatus, ListGateEvents, ListGateMembers } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'
import Modal from '../components/Modal.vue'
import { formatDateTime } from '../composables/format'

const MAX_EVENTS = 100
const status = ref<models.GateStatus | null>(null)
const events = ref<models.GateEvent[]>([])
const error = ref('')

// Verificação no balcão: busca o aluno e libera se puder.
const term = ref('')
const results = ref<models.GateMember[]>([])
const check = reactive({ busy: false, member: null as models.GateMember | null, result: null as models.GateCheck | null, error: '' })
const manual = reactive({ open: false, reason: '', busy: false, error: '' })

async function loadStatus() {
  try {
    status.value = await call(GateStatus())
    error.value = ''
  } catch (e) {
    error.value = errorMessage(e)
  }
}

let timer: number | undefined
let offAcesso: (() => void) | undefined
onMounted(async () => {
  loadStatus()
  try {
    events.value = await call(ListGateEvents(MAX_EVENTS))
  } catch (e) {
    error.value = errorMessage(e)
  }
  // Passagens chegam ao vivo; a situação do aparelho é relida a cada 5 s.
  // O aviso com a foto (AccessToast, em App.vue) também escuta "acesso".
  offAcesso = EventsOn('acesso', (e: models.GateEvent) => {
    events.value = [e, ...events.value].slice(0, MAX_EVENTS)
    loadStatus()
  })
  timer = window.setInterval(loadStatus, 5000)
})
onBeforeUnmount(() => {
  offAcesso?.()
  window.clearInterval(timer)
})

let searchTimer: number | undefined
watch(term, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(async () => {
    const t = term.value.trim()
    results.value = t ? (await call(ListGateMembers(t)).catch(() => [])).slice(0, 8) : []
  }, 200)
})

async function checkMember(m: models.GateMember) {
  Object.assign(check, { busy: true, member: m, result: null, error: '' })
  term.value = ''
  results.value = []
  try {
    check.result = await call(GateCheckAndRelease(m.customerId))
  } catch (e) {
    check.error = errorMessage(e)
  } finally {
    check.busy = false
  }
}

async function releaseManual() {
  manual.busy = true
  manual.error = ''
  try {
    await call(GateManualRelease(manual.reason))
    manual.open = false
  } catch (e) {
    manual.error = errorMessage(e)
  } finally {
    manual.busy = false
  }
}

const deviceState = computed(() => {
  const s = status.value
  if (!s) return { text: '…', cls: '' }
  if (!s.serverRunning) return { text: 'Servidor parado', cls: 'bad' }
  if (!s.deviceLastSeen) return { text: 'Aguardando o iDFace', cls: 'warn' }
  const secs = (Date.now() - new Date(s.deviceLastSeen).getTime()) / 1000
  return secs < 120 ? { text: 'iDFace conectado', cls: 'ok' } : { text: `Sem contato há ${Math.round(secs / 60)} min`, cls: 'warn' }
})
const relayLabel: Record<string, string> = { idface: 'pelo iDFace', serial: 'pelo relé serial', nenhum: 'desligado (só registra)' }
const sourceLabel: Record<string, string> = { leitor: 'Leitor facial', balcao: 'Balcão', manual: 'Manual' }
</script>

<template>
  <div class="stack">
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-if="status?.serverError" class="alert error">{{ status.serverError }}</div>

    <div class="grid grid-4">
      <div class="stat">
        <div class="label">Leitor facial</div>
        <div class="value"><span class="badge" :class="deviceState.cls">{{ deviceState.text }}</span></div>
        <div v-if="status" class="small muted">Modo online: {{ status.localIps.join(' / ') || 'sem rede' }} : {{ status.config.serverPort }}</div>
      </div>
      <div class="stat">
        <div class="label">Acionamento</div>
        <div class="value" style="font-size: 16px">{{ status ? relayLabel[status.config.relay] : '…' }}</div>
      </div>
      <div class="stat">
        <div class="label">Liberados hoje</div>
        <div class="value" style="color: var(--success)">{{ status?.allowedToday ?? '…' }}</div>
      </div>
      <div class="stat">
        <div class="label">Negados hoje</div>
        <div class="value" style="color: var(--danger)">{{ status?.deniedToday ?? '…' }}</div>
      </div>
    </div>

    <section class="card stack">
      <div class="row">
        <div style="position: relative; flex: 1; max-width: 520px">
          <input v-model="term" type="search" placeholder="Buscar aluno por nome, telefone ou CPF para liberar" aria-label="Buscar aluno" style="width: 100%" />
          <div v-if="results.length" class="user-pop" style="left: 0; right: 0">
            <button v-for="m in results" :key="m.customerId" @click="checkMember(m)">
              {{ m.name }} <span class="small muted">· {{ m.plans.map((p) => p.name).join(', ') || 'sem plano' }}</span>
            </button>
          </div>
        </div>
        <span class="spacer" />
        <button class="primary" @click="Object.assign(manual, { open: true, reason: '', error: '' })">Liberação manual</button>
      </div>
      <div v-if="check.busy" class="state">Verificando {{ check.member?.name }}…</div>
      <div v-else-if="check.error" class="alert error">{{ check.member?.name }}: {{ check.error }}</div>
      <div v-else-if="check.result" class="alert" :class="check.result.allowed ? 'success' : 'error'">
        <strong>{{ check.member?.name }}</strong> — {{ check.result.allowed ? 'LIBERADO' : 'NEGADO' }}: {{ check.result.reason }}
        <span v-if="check.result.allowed && !check.result.released"> (acionamento desligado: a catraca não foi aberta)</span>
        <RouterLink v-if="!check.result.allowed" to="/assinaturas"> · Ir para Assinaturas</RouterLink>
      </div>
    </section>

    <section class="card stack">
      <h2 style="margin: 0">Últimas passagens</h2>
      <div v-if="events.length === 0" class="state">Nenhuma passagem registrada ainda.</div>
      <div v-else class="table-wrap">
        <table>
          <thead><tr><th>Hora</th><th class="left">Aluno</th><th>Situação</th><th class="left">Motivo</th><th>Origem</th></tr></thead>
          <tbody>
            <tr v-for="e in events" :key="e.id">
              <td class="mono nowrap">{{ formatDateTime(e.at) }}</td>
              <td class="left">{{ e.name || '—' }}</td>
              <td><span class="badge" :class="e.allowed ? 'ok' : 'bad'">{{ e.allowed ? 'Liberado' : 'Negado' }}</span></td>
              <td class="left small">{{ e.reason }}</td>
              <td class="small muted nowrap">{{ sourceLabel[e.source] ?? e.source }}<template v-if="e.userName"> · {{ e.userName }}</template></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>

  <Modal v-if="manual.open" title="Liberação manual" :busy="manual.busy" @close="manual.open = false">
    <form class="stack" @submit.prevent="releaseManual">
      <label class="field">Motivo (fica registrado)<input v-model="manual.reason" maxlength="200" placeholder="Ex.: visitante, aula experimental, teste" required /></label>
      <div v-if="manual.error" class="alert error" role="alert">{{ manual.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="manual.busy" @click="manual.open = false">Cancelar</button>
        <button type="submit" class="success" :disabled="manual.busy">Liberar catraca</button>
      </footer>
    </form>
  </Modal>
</template>
