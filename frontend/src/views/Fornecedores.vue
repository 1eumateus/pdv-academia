<!-- Caminho: pdv/frontend/src/views/Fornecedores.vue -->
<script setup lang="ts">
import Icon from '../components/Icon.vue'
import { onMounted, reactive, ref, watch } from 'vue'
import { ListSuppliers, SaveSupplier } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import Modal from '../components/Modal.vue'
import { formatAddress } from '../composables/format'
import { useToast } from '../composables/toast'
import { models, type Supplier } from '../types'
import LoadMore from '../components/LoadMore.vue'
import { usePaged } from '../composables/paged'

const toast = useToast()
const suppliers = ref<Supplier[]>([])
const term = ref('')
const loading = ref(true)
const error = ref('')

const empty = (): Supplier => models.Supplier.createFrom({ id: 0, legalName: '', tradeName: '', document: '', stateRegistration: '',
  contact: '', phone: '', email: '', zipCode: '', street: '', number: '', complement: '', district: '', city: '', state: '', notes: '', active: true })
const form = reactive({ open: false, data: empty(), busy: false, error: '' })

async function load() {
  loading.value = true
  error.value = ''
  try {
    suppliers.value = await call(ListSuppliers(term.value))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
let timer: number | undefined
watch(term, () => {
  window.clearTimeout(timer)
  timer = window.setTimeout(load, 250)
})

function edit(s?: Supplier) {
  Object.assign(form, { open: true, data: models.Supplier.createFrom(s ?? empty()), busy: false, error: '' })
}

async function save() {
  form.busy = true
  form.error = ''
  try {
    const saved = await call(SaveSupplier(form.data))
    toast.success(form.data.id ? `Fornecedor "${saved.legalName}" alterado.` : `Fornecedor "${saved.legalName}" cadastrado.`)
    form.open = false
    load()
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}

function formatDoc(d: string): string {
  if (d.length === 14) return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5, 8)}/${d.slice(8, 12)}-${d.slice(12)}`
  if (d.length === 11) return `${d.slice(0, 3)}.${d.slice(3, 6)}.${d.slice(6, 9)}-${d.slice(9)}`
  return d || '—'
}
// Lista longa aos poucos (50 por vez; volta ao começo quando muda o filtro).
const pgFornecedores = usePaged(() => suppliers.value, [term])
</script>

<template>
  <section class="card stack">
    <div class="row">
      <input v-model="term" type="search" placeholder="Buscar por razão social, nome fantasia, CNPJ, contato, telefone ou cidade" aria-label="Buscar fornecedores" style="max-width: 460px" />
      <span class="spacer" />
      <button class="primary" @click="edit()">Novo fornecedor</button>
    </div>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-else-if="loading && suppliers.length === 0" class="state">Carregando…</div>
    <div v-else-if="suppliers.length === 0" class="state">{{ term ? 'Nenhum fornecedor encontrado.' : 'Nenhum fornecedor cadastrado.' }}</div>
    <div v-else class="table-wrap">
      <table>
        <thead><tr><th>Fornecedor</th><th>CNPJ/CPF</th><th>Contato</th><th>Cidade</th><th></th></tr></thead>
        <tbody>
          <tr v-for="s in pgFornecedores.visible" :key="s.id" :class="{ inactive: !s.active }">
            <td class="left">
              {{ s.legalName }} <span v-if="!s.active" class="badge">Inativo</span>
              <div v-if="s.tradeName" class="small muted">{{ s.tradeName }}</div>
            </td>
            <td class="mono small nowrap">{{ formatDoc(s.document) }}</td>
            <td class="small">{{ s.contact || '—' }}<div v-if="s.phone" class="muted">{{ s.phone }}</div></td>
            <td class="small">{{ [s.city, s.state].filter(Boolean).join('/') || '—' }}</td>
            <td class="right actions-cell"><button class="link icon-btn" @click="edit(s)" title="Editar" aria-label="Editar"><Icon name="edit" /><span>Editar</span></button></td>
          </tr>
        </tbody>
      </table>
          <LoadMore :paged="pgFornecedores" />
    </div>
  </section>

  <Modal v-if="form.open" :title="form.data.id ? 'Editar fornecedor' : 'Novo fornecedor'" wide :busy="form.busy" @close="form.open = false">
    <form class="stack" @submit.prevent="save">
      <p class="small muted" style="margin: 0">Só a razão social é obrigatória. O CNPJ é usado para reconhecer o fornecedor ao importar o XML da nota.</p>
      <div class="grid grid-2">
        <label class="field">Razão social<input v-model="form.data.legalName" maxlength="200" required /></label>
        <label class="field">Nome fantasia<input v-model="form.data.tradeName" maxlength="200" /></label>
        <label class="field">CNPJ ou CPF<input v-model="form.data.document" maxlength="20" placeholder="00.000.000/0000-00" /></label>
        <label class="field">Inscrição estadual<input v-model="form.data.stateRegistration" maxlength="20" /></label>
        <label class="field">Contato (vendedor/representante)<input v-model="form.data.contact" maxlength="200" /></label>
        <label class="field">Telefone<input v-model="form.data.phone" maxlength="30" inputmode="tel" /></label>
        <label class="field">E-mail<input v-model="form.data.email" maxlength="254" type="email" /></label>
        <label class="field">CEP<input v-model="form.data.zipCode" maxlength="10" inputmode="numeric" /></label>
        <label class="field">Rua<input v-model="form.data.street" maxlength="200" /></label>
        <label class="field">Número<input v-model="form.data.number" maxlength="20" /></label>
        <label class="field">Complemento<input v-model="form.data.complement" maxlength="200" /></label>
        <label class="field">Bairro<input v-model="form.data.district" maxlength="200" /></label>
        <label class="field">Cidade<input v-model="form.data.city" maxlength="200" /></label>
        <label class="field">UF<input v-model="form.data.state" maxlength="2" placeholder="SP" /></label>
      </div>
      <label class="field">Observações<textarea v-model="form.data.notes" maxlength="1000" /></label>
      <label v-if="form.data.id" class="check"><input v-model="form.data.active" type="checkbox" /> Fornecedor ativo</label>
      <p v-if="formatAddress(form.data).length" class="small muted" style="margin: 0">{{ formatAddress(form.data).join(' · ') }}</p>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="form.open = false">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">Salvar</button>
      </footer>
    </form>
  </Modal>
</template>
