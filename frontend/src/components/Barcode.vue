<!-- Caminho: pdv/frontend/src/components/Barcode.vue -->
<script setup lang="ts">
// Código de barras em SVG (EAN-13 ou Code128), com margem branca dos lados
// (zona de silêncio) para o leitor conseguir ler.
import { computed } from 'vue'
import { encodeBarcode } from '../composables/barcode'

const props = defineProps<{ value: string; showText?: boolean }>()
const QUIET = 10
const bars = computed(() => encodeBarcode(props.value))
// Junta módulos vizinhos numa barra só (menos retângulos no SVG).
const rects = computed(() => {
  const out: { x: number; w: number }[] = []
  const bits = bars.value?.bits ?? ''
  for (let i = 0; i < bits.length; i++) {
    if (bits[i] !== '1') continue
    if (out.length && out[out.length - 1].x + out[out.length - 1].w === i + QUIET) out[out.length - 1].w++
    else out.push({ x: i + QUIET, w: 1 })
  }
  return out
})
const width = computed(() => (bars.value?.bits.length ?? 0) + QUIET * 2)
</script>

<template>
  <div v-if="bars" class="barcode">
    <svg :viewBox="`0 0 ${width} 40`" preserveAspectRatio="none" role="img" :aria-label="`Código de barras ${value}`">
      <rect :width="width" height="40" fill="#fff" />
      <rect v-for="(r, i) in rects" :key="i" :x="r.x" y="0" :width="r.w" height="40" fill="#000" />
    </svg>
    <div v-if="showText" class="barcode-text">{{ value }}</div>
  </div>
  <div v-else class="barcode-text">{{ value }}</div>
</template>

<style scoped>
.barcode { display: flex; flex-direction: column; align-items: stretch; min-height: 0; flex: 1; }
.barcode svg { width: 100%; flex: 1; min-height: 0; display: block; }
.barcode-text { font-family: 'Consolas', monospace; font-size: 2.6mm; text-align: center; line-height: 1.1; letter-spacing: 0.3mm; color: #000; }
</style>
