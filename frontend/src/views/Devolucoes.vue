<!-- Caminho: pdv/frontend/src/views/Devolucoes.vue -->
<script setup lang="ts">
// Trocas e devoluções: nova devolução pelo nº da venda e a lista das já feitas.
import { computed, onMounted, ref } from 'vue'
import { CancelSale, GetReturnInfo, ListReturns, ListSalesByPeriod } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { exportTable } from '../composables/exportTable'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { formatDateTime, money, paymentLabels, today } from '../composables/format'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Icon from '../components/Icon.vue'
import ReceiptModal from '../components/ReceiptModal.vue'
import ReturnModal from '../components/ReturnModal.vue'
import SaleDetailsModal from '../components/SaleDetailsModal.vue'
import type { models } from '../types'

const refundLabels: Record<string, string> = { vale_troca: 'Vale-troca', dinheiro: 'Dinheiro', fiado: 'Abatido no a prazo', pix: 'Pix', cartao: 'Estorno no cartão' }
const refundBadge: Record<string, string> = { vale_troca: 'ok', dinheiro: 'warn', fiado: '', pix: 'warn', cartao: 'warn' }

const start = ref(today(-29))
const end = ref(today())
const list = ref<models.Return[]>([])
const loading = ref(false)
const error = ref('')
const term = ref('')

// Nova devolução: o operador digita o nº da venda (está no cupom).
const saleNo = ref('')
const finding = ref(false)
const findError = ref('')
const returnSaleId = ref<number | null>(null)
const receiptId = ref<number | null>(null)
const saleId = ref<number | null>(null)
const saleAsCupom = ref(false) // clique na linha da venda recente abre já no cupom
function openCupom(id: number) {
  saleAsCupom.value = true
  saleId.value = id
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    list.value = await call(ListReturns(start.value, end.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
// Vendas recentes (7 dias) para escolher sem precisar do número; o campo filtra por nº ou cliente.
const recent = ref<models.Sale[]>([])
async function loadRecent() {
  try {
    const sales = await call(ListSalesByPeriod(today(-6), today(), 0))
    recent.value = sales.filter((s) => s.status !== 'cancelada').sort((a, b) => b.id - a.id)
  } catch (e) {
    findError.value = errorMessage(e)
  }
}
const recentShown = computed(() => {
  const t = saleNo.value.trim().toLowerCase()
  const list = t ? recent.value.filter((s) => String(s.id).includes(t) || (s.customerName ?? '').toLowerCase().includes(t)) : recent.value
  return list
})
onMounted(() => {
  load()
  if (can('trocas_devolucoes')) loadRecent()
})

function preset(days: number) {
  start.value = today(-days)
  end.value = today()
  load()
}

async function findSale() {
  findError.value = ''
  // Texto (nome do cliente) com uma venda só na lista: abre essa.
  const one = !/^\s*\d+\s*$/.test(saleNo.value) && recentShown.value.length === 1 ? recentShown.value[0].id : 0
  const id = one || Number(saleNo.value.replace(/\D/g, ''))
  if (!id) return (findError.value = 'Digite o número da venda (está no cupom) ou escolha uma venda abaixo.')
  finding.value = true
  try {
    const info = await call(GetReturnInfo(id))
    if (info.sale.status === 'cancelada') return (findError.value = `A venda nº ${id} foi cancelada; não há o que devolver.`)
    returnSaleId.value = id
  } catch (e) {
    findError.value = errorMessage(e)
  } finally {
    finding.value = false
  }
}

// Cancelar a venda inteira (mesma regra do Histórico: estoque volta e o estorno vai para o caixa).
const toast = useToast()
const cancelSale = ref<models.Sale | null>(null)
const cancelling = ref(false)
async function confirmCancel(reason: string) {
  if (!cancelSale.value) return
  cancelling.value = true
  try {
    const d = await call(CancelSale(cancelSale.value.id, reason))
    toast.success(d.sale.paymentMethod === 'fiado'
      ? `Venda nº ${d.sale.id} cancelada. A dívida do cliente foi reduzida.`
      : `Venda nº ${d.sale.id} cancelada. Estorno de ${money(d.sale.totalCents)} registrado no caixa: faça a devolução ao cliente.`)
    cancelSale.value = null
    loadRecent()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    cancelling.value = false
  }
}

function onReturned() {
  saleNo.value = ''
  load()
  loadRecent()
}

function exportReturns() {
  exportTable('Devoluções', `Período: ${start.value.split('-').reverse().join('/')} a ${end.value.split('-').reverse().join('/')}`, [['Nº', 'int'], ['Data', 'text'], ['Venda', 'int'], ['Cliente', 'text'], ['Itens', 'text'], ['Motivo', 'text'], ['Reembolso', 'text'], ['Valor', 'money'], ['Operador', 'text']],
    filtered.value.map((r) => [r.id, formatDateTime(r.createdAt), r.saleId, r.customerName ?? '', r.items.map((i) => `${i.qty}x ${i.productName}`).join(', '), r.reason, refundLabels[r.refundMethod], r.totalCents, r.userName ?? '']))
}

const filtered = computed(() => {
  const t = term.value.trim().toLowerCase()
  if (!t) return list.value
  return list.value.filter((r) =>
    [String(r.id), String(r.saleId), r.customerName ?? '', r.reason, r.userName ?? '', ...r.items.map((i) => i.productName)]
      .some((x) => x.toLowerCase().includes(t)),
  )
})
const totals = computed(() => {
  const by: Record<string, number> = { vale_troca: 0, dinheiro: 0, fiado: 0, pix: 0, cartao: 0 }
  let total = 0
  for (const r of filtered.value) {
    by[r.refundMethod] = (by[r.refundMethod] ?? 0) + r.totalCents
    total += r.totalCents
  }
  return { total, by }
})
</script>

<template>
  <div class="grid top">
    <section v-if="can('trocas_devolucoes')" class="card stack">
      <h3>Nova devolução</h3>
      <form class="row" @submit.prevent="findSale">
        <label class="field" style="flex: 1">Nº da venda ou cliente<input v-model="saleNo" placeholder="Ex.: 125 (está no cupom) ou o nome do cliente" autofocus /></label>
        <button class="primary" style="align-self: end" type="submit" :disabled="finding">{{ finding ? 'Buscando…' : 'Buscar venda' }}</button>
      </form>
      <div v-if="findError" class="alert error" role="alert">{{ findError }}</div>
      <div class="small muted">Vendas dos últimos 7 dias{{ saleNo.trim() ? ' (filtradas)' : '' }} — para mais antigas, use o <router-link to="/historico">Histórico de vendas</router-link>.</div>
      <div v-if="!recentShown.length" class="state" style="padding: 12px">Nenhuma venda encontrada.</div>
      <div v-else class="recent">
        <table>
          <colgroup><col style="width: 7%" /><col style="width: 19%" /><col style="width: 17%" /><col style="width: 11%" /><col style="width: 14%" /><col style="width: 32%" /></colgroup>
          <thead><tr><th>Nº</th><th>Data</th><th>Cliente</th><th>Forma</th><th class="right">Total</th><th></th></tr></thead>
          <tbody>
            <tr v-for="s in recentShown" :key="s.id" class="clickable" tabindex="0" title="Ver o cupom desta venda" @click="openCupom(s.id)" @keydown.enter="openCupom(s.id)">
              <td class="mono">{{ s.id }}</td>
              <td class="nowrap small">{{ formatDateTime(s.createdAt) }}</td>
              <td class="left">{{ s.customerName ?? '—' }}</td>
              <td class="small nowrap">{{ paymentLabels[s.paymentMethod] ?? s.paymentMethod }}</td>
              <td class="right mono nowrap">{{ money(s.totalCents) }}</td>
              <td class="right actions-cell">
                <button class="link icon-btn" title="Devolver alguns itens desta venda" aria-label="Devolver itens desta venda" @click.stop="returnSaleId = s.id"><Icon name="devolucao" /><span>Devolver itens</span></button>
                <button v-if="can('cancelar_venda')" class="link icon-btn" style="color: var(--danger)" title="Cancelar a venda inteira" aria-label="Cancelar a venda inteira" @click.stop="cancelSale = s"><Icon name="cancelar" /><span>Cancelar venda</span></button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <section v-else class="card stack">
      <h3>Nova devolução</h3>
      <div class="alert warn">Seu usuário não tem a permissão "fazer trocas e devoluções". Peça a um gerente.</div>
    </section>

    <section class="card stack about">
      <h3>Como funciona</h3>
      <ul>
        <li>Os itens devolvidos <strong>voltam ao estoque</strong>. Dá para devolver parte da venda e o resto depois.</li>
        <li>O valor é o que o cliente <strong>pagou</strong> pelo item, já com descontos e promoções.</li>
        <li><strong>Vale-troca:</strong> vira saldo do cliente para usar em outra compra (forma "Vale-troca" no pagamento).</li>
        <li><strong>Dinheiro:</strong> sai da gaveta do caixa; entregue o valor ao cliente.</li>
        <li><strong>Pix:</strong> a loja faz o Pix para o cliente; aparece como saída em Pix no caixa.</li>
        <li><strong>Estorno no cartão:</strong> só para venda paga no cartão, até o valor pago nele; faça o estorno na maquininha.</li>
        <li><strong>Abater na dívida:</strong> só para venda feita a prazo; reduz a dívida.</li>
        <li>Venda com devolução não pode mais ser cancelada. Tudo fica na Auditoria e gera comprovante.</li>
      </ul>
    </section>
  </div>

  <section class="card stack" style="margin-top: 16px">
    <form class="row" @submit.prevent="load">
      <label class="field">De<input v-model="start" type="date" required /></label>
      <label class="field">Até<input v-model="end" type="date" required /></label>
      <button class="primary" style="align-self: end" type="submit" :disabled="loading">{{ loading ? 'Buscando…' : 'Buscar' }}</button>
      <span class="spacer" />
      <button type="button" style="align-self: end" @click="preset(0)">Hoje</button>
      <button type="button" style="align-self: end" @click="preset(6)">7 dias</button>
      <button type="button" style="align-self: end" @click="preset(29)">30 dias</button>
    </form>
    <div v-if="error" class="alert error">{{ error }}</div>

    <div class="grid stats">
      <div class="stat"><div class="label">Total devolvido</div><div class="value mono">{{ money(totals.total) }}</div><div class="small muted">{{ filtered.length }} devolução(ões)</div></div>
      <div class="stat"><div class="label">Em vale-troca</div><div class="value mono">{{ money(totals.by.vale_troca) }}</div></div>
      <div class="stat"><div class="label">Em dinheiro</div><div class="value mono">{{ money(totals.by.dinheiro) }}</div></div>
      <div class="stat"><div class="label">Por Pix</div><div class="value mono">{{ money(totals.by.pix) }}</div></div>
      <div class="stat"><div class="label">Estorno no cartão</div><div class="value mono">{{ money(totals.by.cartao) }}</div></div>
      <div class="stat"><div class="label">Abatido no a prazo</div><div class="value mono">{{ money(totals.by.fiado) }}</div></div>
    </div>

    <div class="row">
      <input v-model="term" type="search" placeholder="Filtrar por nº, venda, cliente, produto, motivo ou operador" aria-label="Filtrar devoluções" style="max-width: 420px" />
      <span class="spacer" />
      <button type="button" :disabled="!filtered.length" @click="exportReturns">Exportar Excel</button>
    </div>
    <div v-if="!filtered.length" class="state">{{ loading ? 'Carregando…' : 'Nenhuma devolução no período.' }}</div>
    <div v-else class="table-wrap">
      <table>
        <colgroup><col style="width: 6%" /><col style="width: 13%" /><col style="width: 8%" /><col style="width: 14%" /><col style="width: 25%" /><col style="width: 12%" /><col style="width: 10%" /><col style="width: 12%" /></colgroup>
        <thead><tr><th>Nº</th><th>Data</th><th>Venda</th><th>Cliente</th><th>Itens e motivo</th><th>Reembolso</th><th class="right">Valor</th><th></th></tr></thead>
        <tbody>
          <tr v-for="r in filtered" :key="r.id">
            <td class="muted">{{ r.id }}</td>
            <td class="nowrap">{{ formatDateTime(r.createdAt) }}<div v-if="r.userName" class="small muted">{{ r.userName }}</div></td>
            <td><button class="link" @click="saleAsCupom = false; saleId = r.saleId">nº {{ r.saleId }}</button></td>
            <td>{{ r.customerName ?? '—' }}</td>
            <td>
              {{ r.items.map((i) => `${i.qty}× ${i.productName}`).join(', ') }}
              <div class="small muted">{{ r.reason }}</div>
            </td>
            <td><span class="badge" :class="refundBadge[r.refundMethod]">{{ refundLabels[r.refundMethod] }}</span></td>
            <td class="right mono">{{ money(r.totalCents) }}</td>
            <td class="right actions-cell">
              <button class="link icon-btn" title="Comprovante" aria-label="Comprovante" @click="receiptId = r.id"><Icon name="recibo" /><span>Comprovante</span></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <ReturnModal v-if="returnSaleId" :sale-id="returnSaleId" @close="returnSaleId = null" @changed="onReturned" />
  <ConfirmDialog
    v-if="cancelSale"
    :title="`Cancelar a venda nº ${cancelSale.id}`"
    :message="`A venda inteira (${money(cancelSale.totalCents)}) será desfeita: o estoque volta e o cancelamento fica registrado. Pagamentos em dinheiro, Pix ou cartão geram estorno no caixa; a prazo reduz a dívida. Para devolver só alguns itens, use Devolver itens.`"
    reason-label="Motivo do cancelamento"
    confirm-label="Confirmar cancelamento"
    danger
    :busy="cancelling"
    @confirm="confirmCancel"
    @cancel="cancelSale = null"
  />
  <ReceiptModal v-if="receiptId" kind="devolucao" :id="receiptId" @close="receiptId = null" />
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" allow-cancel :start-with-receipt="saleAsCupom" @close="saleId = null" @changed="load(); loadRecent()" />
</template>

<style scoped>
.grid > .card { margin-top: 0; }
/* Só rolagem vertical (lateral); nunca horizontal. */
.recent { max-height: 340px; overflow-y: auto; overflow-x: hidden; }
.recent td { overflow-wrap: normal; }
.recent thead th { position: sticky; top: 0; z-index: 1; }
.top { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); }
@media (max-width: 1100px) { .top { grid-template-columns: 1fr; } }
.stats { grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); } /* main.css dá margem a .card + .card; na grade desalinha */
table { table-layout: fixed; }
td { overflow-wrap: anywhere; }
.about ul { margin: 0; padding-left: 18px; display: flex; flex-direction: column; gap: 4px; font-size: 14px; }
</style>
