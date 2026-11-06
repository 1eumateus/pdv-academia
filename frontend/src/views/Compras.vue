<!-- Caminho: pdv/frontend/src/views/Compras.vue -->
<script setup lang="ts">
import BarList, { type BarItem } from '../components/BarList.vue'
import Icon from '../components/Icon.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { CancelPurchase, CreateProduct, CreatePurchase, GetPurchase, ListPurchases, ListSuppliers, PickNFeFiles } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Modal from '../components/Modal.vue'
import NFeImportModal from '../components/NFeImportModal.vue'
import ProductSearch from '../components/ProductSearch.vue'
import { centsToInput, formatDate, formatDateTime, money, parseMoney, parseQty, today } from '../composables/format'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { models, type Product, type Supplier } from '../types'

const toast = useToast()
const start = ref(today(-30))
const end = ref(today())
const purchases = ref<models.Purchase[]>([])
// Indicadores do período (estornadas ficam fora do total).
const valid = computed(() => purchases.value.filter((p) => !p.cancelledAt))
const periodTotal = computed(() => valid.value.reduce((a, p) => a + p.totalCents, 0))
const suppliersCount = computed(() => new Set(valid.value.map((p) => p.supplierName)).size)
// Gráfico: total comprado de cada fornecedor no período (os 8 maiores).
const supplierBars = computed<BarItem[]>(() => {
  const m = new Map<string, { cents: number; count: number }>()
  for (const p of valid.value) {
    const s = m.get(p.supplierName) ?? { cents: 0, count: 0 }
    s.cents += p.totalCents
    s.count++
    m.set(p.supplierName, s)
  }
  return [...m].sort((a, b) => b[1].cents - a[1].cents).slice(0, 8)
    .map(([name, s]) => ({ key: name, label: name, value: s.cents, text: money(s.cents), sub: `${s.count} entrada(s)`, color: '#06b6d4' }))
})
const loading = ref(false)
const error = ref('')
const detail = ref<models.PurchaseDetails | null>(null)
const term = ref('')
const nfeFiles = ref<string[]>([])
const cancelling = ref(false)
const cancelBusy = ref(false)

async function cancelPurchase(reason: string) {
  if (!detail.value) return
  cancelBusy.value = true
  try {
    detail.value = await call(CancelPurchase(detail.value.purchase.id, reason))
    cancelling.value = false
    toast.success(`Entrada nº ${detail.value.purchase.id} estornada. ${detail.value.purchase.cancelledAccessKey ? 'A nota pode ser importada de novo.' : ''}`)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    cancelBusy.value = false
  }
}

async function importXML() {
  try {
    const files = await call(PickNFeFiles())
    if (files?.length) nfeFiles.value = files
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    purchases.value = await call(ListPurchases(start.value, end.value, term.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function openDetail(id: number) {
  try {
    detail.value = await call(GetPurchase(id))
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// ---------- nova entrada ----------
interface Line { product: Product; qty: string; cost: string; price: string }
const suppliers = ref<Supplier[]>([])
const form = reactive({ open: false, supplierId: null as number | null, invoice: '', date: today(), notes: '', lines: [] as Line[], busy: false, error: '' })

async function openNew() {
  Object.assign(form, { open: true, supplierId: null, invoice: '', date: today(), notes: '', lines: [], busy: false, error: '' })
  try {
    suppliers.value = (await call(ListSuppliers(''))).filter((s) => s.active)
  } catch (e) {
    form.error = errorMessage(e)
  }
}

// Produto novo cadastrado aqui mesmo e já incluído na entrada.
const newProduct = reactive({ open: false, name: '', code: '', price: '', busy: false, error: '' })
async function createAndAdd() {
  const price = parseMoney(newProduct.price)
  if (!newProduct.name.trim()) return (newProduct.error = 'Informe o nome.')
  if (price === null) return (newProduct.error = 'Informe o preço de venda. Exemplo: 12,90')
  newProduct.busy = true
  newProduct.error = ''
  try {
    const p = await call(CreateProduct(models.ProductInput.createFrom({
      name: newProduct.name, barcode: newProduct.code.trim() || null, priceCents: price, active: true, initialStock: 0,
    })))
    addProduct(p)
    Object.assign(newProduct, { open: false, name: '', code: '', price: '' })
    toast.success(`Produto "${p.name}" cadastrado e incluído na entrada. Informe a quantidade e o custo.`)
  } catch (e) {
    newProduct.error = errorMessage(e)
  } finally {
    newProduct.busy = false
  }
}

function addProduct(p: Product) {
  if (form.lines.some((l) => l.product.id === p.id)) return toast.info(`"${p.name}" já está na lista: ajuste a quantidade.`)
  form.lines.push({ product: p, qty: '1', cost: p.costCents ? centsToInput(p.costCents) : '', price: centsToInput(p.priceCents) })
}

// Prévia do custo médio, igual à regra do backend (o backend recalcula ao gravar).
function newAvg(l: Line): number | null {
  const q = parseQty(l.qty)
  const u = parseMoney(l.cost)
  if (q === null || u === null) return null
  const s = l.product.stockQty
  const c = l.product.costCents
  return s <= 0 || c === 0 ? u : Math.round((s * c + q * u) / (s + q))
}
function margin(price: number | null, cost: number | null): string {
  if (!price || !cost) return '—'
  return `${(Math.round(((price - cost) / price) * 1000) / 10).toString().replace('.', ',')}%`
}
const total = computed(() => form.lines.reduce((a, l) => a + (parseQty(l.qty) ?? 0) * (parseMoney(l.cost) ?? 0), 0))

async function save() {
  if (!form.supplierId) return (form.error = 'Escolha o fornecedor.')
  if (form.lines.length === 0) return (form.error = 'Inclua ao menos um produto.')
  const items = []
  for (const l of form.lines) {
    const qty = parseQty(l.qty)
    const cost = parseMoney(l.cost)
    if (qty === null) return (form.error = `Quantidade inválida em "${l.product.name}".`)
    if (cost === null) return (form.error = `Custo inválido em "${l.product.name}".`)
    const price = can('alterar_preco') ? parseMoney(l.price) : null
    items.push({ productId: l.product.id, qty, unitCostCents: cost, newPriceCents: price !== null && price !== l.product.priceCents ? price : null })
  }
  form.busy = true
  form.error = ''
  try {
    const d = await call(CreatePurchase(models.PurchaseInput.createFrom({
      supplierId: form.supplierId, invoiceNumber: form.invoice, issueDate: form.date, notes: form.notes, items,
    })))
    toast.success(`Entrada nº ${d.purchase.id} registrada: ${d.items.length} produto(s), ${money(d.purchase.totalCents)}.`)
    form.open = false
    load()
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}
</script>

<template>
  <div class="kpi-row">
    <div class="kpi">
      <div class="kpi-head">Entradas no período<span class="kpi-ic" style="--c: #5b4cf0"><Icon name="t-entrada" :size="18" /></span></div>
      <div class="kpi-value mono">{{ valid.length }}</div>
      <div class="small muted">{{ purchases.length - valid.length }} estornada(s)</div>
    </div>
    <div class="kpi">
      <div class="kpi-head">Total comprado<span class="kpi-ic" style="--c: #0e9466"><Icon name="receber" :size="18" /></span></div>
      <div class="kpi-value mono">{{ money(periodTotal) }}</div>
      <div class="small muted">a preço de custo</div>
    </div>
    <div class="kpi">
      <div class="kpi-head">Fornecedores<span class="kpi-ic" style="--c: #06b6d4"><Icon name="t-fornecedor" :size="18" /></span></div>
      <div class="kpi-value mono">{{ suppliersCount }}</div>
      <div class="small muted">com entrada no período</div>
    </div>
    <div class="kpi">
      <div class="kpi-head">Com XML da NF-e<span class="kpi-ic" style="--c: #d98b06"><Icon name="detalhes" :size="18" /></span></div>
      <div class="kpi-value mono">{{ valid.filter((p) => p.accessKey).length }}</div>
      <div class="small muted">importadas pela nota</div>
    </div>
  </div>
  <section v-if="supplierBars.length > 1" class="card" style="margin-bottom: 18px">
    <div class="card-head"><strong>Compras por fornecedor</strong><span class="small muted">no período</span></div>
    <BarList :items="supplierBars" />
  </section>
  <section class="card stack">
    <form class="row" @submit.prevent="load">
      <label class="field" style="flex: 1; min-width: 220px">Buscar (nº da nota, chave ou fornecedor)<input v-model="term" type="search" placeholder="Ex.: 12345" /></label>
      <label class="field">De<input v-model="start" type="date" required :disabled="!!term.trim()" /></label>
      <label class="field">Até<input v-model="end" type="date" required :disabled="!!term.trim()" /></label>
      <button style="align-self: end" type="submit" :disabled="loading">{{ loading ? 'Buscando…' : 'Buscar' }}</button>
      <span class="spacer" />
      <button style="align-self: end" @click.prevent="importXML">Importar XML da NF-e</button>
      <button class="primary" style="align-self: end" @click.prevent="openNew">Nova entrada manual</button>
    </form>
    <p v-if="term.trim()" class="small muted" style="margin: 0">Com busca preenchida, procura em todas as datas.</p>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="!loading && purchases.length === 0" class="state">{{ term.trim() ? 'Nenhuma entrada encontrada.' : 'Nenhuma entrada no período.' }}</div>
    <div v-else class="table-wrap">
      <table class="left-table">
        <thead><tr><th>Entrada</th><th>Registrada em</th><th>Fornecedor</th><th>Nota</th><th class="num">Total</th><th>Por</th></tr></thead>
        <tbody>
          <tr v-for="p in purchases" :key="p.id" class="clickable" :class="{ inactive: p.cancelledAt }" @click="openDetail(p.id)">
            <td><button class="link mono"><strong>nº {{ p.id }}</strong></button></td>
            <td class="nowrap muted">{{ formatDateTime(p.createdAt) }}</td>
            <td><div class="sup"><span class="kpi-ic solid" style="--c: #06b6d4; width: 30px; height: 30px"><Icon name="t-fornecedor" :size="15" /></span>{{ p.supplierName }}</div></td>
            <td class="small">
              {{ p.invoiceNumber || '—' }} <span v-if="p.accessKey || p.cancelledAccessKey" class="badge acc">XML</span>
              <span v-if="p.cancelledAt" class="badge bad">Estornada</span>
            </td>
            <td class="num mono"><strong>{{ money(p.totalCents) }}</strong></td>
            <td class="small">{{ p.userName ?? '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <Modal v-if="form.open" title="Nova entrada de mercadoria" full :busy="form.busy" @close="form.open = false">
    <form class="stack" @submit.prevent="save">
      <div class="grid grid-2">
        <label class="field">Fornecedor
          <select v-model="form.supplierId" required>
            <option :value="null">Escolha o fornecedor</option>
            <option v-for="s in suppliers" :key="s.id" :value="s.id">{{ s.legalName }}{{ s.tradeName ? ` (${s.tradeName})` : '' }}</option>
          </select>
        </label>
        <div class="grid grid-2">
          <label class="field">Nº da nota<input v-model="form.invoice" maxlength="60" /></label>
          <label class="field">Data da nota<input v-model="form.date" type="date" required /></label>
        </div>
      </div>
      <p v-if="suppliers.length === 0" class="small muted" style="margin: 0">Nenhum fornecedor ativo: cadastre em Fornecedores.</p>
      <div class="row" style="gap: 8px; align-items: flex-start; flex-wrap: nowrap">
        <div style="flex: 1"><ProductSearch :only-active="false" no-categories placeholder="Adicionar produto: código de barras ou nome" @pick="addProduct" /></div>
        <button v-if="can('cadastrar_produto') && !newProduct.open" type="button" class="big" @click="newProduct.open = true">+ Novo produto</button>
      </div>
      <!-- Bloco comum (não form): já estamos dentro do formulário da entrada. -->
      <div v-if="newProduct.open" class="card stack" style="gap: 6px; padding: 10px" @keydown.enter.prevent="createAndAdd">
        <strong class="small">Cadastrar produto novo e incluir nesta entrada</strong>
        <div class="row" style="gap: 6px; flex-wrap: nowrap; align-items: flex-end">
          <label class="field" style="flex: 2">Nome<input v-model="newProduct.name" maxlength="200" /></label>
          <label class="field" style="flex: 1">Código<input v-model="newProduct.code" maxlength="64" placeholder="opcional" /></label>
          <label class="field">Preço de venda<input v-model="newProduct.price" style="width: 110px; text-align: right" inputmode="decimal" placeholder="0,00" /></label>
          <button type="button" class="primary" :disabled="newProduct.busy" @click="createAndAdd">{{ newProduct.busy ? 'Salvando…' : 'Cadastrar e incluir' }}</button>
          <button type="button" :disabled="newProduct.busy" @click="newProduct.open = false">Cancelar</button>
        </div>
        <span v-if="newProduct.error" class="small" style="color: var(--danger)">{{ newProduct.error }}</span>
      </div>
      <div v-if="form.lines.length" class="table-wrap">
        <table>
          <thead>
            <tr>
              <th class="left">Produto</th><th class="right">Qtd.</th><th class="right">Custo unit. (R$)</th>
              <th v-if="can('ver_custo')" class="right">Custo médio</th>
              <th v-if="can('alterar_preco')" class="right">Preço de venda (R$)</th>
              <th v-if="can('ver_custo')" class="right">Margem</th><th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(l, i) in form.lines" :key="l.product.id">
              <td class="left">{{ l.product.name }}<div class="small muted">estoque atual {{ l.product.stockQty }}</div></td>
              <td class="right"><input v-model="l.qty" class="qty-input" inputmode="numeric" /></td>
              <td class="right"><input v-model="l.cost" style="width: 100px; text-align: right" inputmode="decimal" placeholder="0,00" /></td>
              <td v-if="can('ver_custo')" class="right mono small">
                {{ l.product.costCents ? money(l.product.costCents) : '—' }} → <strong>{{ newAvg(l) !== null ? money(newAvg(l)) : '—' }}</strong>
              </td>
              <td v-if="can('alterar_preco')" class="right"><input v-model="l.price" style="width: 100px; text-align: right" inputmode="decimal" /></td>
              <td v-if="can('ver_custo')" class="right mono small">{{ margin(parseMoney(l.price), newAvg(l)) }}</td>
              <td class="right"><button class="link icon-btn" type="button" @click="form.lines.splice(i, 1)" title="Remover" aria-label="Remover" style="color: var(--danger)"><Icon name="remover" /></button></td>
            </tr>
          </tbody>
        </table>
      </div>
      <label class="field">Observações<textarea v-model="form.notes" maxlength="1000" /></label>
      <div class="row between">
        <strong>Total: <span class="mono">{{ money(total) }}</span></strong>
        <span class="small muted">Dá entrada no estoque e recalcula o custo médio de cada produto.</span>
      </div>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">{{ form.busy ? 'Registrando…' : 'Confirmar entrada' }}</button>
      </footer>
    </form>
  </Modal>

  <Modal v-if="detail" :title="`Entrada nº ${detail.purchase.id}`" wide @close="detail = null">
    <div class="stack small">
      <div><strong>{{ detail.purchase.supplierName }}</strong> · nota {{ detail.purchase.invoiceNumber || '—' }} de {{ formatDate(detail.purchase.issueDate + 'T12:00:00') }}</div>
      <div v-if="detail.purchase.accessKey || detail.purchase.cancelledAccessKey" class="mono">Chave: {{ detail.purchase.accessKey ?? detail.purchase.cancelledAccessKey }}</div>
      <div v-if="detail.purchase.cancelledAt" class="alert warn" style="margin: 4px 0">
        Estornada em {{ formatDateTime(detail.purchase.cancelledAt) }} por {{ detail.purchase.cancelledByName ?? '—' }}. Motivo: {{ detail.purchase.cancellationReason }}
      </div>
      <div class="muted">Registrada em {{ formatDateTime(detail.purchase.createdAt) }} por {{ detail.purchase.userName ?? '—' }}</div>
      <div v-if="detail.purchase.notes">Obs.: {{ detail.purchase.notes }}</div>
    </div>
    <div class="table-wrap" style="margin-top: 10px">
      <table>
        <thead><tr><th class="left">Produto</th><th class="right">Qtd.</th><th class="right">Custo unit.</th><th class="right">Total</th><th v-if="can('ver_custo')" class="right">Custo antes → depois</th></tr></thead>
        <tbody>
          <tr v-for="it in detail.items" :key="it.id">
            <td class="left">{{ it.productName }}</td>
            <td class="right mono">{{ it.qty }}</td>
            <td class="right mono">{{ money(it.unitCostCents) }}</td>
            <td class="right mono">{{ money(it.totalCents) }}</td>
            <td v-if="can('ver_custo')" class="right mono small">{{ money(it.costBeforeCents) }} → {{ money(it.costAfterCents) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="right"><strong>Total: <span class="mono">{{ money(detail.purchase.totalCents) }}</span></strong></p>
    <template #footer>
      <button v-if="!detail.purchase.cancelledAt" @click="$router.push(`/etiquetas?compra=${detail.purchase.id}`)">Imprimir etiquetas</button>
      <button v-if="!detail.purchase.cancelledAt" class="danger" @click="cancelling = true">Estornar entrada</button>
      <span class="spacer" />
      <button @click="detail = null">Fechar</button>
    </template>
  </Modal>
  <ConfirmDialog
    v-if="cancelling && detail"
    title="Estornar entrada"
    message="As quantidades desta entrada saem do estoque e o custo de cada produto volta ao anterior (se não mudou depois). A entrada continua no histórico como estornada e, se veio de XML, a nota pode ser importada de novo. Se parte da mercadoria já foi vendida, o estorno não é possível."
    reason-label="Motivo do estorno"
    confirm-label="Confirmar estorno"
    danger
    :busy="cancelBusy"
    @confirm="cancelPurchase"
    @cancel="cancelling = false"
  />
  <NFeImportModal v-if="nfeFiles.length" :files="nfeFiles" @close="nfeFiles = []" @imported="load" />
</template>

<style scoped>
.sup { display: flex; align-items: center; gap: 10px; }
</style>
