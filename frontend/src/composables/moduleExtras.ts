// Caminho: catraca/frontend/src/composables/moduleExtras.ts

// O que o módulo Academia acrescenta à barra de comando: alunos na busca,
// ações rápidas (receber mensalidade, liberar a catraca) e alertas do dia.
// No PDV genérico este arquivo existe com as listas vazias; o resto da barra é
// igual nos dois projetos.
import { GateManualRelease, GateStatus, ListGateMembers, ListSubscriptions } from '../../wailsjs/go/main/App'
import { call } from '../api'
import { today } from './format'
import { can } from './session'

export type SearchHit = { group: string; icon: string; label: string; sub?: string; to?: string; run?: () => Promise<void> | void }
export type QuickAction = { key: string; label: string; icon: string; primary?: boolean; show: () => boolean; run: (ctx: QuickCtx) => Promise<void> | void }
export type QuickCtx = { openSearch: (scope?: string) => void; toast: (msg: string, kind?: 'success' | 'error') => void }
export type ModuleAlert = { id: string; level: 'bad' | 'warn' | 'info'; text: string; to: string; action?: string }

export const moduleName = 'Academia'

// Busca: alunos (com a situação na catraca). scope 'alunos' = só eles.
export async function moduleSearch(term: string): Promise<SearchHit[]> {
  const list = await call(ListGateMembers(term)).catch(() => [])
  return list.slice(0, 6).map((m) => ({
    group: 'Alunos', icon: 't-rosto', label: m.name,
    sub: `${m.allowed ? 'pode passar' : 'não pode passar'} · ${m.reason}`,
    to: `/catraca/alunos?busca=${encodeURIComponent(m.name)}&ficha=${m.customerId}`,
  }))
}

export const moduleActions: QuickAction[] = [
  { key: 'mensalidade', label: 'Receber mensalidade', icon: 'receber', primary: true, show: () => true, run: (c) => c.openSearch('alunos') },
  {
    key: 'liberar', label: 'Liberar catraca', icon: 'liberar', show: () => can('catraca_liberar_manual'),
    run: async (c) => {
      try {
        await call(GateManualRelease(''))
        c.toast('Catraca liberada.', 'success')
      } catch (e) {
        c.toast(e instanceof Error ? e.message : String(e), 'error')
      }
    },
  },
]

// Alertas: mensalidades vencidas e de hoje; leitor facial sem contato.
export async function moduleAlerts(): Promise<ModuleAlert[]> {
  const out: ModuleAlert[] = []
  const [subs, gate] = await Promise.all([call(ListSubscriptions()).catch(() => []), call(GateStatus()).catch(() => null)])
  const t = today()
  const open = subs.filter((s) => s.status === 'ativa' && !s.pausedAt && !s.exempt && s.openInvoice)
  const late = open.filter((s) => s.openInvoice!.dueDate < t).length
  const due = open.filter((s) => s.openInvoice!.dueDate === t).length
  if (late) out.push({ id: 'mens-atraso', level: 'bad', text: `${late} mensalidade(s) em atraso`, to: '/catraca/alunos?aba=mensalidades', action: 'Ver' })
  if (due) out.push({ id: 'mens-hoje', level: 'warn', text: `${due} mensalidade(s) vencem hoje`, to: '/catraca/alunos?aba=mensalidades', action: 'Ver' })
  if (gate && gate.config.relay === 'idface' && gate.config.deviceHost) {
    const seen = gate.deviceLastSeen ? (Date.now() - new Date(gate.deviceLastSeen).getTime()) / 60000 : Infinity
    if (seen > 5) out.push({ id: 'idface', level: 'bad', text: gate.deviceLastSeen ? `Leitor facial sem contato há ${Math.round(seen)} min` : 'Leitor facial ainda não se conectou', to: '/catraca', action: 'Ver' })
  }
  return out
}
