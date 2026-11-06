<!-- Caminho: pdv/frontend/src/views/Produtos.vue -->
<script setup lang="ts">
// Produtos (cadastro: Mercadorias e Serviços) e Estoque (saldo e histórico de movimentações)
// usam esta tela; a rota diz qual (meta.mode). Serviços e planos não têm estoque
// e ficam só na sub-aba Serviços.
import Icon from '../components/Icon.vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  AdjustStock, CreateProduct, DeleteProduct, ListCategories, ListProducts, ListStockMovements,
  RegisterStockEntry, RegisterStockReturn, SaveCategory, SuggestProductCode, UpdateProduct,
} from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { can } from '../composables/session'
import CategoriesModal from '../components/CategoriesModal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import GradeModal from '../components/GradeModal.vue'
import ImportModal from '../components/ImportModal.vue'
import ServicesList from '../components/ServicesList.vue'
import StockMovements from '../components/StockMovements.vue'
import Modal from '../components/Modal.vue'
import { useRoute } from 'vue-router'
import { exportTable } from '../composables/exportTable'
import { settings } from '../composables/cashState'
import { centsToInput, formatDateTime, money, parseMoney, parseQty, stockLevel, stockReasonLabels, today } from '../composables/format'
import { useToast } from '../composables/toast'
import { models, type Category, type Product, type StockMovement } from '../types'

const toast = useToast()
const route = useRoute()
const stockMode = computed(() => route.meta.mode === 'estoque')
const sub = ref<'mercadorias' | 'servicos'>('mercadorias')
const stockSub = ref<'saldo' | 'historico'>('saldo')
const products = ref<Product[]>([])
const loading = ref(true)
const importOpen = ref(false)
const loadError = ref('')
const filter = ref('')
// Visão rápida (chips): uma por vez, com a contagem de cada uma.
type View = 'todos' | 'ativos' | 'venda' | 'inativos' | 'alerta' | 'zerado' | 'sem_preco'
const view = ref<View>('todos')
const sortBy = ref<'nome' | 'estoque' | 'preco' | 'recentes'>('nome')
// Sem preço de venda: "a precificar" (veio da nota) ou preço zerado.
const noPrice = (p: Product) => p.pricePending || p.priceCents === 0
const noPriceCount = computed(() => products.value.filter((p) => p.active && noPrice(p)).length)
// Produto que não controla estoque nunca entra em alerta de reposição.
const level = (p: Product) => (p.noStock ? 'ok' : stockLevel(p.stockQty, p.minStock, settings.value?.lowStockDefault ?? 0))
const isLow = (p: Product) => level(p) === 'baixo'
// margin: % do preço que sobra depois do custo (null sem custo informado).
function margin(price: number, cost: number): number | null {
  return cost > 0 && price > 0 ? Math.round(((price - cost) / price) * 1000) / 10 : null
}
const categories = ref<Category[]>([])
const categoryFilter = ref<number | 'todas' | 'sem'>('todas')
const showCategories = ref(false)
// grade: undefined = fechado; null = nova; número = ampliar a grade desse modelo
const grade = ref<number | null | undefined>(undefined)

// Busca sem acento e por palavras em qualquer ordem: "cad 10" acha "Caderno 10 matérias".
const fold = (t: string) => t.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase()
const allViews: { key: View; label: string; test: (p: Product) => boolean; stock: boolean }[] = [
  { key: 'todos', label: 'Todos', test: () => true, stock: true },
  { key: 'ativos', label: 'Ativos', test: (p) => p.active, stock: false },
  // Vendável agora: ativo, com preço e com saldo (a venda não deixa o estoque negativo).
  { key: 'venda', label: 'Disponível para venda', test: (p) => p.active && !noPrice(p) && p.stockQty > 0, stock: false },
  { key: 'inativos', label: 'Inativos', test: (p) => !p.active, stock: false },
  // Repor = estoque baixo ou zerado (o "Sem estoque" ao lado separa os zerados).
  { key: 'alerta', label: 'Repor estoque', test: (p) => p.active && level(p) !== 'ok', stock: true },
  { key: 'zerado', label: 'Sem estoque', test: (p) => p.active && p.stockQty <= 0, stock: true },
  { key: 'sem_preco', label: 'Sem preço', test: (p) => p.active && noPrice(p), stock: false },
]
// Estoque: só as visões de saldo; Produtos: as de cadastro.
const views = computed(() => allViews.filter((v) => v.key === 'todos' || v.stock === stockMode.value))
// Busca e categoria valem para as contagens dos chips.
const searched = computed(() => {
  const words = fold(filter.value).split(/\s+/).filter(Boolean)
  const cat = categoryFilter.value
  return products.value.filter((p) => {
    if (cat !== 'todas' && (cat === 'sem' ? p.categoryId != null : p.categoryId !== cat)) return false
    const hay = fold(`${p.name} ${p.barcode ?? ''} ${p.sku ?? ''} ${p.categoryName ?? ''}`)
    return words.every((w) => hay.includes(w))
  })
})
const viewCount = (v: (typeof allViews)[number]) => searched.value.filter(v.test).length
const filtered = computed(() => {
  const test = (views.value.find((v) => v.key === view.value) ?? allViews[0]).test
  const list = searched.value.filter(test)
  const by: Record<string, (a: Product, b: Product) => number> = {
    nome: (a, b) => a.name.localeCompare(b.name, 'pt-BR'),
    estoque: (a, b) => a.stockQty - b.stockQty,
    preco: (a, b) => b.priceCents - a.priceCents,
    recentes: (a, b) => b.id - a.id,
  }
  return [...list].sort(by[sortBy.value])
})
const hasFilters = computed(() => !!filter.value.trim() || categoryFilter.value !== 'todas' || view.value !== 'todos')
function clearFilters() {
  filter.value = ''
  categoryFilter.value = 'todas'
  view.value = 'todos'
}

async function loadCategories() {
  try {
    categories.value = await call(ListCategories())
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    // Serviços e planos (sem estoque) ficam na sub-aba Serviços.
    products.value = (await call(ListProducts())).filter((p) => !p.noStock)
  } catch (e) {
    loadError.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
// Exporta a lista como está na tela (busca, categoria e visão aplicadas).
function exportProducts() {
  const cost = can('ver_custo')
  exportTable('Produtos e estoque', `${filtered.value.length} produto(s)`, [['Produto', 'text'], ['Código de barras', 'text'], ['SKU', 'text'], ['Categoria', 'text'], ...(cost ? [['Custo', 'money'] as [string, 'money']] : []), ['Preço', 'money'], ['Estoque', 'int'], ['Estoque mínimo', 'int'], ['Situação', 'text']],
    filtered.value.map((p) => [p.name, p.barcode ?? '', p.sku ?? '', p.categoryName ?? 'Sem categoria', ...(cost ? [p.costCents] : []), p.priceCents, p.stockQty, p.minStock, p.active ? (level(p) === 'ok' ? 'Ativo' : level(p) === 'zerado' ? 'Sem estoque' : 'Estoque baixo') : 'Inativo']))
}

// O aviso de estoque do painel abre direto em "Repor estoque"; trocar entre
// Produtos e Estoque (mesma tela) volta para "Todos".
watch(() => route.fullPath, () => (view.value = route.query.view === 'alerta' ? 'alerta' : 'todos'), { immediate: true })
onMounted(() => {
  load()
  loadCategories()
})

// ---------- cadastro / edição ----------
const form = reactive({ open: false, id: 0, name: '', barcode: '', sku: '', price: '', active: true, initialStock: '0', categoryId: null as number | null, cost: '', minStock: '0', busy: false, error: '' })
const formMargin = computed(() => margin(parseMoney(form.price) ?? 0, parseMoney(form.cost) ?? 0))
const belowCost = computed(() => (parseMoney(form.cost) ?? 0) > 0 && (parseMoney(form.price) ?? 0) < (parseMoney(form.cost) ?? 0))

// SKU sugerido no cadastro novo (o código de barras fica em branco); se o usuário
// não mexer, o backend grava o próximo livre ao salvar (outro caixa pode ter usado este).
const suggested = ref<models.ProductCode | null>(null)

async function openForm(p?: Product) {
  Object.assign(form, {
    open: true, id: p?.id ?? 0, name: p?.name ?? '', barcode: p?.barcode ?? '', sku: p?.sku ?? '',
    price: p ? centsToInput(p.priceCents) : '', active: p?.active ?? true, initialStock: '0', categoryId: p?.categoryId ?? null,
    cost: p?.costCents ? centsToInput(p.costCents) : '', minStock: String(p?.minStock ?? 0), busy: false, error: '',
  })
  suggested.value = null
  Object.assign(newCat, { open: false, name: '', busy: false })
  if (p) return
  try {
    const c = await call(SuggestProductCode())
    if (form.open && !form.id && !form.sku) {
      suggested.value = c
      form.sku = c.sku
    }
  } catch {
    // sem sugestão o cadastro segue com os campos vazios
  }
}

async function saveForm() {
  const price = parseMoney(form.price)
  if (!form.name.trim()) return (form.error = 'Informe o nome do produto.')
  if (price === null) return (form.error = 'Preço inválido. Exemplo: 10,50')
  const initial = form.initialStock.trim() === '0' || form.initialStock.trim() === '' ? 0 : parseQty(form.initialStock)
  if (!form.id && initial === null) return (form.error = 'Estoque inicial inválido.')
  const cost = form.cost.trim() === '' ? 0 : parseMoney(form.cost)
  if (cost === null) return (form.error = 'Custo inválido. Exemplo: 6,50')
  const minStock = form.minStock.trim() === '' || form.minStock.trim() === '0' ? 0 : parseQty(form.minStock)
  if (minStock === null) return (form.error = 'Estoque mínimo inválido.')
  form.busy = true
  form.error = ''
  try {
    const input = models.ProductInput.createFrom({
      id: form.id, name: form.name, barcode: form.barcode.trim() || null, sku: form.sku.trim() || null, priceCents: price,
      active: form.active, initialStock: form.id ? 0 : initial, categoryId: form.categoryId,
      // Sem ver_custo o campo nem aparece: manda null para manter o custo gravado.
      costCents: can('ver_custo') ? cost : null, minStock,
      autoSku: !form.id && !!suggested.value && form.sku.trim() === suggested.value.sku,
    })
    const saved = await call( (form.id ? UpdateProduct(input) : CreateProduct(input)))
    toast.success(form.id ? `Produto "${saved.name}" atualizado.` : `Produto "${saved.name}" cadastrado.`)
    form.open = false
    load()
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}

// Preço digitado direto na lista para produtos sem preço (tira o "a precificar").
const quickPrice = reactive<Record<number, string>>({})
// editAllPrices: caixinha no título "Preço" que deixa todos os preços editáveis.
const editAllPrices = ref(false)
const priceEditable = (p: Product) => can('alterar_preco') && (editAllPrices.value || noPrice(p))
function toggleEditAll() {
  for (const p of products.value) {
    if (editAllPrices.value && !noPrice(p)) quickPrice[p.id] = centsToInput(p.priceCents)
    else if (!editAllPrices.value) delete quickPrice[p.id]
  }
}
async function saveQuickPrice(p: Product) {
  const text = (quickPrice[p.id] ?? '').trim()
  if (!text) return
  const price = parseMoney(text)
  if (price === p.priceCents) return // não mudou
  if (price === null || price === 0) return toast.error('Preço inválido. Exemplo: 10,50')
  try {
    await call(UpdateProduct(models.ProductInput.createFrom({ ...p, priceCents: price, costCents: null })))
    if (editAllPrices.value) quickPrice[p.id] = centsToInput(price)
    else delete quickPrice[p.id]
    toast.success(`Preço de "${p.name}" definido: ${money(price)}.`)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// Desativar pede confirmação (some da venda); ativar vai direto.
const deactivateAsk = ref<Product | null>(null)
async function toggleActive(p: Product, confirmed = false) {
  if (p.active && !confirmed) return (deactivateAsk.value = p)
  try {
    await call( UpdateProduct(models.ProductInput.createFrom({ ...p, active: !p.active, costCents: null })))
    toast.success(p.active ? `"${p.name}" desativado.` : `"${p.name}" ativado.`)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// Categoria nova direto do formulário do produto, já selecionada.
const newCat = reactive({ open: false, name: '', busy: false })
async function addCategory() {
  const name = newCat.name.trim()
  if (!name) return
  newCat.busy = true
  try {
    const c = await call(SaveCategory(models.Category.createFrom({ id: 0, name, active: true })))
    categories.value = [...categories.value, c]
    form.categoryId = c.id
    Object.assign(newCat, { open: false, name: '' })
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    newCat.busy = false
  }
}

// Excluir: sem histórico apaga; com histórico o backend só desativa.
const deleteAsk = ref<Product | null>(null)
async function deleteProduct(p: Product) {
  try {
    const deleted = await call(DeleteProduct(p.id))
    toast.success(deleted ? `"${p.name}" excluído.` : `"${p.name}" já tem movimentação e foi desativado em vez de excluído.`)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// ---------- operações de estoque ----------
type StockOp = 'entrada' | 'ajuste' | 'devolucao'
const op = reactive({ open: false, product: null as Product | null, kind: 'entrada' as StockOp, qty: '', notes: '', busy: false, error: '' })

function openOp(p: Product, kind: StockOp) {
  Object.assign(op, { open: true, product: p, kind, qty: '', notes: '', busy: false, error: '' })
}

async function saveOp() {
  if (!op.product) return
  const qty = parseQty(op.qty, op.kind === 'ajuste')
  if (qty === null) return (op.error = op.kind === 'ajuste' ? 'Informe um número inteiro diferente de zero (use - para reduzir).' : 'Informe um número inteiro maior que zero.')
  if (op.kind !== 'entrada' && !op.notes.trim()) return (op.error = 'Informe o motivo.')
  op.busy = true
  op.error = ''
  try {
    const id = op.product.id
    const p = await call(
      op.kind === 'entrada' ? RegisterStockEntry(id, qty, op.notes) : op.kind === 'ajuste' ? AdjustStock(id, qty, op.notes) : RegisterStockReturn(id, qty, op.notes),
    )
    toast.success(`Estoque de "${p.name}" atualizado: ${p.stockQty} un.`)
    op.open = false
    load()
  } catch (e) {
    op.error = errorMessage(e)
  } finally {
    op.busy = false
  }
}

// ---------- histórico ----------
const hist = reactive({ open: false, product: null as Product | null, start: today(-30), end: today(), items: [] as StockMovement[], loading: false, error: '' })

async function openHist(p: Product) {
  Object.assign(hist, { open: true, product: p, items: [], error: '' })
  await loadHist()
}

async function loadHist() {
  if (!hist.product) return
  hist.loading = true
  hist.error = ''
  try {
    hist.items = await call(ListStockMovements(hist.product.id, hist.start, hist.end))
  } catch (e) {
    hist.error = errorMessage(e)
  } finally {
    hist.loading = false
  }
}

const opTitles: Record<StockOp, string> = { entrada: 'Entrada de estoque', ajuste: 'Ajuste de estoque', devolucao: 'Devolução ao estoque' }
</script>

<template>
  <section class="card stack">
    <div v-if="!stockMode" class="tabs seg" role="tablist" aria-label="Tipo de produto">
      <button role="tab" :aria-selected="sub === 'mercadorias'" @click="sub = 'mercadorias'">Mercadorias</button>
      <button role="tab" :aria-selected="sub === 'servicos'" @click="sub = 'servicos'">Serviços</button>
    </div>
    <div v-else class="tabs seg" role="tablist" aria-label="Estoque">
      <button role="tab" :aria-selected="stockSub === 'saldo'" @click="stockSub = 'saldo'">Saldo</button>
      <button role="tab" :aria-selected="stockSub === 'historico'" @click="stockSub = 'historico'">Histórico</button>
    </div>
    <ServicesList v-if="!stockMode && sub === 'servicos'" />
    <StockMovements v-else-if="stockMode && stockSub === 'historico'" />
    <template v-else>
    <div class="row products-toolbar">
      <input v-model="filter" class="products-search" type="search" placeholder="Buscar produto, SKU ou código" aria-label="Buscar produtos" />
      <select v-model="categoryFilter" aria-label="Filtrar por categoria" style="max-width: 170px">
        <option value="todas">Por categoria</option>
        <option value="sem">Sem categoria</option>
        <option v-for="c in categories" :key="c.id" :value="c.id">{{ c.name }}{{ c.active ? '' : ' (inativa)' }}</option>
      </select>
      <select v-model="sortBy" class="sort-select" aria-label="Ordenar">
        <option value="nome">Ordem: nome (A–Z)</option>
        <option value="estoque">Ordem: menor estoque</option>
        <option value="preco">Ordem: maior preço</option>
        <option value="recentes">Ordem: mais recentes</option>
      </select>
      <button class="ghost" @click="exportProducts">Exportar Excel</button>
      <template v-if="!stockMode">
        <button v-if="can('cadastrar_produto')" class="ghost" @click="importOpen = true">Importar planilha</button>
        <button v-if="can('cadastrar_produto')" class="ghost" @click="showCategories = true">Categorias</button>
      </template>
      <span class="spacer" />
      <button v-if="can('cadastrar_produto') && !stockMode" class="primary" @click="openForm()">+ Novo produto</button>
    </div>
    <div class="row chips" role="group" aria-label="Mostrar">
      <div class="chip-track">
        <button v-for="v in views" :key="v.key" class="chip" :aria-pressed="view === v.key" @click="view = v.key">
          {{ v.label }} <span class="chip-count">{{ viewCount(v) }}</span>
        </button>
      </div>
      <span class="spacer" />
      <span class="small muted">{{ filtered.length }} de {{ products.length }} produto(s)</span>
      <button v-if="hasFilters" class="link small" @click="clearFilters">Limpar filtros</button>
    </div>
    <div v-if="noPriceCount && view !== 'sem_preco' && !stockMode" class="alert warn row between">
      <span><strong>⚠ {{ noPriceCount }} produto(s) sem preço de venda.</strong> Defina o preço na coluna Preço para que possam ser vendidos.</span>
      <button @click="view = 'sem_preco'">Ver só esses</button>
    </div>
    <div v-if="loading" class="state">Carregando produtos…</div>
    <div v-else-if="loadError" class="alert error">{{ loadError }} <button class="link" @click="load">Tentar novamente</button></div>
    <div v-else-if="filtered.length === 0" class="state">
      {{ products.length === 0 ? 'Nenhum produto cadastrado. Clique em "Novo produto".' : 'Nenhum produto corresponde ao filtro.' }}
    </div>
    <div v-else class="table-wrap">
      <table class="products-table">
        <thead>
          <tr v-if="stockMode"><th class="left">Produto</th><th class="center col-stock">Estoque</th><th class="center col-stock">Mínimo</th><th class="right">Ações</th></tr>
          <tr v-else><th class="left">Produto</th><th v-if="can('ver_custo')" class="center col-money">Custo</th><th class="center nowrap col-price">
            <!-- A palavra fica no centro; a caixinha vem logo depois dela, fora do centro. -->
            <label v-if="can('alterar_preco')" class="price-head" title="Deixar todos os preços editáveis">
              Preço<input v-model="editAllPrices" type="checkbox" aria-label="Deixar todos os preços editáveis" @change="toggleEditAll" />
            </label>
            <template v-else>Preço</template>
          </th><th v-if="can('ver_custo')" class="center col-money">Margem</th><th class="right">Ações</th></tr>
        </thead>
        <tbody>
          <tr v-for="p in filtered" :key="p.id" :class="{ inactive: !p.active, 'stock-zerado': stockMode && p.active && level(p) === 'zerado', 'stock-baixo': stockMode && p.active && isLow(p) }">
            <td class="product-cell left">
              <span class="product-name">{{ p.name }}</span>
              <span v-if="!p.active" class="badge">Inativo</span>
              <button v-if="p.modelId && can('cadastrar_produto')" class="link small" title="Ver e ampliar a grade" @click="grade = p.modelId">grade</button>
              <div class="product-meta"><template v-if="p.sku">SKU <span class="mono">{{ p.sku }}</span> · </template><template v-if="p.barcode"><span class="mono">{{ p.barcode }}</span> · </template>{{ p.categoryName ?? 'Sem categoria' }}</div>
            </td>
            <template v-if="stockMode">
            <td class="center mono">
              <span :class="level(p) === 'zerado' ? 'badge bad' : isLow(p) ? 'badge warn' : ''">{{ p.stockQty }}</span>
              <div v-if="p.active && level(p) !== 'ok'" class="small" :style="{ color: level(p) === 'zerado' ? 'var(--danger)' : 'var(--warning)', fontWeight: 600 }">{{ level(p) === 'zerado' ? 'repor já' : 'repor' }}</div>
            </td>
            <td class="center mono muted" :title="p.minStock ? '' : 'Sem mínimo próprio: vale o padrão das Configurações'">{{ p.minStock || `${settings?.lowStockDefault ?? 0}*` }}</td>
            <td class="right actions-cell products-actions">
              <button v-if="can('entrada_estoque')" class="link icon-btn" title="Entrada no estoque" aria-label="Entrada no estoque" @click="openOp(p, 'entrada')"><Icon name="entrada" /><span>Entrada</span></button>
              <button v-if="can('ajuste_estoque')" class="link icon-btn" title="Ajuste de estoque" aria-label="Ajuste de estoque" @click="openOp(p, 'ajuste')"><Icon name="ajuste" /><span>Ajuste</span></button>
              <button v-if="can('devolucao_estoque')" class="link icon-btn" title="Devolução" aria-label="Devolução" @click="openOp(p, 'devolucao')"><Icon name="devolucao" /><span>Devolver</span></button>
              <button class="link icon-btn" title="Histórico de movimentações" aria-label="Histórico" @click="openHist(p)"><Icon name="historico" /><span>Histórico</span></button>
            </td>
            </template>
            <template v-else>
            <td v-if="can('ver_custo')" class="center mono">{{ p.costCents ? money(p.costCents) : '—' }}</td>
            <td class="center mono">
              <input
                v-if="priceEditable(p)" v-model="quickPrice[p.id]" class="quick-price" :class="{ 'no-price': noPrice(p) }" inputmode="decimal"
                placeholder="Preço" :aria-label="`Preço de venda de ${p.name}`" title="Digite o preço e tecle Enter"
                @keydown.enter.prevent="saveQuickPrice(p)" @blur="saveQuickPrice(p)"
              />
              <span v-else-if="noPrice(p)" style="color: var(--danger); font-weight: 700">sem preço</span>
              <template v-else>{{ money(p.priceCents) }}</template>
            </td>
            <td v-if="can('ver_custo')" class="center mono" :style="{ color: p.costCents > p.priceCents ? 'var(--danger)' : '' }">
              {{ margin(p.priceCents, p.costCents) ?? '—' }}{{ margin(p.priceCents, p.costCents) != null ? '%' : '' }}
            </td>
            <td class="right actions-cell products-actions">
              <button class="link icon-btn" title="Editar" aria-label="Editar" @click="openForm(p)"><Icon name="edit" /><span>Editar</span></button>
              <button v-if="can('ativar_produto')" class="link icon-btn" :title="p.active ? 'Desativar' : 'Ativar'" :aria-label="p.active ? 'Desativar' : 'Ativar'"
                :style="{ color: p.active ? 'var(--danger)' : 'var(--success)' }" @click="toggleActive(p)"><Icon :name="p.active ? 'desativar' : 'ativar'" /><span>{{ p.active ? 'Desativar' : 'Ativar' }}</span></button>
              <button v-if="can('cadastrar_produto')" class="link icon-btn" title="Excluir" aria-label="Excluir" style="color: var(--danger)" @click="deleteAsk = p"><Icon name="remover" /><span>Excluir</span></button>
            </td>
            </template>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="small muted" style="margin: 0">
      <template v-if="stockMode">O estoque só muda por entrada, ajuste, devolução, venda ou cancelamento, sempre com registro. * = mínimo padrão das Configurações.</template>
      <template v-else>Produto com histórico não é excluído, só desativado. Para mexer no saldo, use a aba Estoque.</template>
    </p>
    </template>
  </section>


  <ImportModal v-if="importOpen" kind="produtos" @close="importOpen = false" @done="load(); loadCategories()" />
  <Modal v-if="form.open" :title="form.id ? 'Editar produto' : 'Novo produto'" :busy="form.busy" @close="form.open = false">
    <form class="stack" @submit.prevent="saveForm">
      <label class="field">Nome<input v-model="form.name" maxlength="200" required /></label>
      <label class="field">Código de barras (opcional)<input v-model="form.barcode" maxlength="64" /></label>
      <label class="field">SKU / código interno (opcional)<input v-model="form.sku" maxlength="64" /></label>
      <p v-if="suggested" class="small muted" style="margin: 0">SKU gerado automaticamente; dá para trocar ou apagar.</p>
      <label class="field">Categoria (opcional)
        <select v-model="form.categoryId">
          <option :value="null">Sem categoria</option>
          <option v-for="c in categories.filter((c) => c.active || c.id === form.categoryId)" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </label>
      <div v-if="newCat.open" class="row" style="gap: 6px; flex-wrap: nowrap">
        <input v-model="newCat.name" maxlength="80" placeholder="Nome da nova categoria" aria-label="Nome da nova categoria" style="flex: 1"
          @keydown.enter.prevent="addCategory" />
        <button type="button" class="primary" :disabled="newCat.busy || !newCat.name.trim()" @click="addCategory">Adicionar</button>
        <button type="button" :disabled="newCat.busy" @click="newCat.open = false">Cancelar</button>
      </div>
      <button v-else-if="can('cadastrar_produto')" type="button" class="link small" style="align-self: start" @click="newCat.open = true">+ Nova categoria</button>
      <div class="grid grid-2">
        <label v-if="can('ver_custo')" class="field">Preço de custo (R$)<input v-model="form.cost" inputmode="decimal" placeholder="0,00" /></label>
        <label v-if="!form.id || can('alterar_preco')" class="field">Preço de venda (R$)<input v-model="form.price" inputmode="decimal" placeholder="0,00" required /></label>
        <label v-if="!form.id" class="field">Estoque inicial<input v-model="form.initialStock" inputmode="numeric" /></label>
        <label class="field">Estoque mínimo (aviso de reposição)<input v-model="form.minStock" inputmode="numeric" /></label>
      </div>
      <p v-if="can('ver_custo') && formMargin != null" class="small muted" style="margin: 0">Margem: <strong>{{ formMargin }}%</strong> do preço de venda.</p>
      <div v-if="can('ver_custo') && belowCost" class="alert warn" role="alert">Atenção: o preço de venda está abaixo do custo.</div>
      <label v-if="can('ativar_produto')" class="check"><input v-model="form.active" type="checkbox" /> Produto ativo (disponível para venda)</label>
      <p v-if="form.id" class="small muted">Para alterar o estoque, use a aba Estoque (Entrada, Ajuste ou Devolução). Vendas anteriores mantêm o nome e o preço da época.</p>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">{{ form.busy ? 'Salvando…' : 'Salvar' }}</button>
      </footer>
    </form>
  </Modal>

  <Modal v-if="op.open && op.product" :title="`${opTitles[op.kind]} — ${op.product.name}`" :busy="op.busy" @close="op.open = false">
    <form class="stack" @submit.prevent="saveOp">
      <p class="muted">Estoque atual: <strong>{{ op.product.stockQty }}</strong></p>
      <label class="field">
        {{ op.kind === 'ajuste' ? 'Quantidade (positiva para acrescentar, negativa para reduzir)' : 'Quantidade' }}
        <input v-model="op.qty" inputmode="numeric" required />
      </label>
      <label class="field">
        {{ op.kind === 'entrada' ? 'Observação (nota fiscal, fornecedor…)' : 'Motivo (obrigatório)' }}
        <textarea v-model="op.notes" maxlength="500" :required="op.kind !== 'entrada'" />
      </label>
      <div v-if="op.error" class="alert error" role="alert">{{ op.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="op.busy" @click="op.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="op.busy">{{ op.busy ? 'Registrando…' : 'Registrar' }}</button>
      </footer>
    </form>
  </Modal>

  <Modal v-if="hist.open && hist.product" :title="`Movimentações — ${hist.product.name}`" wide @close="hist.open = false">
    <div class="row" style="margin-bottom: 10px">
      <label class="field">De<input v-model="hist.start" type="date" /></label>
      <label class="field">Até<input v-model="hist.end" type="date" /></label>
      <button style="align-self: end" :disabled="hist.loading" @click="loadHist">Filtrar</button>
    </div>
    <div v-if="hist.loading" class="state">Carregando…</div>
    <div v-else-if="hist.error" class="alert error">{{ hist.error }}</div>
    <div v-else-if="hist.items.length === 0" class="state">Nenhuma movimentação no período.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Data</th><th>Motivo</th><th class="right">Qtd.</th><th>Referência</th><th>Observação</th></tr></thead>
        <tbody>
          <tr v-for="m in hist.items" :key="m.id">
            <td class="nowrap">{{ formatDateTime(m.createdAt) }}</td>
            <td>{{ stockReasonLabels[m.reason] ?? m.reason }}</td>
            <td class="right mono">{{ m.qtyChange > 0 ? '+' : '' }}{{ m.qtyChange }}</td>
            <td class="small">{{ m.referenceType === 'sale' ? `Venda nº ${m.referenceId}` : m.referenceType ?? '' }}</td>
            <td class="small">{{ m.notes ?? '' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <template #footer><button @click="hist.open = false">Fechar</button></template>
  </Modal>

  <ConfirmDialog v-if="deleteAsk" title="Excluir produto" danger confirm-label="Excluir"
    :message="`Excluir “${deleteAsk.name}”? Se ele já teve venda, entrada ou movimento de estoque, será desativado em vez de excluído, para não quebrar o histórico.`"
    @confirm="deleteProduct(deleteAsk); deleteAsk = null" @cancel="deleteAsk = null" />
  <ConfirmDialog v-if="deactivateAsk" title="Desativar produto" danger confirm-label="Desativar"
    :message="`Desativar “${deactivateAsk.name}”? Ele deixa de aparecer na venda. Dá para ativar de novo depois.`"
    @confirm="toggleActive(deactivateAsk, true); deactivateAsk = null" @cancel="deactivateAsk = null" />
  <GradeModal v-if="grade !== undefined" :model-id="grade" :categories="categories" @close="grade = undefined" @changed="load" />
  <CategoriesModal v-if="showCategories" :categories="categories" @close="showCategories = false" @changed="loadCategories(); load()" />
</template>
