<!-- Caminho: pdv/frontend/src/views/Promocoes.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { ListCategories, ListPromotions, SavePromotion } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import Modal from '../components/Modal.vue'
import ProductSearch from '../components/ProductSearch.vue'
import { centsToInput, money, parseMoney, today } from '../composables/format'
import { useToast } from '../composables/toast'
import { models, type Category, type Product } from '../types'

const toast = useToast()
const promos = ref<models.Promotion[]>([])
const categories = ref<Category[]>([])
const term = ref('')
const statusFilter = ref<'todas' | 'vigente' | 'futura' | 'encerrada' | 'inativa'>('todas')
const error = ref('')

const statusLabels: Record<string, string> = { vigente: 'Vigente', futura: 'Futura', encerrada: 'Encerrada', inativa: 'Inativa' }
const statusClass: Record<string, string> = { vigente: 'ok', futura: 'warn', encerrada: '', inativa: 'bad' }
const kindLabels: Record<string, string> = { preco: 'Preço promocional', percentual: '% de desconto', leve_pague: 'Leve X pague Y' }

const filtered = computed(() => {
  const t = term.value.trim().toLowerCase()
  return promos.value.filter((p) => (statusFilter.value === 'todas' || p.status === statusFilter.value) &&
    (!t || p.name.toLowerCase().includes(t) || (p.productName ?? '').toLowerCase().includes(t) || (p.categoryName ?? '').toLowerCase().includes(t)))
})

function rule(p: models.Promotion): string {
  if (p.kind === 'preco') return `por ${money(p.priceCents)}`
  if (p.kind === 'percentual') return `${p.percent}% de desconto`
  return `leve ${p.buyQty}, pague ${p.payQty}`
}
const br = (d: string) => d.split('-').reverse().join('/')

async function load() {
  try {
    promos.value = await call(ListPromotions())
    categories.value = (await call(ListCategories())).filter((c) => c.active)
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)

const form = reactive({
  open: false, id: 0, name: '', kind: 'preco', target: 'produto' as 'produto' | 'categoria', productId: null as number | null,
  productName: '', categoryId: null as number | null, price: '', percent: '10', buy: '3', pay: '2', start: today(), end: today(7),
  active: true, busy: false, error: '',
})

function edit(p?: models.Promotion) {
  Object.assign(form, p ? {
    open: true, id: p.id, name: p.name, kind: p.kind, target: p.productId ? 'produto' : 'categoria', productId: p.productId ?? null,
    productName: p.productName ?? '', categoryId: p.categoryId ?? null, price: p.priceCents ? centsToInput(p.priceCents) : '',
    percent: String(p.percent || 10), buy: String(p.buyQty || 3), pay: String(p.payQty || 2), start: p.startDate, end: p.endDate,
    active: p.active, busy: false, error: '',
  } : {
    open: true, id: 0, name: '', kind: 'preco', target: 'produto', productId: null, productName: '', categoryId: null, price: '',
    percent: '10', buy: '3', pay: '2', start: today(), end: today(7), active: true, busy: false, error: '',
  })
}

function pickProduct(p: Product) {
  form.productId = p.id
  form.productName = p.name
}

async function save() {
  const price = form.kind === 'preco' ? parseMoney(form.price) : 0
  if (price === null) return (form.error = 'Preço promocional inválido.')
  form.busy = true
  form.error = ''
  try {
    const saved = await call(SavePromotion(models.Promotion.createFrom({
      id: form.id, name: form.name, kind: form.kind, active: form.active, startDate: form.start, endDate: form.end,
      productId: form.target === 'produto' ? form.productId : null, categoryId: form.target === 'categoria' ? form.categoryId : null,
      priceCents: price, percent: Number(form.percent) || 0, buyQty: Number(form.buy) || 0, payQty: Number(form.pay) || 0,
    })))
    toast.success(`Promoção "${saved.name}" salva (${statusLabels[saved.status].toLowerCase()}).`)
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
  <section class="card stack">
    <div class="row">
      <input v-model="term" type="search" placeholder="Buscar por nome, produto ou categoria" aria-label="Buscar promoções" style="max-width: 360px" />
      <select v-model="statusFilter" aria-label="Situação" style="max-width: 180px">
        <option value="todas">Todas</option>
        <option value="vigente">Vigentes</option>
        <option value="futura">Futuras</option>
        <option value="encerrada">Encerradas</option>
        <option value="inativa">Inativas</option>
      </select>
      <span class="spacer" />
      <button class="primary" @click="edit()">Nova promoção</button>
    </div>
    <p class="small muted" style="margin: 0">
      Aplicadas sozinhas na venda, no período escolhido. Se um produto tiver mais de uma promoção, vale a que dá mais desconto
      (não se somam). Desconto de promoção não conta no limite de desconto do operador.
    </p>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="filtered.length === 0" class="state">Nenhuma promoção.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Promoção</th><th>Vale para</th><th>Regra</th><th>Período</th><th>Situação</th><th></th></tr></thead>
        <tbody>
          <tr v-for="p in filtered" :key="p.id" :class="{ inactive: p.status === 'encerrada' || p.status === 'inativa' }">
            <td class="left">{{ p.name }}<div class="small muted">{{ kindLabels[p.kind] }}</div></td>
            <td>{{ p.productName ?? `Categoria: ${p.categoryName}` }}</td>
            <td>{{ rule(p) }}</td>
            <td class="nowrap small">{{ br(p.startDate) }} a {{ br(p.endDate) }}</td>
            <td><span class="badge" :class="statusClass[p.status]">{{ statusLabels[p.status] }}</span></td>
            <td class="right actions-cell"><button class="link icon-btn" @click="edit(p)" title="Editar" aria-label="Editar"><Icon name="edit" /><span>Editar</span></button></td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <Modal v-if="form.open" :title="form.id ? 'Editar promoção' : 'Nova promoção'" :busy="form.busy" @close="form.open = false">
    <form class="stack" @submit.prevent="save">
      <label class="field">Nome<input v-model="form.name" maxlength="200" placeholder="Ex.: Semana das bebidas" required /></label>
      <label class="field">Tipo
        <select v-model="form.kind">
          <option value="preco">Preço promocional (R$)</option>
          <option value="percentual">% de desconto</option>
          <option value="leve_pague">Leve X pague Y</option>
        </select>
      </label>
      <label v-if="form.kind === 'preco'" class="field">Preço promocional (R$)<input v-model="form.price" inputmode="decimal" placeholder="0,00" /></label>
      <label v-else-if="form.kind === 'percentual'" class="field">Desconto (%)<input v-model="form.percent" inputmode="numeric" /></label>
      <div v-else class="row" style="gap: 8px; flex-wrap: nowrap">
        <label class="field" style="flex: 1">Leve<input v-model="form.buy" inputmode="numeric" /></label>
        <label class="field" style="flex: 1">Pague<input v-model="form.pay" inputmode="numeric" /></label>
      </div>
      <div class="row" style="gap: 14px">
        <label class="check"><input v-model="form.target" type="radio" value="produto" /> Um produto</label>
        <label class="check"><input v-model="form.target" type="radio" value="categoria" /> Uma categoria inteira</label>
      </div>
      <template v-if="form.target === 'produto'">
        <div v-if="form.productId" class="row between small"><strong>{{ form.productName }}</strong><button type="button" class="link" @click="form.productId = null">trocar</button></div>
        <ProductSearch v-else :only-active="false" no-categories placeholder="Buscar o produto" @pick="pickProduct" />
      </template>
      <label v-else class="field">Categoria
        <select v-model="form.categoryId">
          <option :value="null">Escolha</option>
          <option v-for="c in categories" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </label>
      <div class="grid grid-2">
        <label class="field">Início<input v-model="form.start" type="date" required /></label>
        <label class="field">Fim<input v-model="form.end" type="date" required /></label>
      </div>
      <label class="check"><input v-model="form.active" type="checkbox" /> Ativa</label>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">Salvar</button>
      </footer>
    </form>
  </Modal>
</template>
