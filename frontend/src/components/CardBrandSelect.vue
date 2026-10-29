<!-- Caminho: pdv/frontend/src/components/CardBrandSelect.vue -->
<script setup lang="ts">
// Bandeira do cartão (só informativa): as mais comuns no Brasil, os cartões de
// alimentação/refeição e "Outra" para digitar. O backend aceita até 30 caracteres.
import { ref, watch } from 'vue'

const model = defineModel<string>({ required: true })
defineProps<{ disabled?: boolean }>()

const cards = ['Visa', 'Mastercard', 'Elo', 'Hipercard', 'American Express', 'Diners Club', 'Cabal', 'Hiper', 'Discover', 'JCB', 'Sorocred', 'Banescard', 'Credz', 'Aura']
const food = ['Alelo', 'Pluxee (Sodexo)', 'Ticket', 'VR', 'Ben', 'Flash', 'Caju', 'Swile', 'iFood Benefícios', 'Up Brasil', 'Verocard', 'Vale Card', 'GreenCard', 'Senff', 'Planvale']
const known = new Set([...cards, ...food])
const OTHER = '__outra'

// Valor do select: a bandeira conhecida, "Outra" (texto livre) ou vazio.
const choice = ref(model.value && !known.has(model.value) ? OTHER : model.value)
watch(choice, (v) => {
  if (v !== OTHER) model.value = v
  else if (known.has(model.value)) model.value = ''
})
watch(model, (v) => {
  if (!v && choice.value !== OTHER) choice.value = ''
})
</script>

<template>
  <div class="brand">
    <select v-model="choice" :disabled="disabled" aria-label="Bandeira do cartão">
      <option value="">Bandeira (opcional)</option>
      <optgroup label="Cartões">
        <option v-for="b in cards" :key="b" :value="b">{{ b }}</option>
      </optgroup>
      <optgroup label="Alimentação / refeição">
        <option v-for="b in food" :key="b" :value="b">{{ b }}</option>
      </optgroup>
      <option :value="OTHER">Outra (digitar)…</option>
    </select>
    <input v-if="choice === OTHER" v-model="model" maxlength="30" placeholder="Nome da bandeira" :disabled="disabled" aria-label="Outra bandeira" />
  </div>
</template>

<style scoped>
.brand { display: flex; gap: 6px; flex: 1; min-width: 0; }
.brand > * { flex: 1; min-width: 0; }
</style>
