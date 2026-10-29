// Caminho: pdv/frontend/src/composables/session.ts

import { computed, ref } from 'vue'
import { GetSession, Login, Logout, SetupAdmin } from '../../wailsjs/go/main/App'
import { call, setLoginRequiredHandler } from '../api'
import { models, type Session } from '../types'

// Sessão só para exibição: o backend confere o usuário e as permissões em
// cada operação.
export const session = ref<Session | null>(null)
export const currentUser = computed(() => session.value?.user ?? null)

export const roleLabels: Record<string, string> = { admin: 'Administrador', gerente: 'Gerente', operador: 'Operador' }

export function can(permission: string): boolean {
  return session.value?.permissions.includes(permission) ?? false
}

export async function refreshSession() {
  session.value = await call(GetSession())
}

// force: entra mesmo com o usuário logado em outro lugar (encerra a outra sessão).
export async function login(user: string, password: string, force = false) {
  session.value = await call(Login(user, password, force))
}

export async function setupAdmin(name: string, user: string, password: string) {
  session.value = await call(SetupAdmin(models.UserInput.createFrom({ name, login: user, password })))
}

export async function logout() {
  try {
    await call(Logout())
  } finally {
    await refreshSession().catch(() => (session.value = null))
  }
}

setLoginRequiredHandler(() => {
  if (session.value?.user) session.value = models.Session.createFrom({ ...session.value, user: null, permissions: [] })
})
