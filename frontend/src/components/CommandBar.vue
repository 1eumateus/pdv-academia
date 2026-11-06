<!-- Caminho: catraca/frontend/src/components/CommandBar.vue -->
<script setup lang="ts">
// Barra de comando, logo abaixo do menu: a busca universal (Ctrl+K), as ações
// mais repetidas e o sino com os alertas do dia. Igual no PDV e no PDV da
// academia; o que é do módulo vem de moduleExtras.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { alerts, refreshAlerts, startAlerts, stopAlerts } from '../composables/alerts'
import { moduleActions } from '../composables/moduleExtras'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import CommandPalette from './CommandPalette.vue'
import type { PageLink } from '../composables/commandTypes'
import Icon from './Icon.vue'

defineProps<{ pages: PageLink[] }>()
const router = useRouter()
const toast = useToast()

const palette = ref<{ scope?: string } | null>(null)
function openSearch(scope?: string) {
  palette.value = { scope }
}

// Ações rápidas: as do módulo primeiro (na academia, Receber mensalidade é a principal).
const core = [
  { key: 'venda', label: 'Nova venda', icon: 'm-vendas', kbd: 'F10', primary: false, show: () => true, run: () => router.push('/vendas') },
  { key: 'receber', label: 'A receber', icon: 't-aprazo', primary: false, show: () => true, run: () => router.push('/fiado') },
]
const actions = computed(() => {
  const mod = moduleActions.filter((a) => a.show()).map((a) => ({ ...a, kbd: '' }))
  const list = [...mod, ...core.filter((a) => a.show())]
  if (!mod.some((a) => a.primary)) list[0] = { ...list[0], primary: true }
  return list
})
async function runAction(a: (typeof actions.value)[number]) {
  await a.run({ openSearch, toast: (m: string, k?: 'success' | 'error') => (k === 'error' ? toast.error(m) : toast.success(m)) } as never)
}

// Sino: lista dos alertas; vermelho quando algum pede ação já.
const bellOpen = ref(false)
const badLevel = computed(() => (alerts.value.some((a) => a.level === 'bad') ? 'bad' : alerts.value.length ? 'warn' : ''))
function go(to: string) {
  bellOpen.value = false
  router.push(to)
}
function closeBell() {
  bellOpen.value = false
}

function onKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K')) {
    e.preventDefault()
    openSearch()
  }
}
onMounted(() => {
  startAlerts()
  window.addEventListener('keydown', onKey)
  document.addEventListener('click', closeBell)
})
onBeforeUnmount(() => {
  stopAlerts()
  window.removeEventListener('keydown', onKey)
  document.removeEventListener('click', closeBell)
})
defineExpose({ openSearch, refreshAlerts, canSeeBills: () => can('contas_a_pagar') })
</script>

<template>
  <div class="cmdbar no-print">
    <button type="button" class="cmd-search" title="Buscar (Ctrl+K)" @click="openSearch()">
      <Icon name="busca" :size="16" /><span>Buscar telas, clientes, produtos ou venda…</span><kbd>Ctrl K</kbd>
    </button>
    <button v-for="a in actions" :key="a.key" type="button" class="cmd-btn" :class="{ primary: a.primary }" @click="runAction(a)">
      <Icon :name="a.icon" :size="16" />{{ a.label }}<kbd v-if="a.kbd">{{ a.kbd }}</kbd>
    </button>
    <div class="cmd-bell-wrap">
      <button type="button" class="cmd-bell" :class="badLevel" :title="alerts.length ? `${alerts.length} alerta(s)` : 'Sem alertas'" :aria-expanded="bellOpen" @click.stop="bellOpen = !bellOpen">
        <Icon name="sino" :size="17" /><b v-if="alerts.length">{{ alerts.length }}</b>
      </button>
      <div v-if="bellOpen" class="cmd-pop" @click.stop>
        <div class="cmd-pop-head"><strong>Alertas do dia</strong><button type="button" class="link small" @click="refreshAlerts()">Atualizar</button></div>
        <div v-if="!alerts.length" class="cmd-empty">Nada pendente. 👍</div>
        <button v-for="a in alerts" :key="a.id" type="button" class="cmd-alert" @click="go(a.to)">
          <span class="dot" :class="a.level" /><span class="txt">{{ a.text }}</span><span v-if="a.action" class="act">{{ a.action }} →</span>
        </button>
      </div>
    </div>
  </div>
  <CommandPalette v-if="palette" :pages="pages" :scope="palette.scope" @close="palette = null" />
</template>

<style scoped>
.cmdbar { display: flex; align-items: center; gap: 8px; padding: 8px 20px; background: var(--surface-2); border-bottom: 1px solid var(--border); }
.cmd-search { flex: 1; display: flex; align-items: center; gap: 9px; min-height: 38px; padding: 0 12px; border: 1px solid var(--field-border); border-radius: 10px; background: var(--field-bg); color: var(--muted); font-weight: 400; box-shadow: none; text-align: left; }
.cmd-search span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cmd-search:hover:not(:disabled) { border-color: var(--primary); background: var(--field-bg); }
.cmd-btn { display: inline-flex; align-items: center; gap: 7px; min-height: 38px; padding: 0 13px; border-radius: 10px; font-weight: 600; white-space: nowrap; }
.cmd-btn.primary { background: var(--primary); border-color: var(--primary); color: #fff; box-shadow: var(--shadow-primary, none); }
.cmd-btn.primary:hover:not(:disabled) { background: var(--primary-dark); }
.cmd-btn kbd { font-size: 10px; }
.cmd-btn.primary kbd { background: rgba(255, 255, 255, 0.16); border-color: rgba(255, 255, 255, 0.25); color: #fff; }
.cmd-bell-wrap { position: relative; }
.cmd-bell { position: relative; width: 40px; min-height: 38px; padding: 0; display: grid; place-items: center; border-radius: 10px; color: var(--muted); }
.cmd-bell b { position: absolute; top: -6px; right: -6px; min-width: 18px; height: 18px; padding: 0 4px; border-radius: 9px; background: var(--warning); color: #fff; font-size: 10px; display: grid; place-items: center; }
.cmd-bell.bad b { background: var(--danger); }
.cmd-pop { position: absolute; right: 0; top: calc(100% + 6px); z-index: 50; width: 380px; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: 0 16px 40px -12px rgba(17, 24, 39, 0.35); padding: 6px; }
.cmd-pop-head { display: flex; justify-content: space-between; align-items: center; padding: 6px 8px 8px; }
.cmd-empty { padding: 14px 8px; color: var(--muted); font-size: 13px; }
.cmd-alert { display: flex; align-items: center; gap: 9px; width: 100%; padding: 9px 8px; border: 0; border-radius: 8px; background: transparent; text-align: left; min-height: 0; box-shadow: none; font-weight: 500; color: var(--text); }
.cmd-alert:hover:not(:disabled) { background: var(--surface-2); }
.cmd-alert .txt { flex: 1; font-size: 13px; }
.cmd-alert .act { color: var(--primary); font-size: 12px; font-weight: 600; white-space: nowrap; }
.dot { width: 8px; height: 8px; border-radius: 50%; flex: none; background: var(--muted); }
.dot.bad { background: var(--danger); } .dot.warn { background: var(--warning); } .dot.info { background: var(--primary); }
</style>
