<!-- Caminho: catraca/frontend/src/views/Catraca.vue -->
<script setup lang="ts">
// Monitor da catraca: situação do leitor facial, passagens ao vivo, consulta
// do aluno (a busca só mostra se ele passa, não abre a catraca) e liberação
// manual com um clique.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { GateManualRelease, GateStatus, ListGateEvents, ListGateMembers } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'
import { formatDateTime } from '../composables/format'
import { useToast } from '../composables/toast'

const MAX_EVENTS = 100
const status = ref<models.GateStatus | null>(null)
const events = ref<models.GateEvent[]>([])
const error = ref('')

const toast = useToast()

// Consulta no balcão: busca o aluno e mostra se ele passa agora. Não libera.
const term = ref('')
const results = ref<models.GateMember[]>([])
const checked = ref<models.GateMember | null>(null)
const manualBusy = ref(false)

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

function showMember(m: models.GateMember) {
  checked.value = m
  term.value = ''
  results.value = []
}

// Liberação manual: um clique abre a catraca (fica registrada como manual).
async function releaseManual() {
  if (manualBusy.value) return
  manualBusy.value = true
  try {
    await call(GateManualRelease(''))
    toast.success('Catraca liberada.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    manualBusy.value = false
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
          <input v-model="term" type="search" placeholder="Consultar aluno por nome, telefone ou CPF" aria-label="Consultar aluno" style="width: 100%" />
          <div v-if="results.length" class="user-pop" style="left: 0; right: 0">
            <button v-for="m in results" :key="m.customerId" @click="showMember(m)">
              {{ m.name }} <span class="small muted">· {{ m.plans.map((p) => p.name).join(', ') || 'sem plano' }}</span>
            </button>
          </div>
        </div>
        <span class="spacer" />
        <button class="primary" :disabled="manualBusy" title="Abre a catraca agora (fica registrada como liberação manual)" @click="releaseManual">
          {{ manualBusy ? 'Liberando…' : 'Liberação manual' }}
        </button>
      </div>
      <div v-if="checked" class="alert" :class="checked.allowed ? 'success' : 'error'">
        <strong>{{ checked.name }}</strong> — {{ checked.allowed ? 'pode passar' : 'não pode passar' }}: {{ checked.reason }}
        <RouterLink to="/catraca/alunos"> · Abrir em Alunos e rostos</RouterLink>
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

</template>
