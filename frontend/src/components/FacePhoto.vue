<!-- Caminho: catraca/frontend/src/components/FacePhoto.vue -->
<script setup lang="ts">
// Foto do rosto guardada no iDFace (na ficha do aluno e na janela de foto).
import { onMounted, ref } from 'vue'
import { MemberPhoto } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'

const props = defineProps<{ customerId: number; name: string; maxHeight?: string }>()

const url = ref('')
const loading = ref(true)
const error = ref('')
onMounted(async () => {
  try {
    url.value = await call(MemberPhoto(props.customerId))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="state">Buscando a foto no leitor…</div>
  <div v-else-if="error" class="alert error" role="alert">{{ error }}</div>
  <img v-else-if="url" :src="url" :alt="`Rosto de ${name}`" :style="{ maxWidth: '100%', maxHeight: maxHeight ?? '60vh', borderRadius: '8px' }" />
  <div v-else class="state">Sem foto no leitor.</div>
</template>
