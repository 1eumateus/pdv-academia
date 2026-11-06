// Caminho: catraca/frontend/src/composables/rowMenu.ts

// Menu "Mais" das linhas de tabela: um aberto por vez, fecha ao clicar fora.
// Fica preso à janela (position: fixed) para não ser cortado pela rolagem da
// tabela; perto do fim da tela, abre para cima.
import { onBeforeUnmount, onMounted, ref } from 'vue'

export function useRowMenu() {
  const openId = ref(0)
  const style = ref<Record<string, string>>({})
  const close = () => (openId.value = 0)

  function toggle(id: number, ev: MouseEvent) {
    if (openId.value === id) return close()
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect()
    const right = `${Math.max(window.innerWidth - r.right, 8)}px`
    style.value = window.innerHeight - r.bottom < 240
      ? { position: 'fixed', right, bottom: `${window.innerHeight - r.top}px`, top: 'auto' }
      : { position: 'fixed', right, top: `${r.bottom}px` }
    openId.value = id
  }

  onMounted(() => {
    document.addEventListener('click', close)
    window.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
  })
  onBeforeUnmount(() => {
    document.removeEventListener('click', close)
    window.removeEventListener('scroll', close, true)
    window.removeEventListener('resize', close)
  })
  return { openId, style, toggle, close }
}
