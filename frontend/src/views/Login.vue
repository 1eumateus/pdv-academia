<!-- Caminho: pdv/frontend/src/views/Login.vue -->
<script setup lang="ts">
// Login no estilo "quem está usando?": os usuários ativos aparecem em quadrados;
// clicar em um pede só a senha. "Outro usuário" abre o formulário com login e senha.
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { ListLoginUsers } from '../../wailsjs/go/main/App'
import { call, errorMessage, isCode } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import { backgrounds } from '../composables/background'
import { settings, storeName } from '../composables/cashState'
import { login, roleLabels, session, setupAdmin } from '../composables/session'
import type { models } from '../types'

const bgStyle = computed(() => (backgrounds.value.login ? { backgroundImage: `url("${backgrounds.value.login}")` } : undefined))
const form = reactive({ name: '', login: '', password: '', confirm: '', busy: false, error: '' })
// Usuário já logado em outro lugar: mensagem do aviso; confirmar entra forçando.
const alreadyIn = ref('')

// users: quem aparece nos quadrados; picked: o escolhido (pede só a senha);
// manual: "Outro usuário", com o campo de login aberto.
const users = ref<models.User[]>([])
const picked = ref<models.User | null>(null)
const manual = ref(false)
const passwordInput = ref<HTMLInputElement | null>(null)
onMounted(async () => {
  try {
    users.value = await call(ListLoginUsers())
  } catch {
    // sem a lista, fica o formulário com login e senha
  }
  if (!users.value.length) manual.value = true
})

const colors = ['#5b4cf0', '#0e9466', '#d98b06', '#d63c31', '#0891b2', '#db2777', '#7c3aed', '#65a30d']
const avatarStyle = (u: models.User) => ({ background: colors[u.id % colors.length] })
const initials = (name: string) => name.trim().split(/\s+/).slice(0, 2).map((w) => w.charAt(0)).join('').toUpperCase()

function pick(u: models.User) {
  picked.value = u
  manual.value = false
  Object.assign(form, { login: u.login, password: '', error: '' })
  nextTick(() => passwordInput.value?.focus())
}
function typeLogin() {
  picked.value = null
  manual.value = true
  Object.assign(form, { login: '', password: '', error: '' })
}
function back() {
  picked.value = null
  manual.value = !users.value.length
  Object.assign(form, { login: '', password: '', error: '' })
}

async function submit(force = false) {
  form.error = ''
  alreadyIn.value = ''
  if (session.value?.needsSetup && form.password !== form.confirm) return (form.error = 'As senhas não conferem.')
  form.busy = true
  try {
    if (session.value?.needsSetup) await setupAdmin(form.name, form.login, form.password)
    else await login(form.login, form.password, force)
  } catch (e) {
    if (isCode(e, 'USUARIO_JA_LOGADO')) {
      alreadyIn.value = errorMessage(e)
      return
    }
    form.error = errorMessage(e)
    form.password = ''
    form.confirm = ''
    nextTick(() => passwordInput.value?.focus())
  } finally {
    form.busy = false
  }
}
</script>

<template>
  <div class="login" :style="bgStyle">
    <div class="veil" />
    <header class="brand">
      <img v-if="settings?.logo" :src="settings.logo" alt="" class="brand-logo" />
      <span v-else class="brand-mark">{{ (storeName || 'PDV').trim().charAt(0).toUpperCase() }}</span>
      <span>{{ storeName || 'PDV' }}</span>
    </header>

    <!-- Primeiro acesso: cria o administrador. -->
    <section v-if="session?.needsSetup" class="panel">
      <h1>Primeiro acesso</h1>
      <p class="sub">Crie o usuário administrador. Ele cadastra os demais usuários e tem acesso a tudo. Guarde bem a senha.</p>
      <form class="stack" @submit.prevent="submit()">
        <label class="field">Seu nome<input v-model="form.name" maxlength="200" required /></label>
        <label class="field">Usuário<input v-model="form.login" maxlength="50" autocomplete="username" autofocus required /></label>
        <label class="field">Senha<input v-model="form.password" type="password" autocomplete="new-password" required /></label>
        <label class="field">Confirme a senha<input v-model="form.confirm" type="password" required /></label>
        <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
        <button class="primary big" type="submit" :disabled="form.busy">{{ form.busy ? 'Aguarde…' : 'Criar administrador e entrar' }}</button>
      </form>
    </section>

    <!-- Usuário escolhido: só a senha. -->
    <section v-else-if="picked" class="chosen">
      <span class="avatar big" :style="avatarStyle(picked)">{{ initials(picked.name) }}</span>
      <h1>{{ picked.name }}</h1>
      <p class="sub">{{ roleLabels[picked.role] ?? picked.role }}</p>
      <form class="pw-form" @submit.prevent="submit()">
        <input ref="passwordInput" v-model="form.password" type="password" autocomplete="current-password" placeholder="Senha" aria-label="Senha" required />
        <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
        <button class="primary big" type="submit" :disabled="form.busy">{{ form.busy ? 'Entrando…' : 'Entrar' }}</button>
      </form>
      <button class="link back" @click="back">Trocar de usuário</button>
    </section>

    <!-- Outro usuário (ou lista indisponível): login e senha. -->
    <section v-else-if="manual" class="panel">
      <h1>Entrar</h1>
      <p class="sub">Digite o seu usuário e a senha.</p>
      <form class="stack" @submit.prevent="submit()">
        <label class="field">Usuário<input v-model="form.login" maxlength="50" autocomplete="username" autofocus required /></label>
        <label class="field">Senha<input v-model="form.password" type="password" autocomplete="current-password" required /></label>
        <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
        <button class="primary big" type="submit" :disabled="form.busy">{{ form.busy ? 'Entrando…' : 'Entrar' }}</button>
      </form>
      <button v-if="users.length" class="link back" @click="back">Voltar para os usuários</button>
    </section>

    <!-- Quem vai usar: um quadrado por usuário. -->
    <section v-else class="who">
      <h1>Quem vai usar o caixa?</h1>
      <ul class="profiles">
        <li v-for="u in users" :key="u.id">
          <button type="button" class="profile" @click="pick(u)">
            <span class="avatar" :style="avatarStyle(u)">{{ initials(u.name) }}</span>
            <span class="p-name">{{ u.name }}</span>
            <span class="p-role">{{ roleLabels[u.role] ?? u.role }}</span>
          </button>
        </li>
        <li>
          <button type="button" class="profile" @click="typeLogin">
            <span class="avatar other" aria-hidden="true">+</span>
            <span class="p-name">Outro usuário</span>
            <span class="p-role">digitar o login</span>
          </button>
        </li>
      </ul>
    </section>

    <ConfirmDialog v-if="alreadyIn" title="Usuário já logado" :busy="form.busy"
      :message="`${alreadyIn} Entrar mesmo assim? A outra sessão será encerrada.`" confirm-label="Entrar mesmo assim"
      @confirm="submit(true)" @cancel="alreadyIn = ''; form.password = ''" />
  </div>
</template>

<style scoped>
/* Tela cheia escura; a imagem de fundo da loja (Configurações › Aparência) fica sob um véu. */
.login { position: relative; min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 96px 24px 48px; color: #fff;
  background: radial-gradient(circle at 30% 20%, #2d2581 0, #14122b 55%, #0c0b1d 100%); background-size: cover; background-position: center; }
.veil { position: absolute; inset: 0; background: rgba(12, 11, 29, 0.78); }
.login > :not(.veil) { position: relative; }
.login > .brand { position: absolute; top: 28px; left: 36px; display: flex; align-items: center; gap: 12px; font-size: 19px; font-weight: 700; }
.brand-mark { width: 38px; height: 38px; border-radius: 11px; display: grid; place-items: center; font-weight: 800; background: linear-gradient(135deg, #7c6ff7, #22d3a6); }
.brand-logo { max-height: 40px; max-width: 160px; border-radius: 6px; background: #fff; padding: 2px; }
h1 { margin: 0; font-size: 40px; font-weight: 600; letter-spacing: -0.02em; text-align: center; }
.sub { margin: 6px 0 0; color: #b9b5e6; text-align: center; }

/* Quadrados dos usuários. */
.who { display: flex; flex-direction: column; align-items: center; gap: 36px; max-width: 1100px; }
.profiles { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; justify-content: center; gap: 28px; }
.profile { display: flex; flex-direction: column; align-items: center; gap: 10px; width: 150px; padding: 0; min-height: 0; border: 0; background: none;
  color: #b9b5e6; box-shadow: none; font-weight: 400; }
.profile:hover:not(:disabled), .profile:focus-visible { background: none; color: #fff; }
.avatar { width: 140px; height: 140px; border-radius: 22px; display: grid; place-items: center; font-size: 46px; font-weight: 700; color: #fff;
  border: 4px solid transparent; transition: border-color 0.15s, transform 0.15s; }
.profile:hover .avatar, .profile:focus-visible .avatar { border-color: #fff; transform: scale(1.04); }
.profile:focus-visible { outline: none; }
.avatar.other { background: rgba(255, 255, 255, 0.08); color: #b9b5e6; font-size: 64px; font-weight: 300; }
.p-name { font-size: 17px; text-align: center; overflow-wrap: anywhere; }
.p-role { font-size: 12.5px; color: #8d89b9; margin-top: -6px; }
@media (prefers-reduced-motion: reduce) { .avatar { transition: none; } .profile:hover .avatar { transform: none; } }

/* Usuário escolhido: avatar, nome e a senha. */
.chosen { display: flex; flex-direction: column; align-items: center; gap: 10px; width: 100%; max-width: 380px; }
.avatar.big { margin-bottom: 8px; cursor: default; }
.chosen h1 { font-size: 30px; }
.pw-form { display: flex; flex-direction: column; gap: 12px; width: 100%; margin-top: 18px; }
.pw-form input { font-size: 18px; padding: 12px 14px; text-align: center; }
.back { color: #c9c6ee; margin-top: 8px; }

/* Formulário (primeiro acesso / outro usuário). */
.panel { width: 100%; max-width: 420px; padding: 32px; border-radius: 22px; background: var(--surface); color: var(--text); box-shadow: 0 30px 80px rgba(0, 0, 0, 0.4); }
.panel h1 { font-size: 26px; text-align: left; }
.panel .sub { color: var(--muted); text-align: left; margin-bottom: 18px; }
.panel .back { color: var(--primary); margin-top: 12px; padding-left: 0; }
@media (max-width: 700px) { .avatar { width: 104px; height: 104px; font-size: 34px; } .profile { width: 110px; } h1 { font-size: 30px; } }
</style>
