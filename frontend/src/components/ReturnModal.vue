<!-- Caminho: pdv/frontend/src/components/ReturnModal.vue -->
<script setup lang="ts">
// Devolução de itens de uma venda: escolhe quantidades, forma de reembolso e motivo.
// O valor de cada item é o que o cliente pagou (com descontos); o backend calcula e confere tudo.
import { computed, onMounted, reactive, ref } from 'vue'
import { CreateReturn, GetReturnInfo } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { formatDateTime, money } from '../composables/format'
import { useToast } from '../composables/toast'
import { models } from '../types'
import CustomerPicker from './CustomerPicker.vue'
import Modal from './Modal.vue'
import ReceiptModal from './ReceiptModal.vue'

const props = defineProps<{ saleId: number }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const toast = useToast()

const info = ref<models.ReturnInfo | null>(null)
const error = ref('')
const busy = ref(false)
const qty = reactive<Record<number, number>>({})
const form = reactive({ method: 'vale_troca', customerId: null as number | null, reason: '' })
const receiptId = ref<number | null>(null)
const created = ref(false) // depois de registrar, fechar o comprovante fecha a devolução

const refundLabels: Record<string, string> = { vale_troca: 'Vale-troca', dinheiro: 'Dinheiro', fiado: 'Abatido no a prazo', pix: 'Pix', cartao: 'Estorno no cartão' }

onMounted(async () => {
  try {
    info.value = await call(GetReturnInfo(props.saleId))
    for (const it of info.value.items) qty[it.saleItemId] = 0
    // Começa pela mesma forma em que a venda foi paga.
    const m = info.value.methods ?? []
    if (info.value.hasFiado) form.method = 'fiado'
    else if (info.value.cardLeftCents > 0 && m.every((x) => ['debito', 'credito', 'cartao'].includes(x))) form.method = 'cartao'
    else if (m.length && m.every((x) => x === 'pix')) form.method = 'pix'
    else if (m.length && m.every((x) => x === 'dinheiro')) form.method = 'dinheiro'
  } catch (e) {
    error.value = errorMessage(e)
  }
})

const left = (it: models.ReturnableItem) => it.qty - it.returnedQty
// Prévia do valor (mesma conta do backend, só para mostrar; quem decide é o backend).
const share = (paid: number, sold: number, n: number) => Math.floor((paid * n) / sold)
const preview = (it: models.ReturnableItem) =>
  share(it.paidCents, it.qty, it.returnedQty + (qty[it.saleItemId] || 0)) - share(it.paidCents, it.qty, it.returnedQty)
const total = computed(() => (info.value?.items ?? []).reduce((a, it) => a + preview(it), 0))
const anyQty = computed(() => Object.values(qty).some((n) => n > 0))
const nothingLeft = computed(() => !!info.value && info.value.items.every((it) => left(it) === 0))
const needsCustomer = computed(() => form.method === 'vale_troca' && !info.value?.customer)

function setAll() {
  for (const it of info.value?.items ?? []) qty[it.saleItemId] = left(it)
}

async function submit() {
  if (!info.value) return
  error.value = ''
  for (const it of info.value.items) {
    const n = qty[it.saleItemId] || 0
    if (!Number.isInteger(n) || n < 0 || n > left(it)) return (error.value = `Quantidade inválida para "${it.productName}" (máximo ${left(it)}).`)
  }
  if (!anyQty.value) return (error.value = 'Informe a quantidade de pelo menos um item.')
  if (!form.reason.trim()) return (error.value = 'Informe o motivo da devolução.')
  if (needsCustomer.value && !form.customerId) return (error.value = 'Escolha o cliente que vai receber o vale-troca.')
  busy.value = true
  try {
    const r = await call(CreateReturn(models.ReturnInput.createFrom({
      saleId: props.saleId, refundMethod: form.method, reason: form.reason, customerId: form.customerId,
      items: info.value.items.filter((it) => qty[it.saleItemId] > 0).map((it) => ({ saleItemId: it.saleItemId, qty: qty[it.saleItemId] })),
    })))
    toast.success(form.method === 'dinheiro'
      ? `Devolução registrada. Entregue ${money(r.totalCents)} em dinheiro ao cliente (saiu do caixa).`
      : form.method === 'pix' ? `Devolução registrada. Faça o Pix de ${money(r.totalCents)} para o cliente.`
      : form.method === 'cartao' ? `Devolução registrada. Faça o estorno de ${money(r.totalCents)} na maquininha.`
      : form.method === 'vale_troca' ? `Devolução registrada. ${money(r.totalCents)} em vale-troca para ${r.customerName}.`
        : `Devolução registrada. ${money(r.totalCents)} abatido da dívida de ${r.customerName}.`)
    emit('changed')
    created.value = true
    receiptId.value = r.id
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal :title="`Devolução — venda nº ${saleId}`" wide :busy="busy" @close="emit('close')">
    <div v-if="!info && !error" class="state">Carregando…</div>
    <form v-else-if="info" class="stack" @submit.prevent="submit">
      <div v-if="nothingLeft" class="alert info">Todos os itens desta venda já foram devolvidos.</div>
      <template v-else>
        <div class="row between">
          <span class="muted small">Os itens voltam ao estoque. O valor é o que o cliente pagou, já com os descontos.</span>
          <button type="button" class="link" @click="setAll">Devolver tudo</button>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th class="left">Produto</th><th class="right">Vendido</th><th class="right">Já devolvido</th><th class="right">Devolver</th><th class="right">Valor</th></tr></thead>
            <tbody>
              <tr v-for="it in info.items" :key="it.saleItemId">
                <td class="left">{{ it.productName }}</td>
                <td class="right mono">{{ it.qty }}</td>
                <td class="right mono">{{ it.returnedQty || '—' }}</td>
                <td class="right">
                  <input v-if="left(it) > 0" v-model.number="qty[it.saleItemId]" type="number" min="0" :max="left(it)" class="qty-input"
                    :aria-label="`Quantidade a devolver de ${it.productName}`" />
                  <span v-else class="muted small">nada a devolver</span>
                </td>
                <td class="right mono">{{ qty[it.saleItemId] ? money(preview(it)) : '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <fieldset class="refund">
          <legend>Como devolver o valor</legend>
          <label class="check"><input v-model="form.method" type="radio" value="vale_troca" /> Vale-troca (fica de saldo para o cliente usar em outra compra)</label>
          <label class="check"><input v-model="form.method" type="radio" value="dinheiro" /> Dinheiro (sai da gaveta do caixa)</label>
          <label class="check"><input v-model="form.method" type="radio" value="pix" /> Pix (a loja faz o Pix para o cliente)</label>
          <label v-if="info.cardLeftCents > 0" class="check"><input v-model="form.method" type="radio" value="cartao" /> Estorno no cartão (faça o estorno na maquininha; até {{ money(info.cardLeftCents) }})</label>
          <label v-if="info.hasFiado" class="check"><input v-model="form.method" type="radio" value="fiado" /> Abater da dívida a prazo</label>
        </fieldset>
        <p v-if="form.method === 'vale_troca' && info.customer" class="small muted" style="margin: 0">
          Vale-troca para <strong>{{ info.customer.name }}</strong> (saldo atual {{ money(info.customer.storeCreditCents) }}).
        </p>
        <CustomerPicker v-if="needsCustomer" v-model="form.customerId" label="Cliente que recebe o vale-troca" required />
        <label class="field">Motivo<input v-model="form.reason" maxlength="500" placeholder="Ex.: tamanho errado, defeito" required /></label>

        <div class="row between total-line">
          <span>Total a devolver</span>
          <strong class="mono">{{ money(total) }}</strong>
        </div>
      </template>
      <div v-if="error" class="alert error" role="alert">{{ error }}</div>

      <template v-if="info.returns.length">
        <h3 style="margin: 8px 0 0">Devoluções já feitas</h3>
        <ul class="small" style="margin: 0; padding-left: 18px">
          <li v-for="r in info.returns" :key="r.id">
            Nº {{ r.id }} em {{ formatDateTime(r.createdAt) }}: {{ money(r.totalCents) }} — {{ refundLabels[r.refundMethod] }}
            ({{ r.items.map((i) => `${i.qty}× ${i.productName}`).join(', ') }}). Motivo: {{ r.reason }}
            <button type="button" class="link" @click="receiptId = r.id">comprovante</button>
          </li>
        </ul>
      </template>
    </form>
    <div v-else class="alert error">{{ error }}</div>
    <template #footer>
      <span class="spacer" />
      <button @click="emit('close')">Fechar</button>
      <button v-if="info && !nothingLeft" class="primary" :disabled="busy || !anyQty" @click="submit">{{ busy ? 'Registrando…' : 'Registrar devolução' }}</button>
    </template>
  </Modal>
  <ReceiptModal v-if="receiptId" kind="devolucao" :id="receiptId" @close="receiptId = null; created && emit('close')" />
</template>

<style scoped>
.qty-input { width: 80px; text-align: right; }
.refund { border: 1px solid var(--border); border-radius: var(--radius); padding: 8px 12px; display: flex; flex-direction: column; gap: 6px; margin: 0; }
.refund legend { font-weight: 600; padding: 0 4px; }
.refund label { display: flex; align-items: center; justify-content: flex-start; gap: 8px; text-align: left; cursor: pointer; }
.refund input[type='radio'] { width: 16px; height: 16px; min-height: 0; flex: none; margin: 0; padding: 0; }
.total-line { font-size: 17px; padding: 8px 0; border-top: 1px solid var(--border); }
</style>
