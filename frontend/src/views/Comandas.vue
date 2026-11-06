<!-- Caminho: pdv/frontend/src/views/Comandas.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, onMounted, ref, watch } from 'vue'
import {
  AddItemToComanda, CloseComanda, DiscardComanda, GetComandaDetails, ListOpenComandas, OpenComanda, SetComandaItemQty,
} from '../../wailsjs/go/main/App'
import { call, errorMessage, isCode } from '../api'
import { can } from '../composables/session'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CustomerPicker from '../components/CustomerPicker.vue'
import DebtWarning from '../components/DebtWarning.vue'
import PaymentPanel from '../components/PaymentPanel.vue'
import PixModal from '../components/PixModal.vue'
import { evaluate, newPart } from '../composables/payments'
import ProductSearch from '../components/ProductSearch.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import { cashOpen, refreshCash, settings } from '../composables/cashState'
import { formatDateTime, money, parseDiscount, parseMoney, parseQty, paymentLabels } from '../composables/format'
import { useToast } from '../composables/toast'
import { models, type Comanda, type ComandaDetails, type Customer, type PaymentMethod, type Product } from '../types'

const toast = useToast()
const list = ref<Comanda[]>([])
// Tempo desde a abertura, para o card da comanda ("45 min", "2h10").
function since(openedAt: string) {
  const min = Math.max(0, Math.round((Date.now() - new Date(openedAt).getTime()) / 60000))
  return min < 60 ? `${min} min` : `${Math.floor(min / 60)}h${String(min % 60).padStart(2, '0')}`
}
const loading = ref(true)
const loadError = ref('')
const current = ref<ComandaDetails | null>(null)
const busy = ref(false)
const newTable = ref('')

const parts = ref([newPart('dinheiro')])
const split = ref(false)
const customerId = ref<number | null>(null)
const customer = ref<Customer | null>(null) // para o saldo de vale-troca e o aviso de débitos
const debtAsk = ref(false)
const debtOk = ref<number | null>(null)
watch(customerId, () => (debtOk.value = null))
const discountText = ref('')
const closeError = ref('')
const discarding = ref(false)
const saleId = ref<number | null>(null)

const discount = computed(() => parseDiscount(discountText.value, current.value?.comanda.totalCents ?? 0))
const hasInactive = computed(() => current.value?.items.some((i) => !i.productActive) ?? false)
const isOpen = computed(() => current.value?.comanda.status === 'aberta')

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    list.value = await call(ListOpenComandas())
  } catch (e) {
    loadError.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// Comanda fechada em outra ação: avisa e atualiza a tela.
async function handleConflict(e: unknown) {
  if (isCode(e, 'COMANDA_FECHADA') || isCode(e, 'COMANDA_INEXISTENTE')) {
    toast.error(errorMessage(e))
    if (current.value) await select(current.value.comanda.id)
    load()
    return true
  }
  return false
}

async function select(id: number) {
  closeError.value = ''
  try {
    current.value = await call(GetComandaDetails(id))
    parts.value = [newPart('dinheiro')]
    split.value = false
    customerId.value = null
    discountText.value = ''
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function open() {
  if (!newTable.value.trim()) return
  busy.value = true
  try {
    current.value = await call(OpenComanda(newTable.value))
    newTable.value = ''
    toast.success(`Comanda ${current.value.comanda.tableNo} aberta.`)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}

async function addProduct(p: Product) {
  if (!current.value) return
  busy.value = true
  try {
    current.value = await call(AddItemToComanda(current.value.comanda.id, p.id, 1))
    load()
  } catch (e) {
    if (!(await handleConflict(e))) toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}

async function setQty(itemId: number, value: string | number) {
  const q = Number(value) === 0 ? 0 : parseQty(value)
  if (q === null) {
    toast.error('Quantidade inválida.')
    return
  }
  busy.value = true
  try {
    current.value = await call(SetComandaItemQty(itemId, q))
    load()
  } catch (e) {
    if (!(await handleConflict(e))) toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}

async function close() {
  if (!current.value || busy.value) return
  if (discount.value === null) return (closeError.value = 'Desconto inválido.')
  const p = evaluate(parts.value, Math.max(0, current.value.comanda.totalCents - (discount.value ?? 0)), split.value)
  if (p.error) return (closeError.value = p.error)
  if (p.hasFiado && customerId.value == null) return (closeError.value = 'Selecione o cliente para venda a prazo.')
  // A prazo para quem já deve: pede confirmação antes (uma vez por cliente).
  if (p.hasFiado && customer.value && customer.value.creditBalanceCents > 0 && debtOk.value !== customer.value.id) {
    debtAsk.value = true
    return
  }
  if (parts.value.some((x) => x.method === 'vale_troca') && customerId.value == null) return (closeError.value = 'Selecione o cliente dono do vale-troca.')
  const pixTotal = p.payments.filter((x) => x.method === 'pix').reduce((t, x) => t + x.amountCents, 0)
  if (pixTotal > 0 && settings.value?.pixKey) {
    pixClose.value = { amount: pixTotal, payments: p.payments }
    return
  }
  await doClose(p.payments)
}

// Pix: o QR aparece antes; a comanda só fecha depois de "Pix recebido".
const pixClose = ref<{ amount: number; payments: models.PaymentInput[] } | null>(null)
function pixReceived() {
  const pending = pixClose.value
  pixClose.value = null
  if (pending) doClose(pending.payments)
}

async function doClose(payments: models.PaymentInput[]) {
  if (!current.value) return
  busy.value = true
  closeError.value = ''
  try {
    const input = models.CloseComandaInput.createFrom({
      comandaId: current.value.comanda.id, paymentMethod: parts.value[0]?.method ?? 'dinheiro', payments,
      customerId: customerId.value, discountCents: discount.value,
    })
    const sale = await call( CloseComanda(input))
    toast.success(`Comanda fechada. Venda nº ${sale.sale.id}: ${money(sale.sale.totalCents)}.`)
    saleId.value = sale.sale.id
    current.value = null
    load()
  } catch (e) {
    if (!(await handleConflict(e))) closeError.value = errorMessage(e)
    if (isCode(e, 'CAIXA_NAO_ABERTO')) refreshCash()
  } finally {
    busy.value = false
  }
}

async function discard() {
  if (!current.value) return
  busy.value = true
  try {
    await call(DiscardComanda(current.value.comanda.id))
    toast.success('Comanda descartada.')
    discarding.value = false
    current.value = null
    load()
  } catch (e) {
    discarding.value = false
    if (!(await handleConflict(e))) toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="cashOpen === false" class="alert warn" style="margin-bottom: 16px">
    O caixa está fechado: é possível lançar itens, mas o fechamento da comanda exige <RouterLink to="/caixa">caixa aberto</RouterLink>.
  </div>
  <div class="split">
    <section class="card stack" aria-label="Comandas abertas">
      <form class="row" @submit.prevent="open">
        <input v-model="newTable" maxlength="200" placeholder="Mesa, balcão ou código" aria-label="Identificação da nova comanda" style="max-width: 280px" />
        <button class="primary" type="submit" :disabled="busy || !newTable.trim()">Abrir comanda</button>
      </form>
      <div v-if="loading && list.length === 0" class="state">Carregando…</div>
      <div v-else-if="loadError" class="alert error">{{ loadError }}</div>
      <div v-else-if="list.length === 0" class="state">Nenhuma comanda aberta.</div>
      <template v-else>
        <div class="tabs-sum small muted">{{ list.length }} comanda(s) aberta(s) · <strong class="mono">{{ money(list.reduce((a, c) => a + c.totalCents, 0)) }}</strong> em consumo</div>
        <div class="cmd-grid">
          <button v-for="c in list" :key="c.id" type="button" class="cmd" :class="{ active: current?.comanda.id === c.id }" @click="select(c.id)"
            :title="`Aberta em ${formatDateTime(c.openedAt)}`">
            <span class="cmd-top"><strong class="cmd-no">{{ c.tableNo }}</strong><span class="badge acc">Aberta</span></span>
            <span class="cmd-meta small muted">{{ since(c.openedAt) }} · {{ c.itemCount }} item(ns)</span>
            <strong class="cmd-total mono">{{ money(c.totalCents) }}</strong>
          </button>
        </div>
      </template>
    </section>

    <section v-if="current" class="card stack" :aria-label="`Comanda ${current.comanda.tableNo}`">
      <div class="row between">
        <h2 style="margin: 0">{{ current.comanda.tableNo }}</h2>
        <span class="badge" :class="isOpen ? 'ok' : ''">{{ current.comanda.status }}</span>
      </div>
      <template v-if="isOpen">
        <ProductSearch only-active placeholder="Adicionar produto (código ou nome)" @pick="addProduct" />
        <div v-if="current.items.length === 0" class="state">Sem itens.</div>
        <table v-else>
          <thead><tr><th class="left">Item</th><th class="right">Qtd.</th><th class="right">Subtotal</th><th></th></tr></thead>
          <tbody>
            <tr v-for="i in current.items" :key="i.id">
              <td class="left">
                {{ i.productNameSnapshot }}
                <div class="small muted">{{ money(i.unitPriceCents) }} cada</div>
                <span v-if="!i.productActive" class="badge bad">Produto desativado — remova para fechar</span>
              </td>
              <td class="right">
                <input class="qty-input mono" :value="i.qty" inputmode="numeric" :disabled="busy" :aria-label="`Quantidade de ${i.productNameSnapshot}`"
                  @change="setQty(i.id, ($event.target as HTMLInputElement).value)" />
              </td>
              <td class="right mono">{{ money(i.subtotalCents) }}</td>
              <td class="right"><button class="link icon-btn" :disabled="busy" @click="setQty(i.id, 0)" title="Remover" aria-label="Remover" style="color: var(--danger)"><Icon name="remover" /></button></td>
            </tr>
          </tbody>
        </table>
        <div class="total-box">
          <div class="line"><span>Total dos itens</span><span class="mono">{{ money(current.comanda.totalCents) }}</span></div>
        </div>
        <h3>Fechamento</h3>
        <label class="field"><span>Desconto (R$ ou %)<span v-if="!can('desconto_acima_limite')" class="small muted"> · até {{ settings?.operatorMaxDiscountPct ?? 10 }}%</span></span><input v-model="discountText" placeholder="ex.: 5,00 ou 10%" /></label>
        <PaymentPanel v-model:parts="parts" v-model:split="split" :total="Math.max(0, current.comanda.totalCents - (discount ?? 0))" :busy="busy" :store-credit="customer?.storeCreditCents ?? 0" />
        <CustomerPicker v-model="customerId" v-model:customer="customer" :required="parts.some((x) => x.method === 'fiado' || x.method === 'vale_troca')" :label="parts.some((x) => x.method === 'fiado' || x.method === 'vale_troca') ? 'Cliente' : 'Cliente (opcional, para usar vale-troca)'" />
        <DebtWarning v-if="parts.some((x) => x.method === 'fiado')" :customer="customer" />
        <div v-if="closeError" class="alert error" role="alert">{{ closeError }}</div>
        <button class="success big block" :disabled="busy || current.items.length === 0 || hasInactive" @click="close">
          {{ busy ? 'Processando…' : 'Fechar comanda e registrar venda' }}
        </button>
        <button v-if="current.items.length === 0" class="danger block" :disabled="busy" @click="discarding = true">Descartar comanda vazia</button>
      </template>
      <div v-else class="alert info">Esta comanda já foi encerrada e é mantida como histórico.</div>
    </section>
    <section v-else class="card state">Selecione ou abra uma comanda.</section>
  </div>
  <ConfirmDialog
    v-if="discarding && current"
    title="Descartar comanda"
    :message="`A comanda ${current.comanda.tableNo} não tem itens e será encerrada como descartada.`"
    confirm-label="Descartar"
    danger
    :busy="busy"
    @confirm="discard"
    @cancel="discarding = false"
  />
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" @close="saleId = null" />
  <ConfirmDialog
    v-if="debtAsk && customer"
    title="Cliente com débitos pendentes"
    :message="`${customer.name} já deve ${money(customer.creditBalanceCents)} no a prazo. Fechar a comanda a prazo mesmo assim?`"
    confirm-label="Fechar a prazo"
    @confirm="debtAsk = false; debtOk = customer.id; close()"
    @cancel="debtAsk = false"
  />
  <PixModal v-if="pixClose":amount-cents="pixClose.amount" :txid="`COMANDA${current?.comanda.id ?? ''}`" @received="pixReceived" @cancel="pixClose = null" />
</template>

<style scoped>
/* Comandas abertas em cards (estilo mesas); a selecionada fica com a borda índigo. */
.cmd-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); gap: 14px; }
.cmd { display: flex; flex-direction: column; align-items: stretch; gap: 8px; min-height: 140px; padding: 16px; text-align: left; font-weight: 400;
  border: 1.5px solid var(--border); border-radius: 18px; background: var(--surface); box-shadow: 0 4px 16px rgba(17, 24, 39, 0.05); }
.cmd:hover:not(:disabled) { background: var(--surface); border-color: var(--primary); }
.cmd.active { border-color: var(--primary); box-shadow: 0 0 0 4px var(--primary-soft); }
.cmd-top { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
.cmd-no { font-size: 22px; letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cmd-total { margin-top: auto; font-size: 20px; }
.tabs-sum { margin-top: -2px; }
</style>
