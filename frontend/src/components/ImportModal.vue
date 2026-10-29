<!-- Caminho: pdv/frontend/src/components/ImportModal.vue -->
<script setup lang="ts">
// Importação de planilha (produtos ou clientes): escolher arquivo → ligar as
// colunas aos campos → conferir linha a linha → importar. Toda a validação e a
// gravação (numa transação só) são do backend; aqui só monta o pedido.
import { computed, onMounted, ref } from 'vue'
import { ConfirmImport, ListSuppliers, PickImportFile, PreviewImport, SaveImportTemplate } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { models, type Supplier } from '../types'
import Modal from './Modal.vue'

const props = defineProps<{ kind: 'produtos' | 'clientes' }>()
const emit = defineEmits<{ close: []; done: [] }>()
const toast = useToast()

const file = ref<models.ImportFile | null>(null)
// Coluna escolhida para cada campo (-1 = não importar).
const mapping = ref<Record<string, number>>({})
const update = ref(true)
const preview = ref<models.ImportPreview | null>(null)
const busy = ref(false)
const error = ref('')
// Fornecedor da entrada criada com o estoque dos produtos novos (0 = "Sem fornecedor").
const supplierId = ref(0)
const suppliers = ref<Supplier[]>([])
onMounted(async () => {
  if (props.kind !== 'produtos' || !can('compras')) return
  try {
    suppliers.value = (await call(ListSuppliers(''))).filter((s) => s.active)
  } catch {
    // sem a lista, a entrada fica em "Sem fornecedor"
  }
})
const what = computed(() => (props.kind === 'produtos' ? 'produtos' : 'clientes'))
const actionLabels: Record<string, string> = { novo: 'Novo', atualizar: 'Atualizar', ignorar: 'Ignorado', erro: 'Erro' }
const actionClass: Record<string, string> = { novo: 'ok', atualizar: 'warn', ignorar: '', erro: 'bad' }
const colName = (i: number) => `${String.fromCharCode(65 + (i % 26))}${i >= 26 ? Math.floor(i / 26) : ''}`

async function run<T>(fn: () => Promise<T>): Promise<T | undefined> {
  busy.value = true
  error.value = ''
  try {
    return await fn()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

async function pick() {
  const f = await run(() => call(PickImportFile(props.kind)))
  if (!f) return
  file.value = f
  preview.value = null
  mapping.value = Object.fromEntries(f.fields.map((fl) => [fl.key, f.mapping[fl.key] ?? -1]))
}

async function template() {
  const path = await run(() => call(SaveImportTemplate(props.kind)))
  if (path) toast.success(`Modelo salvo em ${path}.`)
}

const request = () => models.ImportRequest.createFrom({
  kind: props.kind, path: file.value!.path, update: update.value, supplierId: supplierId.value,
  mapping: Object.fromEntries(Object.entries(mapping.value).filter(([, c]) => c >= 0)),
})

async function check() {
  const p = await run(() => call(PreviewImport(request())))
  if (p) preview.value = p
}

async function confirm() {
  const p = await run(() => call(ConfirmImport(request())))
  if (!p) return
  toast.success(`Importação concluída: ${p.new} novo(s), ${p.updated} atualizado(s)${p.skipped ? `, ${p.skipped} ignorado(s)` : ''}.`)
  emit('done')
  emit('close')
}
</script>

<template>
  <Modal :title="`Importar ${what} de planilha`" wide :busy="busy" @close="emit('close')">
    <div class="stack">
      <div class="row">
        <button class="primary" :disabled="busy" @click="pick">{{ file ? 'Trocar arquivo' : 'Escolher arquivo (.xlsx ou .csv)' }}</button>
        <button type="button" :disabled="busy" @click="template">Baixar modelo de planilha</button>
      </div>
      <p v-if="!file" class="small muted" style="margin: 0">
        A primeira linha precisa ser o cabeçalho (nome de cada coluna). CSV com ; ou , e acentos de qualquer Excel funcionam.
        <template v-if="kind === 'produtos'"> O modelo já vem com um SKU reservado em cada linha: preencha nome e preço; linhas que ficarem só com o código são ignoradas.</template>
      </p>

      <template v-if="file">
        <div class="small"><strong>{{ file.name }}</strong> · {{ file.rows }} linha(s)</div>
        <h3 style="margin: 4px 0 0">Qual coluna é cada campo?</h3>
        <div class="grid grid-2">
          <label v-for="f in file.fields" :key="f.key" class="field">
            {{ f.label }} <span class="small" :class="f.required ? '' : 'muted'">{{ f.required ? '(obrigatório)' : '(opcional)' }}</span>
            <select v-model.number="mapping[f.key]" @change="preview = null">
              <option :value="-1">— não importar —</option>
              <option v-for="(h, i) in file.headers" :key="i" :value="i">{{ colName(i) }}: {{ h || '(sem nome)' }}</option>
            </select>
          </label>
        </div>
        <label class="check"><input v-model="update" type="checkbox" @change="preview = null" />
          Atualizar os já cadastrados ({{ kind === 'produtos' ? 'mesmo código de barras ou SKU' : 'mesmo CPF/CNPJ' }})</label>
        <p class="small muted" style="margin: 0">Campos em branco na planilha não apagam o que já está cadastrado.</p>
        <template v-if="kind === 'produtos'">
          <label v-if="suppliers.length" class="field" style="max-width: 360px">Fornecedor da entrada (opcional)
            <select v-model.number="supplierId">
              <option :value="0">Sem fornecedor</option>
              <option v-for="s in suppliers" :key="s.id" :value="s.id">{{ s.tradeName || s.legalName }}</option>
            </select>
          </label>
          <p class="small muted" style="margin: 0">O estoque dos produtos novos entra como uma entrada de mercadoria (aparece em Entrada de mercadoria e pode ser estornada).</p>
        </template>

        <div v-if="preview" class="stack">
          <div class="row" style="gap: 8px">
            <span class="badge ok">{{ preview.new }} novo(s)</span>
            <span class="badge warn">{{ preview.updated }} a atualizar</span>
            <span v-if="preview.skipped" class="badge">{{ preview.skipped }} ignorado(s)</span>
            <span v-if="preview.errors" class="badge bad">{{ preview.errors }} com erro</span>
          </div>
          <div class="table-wrap" style="max-height: 320px; overflow: auto">
            <table>
              <thead><tr><th>Linha</th><th class="left">Nome</th><th>Situação</th><th class="left">Observação</th></tr></thead>
              <tbody>
                <tr v-for="r in preview.rows" :key="r.line">
                  <td class="mono">{{ r.line }}</td>
                  <td class="left">{{ r.name || '—' }}</td>
                  <td><span class="badge" :class="actionClass[r.action]">{{ actionLabels[r.action] }}</span></td>
                  <td class="left small">{{ r.message }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="preview.errors" class="alert warn" style="margin: 0">
            Corrija as linhas com erro na planilha (ou apague-as) e confira de novo. Com erro, nada é gravado.
          </div>
        </div>
      </template>
      <div v-if="error" class="alert error" role="alert">{{ error }}</div>
    </div>
    <template #footer>
      <button :disabled="busy" @click="emit('close')">Cancelar</button>
      <span class="spacer" />
      <button v-if="file" :disabled="busy" @click="check">Conferir planilha</button>
      <button v-if="file" class="success" :disabled="busy || !preview || preview.errors > 0 || preview.new + preview.updated === 0" @click="confirm">
        {{ busy ? 'Aguarde…' : 'Importar' }}
      </button>
    </template>
  </Modal>
</template>
