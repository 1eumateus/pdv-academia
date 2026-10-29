<!-- Caminho: pdv/frontend/src/components/NFeImportModal.vue -->
<script setup lang="ts">
// Conferência do XML da NF-e, um arquivo por vez. As contas aqui são só prévia:
// ao confirmar, o backend relê o XML e recalcula tudo com as escolhas feitas.
import { computed, onMounted, ref } from 'vue'
import { ConfirmNFe, PreviewNFe, SearchProductByBarcode } from '../../wailsjs/go/main/App'
import { call, errorMessage, isCode } from '../api'
import { centsToInput, money, parseMoney } from '../composables/format'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { models } from '../types'
import Modal from './Modal.vue'

const props = defineProps<{ files: string[] }>()
const emit = defineEmits<{ close: []; imported: [] }>()
const toast = useToast()

interface Row {
  item: models.NFeItem; productId: number | null; productName: string; productCode: string; factor: string; price: string
  currentPrice: number; newName: string; newCode: string; linkedByCode: boolean
  // skip: o item não veio na entrega; fica fora da entrada.
  skip: boolean
}
const index = ref(0)
const preview = ref<models.NFePreview | null>(null)
const rows = ref<Row[]>([])
const error = ref('')
const busy = ref(false)
// Produtos novos sem preço entram "a precificar" (fora da venda até alguém definir o preço).
const priceLater = ref(false)
const fileName = computed(() => props.files[index.value]?.split(/[\\/]/).pop() ?? '')

async function load() {
  preview.value = null
  error.value = ''
  try {
    const pv = await call(PreviewNFe(props.files[index.value]))
    preview.value = pv
    rows.value = pv.items.map((it) => ({
      item: it, productId: it.productId ?? null, productName: it.productName ?? '', productCode: it.productCode ?? '', factor: String(it.factor || 1),
      price: it.productId ? centsToInput(it.priceCents) : '', currentPrice: it.priceCents,
      // Vinculada: campos com o produto do sistema; sem produto: com os dados da nota.
      newName: it.productName ?? it.name, newCode: it.productId ? it.productCode ?? '' : it.ean, linkedByCode: false, skip: false,
    }))
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)

// "1,5" × fator → unidades inteiras (null se não fecha), como o backend faz.
function units(r: Row): number | null {
  const [i, d = ''] = r.item.qtyText.split(',')
  const q4 = Number(i) * 10000 + Number(d.padEnd(4, '0'))
  const f = Number(r.factor)
  if (!Number.isInteger(f) || f < 1) return null
  return (q4 * f) % 10000 === 0 ? (q4 * f) / 10000 : null
}
const unitCost = (r: Row) => {
  const u = units(r)
  return u ? Math.round(r.item.costCents / u) : null
}
// problem diz o que falta numa linha (vazio = pronta). Linha sem produto não é
// problema: o produto é cadastrado ao confirmar.
function problem(r: Row): string {
  if (r.skip) return ''
  if (!r.newName.trim()) return 'Informe o novo nome.'
  if (units(r) === null) return 'Ajuste o fator para dar unidades inteiras.'
  if (!r.productId && parseMoney(r.price) === null && !priceLater.value) return 'Informe o preço de venda do produto novo.'
  return ''
}
const pending = computed(() => rows.value.filter((r) => problem(r)).length)
const received = computed(() => rows.value.filter((r) => !r.skip))

// Resumo mostrado depois do último item: o que será cadastrado e o que falta, por item.
const summary = computed(() => {
  const created = received.value.filter((r) => !r.productId).length
  const later = priceLater.value ? received.value.filter((r) => !r.productId && parseMoney(r.price) === null).length : 0
  const byProblem = new Map<string, number[]>()
  for (const r of rows.value) {
    const p = problem(r)
    if (p) byProblem.set(p, [...(byProblem.get(p) ?? []), r.item.number])
  }
  const skipped = rows.value.filter((r) => r.skip).map((r) => r.item.number)
  return { created, later, skipped, problems: [...byProblem].map(([p, items]) => `${p.replace(/\.$/, '')}: item(ns) ${items.join(', ')}`) }
})

// Os dois campos valem na confirmação: linha sem produto cadastra um novo com
// esse nome/código; linha vinculada renomeia o produto se algo mudar. Digitar o
// código de barras de um produto que já existe passa a usar esse produto.
async function lookupCode(r: Row) {
  const code = r.newCode.trim()
  if (!code || code === r.productCode) return
  try {
    const p = await call(SearchProductByBarcode(code))
    r.productId = p.id
    r.productName = p.name
    r.productCode = p.barcode ?? ''
    r.newName = p.name
    r.currentPrice = p.priceCents
    r.price = centsToInput(p.priceCents)
    r.linkedByCode = true
  } catch (e) {
    if (!isCode(e, 'PRODUTO_INEXISTENTE')) return toast.error(errorMessage(e))
    if (r.linkedByCode) {
      // O código digitado antes era de outro produto; agora volta a ser um cadastro novo.
      Object.assign(r, { productId: null, productName: '', productCode: '', linkedByCode: false })
    }
  }
}

// Preço de venda: editável com a permissão, ou na linha sem produto (é o preço do cadastro novo).
const priceEditable = (r: Row) => can('alterar_preco') || !r.productId

function next() {
  if (index.value + 1 < props.files.length) {
    index.value++
    load()
  } else emit('close')
}

async function confirm() {
  if (!preview.value) return
  busy.value = true
  error.value = ''
  try {
    const decisions = rows.value.map((r) => {
      const price = priceEditable(r) ? parseMoney(r.price) : null
      return {
        number: r.item.number, productId: r.productId ?? 0, factor: Number(r.factor) || 1,
        // Produto novo sempre leva o preço; o existente só se mudou.
        newPriceCents: price !== null && (!r.productId || price !== r.currentPrice) ? price : null,
        name: r.newName, code: r.newCode, skip: r.skip,
      }
    })
    const d = await call(ConfirmNFe(props.files[index.value], decisions.map((x) => models.NFeDecision.createFrom(x)), priceLater.value))
    toast.success(`Nota ${preview.value.number} importada: entrada nº ${d.purchase.id}, ${money(d.purchase.totalCents)}.`)
    emit('imported')
    next()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal :title="`Importar NF-e ${files.length > 1 ? `(${index + 1} de ${files.length})` : ''} — ${fileName}`" full :busy="busy" @close="emit('close')">
    <div v-if="error && !preview" class="alert error">{{ error }}</div>
    <div v-else-if="!preview" class="state">Lendo o XML…</div>
    <template v-else>
      <div class="stack small" style="gap: 2px">
        <div>
          <strong>{{ preview.supplier.legalName }}</strong>
          <span v-if="!preview.supplier.id" class="badge warn">fornecedor novo: será cadastrado</span>
        </div>
        <div>Nota {{ preview.number }} série {{ preview.series }} · emitida em {{ preview.issueDate.split('-').reverse().join('/') }} · total da nota {{ money(preview.invoiceCents) }}</div>
        <div class="mono muted">Chave {{ preview.accessKey }}</div>
      </div>
      <label class="alert warn check price-later">
        <input v-model="priceLater" type="checkbox" />
        <span><strong>Precificar depois os produtos novos sem preço</strong><br />
          <span class="small">Eles entram no estoque com o custo da nota, mas ficam "a precificar" e não vendem até alguém definir o preço.</span></span>
      </label>
      <div v-if="preview.alreadyImported" class="alert warn">Esta nota já foi importada. Não é possível lançar de novo.</div>

      <div class="table-wrap" style="margin-top: 10px">
        <table class="nfe-table">
          <thead>
            <tr>
              <th>Item da nota</th><th class="right">Qtd.</th><th class="right">Fator</th><th class="right">Unid.</th>
              <th class="right">Custo unit.</th><th>Produto do sistema</th><th>Preço de venda</th><th></th>

            </tr>
          </thead>
          <tbody>
            <tr v-for="r in rows" :key="r.item.number" :class="{ 'nfe-skip': r.skip }" :style="{ background: problem(r) ? 'var(--warning-soft)' : '' }">
              <td class="mid">
                {{ r.item.name }}
                <div class="small muted mono">cód. {{ r.item.code || '—' }} · {{ r.item.ean || 'sem GTIN' }}</div>
              </td>
              <td class="right nowrap mid">{{ r.item.qtyText }} {{ r.item.unit }}</td>
              <td class="right mid"><input v-model="r.factor" class="qty-input" inputmode="numeric" :title="`Quantas unidades vêm em cada ${r.item.unit}`" /></td>
              <td class="right mono mid">{{ units(r) ?? '—' }}</td>
              <td class="right mono mid">{{ unitCost(r) !== null ? money(unitCost(r)) : '—' }}</td>
              <td style="min-width: 360px">
                <!-- Só dois campos: valem ao confirmar a entrada. -->
                <div class="stack" style="gap: 2px">
                  <div class="row" style="gap: 6px; flex-wrap: nowrap">
                    <label class="field" style="flex: 2">Nome<input v-model="r.newName" maxlength="200" /></label>
                    <label class="field" style="flex: 1; min-width: 130px">Código<input v-model="r.newCode" maxlength="64" placeholder="opcional" @change="lookupCode(r)" /></label>
                  </div>
                </div>
              </td>
              <td>
                <label v-if="priceEditable(r)" class="field">Preço<input v-model="r.price" style="width: 110px; text-align: right" inputmode="decimal" placeholder="0,00" /></label>
                <div v-else class="field">Preço<span class="mono small" style="padding-top: 10px">{{ money(r.currentPrice) }}</span></div>
              </td>
              <td class="mid nowrap">
                <button class="link icon-btn" :style="{ color: r.skip ? '' : 'var(--danger)' }"
                  :title="r.skip ? 'Voltar o item para a entrada' : 'Remover: o item não veio e fica fora da entrada'"
                  :aria-label="r.skip ? 'Voltar o item para a entrada' : 'Remover item da entrada'" @click="r.skip = !r.skip">
                  <svg v-if="!r.skip" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                    <path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v6M14 11v6" />
                  </svg>
                  <svg v-else viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                    <path d="M9 14 4 9l5-5M4 9h11a5 5 0 0 1 0 10h-3" />
                  </svg>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="small" style="margin-top: 6px">
        <div v-if="summary.created">
          {{ summary.created }} produto(s) será(ão) cadastrado(s) ao confirmar<template v-if="summary.later">,
          {{ summary.later }} como "a precificar" (não vendem até alguém definir o preço)</template>.
        </div>
        <div v-if="summary.skipped.length" style="color: var(--danger)">
          Fora da entrada (não vieram): item(ns) {{ summary.skipped.join(', ') }}. Não entram no estoque nem no custo, e esta nota
          não poderá ser importada de novo.
        </div>
        <div v-for="p in summary.problems" :key="p" style="color: var(--danger)">{{ p }}</div>
      </div>
      <p class="small muted" style="margin: 6px 0 0">
        Fator: quantas unidades vêm em cada unidade da nota (ex.: CX com 12 → 12). O produto e o fator ficam lembrados
        para as próximas notas deste fornecedor. Novo nome e novo código: sem produto, cadastra um novo ao confirmar (com o
        preço de venda informado); vinculado, atualiza o nome/código do produto. Código de um produto existente passa a usar esse produto.
      </p>
      <div v-if="error" class="alert error" role="alert" style="margin-top: 8px">{{ error }}</div>
    </template>
    <template #footer>
      <span v-if="preview && pending" class="small" style="color: var(--warning)">{{ pending }} item(ns) pendente(s)</span>
      <span class="spacer" />
      <button v-if="files.length > 1" :disabled="busy" @click="next">Pular esta nota</button>
      <button @click="emit('close')">Fechar</button>
      <button class="primary" :disabled="busy || !preview || preview.alreadyImported || pending > 0 || !received.length" @click="confirm">
        {{ busy ? 'Importando…' : 'Confirmar entrada' }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
/* Item que não veio: riscado e apagado, campos sem uso. */
.nfe-skip td:not(:last-child) { opacity: 0.45; text-decoration: line-through; pointer-events: none; }
</style>
