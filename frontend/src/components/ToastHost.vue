<!-- Caminho: pdv/frontend/src/components/ToastHost.vue -->
<script setup lang="ts">
import { dismiss, useToast } from '../composables/toast'
const { items } = useToast()
</script>

<template>
  <div class="toasts no-print" aria-live="polite">
    <div v-for="t in items" :key="t.id" class="toast" :class="t.kind" :role="t.kind === 'error' ? 'alert' : 'status'">
      <span style="flex: 1">{{ t.text }}</span>
      <button aria-label="Fechar aviso" @click="dismiss(t.id)">×</button>
    </div>
  </div>
</template>
