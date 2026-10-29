<!-- Caminho: pdv/frontend/src/components/ReceiveCreditModal.vue -->
<script setup lang="ts">
// Recebimento de fiado: o valor abate a dívida (as notas mais antigas primeiro).
// Em dinheiro, calcula o troco a partir do que o cliente entregou (só na tela).
import { computed, reactive, watch } from 'vue'
import { PayCustomerCredit } from '../../wailsjs/go/main/App'
import { call, errorMessage, isCode, newRequestId } from '../api'
import { refreshCash } from '../composables/cashState'
import { centsToInput, money, parseMoney } from '../composables/format'
import { models, type Customer } from '../types'
import Modal from './Modal.vue'

// saleIds: notas escolhidas para pagar (vazio = quita as mais antigas).
const props = defineProps<{ customer: Customer; amountCents?: number; saleIds?: number[] }>()
const emit = defineEmits<{ close: []; paid: [payment: models.CreditPayment] }>()

const form = reactive({
  amount: centsToInput(props.amountCents ?? props.customer.creditBalanceCents), method: 'dinheiro', notes: '', given: '',
  busy: false, error: '', requestId: newRequestId(),
})
watch(() => [form.amount, form.method, form.notes], () => (form.requestId = newRequestId()))

const change = computed(() => {
  const amount = parseMoney(form.amount)
  const given = parseMoney(form.given)
  return form.method === 'dinheiro' && amount !== null && given !== null ? given - amount : null
})

async function save() {
  const amount = parseMoney(form.amount)
  if (amount === null || amount <= 0) return (form.error = 'Valor inválido.')
  if (amount > props.customer.creditBalanceCents) return (form.error = `O valor não pode passar da dívida atual (${money(props.customer.creditBalanceCents)}).`)
  if (change.value !== null && change.value < 0) return (form.error = 'O dinheiro entregue é menor que o valor a abater.')
  form.busy = true
  form.error = ''
  try {
    const p = await call(PayCustomerCredit(models.PayCreditInput.createFrom({
      customerId: props.customer.id, amountCents: amount, paymentMethod: form.method, notes: form.notes, requestId: form.requestId,
      saleIds: props.saleIds ?? [],
    })))
    emit('paid', p)
  } catch (e) {
    form.error = errorMessage(e)
    if (isCode(e, 'CAIXA_NAO_ABERTO')) refreshCash()
  } finally {
    form.busy = false
  }
}
</script>

<template>
  <Modal :title="`Receber a prazo — ${customer.name}`" :busy="form.busy" @close="emit('close')">
    <form class="stack" @submit.prevent="save">
      <p class="muted" style="margin: 0">Dívida atual: <strong class="mono">{{ money(customer.creditBalanceCents) }}</strong></p>
      <label class="field">Valor a abater da dívida (R$)<input v-model="form.amount" inputmode="decimal" required /></label>
      <label class="field">
        Forma de pagamento
        <select v-model="form.method">
          <option value="dinheiro">Dinheiro</option>
          <option value="pix">Pix</option>
          <option value="debito">Débito</option>
          <option value="credito">Crédito</option>
        </select>
      </label>
      <template v-if="form.method === 'dinheiro'">
        <label class="field">Dinheiro entregue pelo cliente (para troco)<input v-model="form.given" inputmode="decimal" placeholder="opcional" /></label>
        <div v-if="change !== null && change > 0" class="alert success"><strong>Troco: {{ money(change) }}</strong></div>
        <div v-else-if="change !== null && change < 0" class="alert error">Faltam {{ money(-change) }} para o valor a abater.</div>
      </template>
      <label class="field">Observação<input v-model="form.notes" maxlength="500" /></label>
      <p v-if="saleIds?.length" class="alert info" style="margin: 0">Pagando {{ saleIds.length === 1 ? 'a nota' : 'as notas' }} {{ saleIds.map((id) => `nº ${id}`).join(', ') }}.</p>
      <p v-else class="small muted" style="margin: 0">Sem nota escolhida, o valor quita as notas mais antigas primeiro.</p>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="emit('close')">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">{{ form.busy ? 'Registrando…' : 'Registrar recebimento' }}</button>
      </footer>
    </form>
  </Modal>
</template>
