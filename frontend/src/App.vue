<!-- Caminho: catraca/frontend/src/App.vue -->
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChangeOwnPassword, GetStartupStatus } from '../wailsjs/go/main/App'
import { call, errorMessage } from './api'
import Modal from './components/Modal.vue'
import ToastHost from './components/ToastHost.vue'
import AccessToast from './components/AccessToast.vue'
import Icon from './components/Icon.vue'
import SupportCard from './components/SupportCard.vue'
import { cashOpen, licenseStatus, refreshCash, refreshLicense, refreshSettings, settings, storeName } from './composables/cashState'
import { backgrounds, bgStyle, refreshBackgrounds } from './composables/background'
import { can, currentUser, logout, refreshSession, roleLabels, session } from './composables/session'
import { useToast } from './composables/toast'
import Login from './views/Login.vue'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const startupError = ref('')
const startupNotice = ref('')
const sessionLoaded = ref(false)

// Menu em seções: o menu lateral mostra só a seção; as telas dela viram abas
// no topo (estilo abas do navegador).
const groups = computed(() => [
  { label: 'Painel', icon: 'm-painel', color: '#60a5fa', key: '', tabs: [{ to: '/inicio', label: 'Painel', icon: 'm-painel', show: true }] },
  { label: 'Vendas', icon: 'm-vendas', color: '#34d399', key: 'F10', tabs: [
    { to: '/vendas', label: 'Venda', icon: 't-venda', show: true },
    { to: '/orcamentos', label: 'Orçamentos salvos', icon: 't-orcamento', show: true },
    { to: '/comandas', label: 'Comandas', icon: 't-comanda', show: true },
    { to: '/devolucoes', label: 'Devoluções', icon: 't-devolucao', show: true },
    { to: '/historico', label: 'Histórico de vendas', icon: 't-historico', show: true },
  ] },
  { label: 'Produtos', icon: 'm-produtos', color: '#fbbf24', key: '', tabs: [
    { to: '/produtos', label: 'Produtos', icon: 'm-produtos', show: true },
    { to: '/estoque', label: 'Estoque', icon: 't-estoque', show: true },
    { to: '/promocoes', label: 'Promoções', icon: 't-promocao', show: can('alterar_preco') },
    { to: '/compras', label: 'Entrada de mercadoria', icon: 't-entrada', show: can('compras') },
    { to: '/fornecedores', label: 'Fornecedores', icon: 't-fornecedor', show: can('compras') },
    { to: '/etiquetas', label: 'Etiquetas', icon: 't-etiqueta', show: true },
  ] },
  { label: 'Academia', icon: 'm-academia', color: '#fb923c', key: '', tabs: [
    { to: '/catraca', label: 'Catraca', icon: 't-catraca', show: true },
    { to: '/catraca/entradas', label: 'Entradas', icon: 't-historico', show: true },
    { to: '/catraca/alunos', label: 'Alunos e rostos', icon: 't-rosto', show: true },
    { to: '/catraca/funcionarios', label: 'Funcionários', icon: 't-usuarios', show: can('catraca_funcionarios') },
    { to: '/catraca/config', label: 'Configurar catraca', icon: 'm-admin', show: can('catraca_configurar') },
  ] },
  { label: 'Clientes', icon: 'm-clientes', color: '#f472b6', key: '', tabs: [
    { to: '/clientes', label: 'Clientes', icon: 'm-clientes', show: true },
  ] },
  { label: 'Financeiro', icon: 't-financeiro', color: '#2dd4bf', key: '', tabs: [
    { to: '/fiado', label: 'A prazo', icon: 't-aprazo', show: true },
    { to: '/financeiro/contas', label: 'Contas a pagar', icon: 't-orcamento', show: can('contas_a_pagar') },
    { to: '/financeiro/lucro', label: 'Lucro', icon: 't-rel-vendas', show: can('ver_custo') },
    { to: '/financeiro/descontos', label: 'Descontos', icon: 't-promocao', show: true },
  ] },
  { label: 'Caixa', icon: 'm-caixa', color: '#a3e635', key: '', tabs: [{ to: '/caixa', label: 'Caixa', icon: 'm-caixa', show: true }] },
  { label: 'Relatórios', icon: 'm-relatorios', color: '#a78bfa', key: '', tabs: [
    { to: '/relatorios', label: 'Vendas', icon: 't-rel-vendas', show: true },
    { to: '/relatorios/produtos', label: 'Produtos', icon: 't-rel-produtos', show: true },
  ] },
  { label: 'Administração', icon: 'm-admin', color: '#94a3b8', key: '', tabs: [
    { to: '/configuracoes', label: 'Configurações', icon: 'm-admin', show: can('alterar_configuracoes') || can('restaurar_backup') },
    { to: '/usuarios', label: 'Usuários', icon: 't-usuarios', show: can('gerenciar_usuarios') },
    { to: '/auditoria', label: 'Auditoria', icon: 't-auditoria', show: can('ver_auditoria') },
    { to: '/apagar-testes', label: 'Apagar vendas de teste', icon: 'remover', show: can('apagar_testes') },
    { to: '/licenca', label: 'Licença', icon: 't-licenca', show: true },
  ] },
].map((g) => ({ ...g, tabs: g.tabs.filter((t) => t.show) })).filter((g) => g.tabs.length))

// Aba ativa: mesmo caminho e mesmo "modo" (a aba Orçamento usa ?modo=orcamento).
function isActive(to: string) {
  const [path, query = ''] = to.split('?')
  return path === route.path && (new URLSearchParams(query).get('modo') ?? '') === ((route.query.modo as string) ?? '')
}
const currentGroup = computed(() => groups.value.find((g) => g.tabs.some((t) => t.to.split('?')[0] === route.path)))

// Menu do usuário (canto direito do topo): fecha ao clicar em qualquer lugar.
const userMenu = ref(false)
window.addEventListener('click', () => (userMenu.value = false))
window.addEventListener('keydown', (e) => e.key === 'Escape' && (userMenu.value = false))

// Tema claro/escuro: preferência desta máquina (não é dado do negócio).
const dark = ref(document.documentElement.dataset.theme === 'dark')
function toggleTheme() {
  dark.value = !dark.value
  document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'
  try {
    localStorage.setItem('pdv-theme', dark.value ? 'dark' : 'light')
  } catch {
    // sem armazenamento: vale só até fechar
  }
}

// F1 abre a ajuda de atalhos; F10 vai para a venda (de qualquer tela).
const helpOpen = ref(false)
function onKey(e: KeyboardEvent) {
  if (!currentUser.value) return
  if (e.key === 'F1') {
    e.preventDefault()
    helpOpen.value = !helpOpen.value
  } else if (e.key === 'F10') {
    e.preventDefault()
    router.push('/vendas')
  }
}
const shortcuts = [
  ['F1', 'Abrir/fechar esta ajuda'], ['F10', 'Ir para a venda'], ['Esc', 'Fechar janela / voltar ao código de barras'],
  ['F2', 'Buscar produto (código de barras, nome)'], ['5*código', 'Vende 5 unidades do produto lido'],
  ['F3', 'Escolher cliente'], ['F6', 'Salvar como orçamento'], ['F7', 'Desconto na venda'],
  ['F8 ou F4', 'Finalizar (na tela de pagamento: confirmar)'], ['Enter', 'Confirmar o pagamento'],
  ['F9', 'Tirar do carrinho o item selecionado (o último, se nenhum)'], ['+ / −', 'Aumentar/diminuir a quantidade do item selecionado'],
]

// ---------- bloqueio por inatividade ----------
let idleTimer: number | undefined
function resetIdle() {
  window.clearTimeout(idleTimer)
  const minutes = settings.value?.lockMinutes ?? 0
  if (!currentUser.value || minutes <= 0) return
  idleTimer = window.setTimeout(() => {
    toast.info('Tela bloqueada por inatividade.')
    logout()
  }, minutes * 60_000)
}
const activityEvents = ['mousemove', 'mousedown', 'keydown', 'wheel', 'touchstart'] as const

onMounted(async () => {
  window.addEventListener('keydown', onKey)
  activityEvents.forEach((ev) => window.addEventListener(ev, resetIdle, { passive: true }))
  try {
    const st = await GetStartupStatus()
    startupError.value = st.error ?? ''
    startupNotice.value = st.notice ?? ''
  } catch {
    startupError.value = 'Não foi possível verificar a inicialização do sistema.'
  }
  refreshSettings()
  refreshBackgrounds()
  try {
    await refreshSession()
  } catch {
    // banco indisponível: startupError já explica
  } finally {
    sessionLoaded.value = true
  }
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  activityEvents.forEach((ev) => window.removeEventListener(ev, resetIdle))
  window.clearTimeout(idleTimer)
})
watch(() => route.path, () => {
  if (!currentUser.value) return
  refreshCash()
  refreshLicense()
})
// Ao entrar (ou trocar de operador), recarrega o que depende do usuário.
watch(() => currentUser.value?.id, (id) => {
  resetIdle()
  if (id) {
    // Banco novo: o administrador cai no assistente de primeira configuração.
    refreshSettings().then(() => {
      if (settings.value && !settings.value.setupDone && can('alterar_configuracoes')) router.replace('/assistente')
    })
    refreshCash()
    refreshLicense()
    // Outro usuário entrou: se ele não pode ver a tela atual, volta para Vendas.
    const perms = route.meta.perms as string[] | undefined
    if (perms && !perms.some(can)) router.replace('/vendas')
  }
})
watch(() => settings.value?.lockMinutes, resetIdle)

async function switchUser() {
  await logout()
}

// ---------- minha senha ----------
const pw = reactive({ open: false, current: '', next: '', confirm: '', busy: false, error: '' })
async function savePassword() {
  if (pw.next !== pw.confirm) return (pw.error = 'As senhas novas não conferem.')
  pw.busy = true
  pw.error = ''
  try {
    await call(ChangeOwnPassword(pw.current, pw.next))
    Object.assign(pw, { open: false, current: '', next: '', confirm: '' })
    toast.success('Senha alterada.')
  } catch (e) {
    pw.error = errorMessage(e)
  } finally {
    pw.busy = false
  }
}
</script>

<template>
  <div v-if="!sessionLoaded" class="state">Carregando…</div>
  <template v-else-if="!currentUser && !startupError && session">
    <Login />
    <ToastHost />
  </template>
  <div v-else class="layout">
    <!-- Menu principal no topo; as telas de cada grupo ficam nas abas logo abaixo. -->
    <header class="appbar no-print">
      <div class="brand">PDV<small>{{ storeName || 'Ponto de Venda' }}</small></div>
      <nav aria-label="Menu principal">
        <RouterLink v-for="g in groups" :key="g.label" :to="g.tabs[0].to" :class="{ 'router-link-active': g === currentGroup }" :style="{ '--ic': g.color }">
          <Icon :name="g.icon" :size="30" />{{ g.label }}<span v-if="g.key" class="kbd">{{ g.key }}</span>
        </RouterLink>
      </nav>
      <span class="spacer" />
      <RouterLink to="/caixa" class="badge" :class="cashOpen ? 'ok' : cashOpen === false ? 'bad' : ''">
        {{ cashOpen ? 'Caixa aberto' : cashOpen === false ? 'Caixa fechado' : 'Caixa: verificando…' }}
      </RouterLink>
      <div v-if="currentUser" class="user-menu">
        <button class="user-btn" :aria-expanded="userMenu" aria-haspopup="menu" @click.stop="userMenu = !userMenu">
          {{ currentUser.name }} <small>{{ roleLabels[currentUser.role] }}</small> ▾
        </button>
        <div v-if="userMenu" class="user-pop" role="menu" @click="userMenu = false">
          <button role="menuitem" @click="switchUser">Trocar usuário</button>
          <button role="menuitem" @click="pw.open = true">Minha senha</button>
          <button role="menuitem" @click="toggleTheme">{{ dark ? 'Tema claro' : 'Tema escuro' }}</button>
        </div>
      </div>
    </header>
    <SupportCard v-if="currentUser" />
    <div class="main">
      <main class="content" :class="{ fill: route.meta.fill }" :style="bgStyle(backgrounds.app)">
        <div v-if="startupError" class="alert error" role="alert" style="margin-bottom: 16px">
          <strong>Falha ao iniciar.</strong> {{ startupError }}
        </div>
        <div v-if="startupNotice" class="alert info" role="status" style="margin-bottom: 16px">
          {{ startupNotice }}
          <button class="link" @click="startupNotice = ''">Fechar</button>
        </div>
        <div v-if="licenseStatus?.warning" class="alert no-print" :class="licenseStatus.canWrite ? 'warn' : 'error'" role="status" style="margin-bottom: 16px">
          {{ licenseStatus.warning }} <RouterLink v-if="route.path !== '/licenca'" to="/licenca">Ver licença</RouterLink>
        </div>
        <nav v-if="currentGroup && currentGroup.tabs.length > 1" class="page-tabs no-print" :aria-label="`Telas de ${currentGroup.label}`" :style="{ '--ic': currentGroup.color }">
          <RouterLink v-for="t in currentGroup.tabs" :key="t.to" :to="t.to" :class="{ active: isActive(t.to) }" :aria-current="isActive(t.to) ? 'page' : undefined">
            <Icon v-if="t.icon" :name="t.icon" :size="24" />{{ t.label }}
          </RouterLink>
        </nav>
        <RouterView v-if="currentUser" />
      </main>
    </div>
    <ToastHost />
    <!-- Aviso com a foto a cada passagem pela catraca, em qualquer tela. -->
    <AccessToast v-if="currentUser" />

    <Modal v-if="helpOpen" title="Atalhos de teclado" @close="helpOpen = false">
      <table>
        <tbody>
          <tr v-for="[k, d] in shortcuts" :key="k"><td class="nowrap"><kbd>{{ k }}</kbd></td><td class="left">{{ d }}</td></tr>
        </tbody>
      </table>
      <p class="small muted" style="margin-top: 8px">Na venda, o cursor volta sozinho para o campo do código de barras.</p>
    </Modal>
    <Modal v-if="pw.open" title="Alterar minha senha" :busy="pw.busy" @close="pw.open = false">
      <form class="stack" @submit.prevent="savePassword">
        <label class="field">Senha atual<input v-model="pw.current" type="password" autocomplete="current-password" required /></label>
        <label class="field">Nova senha<input v-model="pw.next" type="password" autocomplete="new-password" required /></label>
        <label class="field">Confirme a nova senha<input v-model="pw.confirm" type="password" autocomplete="new-password" required /></label>
        <div v-if="pw.error" class="alert error" role="alert">{{ pw.error }}</div>
        <footer class="row end">
          <button type="button" :disabled="pw.busy" @click="pw.open = false">Cancelar</button>
          <button type="submit" class="primary" :disabled="pw.busy">Salvar</button>
        </footer>
      </form>
    </Modal>
  </div>
</template>
