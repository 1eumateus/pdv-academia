// Caminho: pdv/frontend/src/composables/background.ts

// Imagens de fundo da tela de login e do sistema (Configurações › Aparência).
// Um véu da cor do fundo por cima da imagem mantém textos soltos legíveis.
import { ref } from 'vue'
import { GetBackgrounds } from '../../wailsjs/go/main/App'
import { models } from '../../wailsjs/go/models'

export const backgrounds = ref(models.Backgrounds.createFrom({ login: '', app: '' }))

export async function refreshBackgrounds() {
  try {
    backgrounds.value = await GetBackgrounds()
  } catch {
    // Sem fundo: fica a cor padrão.
  }
}

export function bgStyle(url: string) {
  if (!url) return undefined
  return {
    backgroundImage: `linear-gradient(color-mix(in srgb, var(--bg) 35%, transparent), color-mix(in srgb, var(--bg) 35%, transparent)), url("${url}")`,
    backgroundSize: 'cover',
    backgroundPosition: 'center',
  }
}

// shrinkImage reduz a foto escolhida para no máximo 1920 px e salva em JPEG,
// para não pesar no banco nem no backup.
export function shrinkImage(file: File, max = 1920): Promise<string> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      const k = Math.min(1, max / Math.max(img.width, img.height))
      const c = document.createElement('canvas')
      c.width = Math.round(img.width * k)
      c.height = Math.round(img.height * k)
      c.getContext('2d')!.drawImage(img, 0, 0, c.width, c.height)
      resolve(c.toDataURL('image/jpeg', 0.82))
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('Não foi possível abrir essa imagem. Use JPEG ou PNG.'))
    }
    img.src = url
  })
}
