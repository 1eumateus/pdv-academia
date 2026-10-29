<!-- Caminho: pdv/frontend/src/components/PixModal.vue -->
<script setup lang="ts">
// QR Code Pix estático com o valor exato. Não há confirmação automática: o
// operador confere no app do banco e clica "Pix recebido".
import { onMounted, ref } from 'vue'
import { GetPixQR, PrintPixQR } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { directPrint } from '../composables/receipt'
import { money } from '../composables/format'
import { useToast } from '../composables/toast'
import Modal from './Modal.vue'

const props = defineProps<{ amountCents: number; txid: string }>()
const emit = defineEmits<{ received: []; cancel: [] }>()
const toast = useToast()

const code = ref<{ payload: string; image: string } | null>(null)
const error = ref('')

onMounted(async () => {
  try {
    code.value = await call(GetPixQR(props.amountCents, props.txid))
  } catch (e) {
    error.value = errorMessage(e)
  }
})

const printing = ref(false)
async function printQR() {
  printing.value = true
  try {
    await call(PrintPixQR(props.amountCents, props.txid))
    toast.success('QR Code enviado para a impressora.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    printing.value = false
  }
}

async function copy() {
  if (!code.value) return
  try {
    await navigator.clipboard.writeText(code.value.payload)
    toast.success('"Pix copia e cola" copiado.')
  } catch {
    toast.error('Não foi possível copiar. Selecione o texto e copie manualmente.')
  }
}
</script>

<template>
  <Modal title="Pagamento com Pix" @close="emit('cancel')">
    <div class="stack center" style="gap: 10px">
      <div class="pix-amount mono">{{ money(amountCents) }}</div>
      <div v-if="error" class="alert error">{{ error }}</div>
      <div v-else-if="!code" class="state">Gerando QR Code…</div>
      <template v-else>
        <img :src="code.image" alt="QR Code Pix" class="pix-qr" />
        <div class="small muted">Peça para o cliente ler o QR Code no app do banco, ou envie o código abaixo.</div>
        <textarea class="mono small pix-payload" readonly :value="code.payload" @focus="($event.target as HTMLTextAreaElement).select()" />
        <div class="row" style="justify-content: center">
          <button type="button" @click="copy">Copiar "Pix copia e cola"</button>
          <button type="button" :disabled="printing" :title="directPrint ? '' : 'Escolha a impressora térmica em Configurações'" @click="printQR">
            {{ printing ? 'Imprimindo…' : 'Imprimir QR Code' }}
          </button>
        </div>
      </template>
      <div class="alert warn small" style="text-align: left">
        O sistema não confirma o pagamento sozinho. <strong>Confira no app do banco</strong> que o Pix de
        {{ money(amountCents) }} caiu antes de clicar em "Pix recebido".
      </div>
    </div>
    <template #footer>
      <button @click="emit('cancel')">Cancelar</button>
      <button class="success" :disabled="!code" @click="emit('received')">Pix recebido</button>
    </template>
  </Modal>
</template>
