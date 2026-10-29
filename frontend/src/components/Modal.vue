<!-- Caminho: pdv/frontend/src/components/Modal.vue -->
<script lang="ts">
const open: symbol[] = []
</script>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

// full: telas largas de conferência (ex.: importação da NF-e).
const props = defineProps<{ title: string; wide?: boolean; full?: boolean; busy?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const box = ref<HTMLElement | null>(null)
let previous: Element | null = null
// Janelas abertas, a última por cima: o Esc fecha só ela.
const me = Symbol()

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && !props.busy && open.at(-1) === me) emit('close')
}

onMounted(async () => {
  previous = document.activeElement
  open.push(me)
  window.addEventListener('keydown', onKey)
  await nextTick()
  const first = box.value?.querySelector<HTMLElement>('input, select, textarea, button.primary, button')
  first?.focus()
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  open.splice(open.indexOf(me), 1)
  if (previous instanceof HTMLElement) previous.focus()
})
</script>

<template>
  <div class="modal-backdrop" @mousedown.self="!busy && emit('close')">
    <div ref="box" class="modal" :class="{ wide, full }" role="dialog" aria-modal="true" :aria-label="title">
      <h2>{{ title }}</h2>
      <slot />
      <footer v-if="$slots.footer" class="no-print"><slot name="footer" /></footer>
    </div>
  </div>
</template>
