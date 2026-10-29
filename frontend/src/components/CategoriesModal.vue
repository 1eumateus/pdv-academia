<!-- Caminho: pdv/frontend/src/components/CategoriesModal.vue -->
<script setup lang="ts">
import Icon from './Icon.vue'
import { ref } from 'vue'
import { SaveCategory } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { models, type Category } from '../types'
import Modal from './Modal.vue'

const props = defineProps<{ categories: Category[] }>()
const emit = defineEmits<{ close: []; changed: [] }>()

const newName = ref('')
const editing = ref<{ id: number; name: string } | null>(null)
const error = ref('')
const busy = ref(false)

async function save(c: Partial<Category> & { name: string }) {
  busy.value = true
  error.value = ''
  try {
    await call(SaveCategory(models.Category.createFrom({ id: 0, active: true, ...c })))
    newName.value = ''
    editing.value = null
    emit('changed')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal title="Categorias" :busy="busy" @close="emit('close')">
    <form class="row" @submit.prevent="newName.trim() && save({ name: newName })">
      <input v-model="newName" maxlength="200" placeholder="Nova categoria (ex.: Bebidas)" aria-label="Nova categoria" style="flex: 1" />
      <button class="primary" type="submit" :disabled="busy || !newName.trim()">Adicionar</button>
    </form>
    <div v-if="error" class="alert error" role="alert">{{ error }}</div>
    <div v-if="props.categories.length === 0" class="state">Nenhuma categoria cadastrada.</div>
    <table v-else>
      <tbody>
        <tr v-for="c in props.categories" :key="c.id" :class="{ inactive: !c.active }">
          <td>
            <form v-if="editing?.id === c.id" class="row" @submit.prevent="save({ ...c, name: editing.name })">
              <input v-model="editing.name" maxlength="200" aria-label="Nome da categoria" style="flex: 1" />
              <button type="submit" class="link" :disabled="busy">Salvar</button>
              <button type="button" class="link" @click="editing = null">Cancelar</button>
            </form>
            <span v-else>{{ c.name }}</span>
          </td>
          <td class="right actions-cell">
            <button class="link icon-btn" v-if="editing?.id !== c.id" @click="editing = { id: c.id, name: c.name }" title="Renomear" aria-label="Renomear"><Icon name="edit" /><span>Renomear</span></button>
            <button class="link icon-btn" :disabled="busy" @click="save({ ...c, active: !c.active })" :title="c.active ? 'Desativar' : 'Ativar'" :aria-label="c.active ? 'Desativar' : 'Ativar'" :style="{ color: c.active ? 'var(--danger)' : 'var(--success)' }"><Icon :name="c.active ? 'desativar' : 'ativar'" /><span>{{ c.active ? 'Desativar' : 'Ativar' }}</span></button>
          </td>
        </tr>
      </tbody>
    </table>
    <p class="small muted">Categorias não são apagadas: desative as que não usar mais. Produtos já vinculados continuam nelas.</p>
    <template #footer><button @click="emit('close')">Fechar</button></template>
  </Modal>
</template>
