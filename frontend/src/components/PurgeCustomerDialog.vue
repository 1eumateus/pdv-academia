<!-- Caminho: catraca/frontend/src/components/PurgeCustomerDialog.vue -->
<script setup lang="ts">
// Apagar a pessoa (cliente ou aluno) com todo o histórico: mostra antes o que
// vai sumir e pede confirmação marcando a caixa. Não tem como desfazer.
import { computed, onMounted, ref } from 'vue'
import { PurgeCustomer, PurgeCustomerPreview } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { money } from '../composables/format'
import { useToast } from '../composables/toast'
import type { models } from '../types'
import Modal from './Modal.vue'

const props = defineProps<{ customerId: number; name: string }>()
const emit = defineEmits<{ done: []; close: [] }>()
const toast = useToast()
const plan = ref<models.CustomerPurge | null>(null)
const error = ref('')
const busy = ref(false)
const sure = ref(false)

onMounted(async () => {
  try {
    plan.value = await call(PurgeCustomerPreview(props.customerId))
  } catch (e) {
    error.value = errorMessage(e)
  }
})

// O que some, só o que existe.
const lines = computed(() => {
  const p = plan.value
  if (!p) return []
  return [
    p.sales && `${p.sales} venda(s), somando ${money(p.salesTotalCents)}; o estoque dos produtos volta`,
    p.subscriptions && `${p.subscriptions} assinatura(s) e ${p.invoices} mensalidade(s), pagas ou não`,
    p.creditPayments && `${p.creditPayments} recebimento(s) de fiado`,
    p.returns && `${p.returns} devolução(ões)`,
    p.passages && `${p.passages} passagem(ns) na catraca`,
    p.hasFace && 'o rosto no leitor facial',
  ].filter(Boolean) as string[]
})

async function confirm() {
  if (!plan.value) return
  busy.value = true
  error.value = ''
  try {
    await call(PurgeCustomer(props.customerId, plan.value.sales))
    toast.success(`${props.name} foi apagado(a) com todo o histórico.`)
    emit('done')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal title="Apagar com todo o histórico" :busy="busy" @close="emit('close')">
    <form class="stack" @submit.prevent="confirm">
      <div v-if="!plan && !error" class="state">Conferindo o histórico…</div>
      <template v-if="plan">
        <p style="margin: 0">Apagar <strong>{{ name }}</strong> de vez? Some junto:</p>
        <ul v-if="lines.length" style="margin: 0; padding-left: 20px">
          <li v-for="l in lines" :key="l">{{ l }}</li>
        </ul>
        <p v-else class="small muted" style="margin: 0">Não tem histórico: só o cadastro sai.</p>
        <div v-if="plan.debtCents > 0" class="alert warn" role="alert">
          {{ name }} ainda deve <strong>{{ money(plan.debtCents) }}</strong> no fiado. Essa dívida será perdida.
        </div>
        <div v-if="plan.storeCreditCents > 0" class="alert warn">Tem {{ money(plan.storeCreditCents) }} de vale-troca, que também some.</div>
        <div v-if="plan.cashSessions > 0" class="alert warn">
          {{ plan.cashSessions }} caixa(s) perdem esses lançamentos: os totais e relatórios desses dias mudam.
        </div>
        <label class="check"><input v-model="sure" type="checkbox" /> Entendo que isso não pode ser desfeito (só restaurando um backup).</label>
      </template>
      <div v-if="error" class="alert error" role="alert">{{ error }}</div>
      <footer class="row end">
        <button type="button" :disabled="busy" @click="emit('close')">Voltar</button>
        <button type="submit" class="danger" :disabled="busy || !plan || !sure">{{ busy ? 'Apagando…' : 'Apagar tudo' }}</button>
      </footer>
    </form>
  </Modal>
</template>
