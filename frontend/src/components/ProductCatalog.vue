<!-- Caminho: pdv/frontend/src/components/ProductCatalog.vue -->
<script setup lang="ts">
// Lista de produtos vendáveis da tela de vendas: filtra enquanto digita
// (nome ou código, sem acento, palavras em qualquer ordem), Enter com código de
// barras adiciona direto (leitor; "5*código" adiciona 5), clique na linha adiciona ao carrinho.
import { computed, onMounted, ref } from 'vue'
import { ListSaleCatalog, SearchProductByBarcode } from '../../wailsjs/go/main/App'
import { call, errorMessage, isCode } from '../api'
import { money } from '../composables/format'
import type { Product } from '../types'

const props = defineProps<{ inCart: Record<number, number> }>()
const emit = defineEmits<{ pick: [product: Product, qty: number] }>()

const all = ref<Product[]>([])
const term = ref('')
const category = ref('')
const error = ref('')
const loading = ref(true)
const input = ref<HTMLInputElement | null>(null)

async function load() {
  try {
    all.value = await call(ListSaleCatalog())
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
defineExpose({ focus: () => input.value?.focus(), reload: load })

const fold = (t: string) => t.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
const categories = computed(() => [...new Set(all.value.map((p) => p.categoryName ?? 'Sem categoria'))].sort((a, b) => a.localeCompare(b, 'pt-BR')))
function matching(text: string) {
  const words = fold(text.replace(/^\s*\d{1,5}\s*\*/, '')).split(/\s+/).filter(Boolean)
  return all.value.filter((p) => {
    if (category.value && (p.categoryName ?? 'Sem categoria') !== category.value) return false
    const hay = fold(`${p.name} ${p.barcode ?? ''} ${p.sku ?? ''}`)
    return words.every((w) => hay.includes(w))
  })
}
const shown = computed(() => matching(term.value))
const price = (p: Product) => p.promo?.priceCents || p.priceCents
// Estoque que ainda sobra depois do que já está no carrinho.
const left = (p: Product) => (p.noStock ? Infinity : p.stockQty - (props.inCart[p.id] ?? 0))

function choose(p: Product, qty = 1) {
  if (left(p) <= 0) return (error.value = `"${p.name}" sem estoque disponível.`)
  if (left(p) < qty) return (error.value = `"${p.name}": só há ${left(p)} disponível(is).`)
  error.value = ''
  emit('pick', p, qty)
  return true
}

async function onEnter() {
  // "5*7891234567890": quantidade antes do asterisco.
  const raw = term.value.trim()
  const m = /^(\d{1,5})\s*\*\s*(.+)$/.exec(raw)
  const qty = m ? Number(m[1]) : 1
  const v = m ? m[2] : raw
  if (!v) return
  if (qty < 1) return (error.value = 'Quantidade inválida: use 1 ou mais antes do *.')
  error.value = ''
  // Libera o campo já: o leitor pode mandar o próximo código antes da resposta.
  // Se não der para adicionar, o texto volta para o operador corrigir.
  term.value = ''
  const keep = () => {
    if (!term.value) term.value = raw
  }
  try {
    const p = await call(SearchProductByBarcode(v))
    if (p.pricePending) {
      keep()
      return (error.value = `"${p.name}" está a precificar (sem preço de venda). Peça ao gerente para definir o preço.`)
    }
    if (!choose(p, qty)) keep()
    return
  } catch (e) {
    if (!isCode(e, 'PRODUTO_INEXISTENTE')) {
      keep()
      return (error.value = errorMessage(e))
    }
  }
  const found = matching(raw)
  if (found.length === 1) {
    if (!choose(found[0], qty)) keep()
    return
  }
  keep()
  error.value = found.length ? 'Mais de um produto: clique no desejado.' : 'Nenhum produto encontrado.'
}
</script>

<template>
  <div class="catalog">
    <input ref="input" v-model="term" class="big" style="width: 100%" type="search" placeholder="Código de barras, SKU ou nome (F2) · 5*código vende 5"
        aria-label="Buscar produto por código de barras ou nome" autocomplete="off" @keydown.enter.prevent="onEnter" @keydown.esc="term = ''" />
    <select v-if="categories.length > 1" v-model="category" aria-label="Filtrar por categoria">
      <option value="">Todas as categorias</option>
      <option v-for="c in categories" :key="c" :value="c">{{ c }}</option>
    </select>
    <p v-if="error" class="small" style="color: var(--danger); margin: 0" role="alert">{{ error }}</p>
    <div class="small muted">{{ loading ? 'Carregando produtos…' : `${shown.length} produto(s)` }}</div>
    <div class="catalog-list">
      <button v-for="p in shown" :key="p.id" type="button" class="item" :class="{ out: left(p) <= 0, added: inCart[p.id] }"
        :title="left(p) <= 0 ? 'Sem estoque disponível' : 'Adicionar ao carrinho'" @click="choose(p)">
        <span class="name">{{ p.name }}
          <small v-if="p.promo" class="promo">{{ p.promo.label }}</small>
        </span>
        <span class="price mono">{{ money(price(p)) }}</span>
        <span class="meta small">
          <span v-if="inCart[p.id]" class="badge ok">{{ inCart[p.id] }} no carrinho</span>
          <span v-else-if="left(p) <= 0" class="badge bad">sem estoque</span>
          <span v-else-if="!p.noStock" class="muted">estoque {{ p.stockQty }}</span>
        </span>
      </button>
      <div v-if="!loading && !shown.length" class="state">Nenhum produto.</div>
    </div>
  </div>
</template>

<style scoped>
.catalog { display: flex; flex-direction: column; gap: 6px; min-height: 0; flex: 1; }
.catalog-list { flex: 1; min-height: 0; overflow-y: auto; border: 1px solid var(--border); border-radius: var(--radius); }
.item { display: grid; grid-template-columns: 1fr auto; gap: 2px 8px; width: 100%; text-align: left; padding: 7px 10px;
  border: 0; border-bottom: 1px solid var(--border); border-radius: 0; background: var(--surface); cursor: pointer; }
.item:hover { background: var(--primary-soft); }
.item.added { background: var(--success-soft); }
.item.out { opacity: 0.55; cursor: not-allowed; }
.name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; }
.promo { display: block; color: var(--success); font-weight: 400; }
.price { text-align: right; }
.meta { grid-column: 1 / -1; }
</style>
