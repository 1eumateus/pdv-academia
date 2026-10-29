<!-- Caminho: pdv/frontend/src/components/DebtWarning.vue -->
<script setup lang="ts">
// Aviso na venda a prazo: o cliente escolhido já tem notas em aberto.
import { computed, ref, watch } from 'vue'
import { GetCustomerHistory } from '../../wailsjs/go/main/App'
import { call } from '../api'
import { money } from '../composables/format'
import type { Customer, models } from '../types'

const props = defineProps<{ customer: Customer | null }>()
const notes = ref<models.FiadoNote[]>([])

watch(() => [props.customer?.id, props.customer?.creditBalanceCents], async () => {
  notes.value = []
  const c = props.customer
  if (!c || c.creditBalanceCents <= 0) return
  try {
    notes.value = (await call(GetCustomerHistory(c.id))).notes.filter((n) => n.openCents > 0)
  } catch {
    // sem as notas o aviso ainda mostra o valor em aberto
  }
}, { immediate: true })

const oldest = computed(() => Math.max(0, ...notes.value.map((n) => n.daysOpen)))
</script>

<template>
  <div v-if="customer && customer.creditBalanceCents > 0" class="alert warn debt" role="alert">
    <strong>Atenção: débitos pendentes de {{ money(customer.creditBalanceCents) }}</strong>
    <span v-if="notes.length">{{ notes.length }} nota(s) em aberto, a mais antiga há {{ oldest }} dia(s).</span>
  </div>
</template>

<style scoped>
.debt { display: flex; flex-direction: column; gap: 2px; margin: 0; font-size: 13px; }
</style>
