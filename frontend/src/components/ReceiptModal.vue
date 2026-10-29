<!-- Caminho: pdv/frontend/src/components/ReceiptModal.vue -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { GetQuote, GetReceipt } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { savePdf } from '../composables/exportTable'
import { directPrint, paperWidth, printReceipt, type ReceiptKind } from '../composables/receipt'
import type { models, Receipt as ReceiptDoc } from '../types'
import Modal from './Modal.vue'
import QuoteA4 from './QuoteA4.vue'
import Receipt from './Receipt.vue'

const props = defineProps<{ kind: ReceiptKind; id: number }>()
const emit = defineEmits<{ close: [] }>()

const doc = ref<ReceiptDoc | null>(null)
const error = ref('')
const busy = ref(false)
// Orçamento em A4 usa o layout de folha (QuoteA4); o resto, as linhas do backend.
const quote = ref<models.QuoteDetails | null>(null)

// Formato escolhido na hora: térmica (cupom) ou folha A4 (impressora comum/PDF).
// Padrão: térmica em todas as telas (A4 só se escolhido).
const format = ref<'termica' | 'a4'>('termica')
const thermalWidth = computed(() => (paperWidth.value === 'a4' ? '80' : paperWidth.value))
const sheet = ref<HTMLElement | null>(null)

onMounted(async () => {
  try {
    doc.value = await call(GetReceipt(props.kind, props.id))
    if (props.kind === 'orcamento') quote.value = await call(GetQuote(props.id))
  } catch (e) {
    error.value = errorMessage(e)
  }
})

async function print() {
  // Térmica configurada: vai direto (ESC/POS). Sem ela, ou em A4: diálogo do Windows.
  if (format.value === 'a4' || !directPrint.value) return window.print()
  busy.value = true
  try {
    await printReceipt(props.kind, props.id)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal :wide="format === 'a4'" :title="doc?.title ? doc.title.charAt(0) + doc.title.slice(1).toLowerCase() : 'Comprovante'" @close="emit('close')">
    <div class="tabs no-print" role="tablist">
      <button role="tab" :aria-selected="format === 'termica'" @click="format = 'termica'">Térmica ({{ thermalWidth }} mm)</button>
      <button role="tab" :aria-selected="format === 'a4'" @click="format = 'a4'">Folha A4 / PDF</button>
    </div>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="!doc" class="state">Carregando…</div>
    <div v-else-if="format === 'a4' && quote" ref="sheet" class="print-area"><QuoteA4 :data="quote" /></div>
    <div v-else ref="sheet" class="print-area"><Receipt :receipt="doc" :paper="format === 'a4' ? 'a4' : thermalWidth" /></div>
    <template #footer>
      <span class="muted small">{{ format === 'a4' ? 'Abre o diálogo do Windows: impressora comum ou salvar em PDF.'
        : directPrint ? 'Impressão direta na térmica.' : 'Sem térmica nas Configurações: abre o diálogo do Windows.' }}</span>
      <span class="spacer" />
      <button v-if="format === 'a4'" :disabled="!doc" @click="savePdf(sheet, doc?.title ?? 'Documento')">Salvar PDF</button>
      <button class="primary" :disabled="!doc || busy" @click="print">{{ busy ? 'Imprimindo…' : format === 'a4' ? 'Imprimir A4 / PDF' : 'Imprimir na térmica' }}</button>
      <button @click="emit('close')">Fechar</button>
    </template>
  </Modal>
</template>
