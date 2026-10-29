<!-- Caminho: pdv/frontend/src/components/ConfirmDialog.vue -->
<script setup lang="ts">
// Confirmação para operações destrutivas, com campo de motivo opcional.
import { ref } from 'vue'
import Modal from './Modal.vue'

const props = defineProps<{
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
  reasonLabel?: string
  busy?: boolean
}>()
const emit = defineEmits<{ confirm: [reason: string]; cancel: [] }>()
const reason = ref('')
const touched = ref(false)

function submit() {
  touched.value = true
  if (props.reasonLabel && !reason.value.trim()) return
  emit('confirm', reason.value.trim())
}
</script>

<template>
  <Modal :title="title" :busy="busy" @close="emit('cancel')">
    <form class="stack" @submit.prevent="submit">
      <p>{{ message }}</p>
      <label v-if="reasonLabel" class="field">
        {{ reasonLabel }}
        <textarea v-model="reason" maxlength="500" :aria-invalid="touched && !reason.trim()" required />
      </label>
      <p v-if="reasonLabel && touched && !reason.trim()" class="small" style="color: var(--danger)">Informe o motivo.</p>
      <footer class="row end">
        <button type="button" :disabled="busy" @click="emit('cancel')">{{ cancelLabel ?? 'Voltar' }}</button>
        <button type="submit" :class="danger ? 'danger' : 'primary'" :disabled="busy">
          {{ busy ? 'Processando…' : confirmLabel ?? 'Confirmar' }}
        </button>
      </footer>
    </form>
  </Modal>
</template>
