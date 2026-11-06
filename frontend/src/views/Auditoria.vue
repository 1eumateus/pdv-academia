<!-- Caminho: pdv/frontend/src/views/Auditoria.vue -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ListAudit, ListProducts } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { formatDateTime, money, today } from '../composables/format'
import { can } from '../composables/session'
import type { AuditEntry, Product } from '../types'
import LoadMore from '../components/LoadMore.vue'
import { usePaged } from '../composables/paged'

const tab = ref<'operacoes' | 'produtos'>('operacoes')
// Produtos cadastrados: todos, pela data de cadastro, mais recentes primeiro.
const products = ref<Product[] | null>(null)
const productsError = ref('')
const registered = computed(() => [...(products.value ?? [])].sort((a, b) => String(b.createdAt).localeCompare(String(a.createdAt)) || b.id - a.id))
async function loadProducts() {
  productsError.value = ''
  try {
    products.value = await call(ListProducts())
  } catch (e) {
    productsError.value = errorMessage(e)
  }
}

const start = ref(today(-7))
const end = ref(today())
const entries = ref<AuditEntry[]>([])
const loading = ref(false)
const error = ref('')
const term = ref('')
// Busca local por usuário, quem autorizou, ação ou detalhe.
const filtered = computed(() => {
  const t = term.value.trim().toLowerCase()
  if (!t) return entries.value
  return entries.value.filter((e) =>
    [e.userName, e.authorizedBy, actionLabels[e.action] ?? e.action, details(e), String(e.entityId ?? '')].some((x) => (x ?? '').toLowerCase().includes(t)),
  )
})

const actionLabels: Record<string, string> = {
  produto_excluido: 'Produto excluído', codigo_interno_gerado: 'Código interno gerado', importacao_produtos: 'Importação de produtos (planilha)',
  importacao_clientes: 'Importação de clientes (planilha)', orcamento_reaberto: 'Orçamento reaberto',
  primeiro_acesso: 'Primeiro acesso', login: 'Entrou', login_falhou: 'Login recusado', logout: 'Saiu',
  autorizacao: 'Autorização de gerente', usuario_criado: 'Usuário cadastrado', usuario_alterado: 'Usuário alterado',
  senha_alterada: 'Senha alterada', venda_cancelada: 'Venda cancelada', forma_pagamento_alterada: 'Forma de pagamento trocada', fatura_isenta: 'Mensalidade isenta', isencao_encerrada: 'Isenção encerrada', devolucao: 'Devolução de venda', desconto_acima_limite: 'Desconto acima do limite',
  caixa_aberto: 'Caixa aberto', caixa_fechado: 'Caixa fechado', sangria: 'Sangria', suprimento: 'Suprimento',
  ajuste_estoque: 'Ajuste de estoque', reconciliacao_estoque: 'Reconciliação de estoque', estorno_fiado: 'Estorno de recebimento a prazo', limite_fiado: 'Limite a prazo', produto_cadastrado: 'Produto cadastrado com autorização', grade_criada: 'Grade criada', grade_ampliada: 'Grade ampliada', preco_grade_alterado: 'Preço da grade alterado', custo_alterado: 'Custo alterado', produto_ativo_alterado: 'Produto ativado/desativado', fornecedor_cadastrado: 'Fornecedor cadastrado', fornecedor_alterado: 'Fornecedor alterado', compra_registrada: 'Entrada de mercadoria', compra_estornada: 'Entrada estornada', promocao_salva: 'Promoção salva', orcamento_convertido: 'Orçamento convertido em venda',
  preco_alterado: 'Preço alterado', configuracoes_alteradas: 'Configurações alteradas', restauracao_agendada: 'Restauração de backup',
}

// details mostra só o que mudou entre antes e depois.
function details(e: AuditEntry): string {
  const parse = (s?: string | null) => {
    try {
      return s ? (JSON.parse(s) as Record<string, unknown>) : {}
    } catch {
      return {}
    }
  }
  const before = parse(e.beforeJson)
  const after = parse(e.afterJson)
  const keys = [...new Set([...Object.keys(before), ...Object.keys(after)])]
  return keys
    .filter((k) => JSON.stringify(before[k]) !== JSON.stringify(after[k]) && (before[k] !== '' || after[k] !== ''))
    .map((k) => (k in before && k in after ? `${k}: ${String(before[k])} → ${String(after[k])}` : `${k}: ${String(after[k] ?? before[k])}`))
    .join(' · ')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    entries.value = await call(ListAudit(start.value, end.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgAuditoria = usePaged(() => filtered.value, [term, start, end, tab])
// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgCadastros = usePaged(() => registered.value, [tab])
</script>

<template>
  <div class="tabs" role="tablist">
    <button role="tab" :aria-selected="tab === 'operacoes'" @click="tab = 'operacoes'">Operações</button>
    <button role="tab" :aria-selected="tab === 'produtos'" @click="tab = 'produtos'; products === null && loadProducts()">Produtos cadastrados</button>
  </div>

  <section v-if="tab === 'operacoes'" class="card stack">
    <form class="row" @submit.prevent="load">
      <label class="field">De<input v-model="start" type="date" required /></label>
      <label class="field">Até<input v-model="end" type="date" required /></label>
      <button class="primary" style="align-self: end" type="submit" :disabled="loading">{{ loading ? 'Buscando…' : 'Buscar' }}</button>
    </form>
    <input v-model="term" type="search" placeholder="Filtrar por usuário, ação ou detalhe" aria-label="Filtrar auditoria" style="max-width: 420px" />
    <p class="small muted" style="margin: 0">Registro permanente de quem fez cada operação sensível. Não pode ser alterado nem apagado.</p>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="!loading && filtered.length === 0" class="state">{{ term ? 'Nada encontrado.' : 'Nada registrado no período.' }}</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Data</th><th>Usuário</th><th>Ação</th><th>Autorizado por</th><th>Detalhes</th></tr></thead>
        <tbody>
          <tr v-for="e in pgAuditoria.visible" :key="e.id">
            <td class="nowrap">{{ formatDateTime(e.createdAt) }}</td>
            <td>{{ e.userName ?? '—' }}</td>
            <td>{{ actionLabels[e.action] ?? e.action }}<span v-if="e.entityId" class="small muted"> nº {{ e.entityId }}</span></td>
            <td>{{ e.authorizedBy ?? '' }}</td>
            <td class="small">{{ details(e) }}</td>
          </tr>
        </tbody>
      </table>
          <LoadMore :paged="pgAuditoria" />
    </div>
  </section>

  <section v-else class="card stack">
    <p class="small muted" style="margin: 0">Todos os produtos pela data de cadastro, mais recentes primeiro.</p>
    <div v-if="productsError" class="alert error">{{ productsError }}</div>
    <div v-else-if="products === null" class="state">Carregando…</div>
    <div v-else-if="registered.length === 0" class="state">Nenhum produto cadastrado.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th class="left">Cadastrado em</th><th class="left">Produto</th><th class="left">SKU / código</th><th v-if="can('ver_custo')" class="right">Custo</th><th class="right">Preço</th><th class="right">Estoque</th></tr></thead>
        <tbody>
          <tr v-for="p in pgCadastros.visible" :key="p.id" :class="{ inactive: !p.active }">
            <td class="nowrap left">{{ formatDateTime(p.createdAt) }}</td>
            <td class="left">{{ p.name }} <span v-if="!p.active" class="badge">Inativo</span></td>
            <td class="small mono left">{{ [p.sku, p.barcode].filter(Boolean).join(' · ') || '—' }}</td>
            <td v-if="can('ver_custo')" class="right mono">{{ p.costCents ? money(p.costCents) : '—' }}</td>
            <td class="right mono">{{ money(p.priceCents) }}</td>
            <td class="right mono">{{ p.stockQty }}</td>
          </tr>
        </tbody>
      </table>
          <LoadMore :paged="pgCadastros" />
    </div>
  </section>
</template>
