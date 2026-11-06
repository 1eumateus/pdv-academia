<!-- Caminho: catraca/frontend/src/components/SubscriptionActions.vue -->
<script setup lang="ts">
// Janelas das ações de assinatura (nova, receber, alterar, pausar, cancelar) e
// as ações diretas (retomar, reativar, liberar, desfazer fora do caixa). Usado
// na tela Assinaturas e na ficha do aluno; quem usa chama pelas funções expostas
// e recarrega no evento "changed". Fica no <body> (Teleport) porque a ficha do
// aluno é um formulário.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  CancelSubscription, ChangeSubscriptionPlan, CreateSubscription, EndSubscriptionExemption, GateCheckAndRelease, ListPlanRules, ListProducts, PauseSubscription,
  PaySubscriptionInvoice, PaySubscriptionOutside, ReactivateSubscription, ResumeSubscription, SetSubscriptionRules, UndoSubscriptionOutside,
} from '../../wailsjs/go/main/App'
import { call, errorMessage, newRequestId } from '../api'
import AccessRulesFields from './AccessRulesFields.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import CustomerPicker from './CustomerPicker.vue'
import Modal from './Modal.vue'
import PaymentPanel from './PaymentPanel.vue'
import ReceiptModal from './ReceiptModal.vue'
import { copyRules, noRules, ruleException, rulesKey } from '../composables/accessRules'
import { autoReceipt } from '../composables/cashState'
import { centsToInput, money, parseMoney, today } from '../composables/format'
import { evaluate, newPart } from '../composables/payments'
import { can } from '../composables/session'
import { brDate, planLabel } from '../composables/subscriptions'
import { useToast } from '../composables/toast'
import { models, type Product } from '../types'

type Sub = models.Subscription
const emit = defineEmits<{ changed: [] }>()
const toast = useToast()
const plans = ref<Product[]>([])
const planRules = ref<Record<number, models.AccessRules>>({})

onMounted(async () => {
  try {
    planRules.value = await call(ListPlanRules())
    plans.value = (await call(ListProducts())).filter((p) => p.planDays && p.active && !p.pricePending)
  } catch (e) {
    toast.error(errorMessage(e))
  }
})

// ---------- nova assinatura ----------
const form = reactive({ open: false, customerId: null as number | null, productId: null as number | null, price: '', own: false, rules: noRules(), busy: false, error: '' })
// Valor especial: vazio = preço do plano.
const specialPrice = (t: string) => (t.trim() === '' ? 0 : parseMoney(t))
function openCreate(customerId: number | null = null) {
  Object.assign(form, { open: true, customerId, productId: null, price: '', own: false, rules: noRules(), busy: false, error: '' })
}
async function create() {
  if (form.customerId == null || form.productId == null) return (form.error = 'Escolha o cliente e o plano.')
  form.busy = true
  form.error = ''
  try {
    const price = specialPrice(form.price)
    if (price === null) return (form.error = 'Valor especial inválido. Exemplo: 90,00')
    const s = await call(CreateSubscription(form.customerId, form.productId, price, ruleException(form.own, form.rules)))
    toast.success(`Assinatura de ${s.customerName} criada. Primeira fatura: ${money(s.openInvoice?.amountCents ?? 0)}, vence hoje.`)
    form.open = false
    emit('changed')
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}

// ---------- receber fatura ----------
const paying = ref<Sub | null>(null)
const parts = ref([newPart('dinheiro')])
const split = ref(false)
const payBusy = ref(false)
const payError = ref('')
const requestId = ref('')
const payAmount = computed(() => paying.value?.openInvoice?.amountCents ?? 0)
// Desconto ao receber e isenção (com motivo; isentar exige a permissão).
const discountText = ref('')
const payReason = ref('')
const exempt = ref(false)
const exemptUntil = ref('') // vazio = permanente
const discount = computed(() => (discountText.value.trim() === '' ? 0 : parseMoney(discountText.value)))
const discountError = computed(() => {
  if (discount.value === null) return 'Desconto inválido. Exemplo: 10,00'
  if (discount.value > 0 && discount.value >= payAmount.value) return 'O desconto precisa ser menor que a mensalidade.'
  return ''
})
const payTotal = computed(() => Math.max(payAmount.value - (discountError.value ? 0 : discount.value ?? 0), 0))
const pay = computed(() => evaluate(parts.value, payTotal.value, split.value))
// Pagar de verdade: nem isenção, nem baixa fora do caixa.
const charging = computed(() => !outside.value && !exempt.value)
const payBlocked = computed(() => {
  if (exempt.value) return !payReason.value.trim()
  if (outside.value) return false
  return !!discountError.value || !!pay.value.error
})
const receipt = ref<number | null>(null)
// Fora do caixa: pagamento retroativo de um caixa já fechado à mão. Isenção: não cobrar.
const canOutside = computed(() => can('pagamento_fora_caixa'))
const canExempt = computed(() => can('isentar_assinatura'))
const outside = ref(false)
const paidDate = ref(today())
// Próximo vencimento: sugere pagamento + duração do plano; pode ser trocado.
const nextDue = ref('')
const nextEdited = ref(false)
function addDays(date: string, days: number) {
  const d = new Date(date + 'T12:00')
  d.setDate(d.getDate() + days)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
function suggestNext() {
  if (!nextEdited.value && paying.value) nextDue.value = addDays(outside.value ? paidDate.value || today() : today(), paying.value.planDays)
}
watch([outside, paidDate], suggestNext)
// Isenção e baixa fora do caixa são caminhos diferentes: marcar um desmarca o outro.
watch(outside, (v) => { if (v) exempt.value = false })
watch(exempt, (v) => { if (v) outside.value = false })

function openPay(s: Sub) {
  paying.value = s
  parts.value = [newPart('dinheiro')]
  split.value = false
  payError.value = ''
  requestId.value = newRequestId()
  outside.value = false
  exempt.value = false
  exemptUntil.value = ''
  discountText.value = ''
  payReason.value = ''
  paidDate.value = today()
  nextEdited.value = false
  suggestNext()
}
async function confirmPay() {
  const s = paying.value
  if (!s?.openInvoice || payBlocked.value || payBusy.value) return
  payBusy.value = true
  payError.value = ''
  try {
    if (outside.value) {
      await call(PaySubscriptionOutside(s.openInvoice.id, paidDate.value, nextDue.value))
      toast.success(`Mensalidade de ${s.customerName} lançada como paga em ${brDate(paidDate.value)}, fora do caixa. Próxima: ${brDate(nextDue.value)}.`)
      paying.value = null
      emit('changed')
      return
    }
    const sale = await call(PaySubscriptionInvoice(models.PayInvoiceInput.createFrom({
      invoiceId: s.openInvoice.id, paymentMethod: parts.value[0]?.method ?? 'dinheiro', payments: exempt.value ? [] : pay.value.payments, requestId: requestId.value, nextDue: nextDue.value,
      discountCents: exempt.value ? 0 : discount.value ?? 0, exempt: exempt.value, exemptUntil: exempt.value ? exemptUntil.value : '', reason: payReason.value,
    })))
    toast.success(exempt.value
      ? `${s.customerName} isenta${exemptUntil.value ? ` até ${brDate(exemptUntil.value)}` : ' de forma permanente'}.`
      : `Fatura de ${s.customerName} recebida. A próxima vence em ${brDate(nextDue.value)}.`)
    paying.value = null
    if (autoReceipt.value) receipt.value = sale.sale.id
    emit('changed')
  } catch (e) {
    payError.value = errorMessage(e)
  } finally {
    payBusy.value = false
  }
}

// ---------- encerrar isenção ----------
const endExemptAsk = ref<Sub | null>(null)
const endExemptBusy = ref(false)
async function confirmEndExempt() {
  if (!endExemptAsk.value) return
  endExemptBusy.value = true
  try {
    const s = await call(EndSubscriptionExemption(endExemptAsk.value.id))
    toast.success(`Isenção de ${s.customerName} encerrada. A mensalidade de ${money(s.openInvoice?.amountCents ?? 0)} vence hoje.`)
    endExemptAsk.value = null
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    endExemptBusy.value = false
  }
}
const askEndExempt = (s: Sub) => (endExemptAsk.value = s)

async function undoOutside(invoiceId: number) {
  try {
    const s = await call(UndoSubscriptionOutside(invoiceId))
    toast.success(`Lançamento desfeito: a mensalidade de ${s.customerName} voltou a ficar em aberto.`)
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// ---------- alterar (trocar de plano) ----------
const change = reactive({ sub: null as Sub | null, productId: 0, price: '', own: false, rules: noRules(), busy: false, error: '' })
function openChange(s: Sub) {
  Object.assign(change, { sub: s, productId: s.productId, price: s.priceCents ? centsToInput(s.priceCents) : '', own: !!s.rules, rules: copyRules(s.rules), busy: false, error: '' })
}
// Trocar plano ou valor mexe na mensalidade (vence hoje); só as regras, não.
const planChanged = computed(() => !!change.sub && (change.productId !== change.sub.productId || (specialPrice(change.price) ?? -1) !== (change.sub.priceCents ?? 0)))
async function saveChange() {
  if (!change.sub || !change.productId) return (change.error = 'Escolha o novo plano.')
  change.busy = true
  change.error = ''
  try {
    const price = specialPrice(change.price)
    if (price === null) return (change.error = 'Valor especial inválido. Exemplo: 90,00')
    let s = change.sub
    const rules = ruleException(change.own, change.rules)
    if (rulesKey(rules) !== rulesKey(s.rules)) s = await call(SetSubscriptionRules(s.id, rules))
    if (planChanged.value) {
      s = await call(ChangeSubscriptionPlan(s.id, change.productId, price))
      toast.success(`Plano de ${s.customerName} alterado para ${s.planName}. A mensalidade de ${money(s.openInvoice?.amountCents ?? 0)} vence hoje.`)
    } else toast.success(`Assinatura de ${s.customerName} salva.`)
    change.sub = null
    emit('changed')
  } catch (e) {
    change.error = errorMessage(e)
  } finally {
    change.busy = false
  }
}

// ---------- pausar / retomar (permissão própria; sem ela os botões ficam inativos) ----------
const canPause = computed(() => can('pausar_assinatura'))
const noPauseTip = 'Peça ao administrador para liberar a permissão "pausar e retomar assinatura".'
const pause = reactive({ sub: null as Sub | null, reason: '', busy: false, error: '' })
function openPause(s: Sub) {
  Object.assign(pause, { sub: s, reason: '', busy: false, error: '' })
}
async function savePause() {
  if (!pause.sub) return
  pause.busy = true
  pause.error = ''
  try {
    const s = await call(PauseSubscription(pause.sub.id, pause.reason))
    toast.success(`Assinatura de ${s.customerName} pausada. Os dias pausados não contam.`)
    pause.sub = null
    emit('changed')
  } catch (e) {
    pause.error = errorMessage(e)
  } finally {
    pause.busy = false
  }
}
async function resume(s: Sub) {
  try {
    const r = await call(ResumeSubscription(s.id))
    toast.success(`Assinatura de ${r.customerName} retomada.${r.openInvoice ? ` A mensalidade agora vence em ${brDate(r.openInvoice.dueDate)}.` : ''}`)
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// Reativar: o cliente volta a ficar ativo e a mensalidade vence hoje (paga antes de entrar).
const reactivating = ref(0)
async function reactivate(s: Sub) {
  reactivating.value = s.id
  try {
    const r = await call(ReactivateSubscription(s.id))
    toast.success(`Assinatura de ${r.customerName} reativada no plano ${r.planName}. A mensalidade vence hoje: receba para liberar a catraca.`)
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    reactivating.value = 0
  }
}

// Liberar: confere a assinatura (como no balcão da catraca) e abre se estiver em dia.
async function release(s: Sub) {
  try {
    const r = await call(GateCheckAndRelease(s.customerId))
    if (r.allowed) toast.success(`${s.customerName}: liberado${r.released ? '' : ' (acionamento desligado: a catraca não foi aberta)'}.`)
    else toast.error(`${s.customerName}: ${r.reason}`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

const cancelAsk = ref<Sub | null>(null)
const cancelBusy = ref(false)
async function cancel() {
  if (!cancelAsk.value) return
  cancelBusy.value = true
  try {
    await call(CancelSubscription(cancelAsk.value.id))
    toast.success(`Assinatura de ${cancelAsk.value.customerName} cancelada.`)
    cancelAsk.value = null
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    cancelBusy.value = false
  }
}
const askCancel = (s: Sub) => (cancelAsk.value = s)

defineExpose({ openCreate, openPay, openChange, openPause, resume, reactivate, release, askCancel, askEndExempt, undoOutside, reactivating, canPause, noPauseTip, canOutside, canExempt, plans })
</script>

<template>
  <Teleport to="body">
    <Modal v-if="form.open" title="Nova assinatura" :busy="form.busy" @close="form.open = false">
      <form class="stack" @submit.prevent="create">
        <CustomerPicker v-model="form.customerId" label="Cliente" required hide-credit />
        <label class="field">Plano
          <select v-model="form.productId" required>
            <option :value="null" disabled>Escolha o plano</option>
            <option v-for="p in plans" :key="p.id" :value="p.id">{{ p.name }} · {{ planLabel(p.planDays ?? 0) }} · {{ money(p.priceCents) }}</option>
          </select>
        </label>
        <label class="field">Valor especial deste aluno (opcional)<input v-model="form.price" inputmode="decimal" placeholder="Vazio = preço do plano" /></label>
        <AccessRulesFields v-model="form.rules" v-model:own="form.own" with-plan :plan="form.productId ? planRules[form.productId] : null" />
        <p class="small muted" style="margin: 0">A primeira fatura vence hoje. Ao receber, a próxima vence na data do pagamento + a duração do plano.</p>
        <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
        <footer class="row end">
          <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
          <button type="submit" class="primary" :disabled="form.busy">{{ form.busy ? 'Salvando…' : 'Criar assinatura' }}</button>
        </footer>
      </form>
    </Modal>

    <Modal v-if="paying?.openInvoice" :title="`Receber fatura de ${paying.customerName}`" :busy="payBusy" @close="paying = null">
      <form class="stack" @submit.prevent="confirmPay">
        <p style="margin: 0">{{ paying.planName }} · vencimento {{ brDate(paying.openInvoice.dueDate) }} · <strong class="mono">{{ money(payAmount) }}</strong></p>
        <label v-if="canOutside" class="check"><input v-model="outside" type="checkbox" /> Já foi pago fora do caixa (não lançar no caixa)</label>
        <label v-if="canExempt" class="check"><input v-model="exempt" type="checkbox" /> Isentar de pagamento (não cobrar nada)</label>
        <label v-if="outside" class="field">Pago em<input v-model="paidDate" type="date" :max="today()" required /></label>
        <p v-if="outside" class="small muted" style="margin: 0">A mensalidade fica paga sem venda e sem mexer no caixa (ex.: caixa já fechado à mão). A catraca libera normalmente.</p>
        <template v-if="exempt">
          <label class="field">Isenta até (vazio = permanente)<input v-model="exemptUntil" type="date" :min="today()" /></label>
          <label class="field">Motivo da isenção<input v-model="payReason" maxlength="200" required placeholder="Ex.: funcionário, cortesia" /></label>
          <p class="small muted" style="margin: 0">
            <template v-if="exemptUntil">A cobrança volta em {{ brDate(addDays(exemptUntil, 1)) }}, com o valor normal.</template>
            <template v-else><strong>Sem data, a isenção é permanente</strong>: não gera cobrança até alguém com a permissão encerrar.</template>
            A catraca libera normalmente. Fica registrada uma venda de R$ 0,00, sem entrada no caixa.
          </p>
        </template>
        <template v-if="charging">
          <div class="row">
            <label class="field">Desconto (R$)<input v-model="discountText" inputmode="decimal" placeholder="0,00" /></label>
            <label v-if="(discount ?? 0) > 0" class="field" style="flex: 1">Motivo do desconto (opcional)<input v-model="payReason" maxlength="200" /></label>
          </div>
          <div v-if="discountError" class="alert error" role="alert">{{ discountError }}</div>
          <p v-else-if="(discount ?? 0) > 0" style="margin: 0">A receber: <strong class="mono">{{ money(payTotal) }}</strong></p>
          <PaymentPanel v-model:parts="parts" v-model:split="split" :total="payTotal" :busy="payBusy" :exclude="['fiado']" />
        </template>
        <label v-if="!exempt" class="field">Próximo vencimento<input v-model="nextDue" type="date" required @input="nextEdited = true" /></label>
        <div v-if="payError" class="alert error" role="alert">{{ payError }}</div>
        <footer class="row end">
          <button type="button" :disabled="payBusy" @click="paying = null">Cancelar</button>
          <button type="submit" class="success" :disabled="payBusy || payBlocked">{{ payBusy ? 'Registrando…' : outside ? 'Lançar como pago' : exempt ? 'Confirmar isenção' : 'Confirmar recebimento' }}</button>
        </footer>
      </form>
    </Modal>

    <Modal v-if="pause.sub" :title="`Pausar assinatura · ${pause.sub.customerName}`" :busy="pause.busy" @close="pause.sub = null">
      <form class="stack" @submit.prevent="savePause">
        <p style="margin: 0">Enquanto pausada, a mensalidade não vence e os dias não contam. Ao retomar, o vencimento anda os mesmos dias que ficou pausada.</p>
        <label class="field">Motivo (opcional)<input v-model="pause.reason" maxlength="200" placeholder="Ex.: viagem, lesão" /></label>
        <div v-if="pause.error" class="alert error" role="alert">{{ pause.error }}</div>
        <footer class="row end">
          <button type="button" :disabled="pause.busy" @click="pause.sub = null">Voltar</button>
          <button type="submit" class="primary" :disabled="pause.busy">{{ pause.busy ? 'Pausando…' : 'Pausar assinatura' }}</button>
        </footer>
      </form>
    </Modal>
    <ConfirmDialog v-if="cancelAsk" title="Cancelar assinatura" danger :busy="cancelBusy"
      :message="`Cancelar o plano ${cancelAsk.planName} de ${cancelAsk.customerName}? A fatura em aberto deixa de existir.`"
      confirm-label="Cancelar assinatura" cancel-label="Voltar" @confirm="cancel" @cancel="cancelAsk = null" />
    <ConfirmDialog v-if="endExemptAsk" title="Encerrar isenção" :busy="endExemptBusy"
      :message="`Encerrar a isenção de ${endExemptAsk.customerName}? A mensalidade de ${money(endExemptAsk.openInvoice?.amountCents ?? 0)} passa a vencer hoje.`"
      confirm-label="Encerrar isenção" cancel-label="Voltar" @confirm="confirmEndExempt" @cancel="endExemptAsk = null" />
    <Modal v-if="change.sub" :title="`Alterar assinatura · ${change.sub.customerName}`" :busy="change.busy" @close="change.sub = null">
      <form class="stack" @submit.prevent="saveChange">
        <p style="margin: 0">Plano atual: <strong>{{ change.sub.planName }}</strong> · {{ planLabel(change.sub.planDays) }}</p>
        <label class="field">Novo plano
          <select v-model.number="change.productId" required>
            <option :value="0" disabled>Escolha o plano</option>
            <option v-for="p in plans" :key="p.id" :value="p.id">{{ p.name }} · {{ planLabel(p.planDays ?? 0) }} · {{ money(p.priceCents) }}{{ p.id === change.sub?.productId ? ' (atual)' : '' }}</option>
          </select>
        </label>
        <label class="field">Valor especial deste aluno (opcional)<input v-model="change.price" inputmode="decimal" placeholder="Vazio = preço do plano" /></label>
        <AccessRulesFields v-model="change.rules" v-model:own="change.own" with-plan :plan="planRules[change.productId]" />
        <p v-if="planChanged" class="small muted" style="margin: 0">A mensalidade em aberto passa a ter o valor do novo plano e vence hoje. Quando a pessoa pagar, a próxima vence na data do pagamento + a duração do novo plano.</p>
        <div v-if="change.error" class="alert error" role="alert">{{ change.error }}</div>
        <footer class="row end">
          <button type="button" :disabled="change.busy" @click="change.sub = null">Cancelar</button>
          <button type="submit" class="primary" :disabled="change.busy">{{ change.busy ? 'Salvando…' : 'Salvar' }}</button>
        </footer>
      </form>
    </Modal>
    <ReceiptModal v-if="receipt" kind="venda" :id="receipt" @close="receipt = null" />
  </Teleport>
</template>
