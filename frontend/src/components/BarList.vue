<!-- Caminho: pdv/frontend/src/components/BarList.vue -->
<script setup lang="ts">
// Ranking em barras horizontais (só na tela): nome, barra proporcional ao maior
// valor e o valor já formatado à direita.
import { computed } from 'vue'

export type BarItem = { key: string | number; label: string; value: number; text: string; sub?: string; color?: string }
const props = defineProps<{ items: BarItem[]; color?: string }>()
const max = computed(() => Math.max(1, ...props.items.map((i) => Math.abs(i.value))))
</script>

<template>
  <ol class="bars">
    <li v-for="(it, i) in items" :key="it.key">
      <span class="pos">{{ i + 1 }}</span>
      <span class="name" :title="it.label">{{ it.label }}<small v-if="it.sub">{{ it.sub }}</small></span>
      <span class="track"><i :style="{ width: `${(Math.abs(it.value) / max) * 100}%`, background: it.color ?? color ?? 'var(--primary)' }" /></span>
      <strong class="val mono">{{ it.text }}</strong>
    </li>
  </ol>
</template>

<style scoped>
.bars { list-style: none; margin: 0; padding: 0; }
.bars li { display: grid; grid-template-columns: 22px minmax(140px, 1.2fr) minmax(80px, 2fr) 104px; align-items: center; gap: 12px; padding: 7px 0; }
.pos { color: var(--muted); font-size: 12px; text-align: right; }
.name { font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.name small { display: block; font-size: 11.5px; color: var(--muted); }
.track { height: 10px; border-radius: 10px; background: var(--surface-2); overflow: hidden; }
.track i { display: block; height: 100%; border-radius: 10px; }
.val { font-size: 13.5px; white-space: nowrap; text-align: right; }
</style>
