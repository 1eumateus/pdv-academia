// Caminho: pdv/frontend/src/composables/toast.ts

import { reactive } from 'vue'

export interface Toast {
  id: number
  kind: 'success' | 'error' | 'info'
  text: string
}

const state = reactive({ items: [] as Toast[] })
let seq = 0

function push(kind: Toast['kind'], text: string, ms = kind === 'error' ? 7000 : 3500) {
  const id = ++seq
  state.items.push({ id, kind, text })
  setTimeout(() => dismiss(id), ms)
}

export function dismiss(id: number) {
  const i = state.items.findIndex((t) => t.id === id)
  if (i >= 0) state.items.splice(i, 1)
}

export function useToast() {
  return {
    items: state.items,
    success: (t: string) => push('success', t),
    error: (t: string) => push('error', t),
    info: (t: string) => push('info', t),
  }
}
