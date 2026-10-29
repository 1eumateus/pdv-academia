<!-- Caminho: pdv/frontend/src/components/SupportCard.vue -->
<script setup lang="ts">
// Caixa com o contato do suporte (rodapé do menu), quando a licença ou o teste está
// vencido ou perto de vencer. Minimizar vale até fechar o programa.
import { computed, ref } from 'vue'
import Icon from './Icon.vue'
import { openWhatsapp } from '../composables/charge'
import { licenseStatus, storeName } from '../composables/cashState'

const SUPPORT_WHATSAPP = '(94) 98448-9585'

const minimized = ref(false)
const st = computed(() => licenseStatus.value)
const expired = computed(() => !!st.value && !st.value.canWrite)

function talk() {
  const s = st.value
  const text = [
    `Olá! ${expired.value ? 'Minha licença do PDV venceu' : 'Quero renovar a licença do PDV'} e preciso de ajuda.`,
    `Loja: ${s?.store || storeName.value}`,
    `ID da máquina: ${s?.machineId ?? ''}`,
  ].join('\n')
  openWhatsapp(SUPPORT_WHATSAPP, text)
}
</script>

<template>
  <!-- Flutua no canto inferior esquerdo; minimizado vira só um botão pequeno. -->
  <div v-if="st?.warning" class="support no-print">
    <button v-if="minimized" class="support-mini" title="Falar com o suporte pelo WhatsApp" @click="minimized = false">
      <Icon name="whatsapp" /> Suporte
    </button>
    <aside v-else class="support-card" :class="{ bad: expired }" aria-label="Contato do suporte">
      <header class="row between">
        <strong>{{ expired ? 'Licença vencida' : 'Licença perto de vencer' }}</strong>
        <button class="link" title="Minimizar" aria-label="Minimizar" @click="minimized = true">—</button>
      </header>
      <p class="small">Fale com o suporte para renovar:</p>
      <div class="phone mono">{{ SUPPORT_WHATSAPP }}</div>
      <button class="success" @click="talk"><Icon name="whatsapp" /> Chamar no WhatsApp</button>
      <p class="small muted">Vai com o nome da loja e o ID desta máquina.</p>
    </aside>
  </div>
</template>

<style scoped>
.support { position: fixed; left: 16px; bottom: 16px; width: 240px; z-index: 50; }
.support-card {
  display: flex; flex-direction: column; gap: 6px; padding: 10px 12px;
  background: var(--surface); color: var(--text); border: 1px solid var(--warn-border);
  border-left: 5px solid var(--warning); border-radius: var(--radius); box-shadow: var(--shadow);
}
.support-card.bad { border-color: var(--bad-border); border-left-color: var(--danger); }
.support-card p { margin: 0; }
.support-card .phone { font-size: 16px; font-weight: 700; }
.support-card header .link { font-size: 18px; line-height: 1; padding: 0 4px; }
.support-card .success, .support-mini { display: flex; align-items: center; justify-content: center; gap: 6px; }
.support-mini { width: auto; background: var(--success); color: #fff; border: none; box-shadow: var(--shadow); }
</style>
