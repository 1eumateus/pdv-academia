<!-- Caminho: pdv/frontend/src/components/SubscriptionImport.vue -->
<script setup lang="ts">
// Importar assinaturas por planilha: a de clientes (com o plano) e, opcional, a
// de mensalidades (histórico). Primeiro mostra a prévia; importar grava.
import { ref } from 'vue'
import { ImportSubscriptions, SaveImportTemplate } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { money } from '../composables/format'
import { useToast } from '../composables/toast'
import type { models } from '../types'
import ConfirmDialog from './ConfirmDialog.vue'

const emit = defineEmits<{ done: [] }>()
const toast = useToast()
const clients = ref<File | null>(null)
const invoices = ref<File | null>(null)
const result = ref<models.SubscriptionImport | null>(null)
const busy = ref(false)
const error = ref('')
const confirm = ref(false)

function read(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result).split(',')[1] ?? '')
    r.onerror = () => reject(new Error('Não foi possível ler o arquivo.'))
    r.readAsDataURL(file)
  })
}

async function run(apply: boolean) {
  if (!clients.value) return (error.value = 'Escolha a planilha de clientes.')
  busy.value = true
  error.value = ''
  confirm.value = false
  try {
    const inv = invoices.value
    result.value = await call(ImportSubscriptions(clients.value.name, await read(clients.value), inv?.name ?? '', inv ? await read(inv) : '', apply))
    if (apply) {
      toast.success('Assinaturas importadas.')
      emit('done')
    }
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

async function template(kind: 'clientes' | 'mensalidades') {
  try {
    const path = await call(SaveImportTemplate('assinaturas_' + kind))
    if (path) toast.success(`Modelo salvo em ${path}.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
const pick = (ev: Event) => (ev.target as HTMLInputElement).files?.[0] ?? null
</script>

<template>
  <div class="stack">
    <p class="small muted" style="margin: 0">
      <strong>Clientes</strong> (obrigatória): um cliente por linha, com o código, o nome e o plano (nome, duração em dias, valor e próximo vencimento).
      <strong>Mensalidades</strong> (opcional): o histórico, com o código do cliente, plano, vencimento, valor e se foi pago.
      Com o histórico, o próximo vencimento é o último vencimento pago + a duração do plano.
      Cria os planos que faltam, os clientes e uma assinatura por cliente, sem mexer no caixa nem nas vendas.
      Cliente que já tem assinatura é pulado, então dá para importar de novo para incluir clientes novos.
    </p>
    <div class="row">
      <button @click="template('clientes')">Baixar modelo de clientes</button>
      <button @click="template('mensalidades')">Baixar modelo de mensalidades</button>
    </div>
    <div class="grid grid-2">
      <label class="field">Planilha de clientes<input type="file" accept=".xlsx,.csv" @change="clients = pick($event); result = null" /></label>
      <label class="field">Planilha de mensalidades (opcional)<input type="file" accept=".xlsx,.csv" @change="invoices = pick($event); result = null" /></label>
    </div>
    <div class="row">
      <button :disabled="busy || !clients" @click="run(false)">{{ busy && !result ? 'Lendo…' : 'Ver prévia' }}</button>
      <button v-if="result && !result.applied" class="primary" :disabled="busy" @click="confirm = true">Importar</button>
    </div>
    <div v-if="error" class="alert error" role="alert">{{ error }}</div>

    <div v-if="result" class="stack" style="gap: 6px">
      <div :class="result.applied ? 'alert success' : 'alert info'" style="margin: 0">
        <strong>{{ result.applied ? 'Importado.' : 'Prévia (nada foi gravado ainda).' }}</strong>
        Clientes novos: <strong>{{ result.customersNew }}</strong> · já cadastrados: {{ result.customersExisting }}
        <template v-if="result.customersWithSub"> · já tinham assinatura (pulados): {{ result.customersWithSub }}</template>
        <template v-if="result.customersSkipped"> · com erro: {{ result.customersSkipped }}</template><br />
        Assinaturas ativas: <strong>{{ result.subsActive }}</strong> · canceladas (clientes inativos): {{ result.subsCancelled }}<br />
        Pagamentos no histórico: <strong>{{ result.invoicesPaid }}</strong> · mensalidades em aberto: {{ result.invoicesOpen }}
        · meses sem pagamento (ficam no histórico como "Não utilizou", sem cobrança): {{ result.invoicesUnpaidOld }}
        <template v-if="result.otherLines"><br />Mensalidades com dado inválido, não importadas: {{ result.otherLines }}</template>
        <template v-if="result.invoicesSkipped"><br />Mensalidades de código que não está na planilha de clientes: {{ result.invoicesSkipped }}</template>
        <template v-if="result.fixedWords"><br />Palavras com "�" corrigidas: {{ result.fixedWords }}</template>
      </div>
      <table>
        <thead><tr><th>Plano</th><th>Preço</th><th>Duração</th><th>Mensalidades</th><th></th></tr></thead>
        <tbody>
          <tr v-for="p in result.plans" :key="p.name">
            <td class="left">{{ p.name }}</td><td class="mono">{{ money(p.priceCents) }}</td><td>{{ p.days }} dias</td><td>{{ p.invoices }}</td>
            <td><span v-if="p.exists" class="badge">já existe</span><span v-else class="badge ok">novo</span></td>
          </tr>
        </tbody>
      </table>
      <p class="small muted" style="margin: 0">Cliente que paga valor diferente do plano fica com o valor especial dele.</p>
      <ul v-if="result.skipped?.length" class="small" style="margin: 0">
        <li v-for="s in result.skipped" :key="s">{{ s }}</li>
      </ul>
    </div>
  </div>
  <ConfirmDialog v-if="confirm" title="Importar assinaturas" confirm-label="Importar" :busy="busy"
    message="Gravar os clientes, planos e assinaturas da prévia? Faça um backup antes."
    @confirm="run(true)" @cancel="confirm = false" />
</template>
