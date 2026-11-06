<!-- Caminho: catraca/frontend/src/components/LoadMore.vue -->
<script setup lang="ts">
// Rodapé das listas longas (usePaged): mostra quantas linhas estão na tela e
// carrega mais sozinho quando chega perto do fim da rolagem (ou no botão).
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{ paged: { shown: number; total: number; more: () => void } }>()
const el = ref<HTMLElement | null>(null)
let obs: IntersectionObserver | undefined

onMounted(() => {
  obs = new IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting) && props.paged.shown < props.paged.total) props.paged.more()
  }, { rootMargin: '300px' })
  if (el.value) obs.observe(el.value)
})
onBeforeUnmount(() => obs?.disconnect())
</script>

<template>
  <div ref="el" class="load-more no-print">
    <template v-if="paged.shown < paged.total">
      <span class="small muted">Mostrando {{ paged.shown.toLocaleString('pt-BR') }} de {{ paged.total.toLocaleString('pt-BR') }}</span>
      <button type="button" class="link" @click="paged.more()">Mostrar mais</button>
    </template>
    <span v-else-if="paged.total > 50" class="small muted">Todos os {{ paged.total.toLocaleString('pt-BR') }} na tela</span>
  </div>
</template>

<style scoped>
.load-more { display: flex; justify-content: center; align-items: center; gap: 12px; min-height: 28px; padding: 8px 0 2px; }
</style>
