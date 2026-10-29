<!-- Caminho: pdv/frontend/src/views/Vendas.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { CreateSale, GetQuote, ListQuotes, ProductsForSale, SaveQuote } from '../../wailsjs/go/main/App'
import { call, errorMessage, isCode, newRequestId } from '../api'
import { can } from '../composables/session'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CustomerPicker from '../components/CustomerPicker.vue'
import DebtWarning from '../components/DebtWarning.vue'
import Modal from '../components/Modal.vue'
import PaymentPanel from '../components/PaymentPanel.vue'
import ReceiptModal from '../components/ReceiptModal.vue'
import PixModal from '../components/PixModal.vue'
import { evaluate, newPart } from '../composables/payments'
import ProductCatalog from '../components/ProductCatalog.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import { autoReceipt, cashOpen, refreshCash, settings } from '../composables/cashState'
import { directPrint, printReceipt } from '../composables/receipt'
import { centsToInput, formatAddress, formatDateTime, money, parseDiscount, parseQty, paymentLabels, today } from '../composables/format'
import { pendingQuote, type QuoteForSale } from '../composables/quoteToSale'
import { useToast } from '../composables/toast'
import { models, type CartLine, type Customer, type Product, type SaleDetails } from '../types'

const toast = useToast()
const search = ref<InstanceType<typeof ProductCatalog> | null>(null)

const cart = ref<CartLine[]>([])
// Quanto de cada produto já está no carrinho (a lista de produtos marca e desconta do estoque).
const inCart = computed(() => Object.fromEntries(cart.value.map((l) => [l.product.id, l.qty])) as Record<number, number>)
const parts = ref([newPart('dinheiro')])
const split = ref(false)
const customerId = ref<number | null>(null)
const customer = ref<Customer | null>(null) // para o saldo de vale-troca
const usesVale = computed(() => parts.value.some((p) => p.method === 'vale_troca'))
const discountText = ref('')
const busy = ref(false)
const error = ref('')
const lastSale = ref<SaleDetails | null>(null)
const showLast = ref(false)
const lastAsReceipt = ref(false) // abrir a última venda já no cupom (impressão automática/entrega)
// Chave de idempotência: muda sempre que o carrinho muda. Cliques repetidos
// no mesmo carrinho reenviam a mesma chave e não duplicam a venda.
const requestId = ref(newRequestId())

// Prévia para o operador. O valor final é sempre o calculado pelo backend.
// Prévia com a promoção vigente (o backend recalcula ao finalizar).
const unitPrice = (l: CartLine) => l.product.promo?.priceCents || l.product.priceCents
const promoOff = (l: CartLine) => {
  const p = l.product.promo
  return p && p.buyQty ? Math.floor(l.qty / p.buyQty) * (p.buyQty - p.payQty) * unitPrice(l) : 0
}
const subtotal = computed(() => cart.value.reduce((s, l) => s + l.qty * unitPrice(l) - promoOff(l), 0))
// Desconto de cada item (R$ ou %) e da venda (R$ ou % sobre o que sobra depois dos itens).
const lineTotal = (l: CartLine) => l.qty * unitPrice(l) - promoOff(l)
const lineDiscount = (l: CartLine) => parseDiscount(l.discountText, lineTotal(l))
const lineInvalid = (l: CartLine) => lineDiscount(l) === null || (lineDiscount(l) ?? 0) > lineTotal(l)
const itemsDiscount = computed(() => cart.value.reduce((a, l) => a + (lineInvalid(l) ? 0 : lineDiscount(l) ?? 0), 0))
const saleDiscount = computed(() => parseDiscount(discountText.value, subtotal.value - itemsDiscount.value))
const discount = computed(() => (saleDiscount.value === null ? null : saleDiscount.value + itemsDiscount.value))
const discountInvalid = computed(() => discount.value === null || (discount.value ?? 0) > subtotal.value || cart.value.some(lineInvalid))
const total = computed(() => subtotal.value - (discount.value ?? 0))
const pay = computed(() => evaluate(parts.value, discountInvalid.value ? subtotal.value : total.value, split.value))
const canFinish = computed(
  () => cart.value.length > 0 && !busy.value && !discountInvalid.value && !pay.value.error && !deliveryInvalid.value &&
    ((!pay.value.hasFiado && !usesVale.value) || customerId.value != null),
)

watch([cart, parts, split, customerId, discountText], () => (requestId.value = newRequestId()), { deep: true })

// Item que +/- e F9 alteram: o último adicionado ou clicado (senão, o último do carrinho).
const selectedId = ref<number | null>(null)
const current = () => cart.value.find((l) => l.product.id === selectedId.value) ?? cart.value[cart.value.length - 1]
// O foco volta sempre ao campo de código (leitor de código de barras).
const refocus = () => nextTick(() => search.value?.focus())

// Quantidade limitada ao estoque (produto que não controla estoque só ao máximo geral).
function limitQty(line: CartLine, q: number) {
  const max = line.product.noStock ? 100_000 : line.product.stockQty
  if (q > max) toast.error(`Só tem ${max} de "${line.product.name}" em estoque.`)
  line.qty = Math.max(Math.min(q, max, 100_000), 1)
}

function add(p: Product, qty = 1) {
  error.value = ''
  const line = cart.value.find((l) => l.product.id === p.id)
  if (line) limitQty(line, line.qty + qty)
  else cart.value.push({ product: p, qty, discountText: '' })
  selectedId.value = p.id
  refocus()
}

function setQty(line: CartLine, value: string) {
  const q = parseQty(value)
  if (q === null) {
    toast.error('Quantidade inválida: use um número inteiro maior que zero.')
    return
  }
  limitQty(line, q)
}

function remove(line: CartLine) {
  cart.value = cart.value.filter((l) => l !== line)
  refocus()
}

// ---------- venda para entrega ----------
// Endereço vem do cadastro do cliente (pode trocar por outro só nesta entrega).
const delivery = reactive({ on: false, address: '', phone: '', notes: '' })
function fillDelivery(force = false) {
  const c = customer.value
  if (!c) return
  if (force || !delivery.address.trim()) {
    delivery.address = [...formatAddress(c), c.reference ? `Ref.: ${c.reference}` : ''].filter(Boolean).join('\n')
  }
  if (force || !delivery.phone.trim()) delivery.phone = c.phone ?? ''
}
watch(() => [delivery.on, customer.value?.id], () => delivery.on && fillDelivery())
const deliveryInvalid = computed(() => delivery.on && !delivery.address.trim())

// Orçamento que veio para esta venda (fica "convertido" ao finalizar).
const quoteId = ref<number | null>(null)

// Carrega no carrinho o orçamento enviado pela tela de Orçamentos.
async function loadPendingQuote() {
  const q = pendingQuote.value
  pendingQuote.value = null
  if (q) await loadQuote(q)
}

// Põe um orçamento no carrinho (vindo de Orçamentos ou de "Abrir orçamento" aqui).
async function loadQuote(q: QuoteForSale) {
  try {
    const all = await call(ProductsForSale(q.items.map((it) => it.productId)))
    const skipped: string[] = []
    cart.value = []
    for (const it of q.items) {
      const p = all.find((x) => x.id === it.productId)
      if (!p || !p.active || p.pricePending) {
        skipped.push(p?.name ?? `produto ${it.productId}`)
        continue
      }
      if (!p.noStock && p.stockQty <= 0) {
        skipped.push(`${p.name} (sem estoque)`)
        continue
      }
      const line: CartLine = { product: p, qty: 1, discountText: it.discountCents ? centsToInput(it.discountCents) : '' }
      limitQty(line, it.qty)
      cart.value.push(line)
    }
    customerId.value = q.customerId
    discountText.value = q.saleDiscountCents ? centsToInput(q.saleDiscountCents) : ''
    quoteId.value = q.quoteId
    toast.info(`Orçamento nº ${q.quoteId} no carrinho. Confira, ajuste se quiser e finalize.`)
    if (skipped.length) toast.error(`Ficaram de fora (inativos, a precificar ou sem estoque): ${skipped.join(', ')}.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// ---------- orçamento na tela de vendas ----------
// O mesmo carrinho vira venda (F4) ou orçamento (F6). Validade padrão das Configurações;
// observação e validade, se o orçamento veio de "Orçamentos salvos", são mantidas.
const quoteNotes = ref('')
const quoteValid = ref('')
const quoteBusy = ref(false)
const quotePrintId = ref<number | null>(null)
// Salva o carrinho como orçamento (atualiza o orçamento que veio para cá, se houver).
async function saveAsQuote() {
  if (cart.value.length === 0 || quoteBusy.value) return
  if (discountInvalid.value) return (error.value = 'Corrija o desconto antes de salvar o orçamento.')
  quoteBusy.value = true
  error.value = ''
  try {
    const d = await call(SaveQuote(models.QuoteInput.createFrom({
      id: quoteId.value ?? 0, customerId: customerId.value, notes: quoteNotes.value, validUntil: quoteValid.value, discountCents: saleDiscount.value ?? 0,
      items: cart.value.map((l) => ({ productId: l.product.id, qty: l.qty, discountCents: lineDiscount(l) ?? 0 })),
    })))
    toast.success(`Orçamento nº ${d.quote.id} salvo: ${money(d.quote.totalCents)}, válido até ${d.quote.validUntil.split('-').reverse().join('/')}.`)
    clearCart()
    quotePrintId.value = d.quote.id
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    quoteBusy.value = false
    search.value?.focus()
  }
}

// Orçamentos abertos para trazer ao carrinho sem sair da tela.
const quotesOpen = ref(false)
const openQuotes = ref<models.Quote[]>([])
const quoteTerm = ref('')
const quotesError = ref('')
async function showQuotes() {
  quotesOpen.value = true
  quotesError.value = ''
  try {
    const list = await call(ListQuotes(today(-90), today(), quoteTerm.value))
    openQuotes.value = list.filter((q) => q.status === 'aberto' || q.status === 'aprovado').sort((a, b) => b.id - a.id)
  } catch (e) {
    quotesError.value = errorMessage(e)
  }
}
async function pickQuote(id: number) {
  try {
    const d = await call(GetQuote(id))
    const itemsDisc = d.items.reduce((a, it) => a + it.discountCents, 0)
    quotesOpen.value = false
    await loadQuote({ quoteId: id, customerId: d.quote.customerId ?? null, saleDiscountCents: d.quote.discountCents - itemsDisc,
      items: d.items.map((it) => ({ productId: it.productId, qty: it.qty, discountCents: it.discountCents })) })
    quoteNotes.value = d.quote.notes
    quoteValid.value = d.quote.validUntil
  } catch (e) {
    quotesError.value = errorMessage(e)
  }
}

function clearCart() {
  quoteId.value = null
  quoteNotes.value = ''
  quoteValid.value = ''
  cart.value = []
  discountText.value = ''
  customerId.value = null
  Object.assign(delivery, { on: false, address: '', phone: '', notes: '' })
  parts.value = [newPart('dinheiro')]
  split.value = false
  error.value = ''
}

// Com Pix e chave configurada, mostra o QR primeiro; a venda só é registrada
// depois que o operador confirma "Pix recebido".
const pixAmount = computed(() => pay.value.payments.filter((p) => p.method === 'pix').reduce((a, p) => a + p.amountCents, 0))
const pixOpen = ref(false)
// Venda a prazo para quem já deve: pede confirmação antes (uma vez por cliente).
const debtAsk = ref(false)
const debtOk = ref<number | null>(null)
watch(customerId, () => (debtOk.value = null))
function finish() {
  if (!canFinish.value) return
  if (pay.value.hasFiado && customer.value && customer.value.creditBalanceCents > 0 && debtOk.value !== customer.value.id) {
    debtAsk.value = true
    return
  }
  if (pixAmount.value > 0 && settings.value?.pixKey) pixOpen.value = true
  else doFinish()
}
function pixReceived() {
  pixOpen.value = false
  doFinish()
}

async function doFinish() {
  if (!canFinish.value) return
  busy.value = true
  error.value = ''
  try {
    const input = models.CreateSaleInput.createFrom({
      items: cart.value.map((l) => ({ productId: l.product.id, qty: l.qty, discountCents: lineDiscount(l) ?? 0 })),
      paymentMethod: parts.value[0]?.method ?? 'dinheiro',
      payments: pay.value.payments,
      customerId: customerId.value,
      discountCents: saleDiscount.value ?? 0,
      requestId: requestId.value,
      quoteId: quoteId.value,
      delivery: delivery.on ? { address: delivery.address, phone: delivery.phone, notes: delivery.notes } : undefined,
    })
    const forDelivery = delivery.on
    const result = await call( CreateSale(input))
    // Só limpa depois da confirmação do backend.
    lastSale.value = result
    checkoutOpen.value = false
    clearCart()
    // Entrega sempre sai impressa (o entregador leva o cupom com o endereço).
    if (autoReceipt.value || forDelivery) {
      // Com térmica configurada imprime direto; senão abre o comprovante na tela.
      if (directPrint.value) printReceipt('venda', result.sale.id)
      else {
        lastAsReceipt.value = true
        showLast.value = true
      }
    }
    toast.success(`Venda nº ${result.sale.id} registrada: ${money(result.sale.totalCents)} (${paymentLabels[result.sale.paymentMethod]}).`)
    search.value?.reload() // estoque mudou
  } catch (e) {
    error.value = errorMessage(e)
    if (isCode(e, 'CAIXA_NAO_ABERTO')) refreshCash()
  } finally {
    busy.value = false
    search.value?.focus()
  }
}

// ---------- tela de pagamento ----------
const checkoutOpen = ref(false)
// O cursor já vai para o primeiro campo de valor (recebido, no dinheiro), ou para o desconto (F7).
const payField = '.checkout-pay input:not([type="search"])'
const discountField = '.checkout-side input.disc-sale'
async function openCheckout(field = payField) {
  if (cart.value.length === 0 || busy.value) return
  error.value = ''
  checkoutOpen.value = true
  // (depois do foco automático da janela, que vai para o primeiro botão)
  await nextTick()
  setTimeout(() => {
    const el = document.querySelector<HTMLInputElement>(field)
    el?.focus()
    el?.select()
  }, 0)
}

const clearAsk = ref(false)
// Fechou a última janela: foco de volta no código de barras.
const anyModal = computed(() => clearAsk.value || checkoutOpen.value || quotesOpen.value || showLast.value || quotePrintId.value != null || pixOpen.value || debtAsk.value)
watch(anyModal, (open) => !open && refocus())

function onKey(e: KeyboardEvent) {
  // Com a tela de pagamento aberta: Enter (ou F4/F8) confirma. Enter já usado por um campo (ex.: busca de cliente) não conta.
  if (checkoutOpen.value) {
    const onButton = e.target instanceof HTMLButtonElement || e.target instanceof HTMLSelectElement
    if ((e.key === 'Enter' && !e.defaultPrevented && !onButton && !pixOpen.value && !debtAsk.value) || e.key === 'F4' || e.key === 'F8') {
      e.preventDefault()
      finish()
    } else if (e.key === 'F7') {
      e.preventDefault()
      document.querySelector<HTMLInputElement>(discountField)?.focus()
    }
    return
  }
  if (anyModal.value) return
  const t = e.target as HTMLElement
  const typing = t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement || t instanceof HTMLSelectElement
  // +/- só fora de campos, ou no campo de código vazio (senão é texto digitado).
  const freeKeys = !typing || (t.closest('.sale-catalog') && t instanceof HTMLInputElement && t.type === 'search' && t.value === '')
  const line = current()
  const keys: Record<string, () => unknown> = {
    F2: () => search.value?.focus(),
    F3: () => document.querySelector<HTMLElement>('.sale-pay [aria-label="Buscar cliente"], .sale-pay .card button.link')?.focus(),
    F4: () => openCheckout(),
    F6: () => saveAsQuote(),
    F7: () => openCheckout(discountField),
    F8: () => openCheckout(),
    F9: () => line && remove(line),
    Escape: () => !(t instanceof HTMLInputElement && t.type === 'search' && t.closest('.sale-catalog')) && refocus(),
  }
  if (freeKeys && line && (e.key === '+' || e.key === '-')) {
    keys[e.key] = () => limitQty(line, line.qty + (e.key === '+' ? 1 : -1))
  }
  const action = keys[e.key]
  if (!action) return
  if (e.key !== 'Escape') e.preventDefault()
  action()
}
onMounted(() => {
  window.addEventListener('keydown', onKey)
  search.value?.focus()
  loadPendingQuote()
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="sale-page">
  <div v-if="cashOpen === false" class="alert warn">
    O caixa está fechado. <RouterLink to="/caixa">Abra o caixa</RouterLink> para registrar vendas.
  </div>
  <div v-if="quoteId" class="alert info row between">
    <span>Venda do <strong>orçamento nº {{ quoteId }}</strong>: ao finalizar, o orçamento fica "convertido".</span>
    <button class="link" @click="quoteId = null">Não vincular ao orçamento</button>
  </div>
  <div class="sale-grid">
    <section class="card sale-catalog" aria-label="Produtos">
      <strong>Produtos</strong>
      <ProductCatalog ref="search" :in-cart="inCart" @pick="add" />
    </section>
    <section class="card stack sale-cart" aria-label="Carrinho">
      <div class="row between"><strong>Carrinho</strong><span class="small muted">{{ cart.length }} item(ns) · {{ cart.reduce((a, l) => a + l.qty, 0) }} un.</span></div>
      <div v-if="cart.length === 0" class="state">Carrinho vazio. Clique num produto ao lado ou leia o código de barras.</div>
      <div v-else class="table-wrap sale-items">
        <table>
          <thead>
            <tr><th class="left">Produto</th><th class="center">Qtd.</th><th class="center">Desconto</th><th class="right">Subtotal</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="l in cart" :key="l.product.id" :class="{ selected: l === current() }" @click="selectedId = l.product.id">
              <td class="left">
                {{ l.product.name }}
                <div class="small muted mono">
                  {{ money(unitPrice(l)) }} un.
                  <span v-if="l.product.promo?.priceCents" style="text-decoration: line-through">{{ money(l.product.priceCents) }}</span>
                </div>
                <div v-if="l.product.promo" class="small" style="color: var(--success)">
                  {{ l.product.promo.label }}<template v-if="promoOff(l)"> (-{{ money(promoOff(l)) }})</template>
                </div>
              </td>
              <td class="right">
                <div class="row" style="gap: 4px; flex-wrap: nowrap; justify-content: center">
                  <button :aria-label="`Diminuir ${l.product.name}`" :disabled="l.qty <= 1 || busy" @click="l.qty--">−</button>
                  <input
                    class="qty-input mono"
                    :value="l.qty"
                    inputmode="numeric"
                    :aria-label="`Quantidade de ${l.product.name}`"
                    :disabled="busy"
                    @change="setQty(l, ($event.target as HTMLInputElement).value); ($event.target as HTMLInputElement).value = String(l.qty)"
                  />
                  <button :aria-label="`Aumentar ${l.product.name}`" :disabled="busy" @click="limitQty(l, l.qty + 1)">+</button>
                </div>
              </td>
              <td class="center">
                <input v-model="l.discountText" class="disc-input" :class="{ bad: lineInvalid(l) }" placeholder="R$ ou %" :disabled="busy"
                  :aria-label="`Desconto em ${l.product.name}`" title="Ex.: 5,00 ou 10%" />
              </td>
              <td class="right mono">
                {{ money(lineTotal(l) - (lineInvalid(l) ? 0 : lineDiscount(l) ?? 0)) }}
                <div v-if="!lineInvalid(l) && lineDiscount(l)" class="small muted" style="text-decoration: line-through">{{ money(lineTotal(l)) }}</div>
              </td>
              <td class="right"><button class="link icon-btn" :disabled="busy" :aria-label="`Remover ${l.product.name}`" title="Remover" style="color: var(--danger)" @click="remove(l)"><Icon name="remover" /></button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="card sale-pay" aria-label="Resumo da venda">
      <!-- Só o essencial; o pagamento abre numa tela própria (Finalizar / F8 ou F4). -->
      <div class="pay-body stack">
      <div class="total-box" :class="{ solo: !((discount ?? 0) > 0) }">
        <template v-if="(discount ?? 0) > 0">
          <div class="line small"><span>Subtotal</span><span class="mono">{{ money(subtotal) }}</span></div>
          <div class="line small"><span>Desconto</span><span class="mono">-{{ money(discount ?? 0) }}</span></div>
        </template>
        <div class="line grand"><span>Total</span><span class="mono">{{ money(discountInvalid ? subtotal : total) }}</span></div>
      </div>
      <CustomerPicker v-model="customerId" v-model:customer="customer" label="Cliente (opcional)" hide-credit />
      <!-- Venda para entrega (já paga): o cupom sai com o endereço para o entregador. -->
      <label class="check delivery-toggle"><input v-model="delivery.on" type="checkbox" :disabled="busy" /> Para entrega</label>
      <div v-if="delivery.on" class="delivery stack">
        <label class="field">Endereço da entrega
          <textarea v-model="delivery.address" rows="3" maxlength="400" placeholder="Rua, número, bairro, referência" :disabled="busy" />
        </label>
        <div class="row" style="flex-wrap: nowrap">
          <label class="field" style="flex: 1">Telefone<input v-model="delivery.phone" maxlength="30" inputmode="tel" :disabled="busy" /></label>
          <label class="field" style="flex: 1.4">Observação<input v-model="delivery.notes" maxlength="300" placeholder="ex.: portão azul" :disabled="busy" /></label>
        </div>
        <button v-if="customer && formatAddress(customer).length" type="button" class="link small" style="align-self: flex-start; padding: 0" @click="fillDelivery(true)">Usar o endereço do cadastro de {{ customer.name }}</button>
      </div>
      </div>
      <div class="pay-actions stack">
      <div v-if="error && !checkoutOpen" class="alert error" role="alert">{{ error }}</div>
      <div class="row" style="flex-wrap: nowrap">
        <button class="success big" style="flex: 2" :disabled="busy || cart.length === 0" @click="openCheckout()">Finalizar (F8)</button>
        <button class="primary big" style="flex: 1.2; white-space: nowrap" :disabled="busy || quoteBusy || cart.length === 0 || discountInvalid"
          title="F6: guarda o carrinho como orçamento (não mexe no estoque) e abre a impressão" @click="saveAsQuote">
          {{ quoteBusy ? 'Salvando…' : quoteId ? 'Atualizar orç.' : 'Orçamento' }}
        </button>
        <button class="big" style="flex: 0.8" :disabled="busy || cart.length === 0" @click="clearAsk = true">Limpar</button>
      </div>
      <div class="row between small">
        <span v-if="lastSale">Última venda: nº {{ lastSale.sale.id }} — {{ money(lastSale.sale.totalCents) }} <button class="link" @click="showLast = true">Ver / comprovante</button></span>
        <span v-else />
        <button class="link" :disabled="busy" @click="showQuotes">Abrir orçamento salvo</button>
      </div>
      </div>
    </section>
  </div>
  <div class="shortcuts small muted" aria-label="Atalhos de teclado">
    <span><kbd>F1</kbd> ajuda</span><span><kbd>F2</kbd> produto</span><span><kbd>F3</kbd> cliente</span>
    <span><kbd>F6</kbd> orçamento</span><span><kbd>F7</kbd> desconto</span><span><kbd>F8</kbd> finalizar</span>
    <span><kbd>F9</kbd> tirar item</span><span><kbd>+</kbd> <kbd>−</kbd> quantidade</span><span><kbd>5*código</kbd> vende 5</span>
    <span><kbd>Esc</kbd> voltar</span>
  </div>
  </div>
  <SaleDetailsModal v-if="showLast && lastSale" :sale-id="lastSale.sale.id" :start-with-receipt="lastAsReceipt" @close="showLast = false; lastAsReceipt = false" />
  <ReceiptModal v-if="quotePrintId" kind="orcamento" :id="quotePrintId" @close="quotePrintId = null" />
  <Modal v-if="quotesOpen" title="Abrir orçamento" wide @close="quotesOpen = false">
    <form class="row" @submit.prevent="showQuotes">
      <input v-model="quoteTerm" type="search" placeholder="Nº ou cliente" aria-label="Buscar orçamento" style="flex: 1" />
      <button type="submit">Buscar</button>
    </form>
    <div v-if="quotesError" class="alert error">{{ quotesError }}</div>
    <div v-else-if="!openQuotes.length" class="state">Nenhum orçamento aberto nos últimos 90 dias.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Nº</th><th>Data</th><th>Cliente</th><th>Validade</th><th class="right">Total</th><th></th></tr></thead>
        <tbody>
          <tr v-for="q in openQuotes" :key="q.id" class="clickable" @click="pickQuote(q.id)">
            <td class="mono">{{ q.id }}</td>
            <td class="nowrap">{{ formatDateTime(q.createdAt) }}</td>
            <td>{{ q.customerName ?? '—' }}</td>
            <td class="nowrap">{{ q.validUntil.split('-').reverse().join('/') }}</td>
            <td class="right mono">{{ money(q.totalCents) }}</td>
            <td class="right"><button class="link" @click.stop="pickQuote(q.id)">Levar ao carrinho</button></td>
          </tr>
        </tbody>
      </table>
    </div>
    <template #footer>
      <span class="small muted">Para editar, duplicar ou cancelar, use a tela Orçamentos.</span>
      <span class="spacer" />
      <button @click="quotesOpen = false">Fechar</button>
    </template>
  </Modal>
  <!-- Tela de pagamento: abre com Finalizar (F4); Enter confirma, Esc volta. -->
  <Modal v-if="checkoutOpen" title="Finalizar venda" wide :busy="busy" @close="checkoutOpen = false">
    <!-- Uma coluna, de cima para baixo: total, forma de pagamento (e troco), desconto e cliente. -->
    <div class="checkout">
      <div class="checkout-total">
        <span>Total a pagar</span>
        <strong class="mono">{{ money(discountInvalid ? subtotal : total) }}</strong>
        <small class="muted">{{ cart.reduce((a, l) => a + l.qty, 0) }} item(ns)<template v-if="(discount ?? 0) > 0"> · subtotal {{ money(subtotal) }} · desconto -{{ money(discount ?? 0) }}</template></small>
      </div>
      <div class="checkout-pay stack">
        <PaymentPanel v-model:parts="parts" v-model:split="split" :total="discountInvalid ? subtotal : total" :busy="busy" :store-credit="customer?.storeCreditCents ?? 0" />
      </div>
      <div class="checkout-side stack">
        <label class="field">
          <span>Desconto na venda (R$ ou %) · F7<span v-if="!can('desconto_acima_limite')" class="small muted"> · até {{ settings?.operatorMaxDiscountPct ?? 10 }}%</span></span>
          <input v-model="discountText" class="big mono disc-sale" placeholder="5,00 ou 10%" :aria-invalid="saleDiscount === null" :disabled="busy" />
        </label>
        <p v-if="discountInvalid" class="small" style="color: var(--danger); margin: 0">
          {{ cart.some(lineInvalid) ? 'Desconto de item inválido ou maior que o item.' : discount === null ? 'Valor de desconto inválido.' : 'O desconto não pode ser maior que o subtotal.' }}
        </p>
        <template v-if="pay.hasFiado || usesVale">
          <CustomerPicker v-model="customerId" v-model:customer="customer" label="Cliente" required :hide-credit="!pay.hasFiado" />
          <DebtWarning v-if="pay.hasFiado" :customer="customer" />
          <p v-if="customerId == null" class="small" style="color: var(--warning); margin: 0">Escolha o cliente para {{ pay.hasFiado ? 'vender a prazo' : 'usar o vale-troca' }}.</p>
        </template>
        <div v-if="delivery.on" class="alert info" style="margin: 0; white-space: pre-line">
          <strong>Para entrega</strong> — o cupom sai com o endereço.
          <template v-if="delivery.address.trim()"><br />{{ delivery.address }}</template>
          <template v-else><br /><span style="color: var(--danger)">Informe o endereço da entrega (Voltar).</span></template>
        </div>
        <div v-if="error" class="alert error" role="alert">{{ error }}</div>
      </div>
    </div>
    <template #footer>
      <button class="big" :disabled="busy" @click="checkoutOpen = false">Voltar (Esc)</button>
      <span class="spacer" />
      <button class="success big" :disabled="!canFinish" @click="finish">{{ busy ? 'Registrando…' : 'Confirmar venda (Enter/F8)' }}</button>
    </template>
  </Modal>
  <PixModal v-if="pixOpen" :amount-cents="pixAmount" :txid="requestId" @received="pixReceived" @cancel="pixOpen = false" />
  <ConfirmDialog v-if="clearAsk" title="Limpar carrinho" danger confirm-label="Limpar"
    :message="`Tirar todos os ${cart.length} item(ns) do carrinho? A venda não foi registrada.`"
    @confirm="clearCart(); clearAsk = false" @cancel="clearAsk = false" />
  <ConfirmDialog
    v-if="debtAsk && customer"
    title="Cliente com débitos pendentes"
    :message="`${customer.name} já deve ${money(customer.creditBalanceCents)} no a prazo. Vender mais a prazo mesmo assim?`"
    confirm-label="Vender a prazo"
    @confirm="debtAsk = false; debtOk = customer.id; finish()"
    @cancel="debtAsk = false"
  />
</template>

