<!-- Caminho: pdv/frontend/src/components/SellerFilter.vue -->
<script setup lang="ts">
// Filtro por operador. Só aparece para quem pode ver as vendas de todos; para
// os demais o backend já devolve apenas as vendas do próprio usuário.
import { onMounted, ref } from 'vue'
import { ListSellers } from '../../wailsjs/go/main/App'
import { call } from '../api'
import type { User } from '../types'

const model = defineModel<number>({ required: true })
const sellers = ref<User[]>([])

onMounted(async () => {
  try {
    sellers.value = await call(ListSellers())
  } catch {
    sellers.value = []
  }
})
</script>

<template>
  <label v-if="sellers.length" class="field">Operador
    <select v-model="model">
      <option :value="0">Todos</option>
      <option v-for="u in sellers" :key="u.id" :value="u.id">{{ u.name }}{{ u.active ? '' : ' (inativo)' }}</option>
    </select>
  </label>
</template>
