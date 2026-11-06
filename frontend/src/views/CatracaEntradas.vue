<!-- Caminho: catraca/frontend/src/views/CatracaEntradas.vue -->
<script setup lang="ts">
// Relatório de entradas na catraca: passagens de um período, com busca pelo
// nome e filtro de situação. Exporta para Excel (tudo) e PDF (o que está na tela).
import { computed, onMounted, ref } from 'vue'
import { GateEventReport } from '../../wailsjs/go/main/App'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'
import { exportTable, savePdf } from '../composables/exportTable'
import { formatDateTime, today } from '../composables/format'
import LoadMore from '../components/LoadMore.vue'
import { usePaged } from '../composables/paged'

// Um ano de academia passa de dezenas de milhares de linhas: a tela mostra as
// primeiras; o Excel leva todas.
const MAX_ROWS = 1000
const sourceLabel: Record<string, string> = { leitor: 'Leitor facial', balcao: 'Balcão', manual: 'Manual' }

const start = ref(today())
const end = ref(today())
const term = ref('')
const status = ref('')
const rows = ref<models.GateEvent[]>([])
const loaded = ref(false)
const busy = ref(false)
const error = ref('')
const sheet = ref<HTMLElement | null>(null)

async function load() {
  busy.value = true
  error.value = ''
  try {
    rows.value = await call(GateEventReport(start.value, end.value, term.value, status.value))
    loaded.value = true
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
onMounted(load)

const allowed = computed(() => rows.value.filter((e) => e.allowed).length)
const people = computed(() => new Set(rows.value.filter((e) => e.allowed && e.customerId).map((e) => e.customerId)).size)
const shown = computed(() => rows.value.slice(0, MAX_ROWS))
const br = (d: string) => d.split('-').reverse().join('/')
const period = computed(() => (start.value === end.value ? br(start.value) : `${br(start.value)} a ${br(end.value)}`))
const statusLabel: Record<string, string> = { '': 'todas', liberadas: 'só liberadas', negadas: 'só negadas' }

function exportExcel() {
  exportTable('Entradas na catraca', `${period.value} · ${statusLabel[status.value]}${term.value.trim() ? ` · busca "${term.value.trim()}"` : ''}`,
    [['Data e hora', 'text'], ['Aluno', 'text'], ['Situação', 'text'], ['Motivo', 'text'], ['Origem', 'text'], ['Liberado por', 'text']],
    rows.value.map((e) => [formatDateTime(e.at), e.name, e.allowed ? 'Liberado' : 'Negado', e.reason, sourceLabel[e.source] ?? e.source, e.userName]))
}
// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgEntradas = usePaged(() => shown.value, [term, start, end, status])
</script>

<template>
  <div class="stack">
    <section class="card stack">
      <form class="row" style="align-items: flex-end; flex-wrap: wrap" @submit.prevent="load">
        <label class="field">De<input v-model="start" type="date" required /></label>
        <label class="field">Até<input v-model="end" type="date" required /></label>
        <label class="field" style="flex: 1; min-width: 200px">Aluno<input v-model="term" maxlength="100" placeholder="Nome (opcional)" /></label>
        <label class="field">Situação
          <select v-model="status">
            <option value="">Todas</option>
            <option value="liberadas">Liberadas</option>
            <option value="negadas">Negadas</option>
          </select>
        </label>
        <button type="submit" class="primary" :disabled="busy">{{ busy ? 'Buscando…' : 'Buscar' }}</button>
        <button type="button" :disabled="!rows.length" @click="exportExcel">Exportar Excel</button>
        <button type="button" :disabled="!rows.length" @click="savePdf(sheet, 'Entradas na catraca')">Salvar PDF</button>
      </form>
      <div v-if="error" class="alert error" role="alert">{{ error }}</div>
    </section>

    <section v-if="loaded" ref="sheet" class="card stack">
      <h2 style="margin: 0">Entradas na catraca · {{ period }}</h2>
      <p style="margin: 0">
        <strong>{{ rows.length }}</strong> passagem(ns): <strong>{{ allowed }}</strong> liberada(s), <strong>{{ rows.length - allowed }}</strong> negada(s),
        <strong>{{ people }}</strong> aluno(s) diferente(s) entraram.
      </p>
      <div v-if="!rows.length" class="state">Nenhuma passagem nesse período.</div>
      <div v-else class="table-wrap">
        <table>
          <thead><tr><th>Data e hora</th><th class="left">Aluno</th><th>Situação</th><th class="left">Motivo</th><th>Origem</th></tr></thead>
          <tbody>
            <tr v-for="e in pgEntradas.visible" :key="e.id">
              <td class="mono nowrap">{{ formatDateTime(e.at) }}</td>
              <td class="left">{{ e.name || '—' }}</td>
              <td><span class="badge" :class="e.allowed ? 'ok' : 'bad'">{{ e.allowed ? 'Liberado' : 'Negado' }}</span></td>
              <td class="left small">{{ e.reason }}</td>
              <td class="small muted nowrap">{{ sourceLabel[e.source] ?? e.source }}<template v-if="e.userName"> · {{ e.userName }}</template></td>
            </tr>
          </tbody>
        </table>
          <LoadMore :paged="pgEntradas" />
      </div>
      <p v-if="rows.length > MAX_ROWS" class="small muted no-print" style="margin: 0">
        Mostrando as primeiras {{ MAX_ROWS }} de {{ rows.length }}. O Excel leva todas; para o PDF, diminua o período.
      </p>
    </section>
  </div>
</template>
