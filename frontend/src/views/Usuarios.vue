<!-- Caminho: pdv/frontend/src/views/Usuarios.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { CreateUser, GetPermissionCatalog, ListUsers, ReleaseUserSession, UpdateUser } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import Modal from '../components/Modal.vue'
import { formatDateTime } from '../composables/format'
import { currentUser, refreshSession, roleLabels } from '../composables/session'
import { useToast } from '../composables/toast'
import { models, type User } from '../types'

type Catalog = models.PermissionCatalog

const toast = useToast()
const users = ref<User[]>([])
const error = ref('')
const loading = ref(true)
const term = ref('')
const filtered = computed(() => {
  const t = term.value.trim().toLowerCase()
  return users.value.filter((u) => !t || u.name.toLowerCase().includes(t) || u.login.includes(t))
})
const catalog = ref<Catalog | null>(null)
const form = reactive({
  open: false, id: 0, name: '', login: '', password: '', role: 'operador', active: true,
  permissions: [] as string[], busy: false, error: '',
})

// Grupos na ordem do catálogo: a caixa do grupo (principal) marca ou desmarca
// todas as secundárias; cada secundária também vale sozinha.
const groups = computed(() => {
  const out: { name: string; items: models.PermissionItem[] }[] = []
  for (const p of catalog.value?.items ?? []) {
    const last = out[out.length - 1]
    if (last?.name === p.group) last.items.push(p)
    else out.push({ name: p.group, items: [p] })
  }
  return out
})
const marked = (items: models.PermissionItem[]) => items.filter((p) => form.permissions.includes(p.key)).length
function toggleGroup(items: models.PermissionItem[], on: boolean) {
  const keys = items.map((p) => p.key)
  form.permissions = on ? [...new Set([...form.permissions, ...keys])] : form.permissions.filter((k) => !keys.includes(k))
}
const capital = (t: string) => t.charAt(0).toUpperCase() + t.slice(1)

function defaultsOf(role: string): string[] {
  return [...(catalog.value?.defaults[role] ?? [])]
}

// Trocar o papel no formulário volta as caixinhas para o padrão do novo papel.
function onRoleChange() {
  form.permissions = defaultsOf(form.role)
}

// Login preso (o programa fechou sem sair): libera para o usuário entrar de novo.
async function release(u: User) {
  try {
    await call(ReleaseUserSession(u.id))
    toast.success(`Login de ${u.name} liberado.`)
    await load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    users.value = await call(ListUsers())
    catalog.value ??= await call(GetPermissionCatalog())
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

function edit(u: User | null) {
  Object.assign(form, u
    ? { open: true, id: u.id, name: u.name, login: u.login, password: '', role: u.role, active: u.active, permissions: [...u.permissions], busy: false, error: '' }
    : { open: true, id: 0, name: '', login: '', password: '', role: 'operador', active: true, permissions: defaultsOf('operador'), busy: false, error: '' })
}

async function save() {
  form.busy = true
  form.error = ''
  try {
    const input = models.UserInput.createFrom({
      id: form.id, name: form.name, login: form.login, password: form.password, role: form.role, active: form.active,
      permissions: form.role === 'admin' ? null : form.permissions,
    })
    await call(form.id ? UpdateUser(input) : CreateUser(input))
    toast.success(form.id ? 'Usuário alterado.' : 'Usuário cadastrado.')
    form.open = false
    await refreshSession() // o próprio cadastro pode ter mudado
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
    <div class="row between">
      <p class="muted" style="margin: 0">
        Cada usuário tem as próprias permissões (em <strong>Editar</strong>). Usuário novo já vem com o padrão do papel:
        o <strong>gerente</strong> pode quase tudo; o <strong>operador</strong> vende, recebe, cadastra produto e dá entrada no
        estoque. As permissões <strong>de risco</strong> (isentar, pagamento fora do caixa, apagar aluno…) nunca vêm no
        padrão: só valem se o administrador marcar. O <strong>administrador</strong> faz tudo, inclusive usuários.
      </p>
      <button class="primary" @click="edit(null)">Novo usuário</button>
    </div>
    <input v-model="term" type="search" placeholder="Filtrar por nome ou login" aria-label="Filtrar usuários" style="max-width: 320px" />
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="loading" class="state">Carregando…</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Nome</th><th>Login</th><th>Papel</th><th>Permissões</th><th>Situação</th><th>Atualizado</th><th></th></tr></thead>
        <tbody>
          <tr v-for="u in filtered" :key="u.id" :class="{ inactive: !u.active }">
            <td class="left">{{ u.name }}</td>
            <td class="mono">{{ u.login }}</td>
            <td>{{ roleLabels[u.role] }}</td>
            <td class="small muted">{{ u.role === 'admin' ? 'Todas' : `${u.permissions.length} de ${catalog?.items.length ?? '—'}` }}</td>
            <td>
              <span class="badge" :class="u.active ? 'ok' : 'bad'">{{ u.active ? 'Ativo' : 'Inativo' }}</span>
              <span v-if="u.loggedIn" class="badge" style="margin-left: 4px">Logado</span>
            </td>
            <td class="nowrap small muted">{{ formatDateTime(u.updatedAt) }}</td>
            <td class="right actions-cell">
              <button v-if="u.loggedIn && u.id !== currentUser?.id" class="link icon-btn" title="Liberar o login preso (o sistema fechou sem sair); se a pessoa ainda estiver usando, ela sai" aria-label="Liberar login" @click="release(u)"><Icon name="liberar" /><span>Liberar</span></button>
              <button class="link icon-btn" @click="edit(u)" title="Editar" aria-label="Editar"><Icon name="edit" /><span>Editar</span></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <Modal v-if="form.open" :title="form.id ? 'Editar usuário' : 'Novo usuário'" :busy="form.busy" @close="form.open = false">
    <form class="stack" @submit.prevent="save">
      <label class="field">Nome<input v-model="form.name" maxlength="200" required /></label>
      <label class="field">Login (sem espaços)<input v-model="form.login" maxlength="50" autocomplete="off" required /></label>
      <label class="field">
        {{ form.id ? 'Nova senha (deixe em branco para manter a atual)' : 'Senha (mínimo 4 caracteres)' }}
        <input v-model="form.password" type="password" autocomplete="new-password" :required="!form.id" />
      </label>
      <label class="field">Papel
        <select v-model="form.role" @change="onRoleChange">
          <option value="operador">Operador</option>
          <option value="gerente">Gerente</option>
          <option value="admin">Administrador</option>
        </select>
      </label>
      <label class="check"><input v-model="form.active" type="checkbox" /> Ativo (pode entrar no sistema)</label>
      <fieldset v-if="catalog" class="stack" style="border: 1px solid var(--border); border-radius: 8px; padding: 12px; gap: 6px">
        <legend class="small" style="font-weight: 600; padding: 0 4px">O que este usuário pode ver e fazer</legend>
        <p v-if="form.role === 'admin'" class="small muted" style="margin: 0">O administrador pode tudo, inclusive usuários e auditoria.</p>
        <template v-else>
          <div v-for="g in groups" :key="g.name" class="perm-group">
            <label class="check perm-main">
              <input type="checkbox" :checked="marked(g.items) === g.items.length" :indeterminate="marked(g.items) > 0 && marked(g.items) < g.items.length"
                @change="toggleGroup(g.items, ($event.target as HTMLInputElement).checked)" />
              <strong>{{ g.name }}</strong>
              <span class="small muted">{{ marked(g.items) }} de {{ g.items.length }}</span>
            </label>
            <label v-for="p in g.items" :key="p.key" class="check perm-sub">
              <input v-model="form.permissions" type="checkbox" :value="p.key" /> {{ capital(p.label) }}
              <span v-if="p.risky" class="badge bad perm-risk" title="Permissão de risco: não vem no padrão de nenhum papel">risco</span>
            </label>
          </div>
          <div class="row" style="margin-top: 4px">
            <button type="button" class="link" @click="form.permissions = defaultsOf(form.role)">Restaurar o padrão de {{ roleLabels[form.role] }}</button>
          </div>
          <p class="small muted" style="margin: 0">
            O que estiver desmarcado não aparece para este usuário (telas, botões e campos). Para fazer, entre com um
            usuário que tenha a permissão (Trocar usuário) ou peça a autorização de quem tem. Cadastrar usuários e dar
            permissões é sempre só do administrador.
          </p>
        </template>
      </fieldset>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">Salvar</button>
      </footer>
    </form>
  </Modal>
</template>

<style scoped>
.perm-group { display: flex; flex-direction: column; gap: 4px; padding: 8px 0; border-top: 1px solid var(--border); }
.perm-group:first-of-type { border-top: 0; padding-top: 0; }
.perm-main { gap: 8px; }
.perm-sub { margin-left: 24px; }
.perm-risk { margin-left: 6px; font-size: 11px; }
</style>
