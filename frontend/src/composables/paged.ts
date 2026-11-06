// Caminho: catraca/frontend/src/composables/paged.ts

// Listas longas aos poucos: a tela desenha só as primeiras linhas (50) e vai
// acrescentando conforme rola ou clica em "Mostrar mais" (LoadMore.vue). A
// lista completa continua na memória: filtros, contagens e exportação usam
// tudo. Volta ao começo quando mudam as dependências de reset (busca, aba…).
import { computed, reactive, ref, watch, type WatchSource } from 'vue'

export const PAGE_SIZE = 50

export function usePaged<T>(source: () => T[], reset: WatchSource[] = [], size = PAGE_SIZE) {
  const limit = ref(size)
  const all = computed(source)
  if (reset.length) watch(reset, () => (limit.value = size))
  return reactive({
    visible: computed(() => all.value.slice(0, limit.value)),
    total: computed(() => all.value.length),
    shown: computed(() => Math.min(limit.value, all.value.length)),
    more: () => (limit.value += size),
  })
}
