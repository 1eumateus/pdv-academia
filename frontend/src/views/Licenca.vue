<!-- Caminho: pdv/frontend/src/views/Licenca.vue -->
<script setup lang="ts">
// Situação da licença, ID da máquina (para enviar ao suporte) e ativação.
// Quem decide se pode gravar é o backend; aqui só mostra e envia a licença.
import { computed, onMounted, ref } from 'vue'
import { ActivateLicense } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { licenseStatus, refreshLicense } from '../composables/cashState'
import { useToast } from '../composables/toast'

const toast = useToast()
const text = ref('')
const busy = ref(false)
const error = ref('')
onMounted(refreshLicense)

const st = computed(() => licenseStatus.value)
const stateLabels: Record<string, string> = {
  ativa: 'Licença ativa', teste: 'Período de teste', vencida: 'Licença vencida',
  teste_encerrado: 'Teste encerrado', relogio: 'Data do computador incorreta',
}
const fmtDate = (d: string) => d.split('-').reverse().join('/')

async function copyId() {
  try {
    await navigator.clipboard.writeText(st.value?.machineId ?? '')
    toast.success('ID da máquina copiado.')
  } catch {
    toast.error('Não foi possível copiar. Selecione o ID e copie com Ctrl+C.')
  }
}

// Arquivo de licença (.txt) enviado pelo suporte: só lê o texto para o campo.
function readFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  if (f.size > 10_000) return (error.value = 'Arquivo grande demais para ser uma licença.')
  const r = new FileReader()
  r.onload = () => (text.value = String(r.result))
  r.readAsText(f)
}

async function activate() {
  busy.value = true
  error.value = ''
  try {
    licenseStatus.value = await call(ActivateLicense(text.value))
    text.value = ''
    toast.success('Licença ativada.')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="stack" style="max-width: 760px">
    <section class="card stack">
      <h2>Situação</h2>
      <div v-if="!st" class="state">Carregando…</div>
      <template v-else>
        <div class="row" style="gap: 10px">
          <span class="badge" :class="st.canWrite ? (st.warning ? 'warn' : 'ok') : 'bad'">{{ stateLabels[st.state] ?? st.state }}</span>
          <span>{{ st.message }}</span>
        </div>
        <div v-if="!st.canWrite" class="alert error" style="margin: 0">
          O sistema está em <strong>modo somente consulta</strong>: dá para ver cadastros, vendas e relatórios, exportar e
          fazer backup, mas não registrar vendas nem alterar dados. Nada foi apagado.
        </div>
        <dl v-if="st.state === 'ativa' || st.state === 'vencida'" class="grid grid-2 small license-data" style="margin: 0">
          <div><dt class="muted">Loja</dt><dd>{{ st.store }}</dd></div>
          <div><dt class="muted">CNPJ/CPF</dt><dd>{{ st.document || '—' }}</dd></div>
          <div><dt class="muted">Plano</dt><dd>{{ st.plan || '—' }}</dd></div>
          <div><dt class="muted">Validade</dt><dd>{{ st.expires ? fmtDate(st.expires) : 'Vitalícia' }}</dd></div>
        </dl>
        <div v-if="st.licenseError" class="alert warn" style="margin: 0">A licença gravada não vale neste computador: {{ st.licenseError }}</div>
      </template>
    </section>

    <section class="card stack">
      <h2>ID desta máquina</h2>
      <p class="small muted" style="margin: 0">Envie este código ao suporte para receber a licença deste computador.</p>
      <div class="row" style="flex-wrap: nowrap">
        <input class="mono big" :value="st?.machineId ?? ''" readonly aria-label="ID da máquina" style="flex: 1" @focus="($event.target as HTMLInputElement).select()" />
        <button type="button" @click="copyId">Copiar</button>
      </div>
    </section>

    <section class="card stack">
      <h2>Ativar licença</h2>
      <form class="stack" @submit.prevent="activate">
        <label class="field">Cole aqui a licença recebida (começa com PDV1.)
          <textarea v-model="text" rows="4" class="mono" spellcheck="false" placeholder="PDV1.…" />
        </label>
        <label class="small">Ou abra o arquivo da licença: <input type="file" accept=".txt,.lic,text/plain" @change="readFile" /></label>
        <div v-if="error" class="alert error" role="alert">{{ error }}</div>
        <div class="row end"><button class="primary" type="submit" :disabled="busy || !text.trim()">{{ busy ? 'Verificando…' : 'Ativar' }}</button></div>
      </form>
    </section>
  </div>
</template>

<style scoped>
.license-data dd { margin: 0; font-weight: 600; }
</style>
