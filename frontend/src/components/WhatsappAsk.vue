<!-- Caminho: pdv/frontend/src/components/WhatsappAsk.vue -->
<script setup lang="ts">
// Pede para quem enviar quando não há celular conhecido: escolher um cliente
// cadastrado ou digitar o número. O texto já vem pronto; quem envia é o operador.
import { ref } from 'vue'
import { ListCustomers } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { openWhatsapp, whatsappPhone } from '../composables/charge'
import CustomerPicker from './CustomerPicker.vue'
import Modal from './Modal.vue'

const props = defineProps<{ text: string }>()
const emit = defineEmits<{ close: [] }>()
const customerId = ref<number | null>(null)
const phone = ref('')
const busy = ref(false)
const error = ref('')

async function send() {
  error.value = ''
  let p = phone.value.trim()
  if (!p && customerId.value) {
    busy.value = true
    try {
      p = (await call(ListCustomers(''))).find((c) => c.id === customerId.value)?.phone ?? ''
    } catch (e) {
      return (error.value = errorMessage(e))
    } finally {
      busy.value = false
    }
    if (!whatsappPhone(p)) return (error.value = 'Esse cliente não tem celular com DDD no cadastro. Digite o número abaixo.')
  }
  if (!whatsappPhone(p)) return (error.value = 'Digite o celular com DDD. Exemplo: (11) 98765-4321')
  openWhatsapp(p, props.text)
  emit('close')
}

function withoutNumber() {
  openWhatsapp(null, props.text)
  emit('close')
}
</script>

<template>
  <Modal title="Enviar pelo WhatsApp" :busy="busy" @close="emit('close')">
    <form class="stack" @submit.prevent="send">
      <p class="small muted" style="margin: 0">Não há celular para enviar. Escolha o cliente cadastrado ou digite o número.</p>
      <CustomerPicker v-model="customerId" label="Cliente" hide-credit />
      <label class="field">Ou digite o celular (com DDD)<input v-model="phone" inputmode="tel" maxlength="30" placeholder="(11) 98765-4321" /></label>
      <div v-if="error" class="alert error" role="alert">{{ error }}</div>
      <footer class="row end">
        <button type="button" :disabled="busy" @click="emit('close')">Cancelar</button>
        <span class="spacer" />
        <button type="button" :disabled="busy" @click="withoutNumber">Abrir sem número</button>
        <button type="submit" class="success" :disabled="busy || (!customerId && !phone.trim())">Abrir WhatsApp</button>
      </footer>
    </form>
  </Modal>
</template>
