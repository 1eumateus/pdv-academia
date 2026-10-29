<!-- Caminho: catraca/frontend/src/views/CatracaConfig.vue -->
<script setup lang="ts">
// Configuração da catraca: quem abre o giro, o leitor facial iDFace (modo
// online) e o relé serial. Exige a permissão da catraca.
import { onMounted, reactive, ref } from 'vue'
import { GateConfig, GateManualRelease, GateStatus, SaveGateConfig, TestGateDevice } from '../../wailsjs/go/main/App'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'

const cfg = ref<models.GateConfig | null>(null)
const ips = ref<string[]>([])
const ports = ref<string[]>([])
const state = reactive({ busy: false, error: '', notice: '' })

onMounted(async () => {
  try {
    cfg.value = await call(GateConfig())
    const s = await call(GateStatus())
    ips.value = s.localIps
    ports.value = s.serialPorts
  } catch (e) {
    state.error = errorMessage(e)
  }
})

async function run(fn: () => Promise<string>) {
  state.busy = true
  state.error = state.notice = ''
  try {
    state.notice = await fn()
  } catch (e) {
    state.error = errorMessage(e)
  } finally {
    state.busy = false
  }
}

const save = () => run(async () => {
  cfg.value = await call(SaveGateConfig(cfg.value!))
  return 'Configuração salva.'
})
const test = () => run(async () => {
  cfg.value = await call(SaveGateConfig(cfg.value!))
  const n = await call(TestGateDevice())
  return `iDFace respondeu: ${n} pessoa(s) cadastrada(s) no aparelho.`
})
const testRelease = () => run(async () => {
  cfg.value = await call(SaveGateConfig(cfg.value!))
  await call(GateManualRelease('Teste de acionamento (configuração da catraca)'))
  return 'Comando de liberação enviado. Confira se a catraca liberou o giro.'
})
</script>

<template>
  <section v-if="cfg" class="card stack" style="max-width: 900px">
    <form class="stack" @submit.prevent="save">
      <h2 style="margin: 0">Acionamento da catraca</h2>
      <div class="grid grid-2">
        <label class="field">Quem abre a catraca
          <select v-model="cfg.relay">
            <option value="idface">iDFace (SecBox ou relé interno)</option>
            <option value="serial">Placa de relé na porta serial</option>
            <option value="nenhum">Ninguém (só registrar)</option>
          </select>
        </label>
        <label class="field">Tolerância depois do vencimento da mensalidade (dias)<input v-model.number="cfg.toleranceDays" type="number" min="0" max="60" /></label>
      </div>
      <p class="small muted" style="margin: 0">
        Passa quem tem plano ativo com a mensalidade em dia (ou vencida há no máximo a tolerância), ou cortesia válida.
        Assinatura pausada, primeira mensalidade não paga, cliente inativo ou bloqueado não passam.
      </p>

      <h2 style="margin: 8px 0 0">Leitor facial (Control iD iDFace)</h2>
      <div class="grid grid-2">
        <label class="field">IP do iDFace<input v-model="cfg.deviceHost" placeholder="192.168.1.221" /></label>
        <label class="field">ID do SecBox (0 = relé interno)<input v-model.number="cfg.secBoxId" type="number" min="0" /></label>
        <label class="field">Usuário<input v-model="cfg.deviceLogin" autocomplete="off" /></label>
        <label class="field">Senha<input v-model="cfg.devicePassword" type="password" autocomplete="off" /></label>
        <label class="field">Porta do modo online neste PC<input v-model.number="cfg.serverPort" type="number" min="1" max="65535" /></label>
      </div>
      <div class="alert info small">
        No iDFace, ative o <strong>modo online</strong> com o servidor em
        <strong>{{ ips.join(' ou ') || 'IP deste computador' }}</strong>, porta <strong>{{ cfg.serverPort }}</strong>.
        A cada rosto reconhecido o aparelho pergunta a este programa se libera. Libere essa porta no Firewall do Windows (rede privada).
      </div>

      <h2 style="margin: 8px 0 0">Relé serial (catraca eletromecânica)</h2>
      <div class="grid grid-3">
        <label class="field">Porta
          <input v-model="cfg.serialPort" list="com-ports" placeholder="COM3" />
          <datalist id="com-ports"><option v-for="p in ports" :key="p" :value="p" /></datalist>
        </label>
        <label class="field">Sinal
          <select v-model="cfg.serialSignal"><option>DTR</option><option>RTS</option></select>
        </label>
        <label class="field">Tempo do pulso (ms)<input v-model.number="cfg.pulseMs" type="number" min="100" max="10000" step="100" /></label>
      </div>
      <p class="small muted" style="margin: 0">Portas encontradas: {{ ports.join(', ') || 'nenhuma' }}. Se a catraca não abrir com DTR, teste RTS.</p>

      <div v-if="state.error" class="alert error" role="alert">{{ state.error }}</div>
      <div v-if="state.notice" class="alert success">{{ state.notice }}</div>
      <footer class="row end">
        <button type="button" :disabled="state.busy" @click="test">Testar iDFace</button>
        <button type="button" :disabled="state.busy" @click="testRelease">Testar liberação</button>
        <button type="submit" class="primary" :disabled="state.busy">Salvar</button>
      </footer>
    </form>
  </section>
  <div v-else-if="state.error" class="alert error">{{ state.error }}</div>
</template>
