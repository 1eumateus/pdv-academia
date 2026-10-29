<!-- Caminho: pdv/frontend/src/views/Etiquetas.vue -->
<script setup lang="ts">
// Etiquetas de preço com código de barras. Três layouts (gôndola, tag de
// roupa e folha A4 tipo Pimaco), com medidas em mm guardadas nesta máquina
// (dependem da impressora e do papel). Impressão pela janela do Windows.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { AssignInternalBarcodes, GetPurchase, ListPurchases, ProductsForSale } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import Barcode from '../components/Barcode.vue'
import Icon from '../components/Icon.vue'
import ProductSearch from '../components/ProductSearch.vue'
import { storeName } from '../composables/cashState'
import { money, today } from '../composables/format'
import { can } from '../composables/session'
import { savePdf } from '../composables/exportTable'
import { useToast } from '../composables/toast'
import type { models, Product } from '../types'

const toast = useToast()
const route = useRoute()

type LayoutKey = 'gondola' | 'tag' | 'a4'
interface Layout { w: number; h: number; cols: number; rows: number; top: number; left: number; gapX: number; gapY: number }
const layoutNames: Record<LayoutKey, string> = { gondola: 'Gôndola (impressora de etiquetas)', tag: 'Tag de roupa', a4: 'Folha A4 (tipo Pimaco)' }
const defaults: Record<LayoutKey, Layout> = {
  gondola: { w: 60, h: 40, cols: 1, rows: 1, top: 0, left: 0, gapX: 0, gapY: 0 },
  tag: { w: 35, h: 60, cols: 1, rows: 1, top: 0, left: 0, gapX: 0, gapY: 0 },
  // Pimaco 6180 (3 × 10, 66,7 × 25,4 mm).
  a4: { w: 66.7, h: 25.4, cols: 3, rows: 10, top: 12.7, left: 4.8, gapX: 2.5, gapY: 0 },
}
const STORAGE = 'pdv-etiquetas'
function loadLayouts(): Record<LayoutKey, Layout> {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE) ?? '{}')
    return { gondola: { ...defaults.gondola, ...saved.gondola }, tag: { ...defaults.tag, ...saved.tag }, a4: { ...defaults.a4, ...saved.a4 } }
  } catch {
    return structuredClone(defaults)
  }
}
const layouts = reactive(loadLayouts())
const kind = ref<LayoutKey>('gondola')
const cfg = computed(() => layouts[kind.value])
watch(layouts, () => {
  try {
    localStorage.setItem(STORAGE, JSON.stringify(layouts))
  } catch {
    // sem armazenamento: as medidas valem até fechar
  }
}, { deep: true })
function resetLayout() {
  Object.assign(layouts[kind.value], defaults[kind.value])
}

// ---------- produtos e quantidades ----------
const lines = ref<{ product: Product; qty: number }[]>([])
function add(p: Product, qty = 1) {
  const l = lines.value.find((x) => x.product.id === p.id)
  if (l) l.qty += qty
  else lines.value.push({ product: p, qty })
}
const labels = computed(() => lines.value.flatMap((l) => Array.from({ length: Math.min(Math.max(l.qty, 0), 500) }, () => l.product)))
const variation = (p: Product) => [p.size, p.color].filter(Boolean).join(' · ')
const noCode = computed(() => lines.value.filter((l) => !l.product.barcode))

// Entrada de mercadoria: uma etiqueta por unidade comprada.
const purchases = ref<models.Purchase[]>([])
const purchaseId = ref<number | null>(null)
async function loadPurchase(id: number) {
  try {
    const d = await call(GetPurchase(id))
    const prods = await call(ProductsForSale(d.items.map((i) => i.productId)))
    for (const it of d.items) {
      const p = prods.find((x) => x.id === it.productId)
      if (p) add(p, it.qty)
    }
    toast.success(`Entrada nº ${id}: ${d.items.length} produto(s) adicionados.`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
onMounted(async () => {
  try {
    if (can('compras')) purchases.value = (await call(ListPurchases(today(-90), today(), ''))).filter((p) => !p.cancelledAt)
  } catch {
    // lista de entradas é opcional aqui
  }
  const q = Number(route.query.compra)
  if (q) loadPurchase(q)
})

const genBusy = ref(false)
async function generateCodes() {
  genBusy.value = true
  try {
    const updated = await call(AssignInternalBarcodes(noCode.value.map((l) => l.product.id)))
    for (const p of updated) {
      const l = lines.value.find((x) => x.product.id === p.id)
      if (l) l.product = { ...l.product, barcode: p.barcode } as Product
    }
    toast.success(`Código interno gerado para ${updated.length} produto(s).`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    genBusy.value = false
  }
}

// ---------- impressão ----------
const perPage = computed(() => (kind.value === 'a4' ? Math.max(1, cfg.value.cols * cfg.value.rows) : 1))
const pages = computed(() => {
  const out: Product[][] = []
  for (let i = 0; i < labels.value.length; i += perPage.value) out.push(labels.value.slice(i, i + perPage.value))
  return out
})
const mm = (v: number) => `${v}mm`
const pageStyle = computed(() => (kind.value === 'a4'
  ? { width: '210mm', height: '297mm', padding: `${mm(cfg.value.top)} 0 0 ${mm(cfg.value.left)}`,
      gridTemplateColumns: `repeat(${cfg.value.cols}, ${mm(cfg.value.w)})`, gridAutoRows: mm(cfg.value.h),
      columnGap: mm(cfg.value.gapX), rowGap: mm(cfg.value.gapY) }
  : { width: mm(cfg.value.w), height: mm(cfg.value.h), gridTemplateColumns: mm(cfg.value.w), gridAutoRows: mm(cfg.value.h) }))

const sheet = ref<HTMLElement | null>(null)

function print() {
  if (!labels.value.length) return
  const style = document.createElement('style')
  style.textContent = `@page { size: ${kind.value === 'a4' ? 'A4' : `${cfg.value.w}mm ${cfg.value.h}mm`}; margin: 0 }`
  document.head.appendChild(style)
  window.addEventListener('afterprint', () => style.remove(), { once: true })
  window.print()
}
</script>

<template>
  <div class="stack">
    <section class="card stack no-print">
      <div class="row">
        <div style="flex: 1; min-width: 260px"><ProductSearch placeholder="Adicionar produto (código, SKU ou nome)" @pick="add($event)" /></div>
        <select v-if="purchases.length" v-model="purchaseId" aria-label="Etiquetas de uma entrada de mercadoria" @change="purchaseId && loadPurchase(purchaseId); purchaseId = null">
          <option :value="null">Etiquetas de uma entrada de mercadoria…</option>
          <option v-for="p in purchases" :key="p.id" :value="p.id">nº {{ p.id }} — {{ p.supplierName }} ({{ p.issueDate.split('-').reverse().join('/') }})</option>
        </select>
      </div>
      <div v-if="!lines.length" class="state">Escolha os produtos (ou uma entrada de mercadoria) para imprimir as etiquetas.</div>
      <div v-else class="table-wrap">
        <table>
          <thead><tr><th class="left">Produto</th><th>Código</th><th>Preço</th><th>Etiquetas</th><th></th></tr></thead>
          <tbody>
            <tr v-for="(l, i) in lines" :key="l.product.id">
              <td class="left">{{ l.product.name }}<div v-if="variation(l.product)" class="small muted">{{ variation(l.product) }}</div></td>
              <td class="mono small">{{ l.product.barcode || l.product.sku || '—' }}</td>
              <td class="mono">{{ money(l.product.priceCents) }}</td>
              <td><input v-model.number="l.qty" class="qty-input mono" type="number" min="0" max="500" :aria-label="`Etiquetas de ${l.product.name}`" /></td>
              <td><button class="link icon-btn" title="Remover" aria-label="Remover" style="color: var(--danger)" @click="lines.splice(i, 1)"><Icon name="remover" /></button></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="noCode.length" class="alert warn row between" style="margin: 0">
        <span>{{ noCode.length }} produto(s) sem código de barras: a etiqueta sai sem barras.</span>
        <button v-if="can('cadastrar_produto')" :disabled="genBusy" @click="generateCodes">Gerar código interno (EAN-13)</button>
      </div>
    </section>

    <section class="card stack no-print">
      <div class="row">
        <label class="field" style="min-width: 260px">Layout
          <select v-model="kind"><option v-for="(n, k) in layoutNames" :key="k" :value="k">{{ n }}</option></select>
        </label>
        <label class="field">Largura (mm)<input v-model.number="cfg.w" type="number" min="15" max="200" step="0.1" /></label>
        <label class="field">Altura (mm)<input v-model.number="cfg.h" type="number" min="10" max="200" step="0.1" /></label>
        <template v-if="kind === 'a4'">
          <label class="field">Colunas<input v-model.number="cfg.cols" type="number" min="1" max="10" /></label>
          <label class="field">Linhas<input v-model.number="cfg.rows" type="number" min="1" max="30" /></label>
          <label class="field">Margem sup. (mm)<input v-model.number="cfg.top" type="number" min="0" max="50" step="0.1" /></label>
          <label class="field">Margem esq. (mm)<input v-model.number="cfg.left" type="number" min="0" max="50" step="0.1" /></label>
          <label class="field">Espaço entre colunas (mm)<input v-model.number="cfg.gapX" type="number" min="0" max="20" step="0.1" /></label>
          <label class="field">Espaço entre linhas (mm)<input v-model.number="cfg.gapY" type="number" min="0" max="20" step="0.1" /></label>
        </template>
      </div>
      <div class="row between">
        <button class="link" @click="resetLayout">Voltar às medidas padrão</button>
        <span class="small muted">{{ labels.length }} etiqueta(s) · {{ pages.length }} página(s)</span>
        <button v-if="kind === 'a4'" :disabled="!labels.length" @click="savePdf(sheet, 'Etiquetas', '@page { size: A4; margin: 0; }')">Salvar PDF</button>
        <button class="primary" :disabled="!labels.length" @click="print">Imprimir etiquetas</button>
      </div>
    </section>

    <!-- Na tela mostra a 1ª página; na impressão, todas. -->
    <div ref="sheet" class="print-area labels-area" :class="kind">
      <div v-for="(pg, pi) in pages" :key="pi" class="label-page" :class="{ 'only-print': pi > 0 }" :style="pageStyle">
        <div v-for="(p, li) in pg" :key="li" class="label">
          <small v-if="kind === 'tag'" class="store">{{ storeName }}</small>
          <strong class="name">{{ p.name }}</strong>
          <small v-if="variation(p)" class="var">{{ variation(p) }}</small>
          <span class="price">{{ money(p.priceCents) }}</span>
          <Barcode v-if="p.barcode" :value="p.barcode" show-text class="code" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.labels-area { overflow: auto; }
.label-page { display: grid; background: #fff; box-shadow: var(--shadow); margin-bottom: 12px; box-sizing: border-box; }
.label { overflow: hidden; display: flex; flex-direction: column; padding: 1.5mm 2mm; color: #000; background: #fff; outline: 1px dashed #bbb; gap: 0.5mm; font-family: Arial, sans-serif; }
.name { font-size: 3.4mm; line-height: 1.15; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; text-transform: uppercase; }
.var, .store { font-size: 2.6mm; }
.price { font-size: 6mm; font-weight: 800; }
.code { min-height: 6mm; }
.gondola .price { font-size: 8mm; }
.tag .label { text-align: center; align-items: center; }
.tag .code { width: 100%; }
.a4 .name { font-size: 3mm; -webkit-line-clamp: 1; }
.a4 .price { font-size: 4.6mm; }
.only-print { display: none; }
@media print {
  .label { outline: none; }
  .label-page { box-shadow: none; margin: 0; break-after: page; }
  .only-print { display: grid; }
}
</style>
