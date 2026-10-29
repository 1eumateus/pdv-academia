<!-- Caminho: pdv/frontend/src/components/PaymentPanel.vue -->
<script setup lang="ts">
// Escolha das formas de pagamento. Um clique = forma única com o total;
// "Dividir pagamento" abre uma linha por forma, cada uma com seu valor.
import { computed, watch } from 'vue'
import CardBrandSelect from './CardBrandSelect.vue'
import { centsToInput, money, parseMoney, paymentLabels } from '../composables/format'
import { evaluate, newPart, payMethods, type PayPart } from '../composables/payments'

const parts = defineModel<PayPart[]>('parts', { required: true })
const split = defineModel<boolean>('split', { required: true })
// storeCredit: vale-troca do cliente escolhido; com saldo, aparece a forma Vale-troca.
// exclude: formas que não valem aqui (ex.: fatura de assinatura não vai para o a prazo).
const props = defineProps<{ total: number; busy?: boolean; storeCredit?: number; exclude?: string[] }>()
const methods = computed<string[]>(() =>
  (props.storeCredit ? [...payMethods, 'vale_troca'] : [...payMethods]).filter((m) => !props.exclude?.includes(m)))

const result = computed(() => evaluate(parts.value, props.total, split.value))
const single = computed(() => parts.value[0])
// Quanto falta: no dividido, o que as formas ainda não cobrem; numa forma só em
// dinheiro, quando o valor recebido é menor que o total.
const missing = computed(() => {
  if (split.value) return Math.max(0, result.value.remaining)
  const p = single.value
  if (p?.method !== 'dinheiro' || !p.received.trim()) return 0
  const rec = parseMoney(p.received)
  return rec !== null && rec < props.total ? props.total - rec : 0
})

// Dinheiro: "Valor recebido" já vem com o total. Se o total mudar (desconto) e o
// operador não tiver editado o campo, o valor acompanha.
let autoReceived = ''
watch(
  () => [props.total, split.value, single.value] as const,
  () => {
    const p = single.value
    if (split.value || p?.method !== 'dinheiro' || (p.received !== '' && p.received !== autoReceived)) return
    p.received = autoReceived = centsToInput(props.total)
  },
  { immediate: true },
)

function choose(method: string) {
  if (split.value) return
  parts.value = [newPart(method)]
}

function toggleSplit() {
  split.value = !split.value
  if (split.value) {
    // Começa com a forma escolhida valendo o total; as próximas vêm com o que falta.
    parts.value = [{ ...parts.value[0], amount: centsToInput(props.total) }]
  } else {
    parts.value = [newPart(parts.value[0]?.method ?? 'dinheiro')]
  }
}

// Dividido: clicar numa forma inclui com o que falta; clicar de novo tira (fica ao menos uma).
const used = (m: string) => parts.value.some((p) => p.method === m)
function toggleMethod(m: string) {
  if (used(m)) {
    if (parts.value.length > 1) parts.value = parts.value.filter((p) => p.method !== m)
    return
  }
  const rest = Math.max(0, result.value.remaining)
  parts.value.push(newPart(m, rest ? centsToInput(rest) : ''))
}
</script>

<template>
  <div class="stack" style="gap: 8px">
    <!-- Forma única -->
    <template v-if="!split">
      <div class="pay-options" role="group" aria-label="Forma de pagamento">
        <button v-for="m in methods" :key="m" type="button" :aria-pressed="single?.method === m" :disabled="busy" @click="choose(m)">
          {{ paymentLabels[m] }}
        </button>
      </div>
      <button type="button" class="split-toggle" :disabled="busy" title="Pagar parte numa forma e parte em outra (ex.: dinheiro + Pix)" @click="toggleSplit">＋ Dividir pagamento</button>
      <label v-if="single?.method === 'dinheiro'" class="field">Valor recebido (para troco)
        <input v-model="single.received" class="big mono" inputmode="decimal" placeholder="0,00" :disabled="busy" />
      </label>
      <div v-else-if="single?.method === 'debito' || single?.method === 'credito'" class="row" style="gap: 6px; flex-wrap: nowrap">
        <label class="field" style="flex: 1">Bandeira<CardBrandSelect v-model="single.brand" :disabled="busy" /></label>
        <label v-if="single.method === 'credito'" class="field" style="width: 90px">Parcelas<input v-model="single.installments" inputmode="numeric" :disabled="busy" /></label>
      </div>
    </template>

    <!-- Dividido -->
    <template v-else>
      <div class="split-head">
        <strong class="small">Pagamento em várias formas</strong>
        <button type="button" class="split-toggle" :disabled="busy" title="Voltar a pagar tudo numa forma só" @click="toggleSplit">✕ Uma forma só</button>
      </div>
      <!-- Uma linha por forma: forma · valor · remover. Extras pequenos só quando precisa. -->
      <div class="pay-options" role="group" aria-label="Formas usadas no pagamento">
        <button v-for="m in methods" :key="m" type="button" :aria-pressed="used(m)" :disabled="busy"
          :title="used(m) ? 'Tirar esta forma' : 'Incluir esta forma com o valor que falta'" @click="toggleMethod(m)">
          {{ paymentLabels[m] }}
        </button>
      </div>
      <!-- Uma linha por forma: nome · recebido/bandeira · valor · tirar. -->
      <div v-for="(p, i) in parts" :key="p.method" class="part">
        <span class="part-name">{{ paymentLabels[p.method] }}</span>
        <div class="part-extra">
          <input v-if="p.method === 'dinheiro'" v-model="p.received" class="mono" inputmode="decimal" placeholder="recebido (troco)" :disabled="busy" aria-label="Valor recebido em dinheiro" />
          <template v-else-if="p.method === 'debito' || p.method === 'credito'">
            <CardBrandSelect v-model="p.brand" :disabled="busy" />
            <input v-if="p.method === 'credito'" v-model="p.installments" class="parcelas" inputmode="numeric" title="Parcelas" aria-label="Parcelas" />
          </template>
        </div>
        <input v-model="p.amount" class="mono amount" inputmode="decimal" placeholder="0,00" :disabled="busy" :aria-label="`Valor em ${paymentLabels[p.method]}`" />
        <button v-if="parts.length > 1" type="button" class="remove" :disabled="busy" :title="`Tirar ${paymentLabels[p.method]}`" :aria-label="`Tirar ${paymentLabels[p.method]}`" @click="parts.splice(i, 1)">✕</button>
        <span v-else />
      </div>
    </template>

    <p v-if="storeCredit && parts.some((p) => p.method === 'vale_troca')" class="small muted" style="margin: 0">Vale-troca disponível: <strong class="mono">{{ money(storeCredit) }}</strong></p>
    <!-- Situação do pagamento em destaque: falta, passou, fechado ou troco. -->
    <div v-if="missing > 0" class="status missing"><span>Faltam</span><strong class="mono">{{ money(missing) }}</strong></div>
    <div v-else-if="split && result.remaining < 0" class="status over"><span>Passou do total</span><strong class="mono">{{ money(-result.remaining) }}</strong></div>
    <div v-else-if="result.change > 0" class="status change"><span>Troco</span><strong class="mono">{{ money(result.change) }}</strong></div>
    <div v-else-if="split && total > 0" class="status ok"><span>✓ Total fechado</span><strong class="mono">{{ money(total) }}</strong></div>
    <p v-if="split && result.error && result.remaining === 0" class="small" style="color: var(--danger); margin: 0">{{ result.error }}</p>
  </div>
</template>

<style scoped>
/* Troco em destaque, do tamanho do total. */
/* Faixa de situação, grande e colorida: falta (laranja), passou (vermelho), troco/fechado (verde). */
.status { display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; border-radius: var(--radius);
  font-size: 16px; font-weight: 700; border: 2px solid; }
.status strong { font-size: 26px; }
.status.missing { background: var(--warning-soft); border-color: var(--warning); color: var(--warning-text); }
.status.over { background: var(--danger-soft); border-color: var(--danger); color: var(--danger); }
.status.change, .status.ok { background: var(--success-soft); border-color: var(--success); color: var(--success); }
/* Pagamento em várias formas: linhas simples, sem caixas. */
.split-head { display: flex; justify-content: space-between; align-items: center; }
.part { display: grid; grid-template-columns: 74px minmax(0, 1fr) 150px 30px; gap: 8px; align-items: center; padding-bottom: 6px; border-bottom: 1px dashed var(--border); }
.part-name { font-weight: 600; }
.part .amount { font-size: 20px; text-align: right; padding: 10px 12px; }
.remove { min-height: 0; height: 30px; padding: 0; border: 0; background: transparent; color: var(--muted); font-size: 15px; }
.remove:hover:not(:disabled) { color: var(--danger); background: var(--danger-soft); }
.part-extra { display: flex; align-items: center; gap: 6px; min-width: 0; }
.part-extra input, .part-extra :deep(select) { padding: 8px 10px; font-size: 14px; flex: 1; min-width: 0; }
.part-extra .parcelas { flex: none; width: 46px; text-align: center; }
.split-foot { display: flex; justify-content: space-between; align-items: center; }
/* Dividir pagamento: botãozinho em pílula, discreto, à direita. */
.split-toggle { align-self: flex-end; min-height: 0; padding: 6px 16px; font-size: 15px; font-weight: 600; border-radius: 999px;
  border: 1px dashed var(--primary); background: transparent; color: var(--primary); }
.split-toggle:hover:not(:disabled) { background: var(--primary-soft); }
</style>
