// Caminho: pdv/frontend/src/composables/cashState.ts

import { ref } from 'vue'
import { GetCurrentCash, GetLicenseStatus, GetSettings } from '../../wailsjs/go/main/App'
import { call } from '../api'
import type { models, Settings } from '../types'

// Estado compartilhado só para exibição (barra superior). As regras continuam
// no backend: uma venda sem caixa aberto é recusada lá.
export const cashOpen = ref<boolean | null>(null)
export const storeName = ref('')
export const autoReceipt = ref(false)
export const settings = ref<Settings | null>(null)
// Licença: o backend recusa gravações sem licença; aqui é só para avisar.
export const licenseStatus = ref<models.LicenseStatus | null>(null)

export async function refreshLicense() {
  try {
    licenseStatus.value = await call(GetLicenseStatus())
  } catch {
    licenseStatus.value = null
  }
}

export async function refreshCash() {
  try {
    cashOpen.value = (await call(GetCurrentCash())) != null
  } catch {
    cashOpen.value = null
  }
}

export async function refreshSettings() {
  try {
    const s = await call(GetSettings())
    settings.value = s
    storeName.value = s.storeName
    autoReceipt.value = s.autoReceipt
  } catch {
    storeName.value = ''
  }
}
