<!-- Caminho: catraca/frontend/src/components/FacePhotoModal.vue -->
<script setup lang="ts">
// Janela com a foto do rosto (logo após o cadastro, ou para conferir depois).
import FacePhoto from './FacePhoto.vue'
import Modal from './Modal.vue'

defineProps<{ customerId: number; name: string }>()
defineEmits<{ close: [] }>()
</script>

<template>
  <Modal :title="`Rosto de ${name}`" @close="$emit('close')">
    <div class="stack" style="align-items: center">
      <FacePhoto :customer-id="customerId" :name="name" />
      <footer class="row end" style="align-self: stretch">
        <button type="button" class="primary" @click="$emit('close')">Fechar</button>
      </footer>
    </div>
  </Modal>
</template>
