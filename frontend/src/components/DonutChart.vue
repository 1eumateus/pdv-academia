<!-- Caminho: pdv/frontend/src/components/DonutChart.vue -->
<script setup lang="ts">
// Gráfico de rosca com legenda (só na tela): as maiores fatias e o resto em "Outras".
import { computed } from 'vue'

export type Slice = { name: string; value: number; text?: string }
const props = withDefaults(defineProps<{ items: Slice[]; center?: string; centerLabel?: string; max?: number }>(), { max: 5 })
const colors = ['#5b4cf0', '#06b6d4', '#10b981', '#f59e0b', '#ec4899', '#94a3b8']

const slices = computed(() => {
  const list = props.items.filter((s) => s.value > 0).sort((a, b) => b.value - a.value)
  const head = list.slice(0, props.max)
  const rest = list.slice(props.max).reduce((a, s) => a + s.value, 0)
  if (rest) head.push({ name: 'Outras', value: rest })
  const sum = head.reduce((a, s) => a + s.value, 0) || 1
  let acc = 0
  return head.map((s, i) => {
    const from = acc / sum
    acc += s.value
    return { ...s, pct: Math.round((s.value / sum) * 100), color: colors[i % colors.length], path: arc(from, acc / sum) }
  })
})
// Arco de raio 70 em volta do centro (90,90), de uma fração a outra da volta.
function arc(from: number, to: number) {
  if (to - from >= 0.9999) to = from + 0.9999
  const p = (f: number) => [90 + 70 * Math.sin(f * 2 * Math.PI), 90 - 70 * Math.cos(f * 2 * Math.PI)].map((n) => n.toFixed(2)).join(',')
  return `M${p(from)} A70,70 0 ${to - from > 0.5 ? 1 : 0},1 ${p(to)}`
}
</script>

<template>
  <div v-if="slices.length" class="donut-wrap">
    <svg viewBox="0 0 180 180" class="donut" role="img" :aria-label="centerLabel ?? 'Participação'">
      <path v-for="s in slices" :key="s.name" :d="s.path" :stroke="s.color" stroke-width="26" fill="none"><title>{{ s.name }}: {{ s.text ?? s.value }}</title></path>
      <text v-if="centerLabel" x="90" y="85" text-anchor="middle" class="sub">{{ centerLabel }}</text>
      <text v-if="center" x="90" y="106" text-anchor="middle" class="num">{{ center }}</text>
    </svg>
    <ul class="legend">
      <li v-for="s in slices" :key="s.name"><i :style="{ background: s.color }" /><span>{{ s.name }}</span><strong>{{ s.pct }}%</strong></li>
    </ul>
  </div>
  <div v-else class="state">Nada no período.</div>
</template>

<style scoped>
.donut-wrap { display: flex; align-items: center; gap: 20px; }
.donut { width: 170px; flex-shrink: 0; }
.donut path:hover { opacity: 0.8; }
.sub { font-size: 12px; fill: var(--muted); }
.num { font-size: 21px; font-weight: 700; fill: var(--text); }
.legend { list-style: none; margin: 0; padding: 0; flex: 1; min-width: 0; }
.legend li { display: flex; align-items: center; gap: 10px; padding: 5px 0; font-size: 14px; }
.legend i { width: 11px; height: 11px; border-radius: 3px; flex-shrink: 0; }
.legend span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
