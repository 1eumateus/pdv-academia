<!-- Caminho: pdv/frontend/src/views/Login.vue -->
<script setup lang="ts">
import { reactive, ref } from 'vue'
import { errorMessage, isCode } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import { backgrounds, bgStyle } from '../composables/background'
import { settings, storeName } from '../composables/cashState'
import { login, session, setupAdmin } from '../composables/session'

const form = reactive({ name: '', login: '', password: '', confirm: '', busy: false, error: '' })
// Usuário já logado em outro lugar: mensagem do aviso; confirmar entra forçando.
const alreadyIn = ref('')

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
  } finally {
    form.busy = false
  }
}
</script>

<template>
  <div class="login-page" :style="bgStyle(backgrounds.login)">
    <section class="card stack login-card">
      <div class="center">
        <img v-if="settings?.logo" :src="settings.logo" alt="Logo" style="max-height: 72px; max-width: 200px" />
        <h2 style="margin: 8px 0 0">{{ storeName || 'PDV' }}</h2>
      </div>
      <template v-if="session?.needsSetup">
        <h3 style="margin: 0">Primeiro acesso</h3>
        <p class="small muted" style="margin: 0">
          Crie o usuário administrador. Ele cadastra os demais usuários e tem acesso a tudo. Guarde bem a senha.
        </p>
      </template>
      <form class="stack" @submit.prevent="submit()">
        <label v-if="session?.needsSetup" class="field">Seu nome<input v-model="form.name" maxlength="200" required /></label>
        <label class="field">Usuário<input v-model="form.login" maxlength="50" autocomplete="username" autofocus required /></label>
        <label class="field">Senha<input v-model="form.password" type="password" autocomplete="current-password" required /></label>
        <label v-if="session?.needsSetup" class="field">Confirme a senha<input v-model="form.confirm" type="password" required /></label>
        <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
        <button class="primary big" type="submit" :disabled="form.busy">
          {{ form.busy ? 'Aguarde…' : session?.needsSetup ? 'Criar administrador e entrar' : 'Entrar' }}
        </button>
      </form>
    </section>
    <ConfirmDialog v-if="alreadyIn" title="Usuário já logado" :busy="form.busy"
      :message="`${alreadyIn} Entrar mesmo assim? A outra sessão será encerrada.`" confirm-label="Entrar mesmo assim"
      @confirm="submit(true)" @cancel="alreadyIn = ''; form.password = ''" />
  </div>
</template>
