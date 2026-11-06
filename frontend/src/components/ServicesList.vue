<!-- Caminho: pdv/frontend/src/components/ServicesList.vue -->
<script setup lang="ts">
// Serviços e planos (Produtos › Serviços): itens sem estoque. Serviço avulso
// (diária, avaliação) ou plano de assinatura com duração; descrição livre.
// São produtos que não controlam estoque: vendem na tela de Vendas normalmente.
import { computed, onMounted, reactive, ref } from 'vue'
import { CreateProduct, ListPlanRules, ListProducts, ListSubscriptions, SetPlanRules, UpdateProduct } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { copyRules, describeRules } from '../composables/accessRules'
import { centsToInput, money, parseMoney } from '../composables/format'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { models, type Product } from '../types'
import AccessRulesFields from './AccessRulesFields.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import Icon from './Icon.vue'
import Modal from './Modal.vue'

const toast = useToast()
const items = ref<Product[]>([])
const active = ref<Record<number, number>>({}) // assinantes ativos por plano
const planRules = ref<Record<number, models.AccessRules>>({}) // regras de acesso na catraca
const loading = ref(true)
const error = ref('')
const term = ref('')

const durations = [
  { days: 30, label: 'Mensal' }, { days: 60, label: 'Bimestral' }, { days: 90, label: 'Trimestral' },
  { days: 180, label: 'Semestral' }, { days: 365, label: 'Anual' },
]
const typeLabel = (p: Product) => (p.planDays ? `Plano ${(durations.find((d) => d.days === p.planDays)?.label ?? `${p.planDays} dias`).toLowerCase()}` : 'Serviço')
const fold = (t: string) => t.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
const filtered = computed(() => {
  const words = fold(term.value).split(/\s+/).filter(Boolean)
  return items.value.filter((p) => words.every((w) => fold(`${p.name} ${p.description ?? ''}`).includes(w)))
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = (await call(ListProducts())).filter((p) => p.noStock)
    const counts: Record<number, number> = {}
    for (const s of await call(ListSubscriptions())) if (s.status === 'ativa') counts[s.productId] = (counts[s.productId] ?? 0) + 1
    active.value = counts
    planRules.value = await call(ListPlanRules())
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---------- cadastro / edição ----------
const form = reactive({ open: false, product: null as Product | null, plan: false, name: '', price: '', days: 30, description: '', active: true, rules: copyRules(), busy: false, error: '' })
function openForm(p?: Product) {
  Object.assign(form, {
    open: true, product: p ?? null, plan: !!p?.planDays, name: p?.name ?? '', price: p ? centsToInput(p.priceCents) : '',
    days: p?.planDays ?? 30, description: p?.description ?? '', active: p?.active ?? true, rules: copyRules(p ? planRules.value[p.id] : null), busy: false, error: '',
  })
}
async function save() {
  const price = parseMoney(form.price)
  if (!form.name.trim()) return (form.error = 'Informe o nome.')
  if (price === null) return (form.error = 'Preço inválido. Exemplo: 90,00')
  form.busy = true
  form.error = ''
  try {
    // Na edição, mantém o que não aparece aqui (código, categoria, custo).
    const base = form.product ? { ...form.product, costCents: null } : { id: 0, initialStock: 0, minStock: 0 }
    const input = models.ProductInput.createFrom({
      ...base, name: form.name, priceCents: price, active: form.active, noStock: true,
      planDays: form.plan ? form.days : null, description: form.description,
    })
    const saved = await call(form.product ? UpdateProduct(input) : CreateProduct(input))
    // Regras da catraca: valem para todos os assinantes do plano.
    if (form.plan) await call(SetPlanRules(saved.id, form.rules))
    toast.success(form.product ? `"${saved.name}" atualizado.` : `"${saved.name}" cadastrado.`)
    form.open = false
    load()
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}

const deactivateAsk = ref<Product | null>(null)
async function toggleActive(p: Product, confirmed = false) {
  if (p.active && !confirmed) return (deactivateAsk.value = p)
  try {
    await call(UpdateProduct(models.ProductInput.createFrom({ ...p, active: !p.active, costCents: null })))
    toast.success(p.active ? `"${p.name}" desativado.` : `"${p.name}" ativado.`)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div class="row">
    <input v-model="term" type="search" placeholder="Buscar serviço ou plano" aria-label="Buscar serviços" style="max-width: 360px" />
    <span class="spacer" />
    <button v-if="can('cadastrar_produto')" class="primary" @click="openForm()">+ Novo serviço ou plano</button>
  </div>
  <div v-if="loading" class="state">Carregando…</div>
  <div v-else-if="error" class="alert error">{{ error }} <button class="link" @click="load">Tentar novamente</button></div>
  <div v-else-if="!filtered.length" class="state">{{ items.length ? 'Nada corresponde à busca.' : 'Nenhum serviço ou plano. Clique em "Novo serviço ou plano".' }}</div>
  <div v-else class="table-wrap">
    <table>
      <thead><tr><th>Nome</th><th>Tipo</th><th class="right">Preço</th><th class="right">Assinantes ativos</th><th class="right">Ações</th></tr></thead>
      <tbody>
        <tr v-for="p in filtered" :key="p.id" :class="{ inactive: !p.active }">
          <td class="left">
            {{ p.name }} <span v-if="!p.active" class="badge">Inativo</span>
            <div v-if="p.description" class="small muted">{{ p.description }}</div>
          </td>
          <td><span class="badge" :class="{ ok: p.planDays }">{{ typeLabel(p) }}</span><div v-if="p.planDays && describeRules(planRules[p.id])" class="small muted">{{ describeRules(planRules[p.id]) }}</div></td>
          <td class="right mono">{{ money(p.priceCents) }}</td>
          <td class="right mono">{{ p.planDays ? active[p.id] ?? 0 : '—' }}</td>
          <td class="right actions-cell">
            <button class="link icon-btn" title="Editar" aria-label="Editar" @click="openForm(p)"><Icon name="edit" /><span>Editar</span></button>
            <button v-if="can('ativar_produto')" class="link icon-btn" :title="p.active ? 'Desativar' : 'Ativar'" :aria-label="p.active ? 'Desativar' : 'Ativar'"
              :style="{ color: p.active ? 'var(--danger)' : 'var(--success)' }" @click="toggleActive(p)"><Icon :name="p.active ? 'desativar' : 'ativar'" /><span>{{ p.active ? 'Desativar' : 'Ativar' }}</span></button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <Modal v-if="form.open" :title="form.product ? 'Editar serviço ou plano' : 'Novo serviço ou plano'" :busy="form.busy" @close="form.open = false">
    <form class="stack" @submit.prevent="save">
      <div class="tabs seg" role="radiogroup" aria-label="Tipo">
        <button type="button" role="radio" :aria-checked="!form.plan" :aria-selected="!form.plan" @click="form.plan = false">Serviço avulso</button>
        <button type="button" role="radio" :aria-checked="form.plan" :aria-selected="form.plan" @click="form.plan = true">Plano (assinatura)</button>
      </div>
      <label class="field">Nome<input v-model="form.name" maxlength="200" required :placeholder="form.plan ? 'Ex.: Mensal 5x por semana' : 'Ex.: Diária, Avaliação física'" /></label>
      <div class="grid grid-2">
        <label v-if="!form.product || can('alterar_preco')" class="field">Preço (R$)<input v-model="form.price" inputmode="decimal" placeholder="0,00" required /></label>
        <label v-if="form.plan" class="field">Duração (dias)
          <input v-model.number="form.days" type="number" min="1" max="3650" step="1" list="plan-durations" required />
          <datalist id="plan-durations">
            <option v-for="d in durations" :key="d.days" :value="d.days">{{ d.label }}</option>
          </datalist>
          <span class="small muted">Em dias. Ex.: 3, 5, 30 (mensal), 90 (trimestral).</span>
        </label>
      </div>
      <AccessRulesFields v-if="form.plan" v-model="form.rules" />
      <label class="field">Descrição (opcional)<textarea v-model="form.description" maxlength="500" placeholder="Ex.: 3x por semana, musculação, horário livre" /></label>
      <label v-if="can('ativar_produto')" class="check"><input v-model="form.active" type="checkbox" /> Ativo (disponível para venda e assinatura)</label>
      <p v-if="form.plan" class="small muted" style="margin: 0">Os alunos assinam pela ficha, em Academia › Alunos e rostos. Mudar o preço vale para as próximas faturas.</p>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">{{ form.busy ? 'Salvando…' : 'Salvar' }}</button>
      </footer>
    </form>
  </Modal>
  <ConfirmDialog v-if="deactivateAsk" title="Desativar" danger confirm-label="Desativar"
    :message="`Desativar “${deactivateAsk.name}”? Ele deixa de aparecer na venda e em novas assinaturas.`"
    @confirm="toggleActive(deactivateAsk, true); deactivateAsk = null" @cancel="deactivateAsk = null" />
</template>
