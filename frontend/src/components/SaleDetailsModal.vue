<!-- Caminho: pdv/frontend/src/components/SaleDetailsModal.vue -->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { CancelSale, ChangeSalePaymentMethod, GetReceipt, GetSaleDetails } from '../../wailsjs/go/main/App'
import { firstPhone, openWhatsapp } from '../composables/charge'
import { call, errorMessage } from '../api'
import { storeName } from '../composables/cashState'
import { can } from '../composables/session'
import { formatDateTime, money, paymentLabels, saleMethodLabel } from '../composables/format'
import { useToast } from '../composables/toast'
import type { Receipt as ReceiptDoc, SaleDetails } from '../types'
import ConfirmDialog from './ConfirmDialog.vue'
import Modal from './Modal.vue'
import ReceiptModal from './ReceiptModal.vue'
import ReturnModal from './ReturnModal.vue'
import WhatsappAsk from './WhatsappAsk.vue'
import Receipt from './Receipt.vue'

const props = defineProps<{ saleId: number; allowCancel?: boolean; startWithReceipt?: boolean }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const toast = useToast()

const details = ref<SaleDetails | null>(null)

// Comprovante como texto pelo WhatsApp (não fiscal): o operador confere e envia.
function sendWhatsapp() {
  const d = details.value
  if (!d) return
  const lines = d.items.map((it) => `• ${it.qty} x ${it.productNameSnapshot}: ${money(it.subtotalCents - it.discountCents)}`)
  const text = [`*Comprovante da venda nº ${d.sale.id}* — ${storeName.value}`, formatDateTime(d.sale.createdAt), ...lines,
    `*Total: ${money(d.sale.totalCents)}* (${saleMethodLabel(d.sale)})`, '_Documento não fiscal._'].join('\n')
  // Celular do cliente cadastrado; senão o digitado na venda (entrega); senão pergunta.
  const phone = firstPhone(d.customer?.phone, d.delivery?.phone)
  if (phone) openWhatsapp(phone, text)
  else waText.value = text
}
const waText = ref('')
const error = ref('')
const showReceipt = ref(props.startWithReceipt ?? false)
const confirming = ref(false)
const busy = ref(false)
const receipt = ref<ReceiptDoc | null>(null)

async function load() {
  error.value = ''
  try {
    details.value = await call(GetSaleDetails(props.saleId))
    receipt.value = showReceipt.value ? await call(GetReceipt('venda', props.saleId)) : null
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)
watch(showReceipt, (v) => v && !receipt.value && load())

async function cancel(reason: string) {
  busy.value = true
  try {
    details.value = await call( CancelSale(props.saleId, reason))
    await load() // o comprovante passa a mostrar "VENDA CANCELADA"
    confirming.value = false
    const s = details.value.sale
    toast.success(
      s.totalCents === 0
        ? `Venda nº ${s.id} cancelada.`
        : s.paymentMethod === 'fiado'
        ? `Venda nº ${s.id} cancelada. A dívida do cliente foi reduzida.`
        : `Venda nº ${s.id} cancelada. Estorno de ${money(s.totalCents)} registrado no caixa: faça a devolução ao cliente.`,
    )
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}

// Troca da forma de pagamento: uma vez por venda, com motivo.
const changing = ref({ open: false, method: '', reason: '' })
function openChange() {
  changing.value = { open: true, method: details.value?.paymentChangeMethods?.[0] ?? '', reason: '' }
}
async function changePayment() {
  busy.value = true
  try {
    await call(ChangeSalePaymentMethod(props.saleId, changing.value.method, changing.value.reason))
    await load()
    changing.value.open = false
    toast.success(`Forma de pagamento da venda nº ${props.saleId} alterada para ${paymentLabels[changing.value.method]}.`)
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}

// Imprimir abre a escolha de formato (térmica ou A4).
const printing = ref(false)
// Quanto de cada item já voltou em devoluções.
const returnedQty = computed(() => {
  const out: Record<number, number> = {}
  for (const r of details.value?.returns ?? []) for (const it of r.items) out[it.saleItemId] = (out[it.saleItemId] ?? 0) + it.qty
  return out
})
const returnedTotal = computed(() => (details.value?.returns ?? []).reduce((a, r) => a + r.totalCents, 0))
const refundLabels: Record<string, string> = { vale_troca: 'Vale-troca', dinheiro: 'Dinheiro', fiado: 'Abatido no a prazo', pix: 'Pix', cartao: 'Estorno no cartão' }
const returning = ref(false)
</script>

<template>
  <Modal :title="`Venda nº ${saleId}`" wide @close="emit('close')">
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="!details" class="state">Carregando…</div>
    <template v-else>
      <div v-if="showReceipt && !printing" class="print-area">
        <Receipt v-if="receipt" :receipt="receipt" />
        <div v-else class="state">Carregando…</div>
      </div>
      <div v-else class="stack">
        <div class="row">
          <span class="badge" :class="details.sale.status === 'cancelada' ? 'bad' : 'ok'">
            {{ details.sale.status === 'cancelada' ? 'Cancelada' : 'Concluída' }}
          </span>
          <span class="muted small">{{ formatDateTime(details.sale.createdAt) }}</span>
          <span class="muted small">· {{ saleMethodLabel(details.sale) }}</span>
          <span v-if="details.sale.customerName" class="muted small">· {{ details.sale.customerName }}</span>
          <span v-if="details.sale.comandaId" class="muted small">· Comanda nº {{ details.sale.comandaId }}</span>
          <span v-if="details.sale.userName" class="muted small">· Operador: {{ details.sale.userName }}</span>
        </div>
        <div v-if="details.delivery" class="alert info" style="white-space: pre-line">
          <strong>Para entrega:</strong> {{ details.delivery.address }}<template v-if="details.delivery.phone"> · Tel. {{ details.delivery.phone }}</template><template v-if="details.delivery.notes"> · {{ details.delivery.notes }}</template>
        </div>
        <div v-if="details.sale.status === 'cancelada'" class="alert warn">
          Cancelada em {{ formatDateTime(details.sale.cancelledAt) }}<template v-if="details.sale.cancelledByName"> por {{ details.sale.cancelledByName }}</template>. Motivo: {{ details.sale.cancellationReason }}
        </div>
        <div v-if="details.paymentChange" class="alert info">
          Forma de pagamento trocada de {{ paymentLabels[details.paymentChange.fromMethod] }} para {{ paymentLabels[details.paymentChange.toMethod] }} em {{ formatDateTime(details.paymentChange.createdAt) }}<template v-if="details.paymentChange.userName"> por {{ details.paymentChange.userName }}</template><template v-if="details.paymentChange.authorizedByName"> (autorizado por {{ details.paymentChange.authorizedByName }})</template>. Motivo: {{ details.paymentChange.reason }}
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th class="left">Produto</th><th class="right">Qtd.</th><th v-if="details.returns?.length" class="right">Devolvido</th><th class="right">Unitário</th><th class="right">Subtotal</th></tr></thead>
            <tbody>
              <tr v-for="it in details.items" :key="it.id">
                <td class="left">{{ it.productNameSnapshot }}</td>
                <td class="right mono">{{ it.qty }}</td>
                <td v-if="details.returns?.length" class="right mono">
                  <span v-if="returnedQty[it.id]" class="badge warn">{{ returnedQty[it.id] }} de {{ it.qty }}</span>
                  <span v-else class="muted">—</span>
                </td>
                <td class="right mono">{{ money(it.unitPriceCents) }}</td>
                <td class="right mono">
                  {{ money(it.subtotalCents - it.discountCents) }}
                  <div v-if="it.discountCents" class="small muted">desc. -{{ money(it.discountCents) }}</div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="total-box">
          <div class="line"><span>Subtotal</span><span class="mono">{{ money(details.sale.subtotalCents) }}</span></div>
          <div class="line"><span>Desconto</span><span class="mono">-{{ money(details.sale.discountCents) }}</span></div>
          <div class="line grand"><span>Total</span><span class="mono">{{ money(details.sale.totalCents) }}</span></div>
          <div v-for="p in details.payments" :key="p.id" class="line small">
            <span>{{ paymentLabels[p.method] }}{{ p.cardBrand ? ` ${p.cardBrand}` : '' }}{{ p.installments > 1 ? ` ${p.installments}x` : '' }}</span>
            <span class="mono">{{ money(p.amountCents) }}<template v-if="p.changeCents"> (recebido {{ money(p.receivedCents) }}, troco {{ money(p.changeCents) }})</template></span>
          </div>
        </div>
        <section v-if="details.returns?.length" class="returns stack">
          <strong>Itens devolvidos</strong>
          <div v-for="r in details.returns" :key="r.id" class="small">
            <div><strong>Devolução nº {{ r.id }}</strong> · {{ formatDateTime(r.createdAt) }}<template v-if="r.userName"> · {{ r.userName }}</template> · {{ refundLabels[r.refundMethod] }} · <span class="mono">{{ money(r.totalCents) }}</span></div>
            <ul>
              <li v-for="(it, i) in r.items" :key="i">{{ it.qty }}× {{ it.productName }} <span class="mono muted">({{ money(it.amountCents) }})</span></li>
            </ul>
            <div class="muted">Motivo: {{ r.reason }}</div>
          </div>
          <div class="row between"><span>Total devolvido</span><strong class="mono">-{{ money(returnedTotal) }}</strong></div>
          <div class="row between"><span>Valor final da venda</span><strong class="mono">{{ money(details.sale.totalCents - returnedTotal) }}</strong></div>
        </section>
      </div>
    </template>
    <template #footer>
      <button v-if="details && allowCancel && can('cancelar_venda') && details.sale.status !== 'cancelada'" class="danger" @click="confirming = true">
        Cancelar venda
      </button>
      <button v-if="details && allowCancel && can('trocas_devolucoes') && details.sale.status !== 'cancelada'" @click="returning = true">
        Devolver itens
      </button>
      <button
        v-if="details && allowCancel && can('alterar_forma_pagamento') && details.sale.status !== 'cancelada' && !details.paymentChange"
        :disabled="!details.paymentChangeMethods?.length"
        :title="details.paymentChangeBlocked || 'Pode ser feito uma única vez nesta venda'"
        @click="openChange"
      >
        Trocar forma de pagamento
      </button>
      <span class="spacer" />
      <button v-if="details" title="Enviar o comprovante como texto pelo WhatsApp" @click="sendWhatsapp">WhatsApp</button>
      <button v-if="details" @click="showReceipt = !showReceipt">{{ showReceipt ? 'Ver detalhes' : 'Comprovante não fiscal' }}</button>
      <button v-if="showReceipt" class="primary" :disabled="!receipt || busy" @click="printing = true">Imprimir</button>
      <button @click="emit('close')">Fechar</button>
    </template>
  </Modal>
  <ReceiptModal v-if="printing" kind="venda" :id="saleId" @close="printing = false" />
  <ReturnModal v-if="returning" :sale-id="saleId" @close="returning = false" @changed="load(); emit('changed')" />
  <ConfirmDialog
    v-if="confirming"
    title="Cancelar venda"
    :message="`O estoque será devolvido e o cancelamento ficará registrado. Pagamentos em dinheiro, Pix ou cartão geram um estorno no caixa; a devolução do valor ao cliente deve ser feita pelo operador.`"
    reason-label="Motivo do cancelamento"
    confirm-label="Confirmar cancelamento"
    danger
    :busy="busy"
    @confirm="cancel"
    @cancel="confirming = false"
  />
  <Modal v-if="changing.open" :title="`Trocar forma de pagamento: venda nº ${saleId}`" :busy="busy" @close="changing.open = false">
    <form id="change-payment" class="stack" @submit.prevent="changePayment">
      <div class="alert warn">Esta troca só pode ser feita <strong>uma única vez</strong> nesta venda, com o caixa de hoje aberto, e não pode ser desfeita. O valor não muda.</div>
      <div class="row between"><span>Forma atual</span><strong>{{ paymentLabels[details?.sale.paymentMethod ?? ''] }}</strong></div>
      <label class="field">Nova forma
        <select v-model="changing.method" required>
          <option v-for="m in details?.paymentChangeMethods" :key="m" :value="m">{{ paymentLabels[m] }}</option>
        </select>
      </label>
      <label class="field">Motivo da troca<input v-model="changing.reason" type="text" required maxlength="200" /></label>
    </form>
    <template #footer>
      <span class="spacer" />
      <button :disabled="busy" @click="changing.open = false">Voltar</button>
      <button type="submit" form="change-payment" class="primary" :disabled="busy || !changing.method || !changing.reason.trim()">Confirmar troca</button>
    </template>
  </Modal>
  <WhatsappAsk v-if="waText" :text="waText" @close="waText = ''" />
</template>

<style scoped>
.returns { border: 1px solid var(--warning); background: var(--warning-soft); border-radius: var(--radius); padding: 10px 12px; gap: 6px; }
.returns ul { margin: 2px 0; padding-left: 18px; }
</style>
