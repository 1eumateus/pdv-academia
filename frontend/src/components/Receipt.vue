<!-- Caminho: pdv/frontend/src/components/Receipt.vue -->
<script setup lang="ts">
// Comprovante NÃO FISCAL. As linhas vêm prontas do backend (as mesmas enviadas
// à impressora térmica); aqui só são desenhadas. Não substitui NFC-e/NF-e/SAT.
import { paperWidth } from '../composables/receipt'
import type { Receipt } from '../types'

// paper força um formato (ex.: orçamento em A4); sem ele, o das Configurações.
const props = defineProps<{ receipt: Receipt; paper?: string }>()
</script>

<template>
  <div class="receipt" :class="`paper-${props.paper ?? paperWidth}`">
    <div v-if="receipt.logo" class="center"><img :src="receipt.logo" alt="Logo" class="receipt-logo" /></div>
    <template v-for="(l, i) in receipt.lines" :key="i">
      <hr v-if="l.kind === 'sep'" />
      <div v-else-if="l.kind === 'pair'" class="pair" :class="{ bold: l.bold }">
        <span>{{ l.text }}</span><span>{{ l.right }}</span>
      </div>
      <div v-else-if="l.kind === 'item'" class="item-row" :class="{ bold: l.bold }">
        <span class="item-name">{{ l.text }}</span><span>{{ l.qty }}</span><span>{{ l.price }}</span><span>{{ l.right }}</span>
      </div>
      <div v-else-if="l.kind === 'qr'" class="center"><img :src="l.right" alt="QR Code Pix" style="width: 160px; height: 160px" /></div>
      <div v-else-if="l.kind === 'signature'" class="signature">
        <div class="line" />
        <div>{{ l.text }}</div>
      </div>
      <div v-else :class="{ center: l.center, bold: l.bold }">{{ l.text }}</div>
    </template>
  </div>
</template>
