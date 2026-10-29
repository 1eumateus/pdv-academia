// Caminho: pdv/frontend/src/main.ts

import { createApp } from 'vue'
import { LogClientError } from '../wailsjs/go/main/App'
import { ApiError } from './api'
import App from './App.vue'
import { router } from './router'
import './assets/main.css'

// Tema da máquina (claro/escuro), aplicado antes de montar para não piscar.
try {
  if (localStorage.getItem('pdv-theme') === 'dark') document.documentElement.dataset.theme = 'dark'
} catch {
  // sem armazenamento: fica o tema claro
}

// Erros da tela vão para o mesmo log do sistema (logs/pdv.log). Erros de
// regra (ApiError: estoque, permissão…) já são tratados e não são bugs.
function report(err: unknown, where: string) {
  if (err instanceof ApiError || (typeof err === 'string' && err.startsWith('{"code"'))) return
  const e = err instanceof Error ? err : new Error(String(err))
  console.error(err)
  LogClientError(`${e.message} [${where} · ${router.currentRoute.value.fullPath}]`, e.stack ?? '').catch(() => {})
}
window.addEventListener('error', (ev) => report(ev.error ?? ev.message, 'janela'))
window.addEventListener('unhandledrejection', (ev) => report(ev.reason, 'promessa'))

const app = createApp(App)
app.config.errorHandler = (err, _vm, info) => report(err, `vue: ${info}`)
app.use(router).mount('#app')
