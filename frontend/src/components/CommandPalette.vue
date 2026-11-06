<!-- Caminho: catraca/frontend/src/components/CommandPalette.vue -->
<script setup lang="ts">
// Busca universal (Ctrl+K): acha telas, clientes, produtos, venda pelo número e
// o que o módulo acrescentar (alunos, na academia). Enter abre o item; setas
// escolhem; Esc fecha. scope 'alunos' mostra só os alunos (Receber mensalidade).
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ListCustomers, SearchProducts } from '../../wailsjs/go/main/App'
import { call } from '../api'
import { money } from '../composables/format'
import { moduleName, moduleSearch, type SearchHit } from '../composables/moduleExtras'
import Icon from './Icon.vue'
import SaleDetailsModal from './SaleDetailsModal.vue'

import type { PageLink } from '../composables/commandTypes'
const props = defineProps<{ pages: PageLink[]; scope?: string }>()
const emit = defineEmits<{ close: [] }>()
const router = useRouter()

const term = ref('')
const input = ref<HTMLInputElement | null>(null)
const hits = ref<SearchHit[]>([])
const loading = ref(false)
const active = ref(0)
const saleId = ref<number | null>(null)
const onlyModule = computed(() => props.scope === 'alunos')

const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()

function pageHits(t: string): SearchHit[] {
  const f = fold(t)
  const list = props.pages.filter((p) => !f || fold(`${p.group} ${p.label}`).includes(f))
  return list.slice(0, t ? 5 : 8).map((p) => ({ group: 'Telas', icon: p.icon, label: p.label, sub: p.group, to: p.to }))
}

let seq = 0
async function search() {
  const t = term.value.trim()
  const my = ++seq
  active.value = 0
  if (onlyModule.value) {
    loading.value = true
    const mod = await moduleSearch(t).catch(() => [])
    if (my === seq) { hits.value = mod; loading.value = false }
    return
  }
  if (!t) {
    hits.value = pageHits('')
    return
  }
  const out: SearchHit[] = []
  const num = t.match(/^#?(\d{1,9})$/)
  if (num) out.push({ group: 'Venda', icon: 't-historico', label: `Abrir a venda nº ${num[1]}`, sub: 'detalhes, recibo, devolução', run: () => { saleId.value = Number(num[1]) } })
  out.push(...pageHits(t))
  hits.value = out
  if (t.length < 2) return
  loading.value = true
  const [customers, products, mod] = await Promise.all([
    call(ListCustomers(t)).catch(() => []),
    call(SearchProducts(t, false, 0)).catch(() => []),
    moduleSearch(t).catch(() => []),
  ])
  if (my !== seq) return
  hits.value = [
    ...out,
    ...mod,
    ...customers.slice(0, 5).map((c) => ({ group: 'Clientes', icon: 'm-clientes', label: c.name, sub: [c.phone, c.creditBalanceCents ? `deve ${money(c.creditBalanceCents)}` : ''].filter(Boolean).join(' · '),
      to: `/clientes?busca=${encodeURIComponent(c.name)}&id=${c.id}` })),
    ...products.slice(0, 5).map((p) => ({ group: 'Produtos', icon: 'm-produtos', label: p.name, sub: `${money(p.priceCents)}${p.noStock ? '' : ` · estoque ${p.stockQty}`}`,
      to: `/produtos?busca=${encodeURIComponent(p.name)}` })),
  ]
  loading.value = false
}

let timer: number | undefined
watch(term, () => {
  window.clearTimeout(timer)
  timer = window.setTimeout(search, 180)
})
onMounted(async () => {
  search()
  await nextTick()
  input.value?.focus()
})

async function choose(h: SearchHit | undefined) {
  if (!h) return
  if (h.run) {
    await h.run()
    if (saleId.value) return // a venda abre aqui mesmo
  } else if (h.to) {
    router.push(h.to)
  }
  emit('close')
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') { e.preventDefault(); emit('close') }
  else if (e.key === 'ArrowDown') { e.preventDefault(); active.value = Math.min(active.value + 1, hits.value.length - 1) }
  else if (e.key === 'ArrowUp') { e.preventDefault(); active.value = Math.max(active.value - 1, 0) }
  else if (e.key === 'Enter') { e.preventDefault(); choose(hits.value[active.value]) }
}

// Itens agrupados, mantendo o índice geral para as setas.
const grouped = computed(() => {
  const out: { group: string; items: { h: SearchHit; i: number }[] }[] = []
  hits.value.forEach((h, i) => {
    const g = out.find((x) => x.group === h.group)
    if (g) g.items.push({ h, i })
    else out.push({ group: h.group, items: [{ h, i }] })
  })
  return out
})
const placeholder = computed(() => (onlyModule.value ? 'Nome do aluno para receber a mensalidade…' : `Buscar telas, clientes, produtos${moduleName ? ', alunos' : ''} ou nº da venda…`))
</script>

<template>
  <Teleport to="body">
    <div class="cp-backdrop no-print" @mousedown.self="emit('close')">
      <div class="cp" role="dialog" aria-label="Busca" @keydown="onKey">
        <div class="cp-input">
          <Icon name="busca" :size="18" />
          <input ref="input" v-model="term" :placeholder="placeholder" aria-label="Buscar" autocomplete="off" />
          <kbd>Esc</kbd>
        </div>
        <div class="cp-list">
          <div v-if="onlyModule" class="cp-hint">Escolha o aluno: a ficha abre com o botão de receber.</div>
          <div v-for="g in grouped" :key="g.group" class="cp-group">
            <div class="cp-gtitle">{{ g.group }}</div>
            <button v-for="{ h, i } in g.items" :key="i" class="cp-item" :class="{ on: i === active }" @mouseenter="active = i" @click="choose(h)">
              <Icon :name="h.icon" :size="17" />
              <span class="cp-label">{{ h.label }}</span>
              <span v-if="h.sub" class="cp-sub">{{ h.sub }}</span>
              <kbd v-if="i === active" class="cp-enter">Enter</kbd>
            </button>
          </div>
          <div v-if="loading" class="cp-hint">Procurando…</div>
          <div v-else-if="!hits.length" class="cp-hint">Nada encontrado para "{{ term }}".</div>
        </div>
        <div class="cp-foot"><span><kbd>↑</kbd><kbd>↓</kbd> escolher</span><span><kbd>Enter</kbd> abrir</span><span><kbd>Ctrl</kbd>+<kbd>K</kbd> de qualquer tela</span></div>
      </div>
    </div>
  </Teleport>
  <SaleDetailsModal v-if="saleId" :sale-id="saleId" @close="saleId = null; emit('close')" />
</template>

<style scoped>
.cp-backdrop { position: fixed; inset: 0; z-index: 1000; background: rgba(17, 24, 39, 0.35); display: flex; justify-content: center; align-items: flex-start; padding-top: 10vh; }
.cp { width: min(640px, 92vw); max-height: 72vh; display: flex; flex-direction: column; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: 0 24px 60px -12px rgba(17, 24, 39, 0.45); overflow: hidden; }
.cp-input { display: flex; align-items: center; gap: 10px; padding: 12px 14px; border-bottom: 1px solid var(--border); color: var(--muted); }
.cp-input input { flex: 1; border: 0; outline: 0; background: transparent; font-size: 16px; min-height: 0; padding: 4px 0; color: var(--text); box-shadow: none; }
.cp-list { overflow: auto; padding: 6px; }
.cp-gtitle { font-size: 10.5px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); padding: 8px 10px 4px; }
.cp-item { display: flex; align-items: center; gap: 10px; width: 100%; padding: 9px 10px; border: 0; border-radius: 9px; background: transparent; text-align: left; color: var(--text); min-height: 0; box-shadow: none; font-weight: 500; }
.cp-item.on { background: var(--primary-soft); color: var(--primary); }
.cp-label { font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.cp-sub { color: var(--muted); font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.cp-enter { margin-left: auto; }
.cp-hint { padding: 12px; color: var(--muted); font-size: 13px; }
.cp-foot { display: flex; gap: 16px; padding: 8px 14px; border-top: 1px solid var(--border); background: var(--surface-2); color: var(--muted); font-size: 11.5px; }
.cp-foot kbd { margin: 0 2px; }
</style>
