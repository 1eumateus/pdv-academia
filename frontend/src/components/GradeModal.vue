<!-- Caminho: pdv/frontend/src/components/GradeModal.vue -->
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ApplyGridPrice, GetProductModel, SaveGrid } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { centsToInput, money, parseMoney } from '../composables/format'
import { can } from '../composables/session'
import { useToast } from '../composables/toast'
import { models, type Category } from '../types'
import Modal from './Modal.vue'

// modelId ausente = grade nova; presente = ampliar a grade existente.
const props = defineProps<{ modelId?: number | null; categories: Category[] }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const toast = useToast()

const model = ref<models.ProductModel | null>(null)
const form = reactive({ name: '', categoryId: null as number | null, price: '', sizes: '', colors: '', newPrice: '', busy: false, error: '' })

// "P, M, G" ou "P M G" viram a lista de valores.
const split = (s: string) => s.split(/[,;]+/).map((x) => x.trim()).filter(Boolean)
const preview = computed(() => {
  const sizes = Math.max(1, split(form.sizes).length + (model.value?.sizes.length ?? 0))
  const colors = Math.max(1, split(form.colors).length + (model.value?.colors.length ?? 0))
  return sizes * colors - (model.value?.variationCount ?? 0)
})

onMounted(async () => {
  if (!props.modelId) return
  try {
    model.value = await call(GetProductModel(props.modelId))
    form.newPrice = centsToInput(model.value.basePriceCents)
  } catch (e) {
    form.error = errorMessage(e)
  }
})

async function save() {
  const price = model.value ? 0 : parseMoney(form.price)
  if (price === null) return (form.error = 'Preço inválido. Exemplo: 49,90')
  form.busy = true
  form.error = ''
  try {
    const m = await call(SaveGrid(models.GridInput.createFrom({
      modelId: model.value?.id ?? 0, name: form.name, categoryId: form.categoryId, basePriceCents: price,
      sizes: split(form.sizes), colors: split(form.colors),
    })))
    toast.success(`Grade "${m.name}" com ${m.variationCount} variações. Cadastre o código de barras e dê entrada no estoque de cada uma.`)
    emit('changed')
    emit('close')
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}

async function applyPrice() {
  const price = parseMoney(form.newPrice)
  if (price === null || !model.value) return (form.error = 'Preço inválido.')
  form.busy = true
  form.error = ''
  try {
    model.value = await call(ApplyGridPrice(model.value.id, price))
    toast.success(`Preço ${money(price)} aplicado às ${model.value.variationCount} variações.`)
    emit('changed')
  } catch (e) {
    form.error = errorMessage(e)
  } finally {
    form.busy = false
  }
}
</script>

<template>
  <Modal :title="model ? `Grade — ${model.name}` : 'Nova grade (tamanho e cor)'" wide :busy="form.busy" @close="emit('close')">
    <form class="stack" @submit.prevent="save">
      <template v-if="!model">
        <p class="small muted" style="margin: 0">
          Cada combinação vira um produto próprio (ex.: "Camiseta básica M Preto"), com código de barras, estoque e
          preço próprios. Todas começam com o preço base e estoque zero.
        </p>
        <div class="grid grid-2">
          <label class="field">Nome do modelo<input v-model="form.name" maxlength="200" placeholder="Camiseta básica" required /></label>
          <label class="field">Preço base (R$)<input v-model="form.price" inputmode="decimal" placeholder="0,00" required /></label>
        </div>
        <label class="field">Categoria (opcional)
          <select v-model="form.categoryId">
            <option :value="null">Sem categoria</option>
            <option v-for="c in categories.filter((c) => c.active)" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
        </label>
      </template>
      <template v-else>
        <div class="small">
          <div><strong>Tamanhos:</strong> {{ model.sizes.join(', ') || '—' }}</div>
          <div><strong>Cores:</strong> {{ model.colors.join(', ') || '—' }}</div>
          <div class="muted">{{ model.variationCount }} variações · preço base {{ money(model.basePriceCents) }}</div>
        </div>
      </template>

      <div class="grid grid-2">
        <label class="field">{{ model ? 'Novos tamanhos' : 'Tamanhos' }} (separe por vírgula)<input v-model="form.sizes" placeholder="P, M, G, GG" /></label>
        <label class="field">{{ model ? 'Novas cores' : 'Cores' }} (separe por vírgula)<input v-model="form.colors" placeholder="Azul, Preto" /></label>
      </div>
      <p class="small muted" style="margin: 0">
        {{ preview > 0 ? `Vai criar até ${preview} variação(ões).` : 'Nenhuma variação nova.' }}
      </p>
      <div v-if="form.error" class="alert error" role="alert">{{ form.error }}</div>
      <footer class="row end">
        <button type="button" :disabled="form.busy" @click="emit('close')">Fechar</button>
        <button type="submit" class="primary" :disabled="form.busy || preview <= 0">{{ model ? 'Adicionar variações' : 'Criar grade' }}</button>
      </footer>
    </form>

    <form v-if="model && can('alterar_preco')" class="row" style="margin-top: 16px; border-top: 1px solid var(--border); padding-top: 12px" @submit.prevent="applyPrice">
      <label class="field" style="flex: 1">Mesmo preço para todas as variações (R$)<input v-model="form.newPrice" inputmode="decimal" /></label>
      <button type="submit" style="align-self: end" :disabled="form.busy">Aplicar a todas</button>
    </form>
  </Modal>
</template>
