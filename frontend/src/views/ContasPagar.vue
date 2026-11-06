<!-- Caminho: pdv/frontend/src/views/ContasPagar.vue -->
<script setup lang="ts">
// Financeiro › Contas a pagar: boletos e despesas (aluguel, energia…), com o
// boleto e o comprovante anexados (ficam no banco e entram no backup). Pagar em
// dinheiro do caixa lança a sangria; conta que repete todo mês gera a próxima.
import { computed, onMounted, reactive, ref } from 'vue'
import {
  AddBillFile, BillCategories, BillSummary, DeleteBill, DeleteBillFile, ListBillFiles, ListBills, OpenBillFile, PayBill, SaveBill, SaveBillFileAs,
} from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import DonutChart from '../components/DonutChart.vue'
import Icon from '../components/Icon.vue'
import Modal from '../components/Modal.vue'
import { centsToInput, money, parseMoney, today } from '../composables/format'
import { useToast } from '../composables/toast'
import { models } from '../types'
import LoadMore from '../components/LoadMore.vue'
import { usePaged } from '../composables/paged'

type Bill = models.Bill
const toast = useToast()
const bills = ref<Bill[]>([])
const categories = ref<string[]>([])
const loading = ref(true)
const error = ref('')
const term = ref('')
const view = ref<'avencer' | 'vencidas' | 'pagas'>('avencer')

const brDate = (d: string) => d.split('-').reverse().join('/')
const methodLabels: Record<string, string> = { pix: 'Pix', dinheiro: 'Dinheiro', transferencia: 'Transferência', boleto: 'Boleto (banco)', debito: 'Débito', credito: 'Crédito' }
const views = [
  { key: 'avencer', label: 'A vencer', test: (b: Bill) => b.status === 'aberta' && b.dueDate >= today() },
  { key: 'vencidas', label: 'Vencidas', test: (b: Bill) => b.status === 'aberta' && b.dueDate < today() },
  { key: 'pagas', label: 'Pagas', test: (b: Bill) => b.status === 'paga' },
] as const
const count = (v: (typeof views)[number]) => bills.value.filter(v.test).length
const filtered = computed(() => {
  const t = term.value.trim().toLowerCase()
  const test = views.find((v) => v.key === view.value)!.test
  return bills.value.filter((b) => test(b) && (!t || `${b.description} ${b.payee} ${b.category}`.toLowerCase().includes(t)))
})
const openTotal = computed(() => filtered.value.filter((b) => b.status === 'aberta').reduce((a, b) => a + b.amountCents, 0))

// Indicadores do topo: vencidas, vencem hoje e o que vence nos próximos 30 dias.
function sumOf(list: Bill[]) {
  return { count: list.length, cents: list.reduce((a, b) => a + b.amountCents, 0) }
}
const in30 = (() => {
  const d = new Date()
  d.setDate(d.getDate() + 30)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
})()
const openBills = computed(() => bills.value.filter((b) => b.status === 'aberta'))
const overdue = computed(() => sumOf(openBills.value.filter((b) => b.dueDate < today())))
const dueToday = computed(() => sumOf(openBills.value.filter((b) => b.dueDate === today())))
const next30 = computed(() => sumOf(openBills.value.filter((b) => b.dueDate >= today() && b.dueDate <= in30)))

// Gráfico por semana: vencidas, esta semana e as próximas 4 (o resto junto em "depois").
const plusDays = (n: number) => {
  const d = new Date()
  d.setDate(d.getDate() + n)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
const weeks = computed(() => {
  const open = openBills.value
  const slot = (label: string, color: string, test: (b: Bill) => boolean) => ({ label, color, ...sumOf(open.filter(test)) })
  const out = [slot('Vencidas', '#d63c31', (b) => b.dueDate < today())]
  for (let w = 0; w < 5; w++) {
    const from = plusDays(w * 7)
    const to = plusDays(w * 7 + 6)
    const br = (d: string) => `${d.slice(8, 10)}/${d.slice(5, 7)}`
    out.push(slot(w === 0 ? 'Próx. 7 dias' : `${br(from)}–${br(to)}`, w === 0 ? '#d98b06' : '#5b4cf0', (b) => b.dueDate >= from && b.dueDate <= to))
  }
  out.push(slot('Depois', '#94a3b8', (b) => b.dueDate > plusDays(34)))
  return out
})
const weeksMax = computed(() => Math.max(1, ...weeks.value.map((w) => w.cents)))

// Resumo do mês: o que foi pago por categoria.
const month = ref(today().slice(0, 7))
const summary = ref<models.BillCategoryTotal[]>([])
async function loadSummary() {
  try {
    summary.value = await call(BillSummary(month.value))
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    bills.value = await call(ListBills())
    loadSummary()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  categories.value = await call(BillCategories()).catch(() => [])
  load()
})

// Lê o arquivo escolhido (PDF ou imagem) em base64 para enviar ao backend.
function readFile(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result).split(',')[1] ?? '')
    r.onerror = () => reject(new Error('Não foi possível ler o arquivo.'))
    r.readAsDataURL(file)
  })
}
async function attach(billId: number, kind: 'boleto' | 'comprovante', file: File) {
  if (file.size > 5 * 1024 * 1024) throw new Error(`O arquivo tem ${(file.size / 1024 / 1024).toFixed(1)} MB; o limite é 5 MB.`)
  return call(AddBillFile(billId, kind, file.name, file.type, await readFile(file)))
}

// ---------- cadastro ----------
const form = reactive({ open: false, id: 0, description: '', category: '', payee: '', amount: '', dueDate: today(), barcode: '', notes: '', recurring: false, file: null as File | null, busy: false, error: '' })
function openForm(b?: Bill) {
  Object.assign(form, {
    open: true, id: b?.id ?? 0, description: b?.description ?? '', category: b?.category ?? categories.value[0] ?? '', payee: b?.payee ?? '',
    amount: b ? centsToInput(b.amountCents) : '', dueDate: b?.dueDate ?? today(), barcode: b?.barcode ?? '', notes: b?.notes ?? '',
    recurring: b?.recurring ?? false, file: null, busy: false, error: '',
  })
}
async function saveForm() {
  const amount = parseMoney(form.amount)
  if (!form.description.trim()) return (form.error = 'Informe a descrição.')
  if (!amount) return (form.error = 'Valor inválido. Exemplo: 1.500,00')
  form.busy = true
  form.error = ''
  try {
    const b = await call(SaveBill(models.BillInput.createFrom({
      id: form.id, description: form.description, category: form.category, payee: form.payee, amountCents: amount,
      dueDate: form.dueDate, barcode: form.barcode, notes: form.notes, recurring: form.recurring,
    })))
    if (form.file) await attach(b.id, 'boleto', form.file)
    toast.success(form.id ? `Conta "${b.description}" alterada.` : `Conta "${b.description}" cadastrada.`)
    form.open = false
    load()
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}

// ---------- pagar ----------
const pay = reactive({ bill: null as Bill | null, date: today(), amount: '', method: 'pix', fromCash: false, file: null as File | null, busy: false, error: '' })
function openPay(b: Bill) {
  Object.assign(pay, { bill: b, date: today(), amount: centsToInput(b.amountCents), method: 'pix', fromCash: false, file: null, busy: false, error: '' })
}
async function savePay() {
  if (!pay.bill) return
  const amount = parseMoney(pay.amount)
  if (!amount) return (pay.error = 'Valor pago inválido.')
  pay.busy = true
  pay.error = ''
  try {
    const b = await call(PayBill(models.PayBillInput.createFrom({
      billId: pay.bill.id, paidDate: pay.date, paidCents: amount, method: pay.method, fromCash: pay.method === 'dinheiro' && pay.fromCash,
    })))
    if (pay.file) await attach(b.id, 'comprovante', pay.file)
    toast.success(`"${b.description}" paga.${b.recurring ? ' A do próximo mês já foi lançada.' : ''}`)
    pay.bill = null
    load()
  } catch (e) {
    pay.error = errorMessage(e)
  } finally {
    pay.busy = false
  }
}

// ---------- arquivos ----------
const files = ref<{ bill: Bill; list: models.BillFile[] } | null>(null)
async function showFiles(b: Bill) {
  try {
    files.value = { bill: b, list: await call(ListBillFiles(b.id)) }
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
async function addFileTo(kind: 'boleto' | 'comprovante', ev: Event) {
  const input = ev.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f || !files.value) return
  try {
    files.value.list = await attach(files.value.bill.id, kind, f)
    toast.success('Arquivo anexado.')
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
async function openFile(id: number) {
  try {
    await call(OpenBillFile(id))
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
async function saveFileAs(id: number) {
  try {
    if (await call(SaveBillFileAs(id))) toast.success('Arquivo salvo.')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
async function removeFile(id: number) {
  if (!files.value) return
  try {
    await call(DeleteBillFile(id))
    files.value.list = files.value.list.filter((f) => f.id !== id)
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
const kb = (n: number) => (n > 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`)

async function copyBarcode(code: string) {
  try {
    await navigator.clipboard.writeText(code)
    toast.success('Código do boleto copiado.')
  } catch {
    toast.error('Não foi possível copiar.')
  }
}

const deleteAsk = ref<Bill | null>(null)
async function remove() {
  if (!deleteAsk.value) return
  try {
    await call(DeleteBill(deleteAsk.value.id))
    toast.success('Conta excluída.')
    deleteAsk.value = null
    load()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgContas = usePaged(() => filtered.value, [term, view, month])
</script>

<template>
  <div v-if="!loading && !error" class="kpi-row">
    <button type="button" class="kpi kpi-btn" @click="view = 'vencidas'">
      <div class="kpi-head">Vencidas<span class="kpi-ic" style="--c: #d63c31"><Icon name="cancelar" :size="18" /></span></div>
      <div class="kpi-value mono" :style="{ color: overdue.count ? 'var(--danger)' : '' }">{{ money(overdue.cents) }}</div>
      <div class="small muted">{{ overdue.count }} conta(s)</div>
    </button>
    <button type="button" class="kpi kpi-btn" @click="view = 'avencer'">
      <div class="kpi-head">Vencem hoje<span class="kpi-ic" style="--c: #d98b06"><Icon name="historico" :size="18" /></span></div>
      <div class="kpi-value mono">{{ money(dueToday.cents) }}</div>
      <div class="small muted">{{ dueToday.count }} conta(s)</div>
    </button>
    <button type="button" class="kpi kpi-btn" @click="view = 'avencer'">
      <div class="kpi-head">Próximos 30 dias<span class="kpi-ic" style="--c: #5b4cf0"><Icon name="t-orcamento" :size="18" /></span></div>
      <div class="kpi-value mono">{{ money(next30.cents) }}</div>
      <div class="small muted">{{ next30.count }} conta(s)</div>
    </button>
    <div class="kpi">
      <div class="kpi-head">Pago no mês<span class="kpi-ic" style="--c: #0e9466"><Icon name="receber" :size="18" /></span></div>
      <div class="kpi-value mono">{{ money(summary.reduce((a, s) => a + s.totalCents, 0)) }}</div>
      <div class="small muted">{{ summary.reduce((a, s) => a + s.count, 0) }} conta(s) paga(s)</div>
    </div>
  </div>
  <section v-if="!loading && !error && openBills.length" class="card" style="margin-bottom: 18px">
    <div class="card-head"><strong>A pagar nas próximas semanas</strong><span class="small muted">contas em aberto, pela data de vencimento</span></div>
    <div class="weeks">
      <div v-for="w in weeks" :key="w.label" class="week" :title="`${w.count} conta(s): ${money(w.cents)}`">
        <span class="w-val mono">{{ w.cents ? money(w.cents) : '—' }}</span>
        <span class="w-bar"><i :style="{ height: `${(w.cents / weeksMax) * 100}%`, background: w.color }" /></span>
        <span class="w-label">{{ w.label }}</span>
        <span class="w-count small muted">{{ w.count }} conta(s)</span>
      </div>
    </div>
  </section>
  <section class="card stack">
    <div class="row">
      <input v-model="term" type="search" placeholder="Buscar conta, favorecido ou categoria" aria-label="Buscar contas" style="max-width: 360px" />
      <span class="spacer" />
      <button class="primary" @click="openForm()">+ Nova conta</button>
    </div>
    <div class="tabs seg" role="tablist" aria-label="Contas">
      <button v-for="v in views" :key="v.key" role="tab" :aria-selected="view === v.key" @click="view = v.key">
        {{ v.label }} <span class="chip-count">{{ count(v) }}</span>
      </button>
    </div>
    <div v-if="loading" class="state">Carregando contas…</div>
    <div v-else-if="error" class="alert error">{{ error }} <button class="link" @click="load">Tentar novamente</button></div>
    <div v-else-if="!filtered.length" class="state">Nenhuma conta aqui.</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>{{ view === 'pagas' ? 'Pago em' : 'Vencimento' }}</th><th>Descrição</th><th>Categoria</th><th>Favorecido</th><th>Valor</th><th>Situação</th><th class="right">Ações</th></tr></thead>
        <tbody>
          <tr v-for="b in pgContas.visible" :key="b.id">
            <td class="nowrap">{{ brDate(b.status === 'paga' && b.paidDate ? b.paidDate : b.dueDate) }}</td>
            <td class="left">
              {{ b.description }} <span v-if="b.recurring" class="small muted" title="Repete todo mês">↻</span>
              <div v-if="b.barcode" class="small muted mono barcode">{{ b.barcode }} <button class="link small" @click="copyBarcode(b.barcode)">copiar</button></div>
            </td>
            <td>{{ b.category }}</td>
            <td>{{ b.payee || '—' }}</td>
            <td class="mono">{{ money(b.status === 'paga' && b.paidCents ? b.paidCents : b.amountCents) }}</td>
            <td>
              <span v-if="b.status === 'paga'" class="badge ok">Paga · {{ methodLabels[b.payMethod ?? ''] ?? b.payMethod }}{{ b.fromCash ? ' (caixa)' : '' }}</span>
              <span v-else-if="b.dueDate < today()" class="badge bad">Vencida</span>
              <span v-else-if="b.dueDate === today()" class="badge warn">Vence hoje</span>
              <span v-else class="badge">A vencer</span>
            </td>
            <td class="right actions-cell">
              <button v-if="b.status === 'aberta'" class="link icon-btn" style="color: var(--success)" title="Registrar o pagamento" aria-label="Pagar" @click="openPay(b)"><Icon name="receber" /><span>Pagar</span></button>
              <button class="link icon-btn" :title="`Boleto e comprovante (${b.boletoFiles + b.receiptFiles} arquivo(s))`" aria-label="Arquivos" @click="showFiles(b)"><Icon name="detalhes" /><span>Arquivos{{ b.boletoFiles + b.receiptFiles ? ` (${b.boletoFiles + b.receiptFiles})` : '' }}</span></button>
              <button v-if="b.status === 'aberta'" class="link icon-btn" title="Editar" aria-label="Editar" @click="openForm(b)"><Icon name="edit" /><span>Editar</span></button>
              <button v-if="b.status === 'aberta'" class="link icon-btn" style="color: var(--danger)" title="Excluir (lançada por engano)" aria-label="Excluir" @click="deleteAsk = b"><Icon name="remover" /><span>Excluir</span></button>
            </td>
          </tr>
        </tbody>
      </table>
          <LoadMore :paged="pgContas" />
    </div>
    <p v-if="view !== 'pagas' && filtered.length" class="small muted" style="margin: 0">Total: <strong class="mono">{{ money(openTotal) }}</strong></p>
  </section>

  <section class="card stack" style="margin-top: 12px">
    <div class="row">
      <h3 style="margin: 0">Pago no mês, por categoria</h3>
      <input v-model="month" type="month" aria-label="Mês" style="max-width: 180px" @change="loadSummary" />
    </div>
    <div v-if="!summary.length" class="state">Nenhuma conta paga neste mês.</div>
    <DonutChart v-if="summary.length" class="no-print" :items="summary.map((s) => ({ name: s.category, value: s.totalCents, text: money(s.totalCents) }))"
      center-label="contas pagas" :center="String(summary.reduce((a, s) => a + s.count, 0))" />
    <table v-if="summary.length">
      <thead><tr><th>Categoria</th><th>Contas</th><th>Total pago</th></tr></thead>
      <tbody>
        <tr v-for="s in summary" :key="s.category"><td class="left">{{ s.category }}</td><td>{{ s.count }}</td><td class="mono">{{ money(s.totalCents) }}</td></tr>
        <tr><td class="left"><strong>Total</strong></td><td>{{ summary.reduce((a, s) => a + s.count, 0) }}</td><td class="mono"><strong>{{ money(summary.reduce((a, s) => a + s.totalCents, 0)) }}</strong></td></tr>
      </tbody>
    </table>
  </section>

  <Modal v-if="form.open" :title="form.id ? 'Editar conta' : 'Nova conta a pagar'" wide :busy="form.busy" @close="form.open = false">
    <form class="stack" @submit.prevent="saveForm">
      <div class="grid grid-2">
        <label class="field">Descrição<input v-model="form.description" maxlength="200" required placeholder="Ex.: Aluguel de novembro" /></label>
        <label class="field">Categoria
          <select v-model="form.category"><option v-for="c in categories" :key="c" :value="c">{{ c }}</option></select>
        </label>
        <label class="field">Favorecido (opcional)<input v-model="form.payee" maxlength="200" placeholder="Ex.: Imobiliária, Equatorial" /></label>
        <label class="field">Valor (R$)<input v-model="form.amount" inputmode="decimal" placeholder="0,00" required /></label>
        <label class="field">Vencimento<input v-model="form.dueDate" type="date" required /></label>
        <label class="check" style="align-self: end"><input v-model="form.recurring" type="checkbox" /> Repete todo mês (ao pagar, lança a do mês seguinte)</label>
      </div>
      <label class="field">Linha digitável / código de barras (opcional)<input v-model="form.barcode" maxlength="100" class="mono" /></label>
      <label class="field">Observação (opcional)<input v-model="form.notes" maxlength="500" /></label>
      <label class="field">Anexar o boleto (PDF ou foto, até 5 MB, opcional)
        <input type="file" accept="application/pdf,image/jpeg,image/png,image/webp" @change="form.file = ($event.target as HTMLInputElement).files?.[0] ?? null" />
      </label>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">{{ form.busy ? 'Salvando…' : 'Salvar' }}</button>
      </footer>
    </form>
  </Modal>

  <Modal v-if="pay.bill" :title="`Pagar · ${pay.bill.description}`" :busy="pay.busy" @close="pay.bill = null">
    <form class="stack" @submit.prevent="savePay">
      <p style="margin: 0">Valor da conta: <strong class="mono">{{ money(pay.bill.amountCents) }}</strong> · vence {{ brDate(pay.bill.dueDate) }}</p>
      <div class="grid grid-2">
        <label class="field">Data do pagamento<input v-model="pay.date" type="date" required /></label>
        <label class="field">Valor pago (com juros ou desconto)<input v-model="pay.amount" inputmode="decimal" required /></label>
      </div>
      <label class="field">Forma de pagamento
        <select v-model="pay.method"><option v-for="(l, k) in methodLabels" :key="k" :value="k">{{ l }}</option></select>
      </label>
      <label v-if="pay.method === 'dinheiro'" class="check"><input v-model="pay.fromCash" type="checkbox" /> Pago com o dinheiro do caixa (lança uma sangria no caixa aberto)</label>
      <label class="field">Comprovante (PDF ou foto, até 5 MB, opcional)
        <input type="file" accept="application/pdf,image/jpeg,image/png,image/webp" @change="pay.file = ($event.target as HTMLInputElement).files?.[0] ?? null" />
      </label>
      <p v-if="pay.bill.recurring" class="small muted" style="margin: 0">Conta mensal: ao pagar, a do mês seguinte é lançada.</p>
      <div v-if="pay.error" class="alert error" role="alert">{{ pay.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="pay.busy" @click="pay.bill = null">Cancelar</button>
        <button type="submit" class="success" :disabled="pay.busy">{{ pay.busy ? 'Registrando…' : 'Confirmar pagamento' }}</button>
      </footer>
    </form>
  </Modal>

  <Modal v-if="files" :title="`Arquivos · ${files.bill.description}`" wide @close="files = null">
    <div class="stack">
      <div v-if="!files.list.length" class="state">Nenhum arquivo anexado.</div>
      <table v-else>
        <thead><tr><th>Tipo</th><th>Arquivo</th><th>Tamanho</th><th class="right">Ações</th></tr></thead>
        <tbody>
          <tr v-for="f in files.list" :key="f.id">
            <td><span class="badge" :class="{ ok: f.kind === 'comprovante' }">{{ f.kind === 'boleto' ? 'Boleto' : 'Comprovante' }}</span></td>
            <td class="left">{{ f.name }}</td>
            <td>{{ kb(f.size) }}</td>
            <td class="right actions-cell">
              <button class="link icon-btn" title="Abrir no programa padrão" aria-label="Abrir" @click="openFile(f.id)"><Icon name="ativar" /><span>Abrir</span></button>
              <button class="link icon-btn" title="Salvar uma cópia" aria-label="Salvar" @click="saveFileAs(f.id)"><Icon name="detalhes" /><span>Salvar</span></button>
              <button class="link icon-btn" style="color: var(--danger)" title="Remover o arquivo" aria-label="Remover" @click="removeFile(f.id)"><Icon name="remover" /><span>Remover</span></button>
            </td>
          </tr>
        </tbody>
      </table>
      <div class="row">
        <label class="field" style="flex: 1">Anexar boleto<input type="file" accept="application/pdf,image/jpeg,image/png,image/webp" @change="addFileTo('boleto', $event)" /></label>
        <label class="field" style="flex: 1">Anexar comprovante<input type="file" accept="application/pdf,image/jpeg,image/png,image/webp" @change="addFileTo('comprovante', $event)" /></label>
      </div>
    </div>
    <template #footer><button @click="files = null">Fechar</button></template>
  </Modal>

  <ConfirmDialog v-if="deleteAsk" title="Excluir conta" danger confirm-label="Excluir"
    :message="`Excluir a conta “${deleteAsk.description}” (${money(deleteAsk.amountCents)})? Use só para conta lançada por engano.`"
    @confirm="remove" @cancel="deleteAsk = null" />
</template>

<style scoped>
.barcode { word-break: break-all; }
/* Colunas por semana: altura proporcional ao valor a pagar. */
.weeks { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 14px; align-items: end; }
.week { display: flex; flex-direction: column; align-items: center; gap: 6px; }
.w-val { font-size: 12.5px; font-weight: 700; white-space: nowrap; }
.w-bar { width: 100%; max-width: 70px; height: 150px; display: flex; align-items: flex-end; border-radius: 10px; background: var(--surface-2); overflow: hidden; }
.w-bar i { display: block; width: 100%; border-radius: 10px 10px 0 0; min-height: 2px; }
.w-label { font-size: 12.5px; font-weight: 600; text-align: center; }
</style>
