// Caminho: pdv/frontend/src/composables/exportTable.ts

// Exporta qualquer listagem da tela para o Excel (.xlsx). O backend monta a
// planilha e pede onde salvar. money vai em centavos; pct como fração (0.25).
import { nextTick, ref } from 'vue'
import { ExportTable, SavePDF } from '../../wailsjs/go/main/App'
import { call, errorMessage } from '../api'
import { models } from '../types'
import { useToast } from './toast'

export type ColKind = 'text' | 'int' | 'money' | 'pct'
export type Col = [label: string, kind: ColKind]

export async function exportTable(title: string, subtitle: string, cols: Col[], rows: (string | number | null | undefined)[][]) {
  const toast = useToast()
  try {
    const saved = await call(ExportTable(models.ExportTable.createFrom({
      title, subtitle,
      columns: cols.map(([label, kind]) => ({ label, kind })),
      rows: rows.map((r) => r.map((v) => (v == null ? '' : String(v)))),
    })))
    if (saved) toast.success('Planilha salva. Abra o arquivo no Excel.')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

// usePrint: marca a área a imprimir (main.css só imprime .print-area) e abre a impressão do Windows.
// savePdf salva a folha (el) em PDF: monta uma página com o CSS atual (regras de
// impressão incluídas) e o backend gera o arquivo com o Edge, perguntando onde salvar.
export async function savePdf(el: HTMLElement | null | undefined, title: string, page = '@page { size: A4; margin: 12mm; }') {
  if (!el) return
  const toast = useToast()
  const css = Array.from(document.styleSheets).map((s) => {
    try {
      return Array.from(s.cssRules).map((r) => r.cssText).join('\n')
    } catch {
      return '' // folha de outra origem: não dá para ler
    }
  }).join('\n')
  const node = el.cloneNode(true) as HTMLElement
  node.classList.add('print-area')
  node.querySelectorAll('.no-print').forEach((n) => n.remove())
  const html = `<!doctype html><html lang="pt-BR" data-theme="light"><head><meta charset="utf-8"><style>${css}\n${page}</style></head><body>${node.outerHTML}</body></html>`
  try {
    if (await call(SavePDF(html, title))) toast.success('PDF salvo.')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

export function usePrint() {
  const printing = ref(false)
  async function print() {
    printing.value = true
    await nextTick()
    window.print()
    printing.value = false
  }
  return { printing, print }
}
