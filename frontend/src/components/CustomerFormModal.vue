<!-- Caminho: catraca/frontend/src/components/CustomerFormModal.vue -->
<script setup lang="ts">
// Cadastro e edição do cliente (Clientes e ficha do aluno). customer null =
// cliente novo. O slot "top" põe conteúdo acima do formulário (foto, plano…).
import { reactive } from 'vue'
import { CreateCustomer, UpdateCustomer } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { centsToInput, formatDocument, formatZip, parseMoney } from '../composables/format'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { models, type Customer } from '../types'
import Modal from './Modal.vue'

const props = defineProps<{ customer: Customer | null; title?: string }>()
const emit = defineEmits<{ close: []; saved: [c: Customer] }>()
const toast = useToast()

// Só o nome é obrigatório; o backend valida e normaliza o restante.
const optionalFields = ['phone', 'document', 'email', 'zipCode', 'street', 'number', 'complement', 'district', 'city', 'state', 'reference', 'notes'] as const
type OptionalField = (typeof optionalFields)[number]
const UFS = ['AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA', 'MT', 'MS', 'MG', 'PA', 'PB', 'PR', 'PE', 'PI', 'RJ', 'RN', 'RS', 'RO', 'RR', 'SC', 'SP', 'SE', 'TO']

const c = props.customer
const opt = Object.fromEntries(optionalFields.map((k) => [k, (c?.[k] as string | undefined) ?? ''])) as Record<OptionalField, string>
if (c) {
  opt.document = formatDocument(c.document)
  opt.zipCode = formatZip(c.zipCode)
}
const form = reactive({
  id: c?.id ?? 0, name: c?.name ?? '', ...opt,
  limit: centsToInput(c?.creditLimitCents ?? 0), active: c?.active ?? true, birthDate: c?.birthDate ?? '', gender: c?.gender ?? '', busy: false, error: '',
})

async function save() {
  const limit = parseMoney(form.limit)
  if (!form.name.trim()) return (form.error = 'Informe o nome do cliente.')
  if (limit === null) return (form.error = 'Limite de crédito inválido. Exemplo: 200,00')
  form.busy = true
  form.error = ''
  try {
    const o = Object.fromEntries(optionalFields.map((k) => [k, form[k].trim() || null]))
    const input = models.CustomerInput.createFrom({ id: form.id, name: form.name, ...o, creditLimitCents: limit, active: form.active, birthDate: form.birthDate, gender: form.gender })
    const saved = await call(form.id ? UpdateCustomer(input) : CreateCustomer(input))
    toast.success(form.id ? `Cliente "${saved.name}" atualizado.` : `Cliente "${saved.name}" cadastrado.`)
    emit('saved', saved)
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}
</script>

<template>
  <Modal :title="title ?? (form.id ? 'Editar cliente' : 'Novo cliente')" wide :busy="form.busy" @close="emit('close')">
    <form class="stack" @submit.prevent="save">
      <slot name="top" />
      <p class="small muted" style="margin: 0">Somente o nome é obrigatório.</p>
      <div class="grid grid-2">
        <label class="field">Nome<input v-model="form.name" maxlength="200" required /></label>
        <label class="field">CPF ou CNPJ<input v-model="form.document" maxlength="20" placeholder="000.000.000-00" /></label>
        <label class="field">Telefone<input v-model="form.phone" maxlength="30" inputmode="tel" /></label>
        <label class="field">E-mail<input v-model="form.email" maxlength="254" type="email" /></label>
        <label class="field">Data de nascimento (opcional)<input v-model="form.birthDate" type="date" /></label>
        <label class="field">Gênero (opcional)
          <select v-model="form.gender">
            <option value="">Não informado</option>
            <option value="feminino">Feminino</option>
            <option value="masculino">Masculino</option>
            <option value="outro">Outro</option>
          </select>
        </label>
      </div>
      <fieldset class="stack" style="border: 1px solid var(--border); border-radius: 8px; padding: 12px">
        <legend class="small muted" style="padding: 0 6px">Endereço (para entregas)</legend>
        <div class="grid" style="grid-template-columns: 140px 1fr 110px">
          <label class="field">CEP<input v-model="form.zipCode" maxlength="10" inputmode="numeric" placeholder="00000-000" /></label>
          <label class="field">Rua<input v-model="form.street" maxlength="200" /></label>
          <label class="field">Número<input v-model="form.number" maxlength="20" /></label>
        </div>
        <div class="grid" style="grid-template-columns: 1fr 1fr 1fr 90px">
          <label class="field">Complemento<input v-model="form.complement" maxlength="200" /></label>
          <label class="field">Bairro<input v-model="form.district" maxlength="200" /></label>
          <label class="field">Cidade<input v-model="form.city" maxlength="200" /></label>
          <label class="field">
            UF
            <select v-model="form.state">
              <option value="">—</option>
              <option v-for="uf in UFS" :key="uf" :value="uf">{{ uf }}</option>
            </select>
          </label>
        </div>
        <label class="field">Ponto de referência<input v-model="form.reference" maxlength="200" /></label>
      </fieldset>
      <label class="field">Observações<textarea v-model="form.notes" maxlength="1000" /></label>
      <div class="grid grid-2">
        <label v-if="can('alterar_limite_fiado')" class="field">Limite para compras a prazo (R$)<input v-model="form.limit" inputmode="decimal" /></label>
        <label class="check" style="align-self: end"><input v-model="form.active" type="checkbox" /> Cliente ativo</label>
      </div>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="emit('close')">Cancelar</button>
        <button type="submit" class="primary" :disabled="form.busy">{{ form.busy ? 'Salvando…' : 'Salvar' }}</button>
      </footer>
    </form>
  </Modal>
</template>
