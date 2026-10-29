<!-- Caminho: catraca/frontend/src/views/ApagarTestes.vue -->
<script setup lang="ts">
// Apagar vendas de teste (só o administrador): as vendas de um dia de um
// operador (ou de todos) e o caixa que ele abriu nesse dia. Mostra antes o que
// vai sumir; ao confirmar, faz um backup na pasta de dados e apaga de vez
// (estoque, fiado e mensalidades voltam).
import { computed, onMounted, ref } from 'vue'
import { ApplyTestPurge, ListUsers, PreviewTestPurge } from '../../wailsjs/go/main/App'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import { formatDateTime, money, paymentLabels, today } from '../composables/format'
import { useToast } from '../composables/toast'

const toast = useToast()
const users = ref<models.User[]>([])
// userId: null = nenhum escolhido; 0 = todos os operadores.
const userId = ref<number | null>(null)
const date = ref(today())
const preview = ref<models.TestPurge | null>(null)
const busy = ref(false)
const error = ref('')
const confirmOpen = ref(false)

onMounted(async () => {
  try {
    users.value = await call(ListUsers())
  } catch (e) {
    error.value = errorMessage(e)
  }
})

async function load() {
  preview.value = null
  if (userId.value === null || !date.value) return
  busy.value = true
  error.value = ''
  try {
    preview.value = await call(PreviewTestPurge(date.value, userId.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
const canApply = computed(() => !!preview.value && !preview.value.blockers.length && (preview.value.sales.length + preview.value.sessions.length) > 0)
const userName = computed(() => (userId.value === 0 ? 'todos os operadores' : users.value.find((u) => u.id === userId.value)?.name ?? ''))
const brDay = computed(() => date.value.split('-').reverse().join('/'))

async function apply() {
  confirmOpen.value = false
  const p = preview.value!
  busy.value = true
  try {
    const r = await call(ApplyTestPurge(date.value, userId.value!, p.sales.length, p.sessions.length))
    toast.success(`Apagadas ${p.sales.length} venda(s) e ${p.sessions.length} caixa(s) de ${userName.value} em ${brDay.value}. Backup antes de apagar: ${r.backupPath}`)
    await load()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="card stack" style="max-width: 900px">
    <h2 style="margin: 0">Apagar vendas de teste</h2>
    <p class="small muted" style="margin: 0">
      Apaga de vez as vendas que o operador fez no dia e o caixa que ele abriu nesse dia (abertura, suprimentos, sangrias e fechamento).
      Caixa que tem outras coisas (venda de outra pessoa, recebimento de fiado) fica, e perde só os lançamentos dessas vendas.
      O estoque volta, o fiado e o vale-troca são desfeitos e as mensalidades pagas nessas vendas voltam a ficar em aberto.
      Antes de apagar, o sistema faz um backup na pasta de dados.
    </p>
    <div class="row">
      <label class="field">Dia<input v-model="date" type="date" :max="today()" @change="load" /></label>
      <label class="field" style="min-width: 260px">Operador
        <select v-model="userId" @change="load">
          <option :value="null" disabled>Escolha o operador</option>
          <option :value="0">Todos os operadores</option>
          <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }} ({{ u.login }})</option>
        </select>
      </label>
    </div>
    <div v-if="error" class="alert error" role="alert">{{ error }}</div>
    <div v-if="busy && !preview" class="state">Carregando…</div>
    <template v-if="preview">
      <div v-if="preview.blockers.length" class="alert error" role="alert">
        <strong>Não dá para apagar:</strong>
        <ul style="margin: 6px 0 0; padding-left: 18px"><li v-for="b in preview.blockers" :key="b">{{ b }}</li></ul>
      </div>
      <div v-if="!preview.sales.length && !preview.sessions.length" class="state">Nada de {{ userName }} em {{ brDay }}.</div>
      <template v-else>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Venda</th><th>Hora</th><th class="left">Cliente</th><th>Forma</th><th>Situação</th><th class="right">Total</th></tr></thead>
            <tbody>
              <tr v-for="s in preview.sales" :key="s.id" :class="{ inactive: s.status === 'cancelada' }">
                <td class="mono">#{{ s.id }}</td>
                <td class="nowrap">{{ formatDateTime(s.at) }}</td>
                <td class="left">{{ s.customerName || '—' }}</td>
                <td>{{ paymentLabels[s.paymentMethod] ?? s.paymentMethod }}</td>
                <td>{{ s.status === 'cancelada' ? 'Cancelada' : 'Concluída' }}</td>
                <td class="right mono">{{ money(s.totalCents) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p style="margin: 0">
          <strong>{{ preview.sales.length }}</strong> venda(s), total concluído <strong class="mono">{{ money(preview.totalCents) }}</strong>.
          <template v-if="preview.sessions.length">
            Caixa(s) apagados inteiros: <span v-for="c in preview.sessions" :key="c.id">#{{ c.id }} ({{ c.status }}, aberto {{ formatDateTime(c.openedAt) }}, {{ c.movements }} lançamento(s)) </span>
          </template>
          <template v-if="preview.touchedSessions.length">
            Caixa(s) que ficam, mas perdem os lançamentos dessas vendas: {{ preview.touchedSessions.map((id) => '#' + id).join(', ') }}.
          </template>
        </p>
        <footer class="row end">
          <button class="danger" :disabled="busy || !canApply" @click="confirmOpen = true">{{ busy ? 'Apagando…' : 'Apagar vendas de teste' }}</button>
        </footer>
      </template>
    </template>
  </section>

  <ConfirmDialog v-if="confirmOpen && preview" title="Apagar vendas de teste" danger
    :message="`Apagar de vez ${preview.sales.length} venda(s) e ${preview.sessions.length} caixa(s) de ${userName} em ${brDay}? Isso não tem volta pela tela (só restaurando o backup feito agora).`"
    confirm-label="Apagar" cancel-label="Voltar" @confirm="apply" @cancel="confirmOpen = false" />
</template>
