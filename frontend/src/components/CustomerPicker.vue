<!-- Caminho: pdv/frontend/src/components/CustomerPicker.vue -->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ListCustomers } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { money } from '../composables/format'
import type { Customer } from '../types'

const model = defineModel<number | null>({ required: true })
// customer: o cliente escolhido inteiro (saldo de vale-troca, dívida), para quem precisar.
const customer = defineModel<Customer | null>('customer', { default: null })
// hideCredit: esconde dívida e limite (venda que não é a prazo).
const props = defineProps<{ required?: boolean; label?: string; hideCredit?: boolean }>()

const MAX_RESULTS = 8
const term = ref('')
const results = ref<Customer[]>([])
const selected = ref<Customer | null>(null)
const error = ref('')
const loading = ref(false)

const available = computed(() =>
  selected.value ? Math.max(0, selected.value.creditLimitCents - selected.value.creditBalanceCents) : 0,
)

async function search() {
  const t = term.value.trim()
  if (!t) return (results.value = [])
  loading.value = true
  try {
    results.value = (await call(ListCustomers(t))).filter((c) => c.active).slice(0, MAX_RESULTS)
    error.value = ''
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}

let timer: number | undefined
watch(term, () => {
  window.clearTimeout(timer)
  timer = window.setTimeout(search, 200)
})

function choose(c: Customer) {
  selected.value = c
  model.value = c.id
  term.value = ''
  results.value = []
}

function clear() {
  selected.value = null
  model.value = null
}

// Se o pai definir o cliente por fora, busca os dados dele; se zerar, limpa.
watch(model, async (id) => {
  if (id == null) return (selected.value = null)
  if (selected.value?.id === id) return
  try {
    selected.value = (await call(ListCustomers(''))).find((c) => c.id === id) ?? null
  } catch (e) {
    error.value = errorMessage(e)
  }
}, { immediate: true })
watch(selected, (c) => (customer.value = c), { immediate: true })
</script>

<template>
  <div class="stack" style="gap: 6px">
    <span class="small" style="font-weight: 600; color: var(--muted)">
      {{ props.label ?? 'Cliente' }}{{ props.required ? ' (obrigatório)' : '' }}
    </span>

    <div v-if="selected" class="card row between" style="padding: 8px 10px">
      <div>
        <strong>{{ selected.name }}</strong>
        <div class="small muted">{{ selected.phone || selected.document || '' }}</div>
      </div>
      <button type="button" class="link" @click="clear">Trocar / remover</button>
    </div>

    <template v-else>
      <input
        v-model="term"
        type="search"
        placeholder="Nome, telefone ou CPF"
        aria-label="Buscar cliente"
        autocomplete="off"
        @keydown.enter.prevent="results.length && choose(results[0])"
      />
      <div v-if="results.length" class="table-wrap card" style="padding: 0; max-height: 240px; overflow: auto">
        <table>
          <tbody>
            <tr v-for="c in results" :key="c.id" class="clickable" @click="choose(c)">
              <td>
                <!-- O clique (ou Enter no botão) sobe até a linha inteira. -->
                <button type="button" class="link" style="text-align: left">{{ c.name }}</button>
                <div class="small muted">{{ c.phone || c.document || 'sem telefone' }}</div>
              </td>
              <td class="right small nowrap muted">Dívida <span class="mono">{{ money(c.creditBalanceCents) }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>
      <span v-else-if="term.trim() && !loading" class="small muted">Nenhum cliente encontrado.</span>
    </template>

    <p v-if="error" class="small" style="color: var(--danger)">{{ error }}</p>
    <div v-if="selected && !hideCredit" class="small muted">
      Dívida: <strong class="mono">{{ money(selected.creditBalanceCents) }}</strong> ·
      Limite: <span class="mono">{{ money(selected.creditLimitCents) }}</span> ·
      Disponível: <strong class="mono">{{ money(available) }}</strong>
      <template v-if="selected.storeCreditCents > 0"> · Vale-troca: <strong class="mono" style="color: var(--success)">{{ money(selected.storeCreditCents) }}</strong></template>
    </div>
  </div>
</template>
