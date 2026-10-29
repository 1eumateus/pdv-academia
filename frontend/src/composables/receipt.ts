// Caminho: pdv/frontend/src/composables/receipt.ts

import { computed } from 'vue'
import { PrintReceipt } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { settings } from './cashState'
import { useToast } from './toast'

export type ReceiptKind = 'venda' | 'recebimento' | 'caixa' | 'movimento' | 'teste' | 'orcamento' | 'relatorio' | 'devolucao'

// directPrint: impressora térmica escolhida em Configurações. Sem ela (ou em
// A4), o comprovante usa o diálogo de impressão do Windows.
export const directPrint = computed(() => !!settings.value && settings.value.paperWidth !== 'a4' && !!settings.value.printerName)
export const paperWidth = computed(() => settings.value?.paperWidth ?? '80')

// printReceipt imprime o comprovante; a operação já está gravada, então uma
// falha aqui só avisa e permite reimprimir.
export async function printReceipt(kind: ReceiptKind, id: number): Promise<boolean> {
  if (!directPrint.value) {
    window.print()
    return true
  }
  const toast = useToast()
  try {
    await call(PrintReceipt(kind, id))
    toast.success('Comprovante enviado para a impressora.')
    return true
  } catch (e) {
    toast.error(errorMessage(e))
    return false
  }
}
