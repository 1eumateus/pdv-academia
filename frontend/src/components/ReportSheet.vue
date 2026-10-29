<!-- Caminho: pdv/frontend/src/components/ReportSheet.vue -->
<script setup lang="ts">
// Folha padrão dos relatórios (A4): cabeçalho com logo e dados da loja,
// título, período e operador; rodapé com quem gerou e quando. O conteúdo
// (tabelas) vem no slot. printOnly: cabeçalho e rodapé só na impressão.
import { computed } from 'vue'
import { settings } from '../composables/cashState'
import { currentUser } from '../composables/session'

const props = defineProps<{ title: string; period?: string; operator?: string | null; printOnly?: boolean }>()

const s = computed(() => settings.value)
const address = computed(() => {
  const v = s.value
  if (!v) return ''
  const street = [v.street, v.number, v.complement].filter(Boolean).join(', ')
  const city = [v.city, v.state].filter(Boolean).join('/')
  return [street, v.district, city].filter(Boolean).join(' · ')
})
const contact = computed(() => [s.value?.phone, s.value?.whatsapp && `WhatsApp ${s.value.whatsapp}`, s.value?.email].filter(Boolean).join(' · '))
const generated = computed(() => {
  const at = new Date().toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
  return `Gerado em ${at}${currentUser.value ? ` por ${currentUser.value.name}` : ''}`
})
</script>

<template>
  <div class="report-sheet">
    <header class="rs-head" :class="{ 'rs-print-only': props.printOnly }">
      <img v-if="s?.logo" :src="s.logo" alt="" class="rs-logo" />
      <div class="rs-store">
        <strong>{{ s?.storeName }}</strong>
        <span v-if="s?.document">{{ s.document }}</span>
        <span v-if="address">{{ address }}</span>
        <span v-if="contact">{{ contact }}</span>
      </div>
      <div class="rs-meta">
        <strong class="rs-title">{{ title }}</strong>
        <span v-if="period">{{ period }}</span>
        <!-- operator não informado: relatório que não é por operador (análises). -->
        <span v-if="operator !== undefined">{{ operator ? `Operador: ${operator}` : 'Todos os operadores' }}</span>
      </div>
    </header>
    <slot />
    <footer class="rs-foot" :class="{ 'rs-print-only': props.printOnly }">
      <span>{{ generated }}</span><span>Documento não fiscal</span>
    </footer>
  </div>
</template>

<style scoped>
.rs-head { display: flex; align-items: center; gap: 14px; padding-bottom: 10px; margin-bottom: 12px; border-bottom: 2px solid var(--text); }
.rs-logo { max-width: 90px; max-height: 60px; object-fit: contain; }
.rs-store { flex: 1; display: flex; flex-direction: column; font-size: 12px; line-height: 1.35; }
.rs-store strong { font-size: 16px; }
.rs-meta { display: flex; flex-direction: column; align-items: flex-end; text-align: right; font-size: 12px; line-height: 1.35; }
.rs-title { font-size: 16px; }
.rs-foot { display: flex; justify-content: space-between; margin-top: 14px; padding-top: 6px; border-top: 1px solid var(--border); font-size: 11px; color: var(--muted); }
.rs-print-only { display: none; }
.print-area .rs-print-only { display: flex; }
@media print {
  .report-sheet :deep(table) { font-size: 11px; }
  .report-sheet :deep(th), .report-sheet :deep(td) { padding: 3px 6px; }
  .report-sheet { color: #000; }
  .rs-head { border-bottom-color: #000; }
  .rs-foot { color: #444; }
}
</style>
