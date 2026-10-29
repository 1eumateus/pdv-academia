<!-- Caminho: catraca/frontend/src/components/AccessToast.vue -->
<script setup lang="ts">
// Aviso pequeno no canto a cada passagem pela catraca, com a foto que o
// iDFace guardou. Fica no alto à direita para não cobrir os avisos do sistema.
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { MemberPhoto, NotifyAccess } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { models } from '../../wailsjs/go/models'

const SHOW_MS = 6000
const MAX_TOASTS = 3

type Toast = { event: models.GateEvent; photo: string }
const toasts = ref<Toast[]>([])
const timers = new Map<number, number>()

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  return (parts[0][0] + (parts.length > 1 ? parts[parts.length - 1][0] : '')).toUpperCase()
}

const hour = (v: string) => new Date(v).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit', second: '2-digit' })

function dismiss(id: number) {
  window.clearTimeout(timers.get(id))
  timers.delete(id)
  toasts.value = toasts.value.filter((t) => t.event.id !== id)
}

function show(e: models.GateEvent) {
  // Liberação manual não tem aluno: nada para mostrar.
  if (e.source === 'manual') return
  // Janela minimizada ou atrás de outro programa: avisa também pelo Windows.
  if (!document.hasFocus()) NotifyAccess(e).catch(() => {})
  const toast = reactive<Toast>({ event: e, photo: '' })
  toasts.value = [toast, ...toasts.value]
  for (const old of toasts.value.slice(MAX_TOASTS)) dismiss(old.event.id)
  timers.set(e.id, window.setTimeout(() => dismiss(e.id), SHOW_MS))
  if (e.customerId) {
    MemberPhoto(e.customerId)
      .then((url) => (toast.photo = url))
      .catch(() => {}) // sem foto: fica com as iniciais
  }
}

let off: (() => void) | undefined
onMounted(() => {
  off = EventsOn('acesso', show)
})
onBeforeUnmount(() => {
  off?.()
  timers.forEach((t) => window.clearTimeout(t))
})
</script>

<template>
  <div class="access-toasts no-print" aria-live="polite">
    <button
      v-for="t in toasts"
      :key="t.event.id"
      class="access-toast"
      :class="t.event.allowed ? 'ok' : 'bad'"
      title="Fechar"
      @click="dismiss(t.event.id)"
    >
      <img v-if="t.photo" :src="t.photo" alt="" class="avatar" />
      <span v-else class="avatar initials">{{ initials(t.event.name) }}</span>
      <span class="info">
        <strong>{{ t.event.name || '—' }}</strong>
        <span class="small">
          <span class="badge" :class="t.event.allowed ? 'ok' : 'bad'">{{ t.event.allowed ? 'Liberado' : 'Negado' }}</span>
          <span class="muted mono"> {{ hour(t.event.at) }}</span>
        </span>
        <span class="small muted reason">{{ t.event.reason }}</span>
      </span>
    </button>
  </div>
</template>

<style scoped>
.access-toasts { position: fixed; right: 16px; top: 64px; z-index: 80; display: flex; flex-direction: column; gap: 8px; pointer-events: none; }
.access-toast {
  pointer-events: auto; display: flex; align-items: center; gap: 12px; width: 320px; padding: 10px 12px; text-align: left;
  justify-content: flex-start; min-height: 0; font: inherit; color: var(--text); cursor: pointer;
  background: var(--surface); border: 1px solid var(--border); border-left: 4px solid; border-radius: var(--radius);
  box-shadow: 0 6px 20px rgba(16, 24, 40, 0.18); animation: slide-in 0.2s ease-out;
}
.access-toast:hover:not(:disabled) { background: var(--surface); }
.access-toast.ok { border-left-color: var(--success); }
.access-toast.bad { border-left-color: var(--danger); }
.avatar { width: 56px; height: 56px; border-radius: 50%; object-fit: cover; flex-shrink: 0; background: var(--surface-2); }
.initials { display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 18px; color: var(--muted); border: 1px solid var(--border); }
.info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.info strong, .reason { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
@keyframes slide-in { from { opacity: 0; transform: translateX(24px); } }
</style>
