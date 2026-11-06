<!-- Caminho: pdv/frontend/src/views/Orcamentos.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { DuplicateQuote, GetQuote, ListQuotes, SaveQuote, SetQuoteStatus } from '../../wailsjs/go/main/App'
import { firstPhone, openWhatsapp } from '../composables/charge'
import { call, errorMessage } from '../api'
import CustomerPicker from '../components/CustomerPicker.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Modal from '../components/Modal.vue'
import WhatsappAsk from '../components/WhatsappAsk.vue'
import ProductSearch from '../components/ProductSearch.vue'
import ReceiptModal from '../components/ReceiptModal.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import { settings, storeName } from '../composables/cashState'
import { pendingQuote } from '../composables/quoteToSale'
import { useRouter } from 'vue-router'
import { centsToInput, formatDateTime, money, parseDiscount, parseQty, today } from '../composables/format'
import { useToast } from '../composables/toast'
import { models, type Product } from '../types'
import LoadMore from '../components/LoadMore.vue'
import { usePaged } from '../composables/paged'

const toast = useToast()
const router = useRouter()
const start = ref(today(-30))
const end = ref(today())
const term = ref('')
const statusFilter = ref('todas')
const quotes = ref<models.Quote[]>([])
const loading = ref(false)
const error = ref('')

const statusLabels: Record<string, string> = { aberto: 'Aberto', aprovado: 'Aprovado', convertido: 'Convertido', cancelado: 'Cancelado', expirado: 'Expirado' }
const statusClass: Record<string, string> = { aberto: 'warn', aprovado: 'ok', convertido: '', cancelado: 'bad', expirado: 'bad' }
const br = (d: string) => d.split('-').reverse().join('/')
// Cor da linha: convertido verde, pendente (aberto/aprovado) amarelo, vencido vermelho.
const rowClass: Record<string, string> = { convertido: 'q-convertido', aberto: 'q-pendente', aprovado: 'q-pendente', expirado: 'q-vencido', cancelado: 'inactive' }
const editable = (s: string) => s === 'aberto' || s === 'aprovado' || s === 'expirado'

const filtered = computed(() => quotes.value.filter((q) => statusFilter.value === 'todas' || q.status === statusFilter.value))

async function load() {
  loading.value = true
  error.value = ''
  try {
    quotes.value = await call(ListQuotes(start.value, end.value, term.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---------- editor ----------
interface Line { productId: number; name: string; unit: number; qty: number; discountText: string; promo: string }
const form = reactive({ open: false, id: 0, status: '', customerId: null as number | null, lines: [] as Line[], discountText: '', notes: '', validUntil: '', busy: false, error: '' })
// Convertido ou cancelado: só para ver.
const readonly = computed(() => !!form.status && !editable(form.status))
const lineTotal = (l: Line) => l.qty * l.unit
const lineDisc = (l: Line) => parseDiscount(l.discountText, lineTotal(l))
const formSubtotal = computed(() => form.lines.reduce((a, l) => a + lineTotal(l), 0))
const formItemsDisc = computed(() => form.lines.reduce((a, l) => a + (lineDisc(l) ?? 0), 0))
const formSaleDisc = computed(() => parseDiscount(form.discountText, formSubtotal.value - formItemsDisc.value))
const formTotal = computed(() => formSubtotal.value - formItemsDisc.value - (formSaleDisc.value ?? 0))

async function openEditor(id?: number) {
  Object.assign(form, { open: true, id: 0, status: '', customerId: null, lines: [], discountText: '', notes: '', validUntil: '', busy: false, error: '' })
  if (!id) return
  try {
    const d = await call(GetQuote(id))
    const itemsDisc = d.items.reduce((a, it) => a + it.discountCents, 0)
    Object.assign(form, {
      id, status: d.quote.status, customerId: d.quote.customerId ?? null, notes: d.quote.notes, validUntil: d.quote.validUntil,
      discountText: d.quote.discountCents - itemsDisc ? centsToInput(d.quote.discountCents - itemsDisc) : '',
      lines: d.items.map((it) => ({ productId: it.productId, name: it.productName, unit: it.unitPriceCents, qty: it.qty,
        discountText: it.discountCents ? centsToInput(it.discountCents) : '', promo: it.promoLabel })),
    })
  } catch (e) {
    form.error = errorMessage(e)
  }
}

function addProduct(p: Product) {
  const l = form.lines.find((x) => x.productId === p.id)
  if (l) l.qty++
  else form.lines.push({ productId: p.id, name: p.name, unit: p.promo?.priceCents || p.priceCents, qty: 1, discountText: '', promo: p.promo?.label ?? '' })
}

async function saveQuote() {
  if (form.lines.length === 0) return (form.error = 'Inclua ao menos um produto.')
  if (form.lines.some((l) => lineDisc(l) === null) || formSaleDisc.value === null) return (form.error = 'Desconto inválido. Use 5,00 ou 10%.')
  form.busy = true
  form.error = ''
  try {
    const d = await call(SaveQuote(models.QuoteInput.createFrom({
      id: form.id, customerId: form.customerId, notes: form.notes, validUntil: form.validUntil, discountCents: formSaleDisc.value ?? 0,
      items: form.lines.map((l) => ({ productId: l.productId, qty: l.qty, discountCents: lineDisc(l) ?? 0 })),
    })))
    toast.success(`Orçamento nº ${d.quote.id} salvo: ${money(d.quote.totalCents)}, válido até ${br(d.quote.validUntil)}.`)
    form.open = false
    load()
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}

// Leva os itens (como estão na tela) para o carrinho de Vendas.
function sendToSales() {
  if (form.lines.length === 0) return (form.error = 'O orçamento não tem produtos.')
  if (form.lines.some((l) => lineDisc(l) === null) || formSaleDisc.value === null) return (form.error = 'Desconto inválido. Use 5,00 ou 10%.')
  pendingQuote.value = {
    quoteId: form.id, customerId: form.customerId, saleDiscountCents: formSaleDisc.value ?? 0,
    items: form.lines.map((l) => ({ productId: l.productId, qty: l.qty, discountCents: lineDisc(l) ?? 0 })),
  }
  form.open = false
  router.push('/vendas')
}

// ---------- ações ----------
const printId = ref<number | null>(null)
const saleId = ref<number | null>(null)

const cancelAsk = ref<models.Quote | null>(null)
const duplicateAsk = ref<models.Quote | null>(null)
async function setStatus(q: models.Quote, status: string) {
  try {
    await call(SetQuoteStatus(q.id, status))
    toast.success(`Orçamento nº ${q.id}: ${statusLabels[status].toLowerCase()}.`)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function duplicate(q: models.Quote) {
  try {
    const d = await call(DuplicateQuote(q.id))
    toast.success(`Orçamento nº ${d.quote.id} criado a partir do nº ${q.id}, com os preços de hoje.`)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

const waText = ref('')

// WhatsApp: abre a conversa com o resumo (sem integração: o operador envia).
async function whatsapp(q: models.Quote) {
  try {
    const d = await call(GetQuote(q.id))
    const lines = d.items.map((it) => `• ${it.qty} x ${it.productName}: ${money(it.qty * it.unitPriceCents - it.promoDiscountCents - it.discountCents)}`)
    const text = [`*Orçamento nº ${q.id}* — ${storeName.value}`, ...lines, `*Total: ${money(q.totalCents)}*`, `Válido até ${br(q.validUntil)}.`].join('\n')
    // Celular do cliente do orçamento; sem cliente ou sem celular, pergunta para quem enviar.
    const phone = firstPhone(d.customer?.phone)
    if (phone) openWhatsapp(phone, text)
    else waText.value = text
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgOrcamentos = usePaged(() => filtered.value, [term, start, end, statusFilter])
</script>

<template>
  <section class="card stack">
    <form class="row" @submit.prevent="load">
      <label class="field" style="flex: 1; min-width: 200px">Buscar (nº, cliente ou operador)<input v-model="term" type="search" placeholder="Ex.: 15, Maria ou João" /></label>
      <label class="field">De<input v-model="start" type="date" :disabled="!!term.trim()" /></label>
      <label class="field">Até<input v-model="end" type="date" :disabled="!!term.trim()" /></label>
      <label class="field">Situação
        <select v-model="statusFilter">
          <option value="todas">Todas</option>
          <option v-for="(l, k) in statusLabels" :key="k" :value="k">{{ l }}</option>
        </select>
      </label>
      <button style="align-self: end" type="submit" :disabled="loading">Buscar</button>
      <span class="spacer" />
      <button class="primary" style="align-self: end" @click.prevent="openEditor()">Novo orçamento</button>
    </form>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="!loading && filtered.length === 0" class="state">Nenhum orçamento.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Nº</th><th>Criado em</th><th>Cliente</th><th>Operador</th><th class="right">Total</th><th>Validade</th><th>Situação</th><th class="right">Ações</th></tr></thead>
        <tbody>
          <tr v-for="q in pgOrcamentos.visible" :key="q.id" class="clickable" :class="rowClass[q.status]"
            title="Clique para ver e editar os itens" @click="openEditor(q.id)">
            <td>nº {{ q.id }}</td>
            <td class="nowrap small">{{ formatDateTime(q.createdAt) }}</td>
            <td>{{ q.customerName ?? '—' }}</td>
            <td>{{ q.userName ?? '—' }}</td>
            <td class="right mono">{{ money(q.totalCents) }}</td>
            <td class="nowrap small">{{ br(q.validUntil) }}</td>
            <td>
              <span class="badge" :class="statusClass[q.status]">{{ statusLabels[q.status] }}</span>
              <button v-if="q.saleId" class="link small" @click.stop="saleId = q.saleId">venda nº {{ q.saleId }}</button>
            </td>
            <td class="right actions-cell" @click.stop>
              <button class="link icon-btn" @click="printId = q.id" title="Imprimir" aria-label="Imprimir"><Icon name="imprimir" /><span>Imprimir</span></button>
              <button class="link icon-btn" v-if="editable(q.status)" @click="whatsapp(q)" title="WhatsApp" aria-label="WhatsApp" style="color: var(--success)"><Icon name="whatsapp" /><span>WhatsApp</span></button>
              <button class="link icon-btn" @click="duplicateAsk = q" title="Duplicar" aria-label="Duplicar"><Icon name="duplicar" /><span>Duplicar</span></button>
              <button class="link icon-btn" v-if="editable(q.status)" @click="cancelAsk = q" title="Cancelar" aria-label="Cancelar" style="color: var(--danger)"><Icon name="cancelar" /><span>Cancelar</span></button>
            </td>
          </tr>
        </tbody>
      </table>
          <LoadMore :paged="pgOrcamentos" />
    </div>
  </section>

  <WhatsappAsk v-if="waText" :text="waText" @close="waText = ''" />
  <ConfirmDialog v-if="duplicateAsk" title="Duplicar orçamento" confirm-label="Duplicar" cancel-label="Cancelar"
    :message="`Criar um novo orçamento com os mesmos itens do nº ${duplicateAsk.id}? Os preços serão os de hoje.`"
    @confirm="duplicate(duplicateAsk); duplicateAsk = null" @cancel="duplicateAsk = null" />
  <ConfirmDialog v-if="cancelAsk" title="Cancelar orçamento" danger confirm-label="Cancelar orçamento"
    :message="`Cancelar o orçamento nº ${cancelAsk.id}? Ele não poderá mais ser editado nem virar venda.`"
    @confirm="setStatus(cancelAsk, 'cancelado'); cancelAsk = null" @cancel="cancelAsk = null" />

  <!-- Editor -->
  <Modal v-if="form.open" :title="form.id ? `Orçamento nº ${form.id}${readonly ? ` — ${statusLabels[form.status].toLowerCase()} (só leitura)` : ''}` : 'Novo orçamento'" full :busy="form.busy" @close="form.open = false">
    <fieldset class="stack plain" :disabled="readonly">
      <div class="grid grid-2">
        <CustomerPicker v-model="form.customerId" label="Cliente (opcional)" />
        <label class="field">Válido até<input v-model="form.validUntil" type="date" placeholder="padrão" />
          <span class="small muted">Vazio = validade padrão das Configurações ({{ settings?.quoteValidityDays ?? 7 }} dias).</span>
        </label>
      </div>
      <ProductSearch v-if="!readonly" only-active no-categories placeholder="Adicionar produto: código de barras ou nome" @pick="addProduct" />
      <div v-if="!form.lines.length" class="state">Nenhum produto. Use a busca acima para adicionar.</div>
      <div v-if="form.lines.length" class="table-wrap">
        <table>
          <thead><tr><th class="left">Produto</th><th class="right">Unitário</th><th class="center">Qtd.</th><th class="center">Desconto</th><th class="right">Total</th><th></th></tr></thead>
          <tbody>
            <tr v-for="(l, i) in form.lines" :key="l.productId">
              <td class="left">{{ l.name }}<div v-if="l.promo" class="small" style="color: var(--success)">{{ l.promo }}</div></td>
              <td class="right mono">{{ money(l.unit) }}</td>
              <td class="center"><input class="qty-input mono" :value="l.qty" inputmode="numeric" @change="l.qty = parseQty(($event.target as HTMLInputElement).value) ?? l.qty" /></td>
              <td class="center"><input v-model="l.discountText" class="disc-input" :class="{ bad: lineDisc(l) === null }" placeholder="R$ ou %" /></td>
              <td class="right mono">{{ money(lineTotal(l) - (lineDisc(l) ?? 0)) }}</td>
              <td class="right"><button class="link icon-btn" v-if="!readonly" @click="form.lines.splice(i, 1)" title="Remover" aria-label="Remover" style="color: var(--danger)"><Icon name="remover" /></button></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="grid grid-2">
        <label class="field">Observações<textarea v-model="form.notes" maxlength="1000" placeholder="Condições, prazo de entrega…" /></label>
        <div class="stack" style="gap: 6px">
          <label class="field">Desconto no orçamento (R$ ou %)<input v-model="form.discountText" placeholder="ex.: 5,00 ou 10%" /></label>
          <div class="total-box">
            <div class="line"><span>Subtotal</span><span class="mono">{{ money(formSubtotal) }}</span></div>
            <div v-if="formItemsDisc + (formSaleDisc ?? 0)" class="line"><span>Descontos</span><span class="mono">-{{ money(formItemsDisc + (formSaleDisc ?? 0)) }}</span></div>
            <div class="line grand"><span>Total</span><span class="mono">{{ money(formTotal) }}</span></div>
          </div>
        </div>
      </div>
      <p v-if="!readonly" class="small muted" style="margin: 0">Os preços são os de hoje (com promoção, se houver) e o sistema recalcula ao salvar. Orçamento não mexe no estoque.</p>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
    </fieldset>
    <template #footer>
      <button v-if="form.id" @click="printId = form.id">Imprimir</button>
      <button v-if="form.id && !readonly" class="success" title="Abre a tela de Vendas com estes produtos no carrinho" @click="sendToSales">Levar para Vendas</button>
      <span class="spacer" />
      <button :disabled="form.busy" @click="form.open = false">{{ readonly ? 'Fechar' : 'Cancelar' }}</button>
      <button v-if="!readonly" class="primary" :disabled="form.busy" @click="saveQuote">{{ form.busy ? 'Salvando…' : 'Salvar orçamento' }}</button>
    </template>
  </Modal>

  <ReceiptModal v-if="printId" kind="orcamento" :id="printId" @close="printId = null" />
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" @close="saleId = null" />
</template>
