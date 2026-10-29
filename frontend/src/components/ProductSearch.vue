<!-- Caminho: pdv/frontend/src/components/ProductSearch.vue -->
<script setup lang="ts">
// Campo único: Enter com código de barras exato adiciona direto; texto livre
// busca por nome e mostra a lista para escolha.
import { onMounted, ref, watch } from 'vue'
import { ListCategories, SearchProductByBarcode, SearchProducts } from '../../wailsjs/go/main/App'
import { call, errorMessage, isCode } from '../api'
import { money } from '../composables/format'
import type { Category, Product } from '../types'

// noCategories esconde o filtro de categoria (usado onde só importa achar o produto).
const props = defineProps<{ onlyActive?: boolean; placeholder?: string; noCategories?: boolean }>()
const emit = defineEmits<{ pick: [product: Product] }>()

const term = ref('')
const results = ref<Product[]>([])
const loading = ref(false)
const error = ref('')
const input = ref<HTMLInputElement | null>(null)
// 0 = todas as categorias; com categoria e sem texto, lista a categoria inteira.
const categoryId = ref(0)
const categories = ref<Category[]>([])
let timer: number | undefined

onMounted(async () => {
  if (props.noCategories) return
  try {
    categories.value = (await call(ListCategories())).filter((c) => c.active)
  } catch {
    // sem categorias o filtro só não aparece
  }
})
let seq = 0

defineExpose({ focus: () => input.value?.focus() })

watch([term, categoryId], ([v]) => {
  window.clearTimeout(timer)
  error.value = ''
  if (v.trim().length < 2 && !categoryId.value) {
    results.value = []
    return
  }
  timer = window.setTimeout(() => search(v), 250)
})

async function search(v: string): Promise<Product[]> {
  const my = ++seq
  loading.value = true
  try {
    const list = await call(SearchProducts(v.trim().length < 2 ? '' : v, props.onlyActive ?? true, categoryId.value))
    if (my === seq) results.value = list
    return list
  } catch (e) {
    if (my === seq) error.value = errorMessage(e)
    return []
  } finally {
    if (my === seq) loading.value = false
  }
}

async function onEnter() {
  const v = term.value.trim()
  if (!v) return
  // Primeiro tenta o código de barras exato (leitor de código envia Enter).
  try {
    const p = await call(SearchProductByBarcode(v))
    if ((props.onlyActive ?? true) && p.pricePending) {
      error.value = `"${p.name}" está a precificar (sem preço de venda). Peça ao gerente para definir o preço.`
      return
    }
    choose(p)
    return
  } catch (e) {
    if (!isCode(e, 'PRODUTO_INEXISTENTE')) {
      error.value = errorMessage(e)
      return
    }
  }
  const list = results.value.length ? results.value : await search(v)
  if (list.length === 1) choose(list[0])
  else if (list.length === 0) error.value = 'Nenhum produto encontrado.'
}

function choose(p: Product) {
  emit('pick', p)
  term.value = ''
  categoryId.value = 0 // a lista some depois de escolher, como na busca sem categoria
  results.value = []
  input.value?.focus()
}
</script>

<template>
  <div class="stack" style="gap: 6px">
    <div class="row" style="gap: 6px; flex-wrap: nowrap">
    <select v-if="categories.length" v-model="categoryId" aria-label="Filtrar por categoria" style="max-width: 180px">
      <option :value="0">Todas</option>
      <option v-for="c in categories" :key="c.id" :value="c.id">{{ c.name }}</option>
    </select>
    <input
      ref="input"
      v-model="term"
      class="big"
      style="flex: 1"
      type="search"
      :placeholder="placeholder ?? 'Código de barras ou nome do produto'"
      aria-label="Buscar produto por código de barras ou nome"
      autocomplete="off"
      @keydown.enter.prevent="onEnter"
      @keydown.esc="term = ''"
    />
    </div>
    <p v-if="error" class="small" style="color: var(--danger)" role="alert">{{ error }}</p>
    <div v-if="loading" class="small muted">Buscando…</div>
    <div v-if="results.length" class="table-wrap card" style="padding: 0; max-height: 260px; overflow: auto">
      <table>
        <tbody>
          <tr v-for="p in results" :key="p.id" class="clickable" :class="{ inactive: !p.active }" @click="choose(p)">
            <td class="left">
              <!-- O clique (ou Enter no botão) sobe até a linha inteira. -->
              <button class="link result-name">{{ p.name }}</button>
              <div class="small muted">{{ p.barcode || 'sem código' }}</div>
            </td>
            <td class="right mono nowrap">
              <template v-if="p.promo?.priceCents">
                <span class="small muted" style="text-decoration: line-through">{{ money(p.priceCents) }}</span>
                <strong style="color: var(--success)"> {{ money(p.promo.priceCents) }}</strong>
              </template>
              <template v-else>{{ money(p.priceCents) }}</template>
              <div v-if="p.promo" class="small" style="color: var(--success)">{{ p.promo.label }}</div>
            </td>
            <td class="right small nowrap" :class="p.stockQty === 0 ? '' : 'muted'">
              <span v-if="p.stockQty === 0" class="badge bad">sem estoque</span>
              <span v-else>estoque {{ p.stockQty }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
