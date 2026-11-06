<!-- Caminho: catraca/frontend/src/components/CustomerAvatar.vue -->
<script setup lang="ts">
// Avatar do cliente: as iniciais numa cor fixa por cliente. No tamanho grande
// (ficha do cliente), mostra a foto do rosto guardada no iDFace quando existe.
// No PDV genérico o componente tem o mesmo nome, só com as iniciais.
import { computed, ref, watch } from 'vue'
import { MemberPhoto } from '../../wailsjs/go/main/App'

const props = defineProps<{ id: number; name: string; big?: boolean }>()
const colors = ['#6366f1', '#10b981', '#f59e0b', '#ef4444', '#06b6d4', '#ec4899', '#8b5cf6', '#84cc16', '#f97316', '#14b8a6']
const initials = computed(() => props.name.trim().split(/\s+/).slice(0, 2).map((w) => w.charAt(0)).join('').toUpperCase())

// A foto só na ficha (big): na lista seriam muitas idas ao aparelho.
const url = ref('')
watch(() => [props.id, props.big] as const, async ([id, big]) => {
  url.value = ''
  if (!big) return
  try {
    const u = await MemberPhoto(id)
    if (id === props.id) url.value = u
  } catch {
    // sem leitor ou fora do ar: ficam as iniciais
  }
}, { immediate: true })
</script>

<template>
  <span class="avatar" :class="{ big }" :style="{ background: url ? 'transparent' : colors[id % colors.length] }">
    <img v-if="url" :src="url" :alt="`Rosto de ${name}`" />
    <template v-else>{{ initials }}</template>
  </span>
</template>

<style scoped>
.avatar img { width: 100%; height: 100%; object-fit: cover; border-radius: inherit; display: block; }
</style>
