<!-- Caminho: pdv/frontend/src/components/BackgroundSettings.vue -->
<script setup lang="ts">
// Aparência: imagem de fundo da tela de login e do sistema. Salva na hora.
import { ref } from 'vue'
import { SaveBackgrounds } from '../../wailsjs/go/main/App'
import { models } from '../../wailsjs/go/models'
import { call, errorMessage } from '../api'
import { backgrounds, shrinkImage } from '../composables/background'
import { useToast } from '../composables/toast'

const toast = useToast()
const busy = ref(false)
const slots = [
  { key: 'login', label: 'Tela de login' },
  { key: 'app', label: 'Sistema (atrás das telas)' },
] as const

async function save(key: 'login' | 'app', value: string) {
  busy.value = true
  try {
    backgrounds.value = await call(SaveBackgrounds(models.Backgrounds.createFrom({ ...backgrounds.value, [key]: value })))
    toast.success(value ? 'Imagem de fundo salva.' : 'Imagem de fundo removida.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}

async function pick(key: 'login' | 'app', ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    await save(key, await shrinkImage(file))
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <section class="card stack">
    <h2 style="margin: 0">Aparência</h2>
    <p class="small muted" style="margin: 0">Imagem de fundo (JPEG ou PNG). O sistema reduz o tamanho sozinho; ela entra no backup.</p>
    <div class="grid grid-2">
      <div v-for="s in slots" :key="s.key" class="stack">
        <strong>{{ s.label }}</strong>
        <img v-if="backgrounds[s.key]" :src="backgrounds[s.key]" :alt="`Fundo: ${s.label}`" style="width: 100%; max-width: 320px; aspect-ratio: 16 / 9; object-fit: cover; border-radius: 6px" />
        <span v-else class="small muted">Sem imagem (fundo padrão).</span>
        <div class="row">
          <label class="btn" :class="{ disabled: busy }">
            {{ backgrounds[s.key] ? 'Trocar imagem' : 'Escolher imagem' }}
            <input type="file" accept="image/png,image/jpeg" hidden :disabled="busy" @change="pick(s.key, $event)" />
          </label>
          <button v-if="backgrounds[s.key]" type="button" class="link" :disabled="busy" @click="save(s.key, '')">Remover</button>
        </div>
      </div>
    </div>
  </section>
</template>
