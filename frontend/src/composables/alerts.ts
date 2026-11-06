// Caminho: catraca/frontend/src/composables/alerts.ts

// Alertas do dia, numa lista só: o sino da barra de comando e o bloco "Para
// resolver agora" do Início mostram os mesmos itens, cada um com o link (e a
// ação) para resolver. Atualiza ao abrir, a cada 2 minutos e quando alguém pede.
import { ref } from 'vue'
import { GetBackupStatus, GetDashboard } from '../../wailsjs/go/main/App'
import { call } from '../api'
import { licenseStatus } from './cashState'
import { money } from './format'
import { moduleAlerts, type ModuleAlert } from './moduleExtras'
import { can, currentUser } from './session'

export type Alert = ModuleAlert

export const alerts = ref<Alert[]>([])
const order = { bad: 0, warn: 1, info: 2 }

export async function refreshAlerts() {
  if (!currentUser.value) {
    alerts.value = []
    return
  }
  const out: Alert[] = []
  const [d, backup, mod] = await Promise.all([
    call(GetDashboard()).catch(() => null),
    call(GetBackupStatus()).catch(() => null),
    moduleAlerts().catch(() => [] as Alert[]),
  ])
  if (d) {
    if (d.zeroStockCount) out.push({ id: 'estoque-zerado', level: 'bad', text: `${d.zeroStockCount} produto(s) sem estoque`, to: '/estoque?view=zerado', action: 'Dar entrada' })
    if (d.lowStockCount) out.push({ id: 'estoque-baixo', level: 'warn', text: `${d.lowStockCount} produto(s) com estoque baixo`, to: '/estoque?view=alerta', action: 'Ver' })
    if (can('contas_a_pagar') && d.billsOverdue) out.push({ id: 'contas-vencidas', level: 'bad', text: `${d.billsOverdue} conta(s) a pagar vencida(s)`, to: '/financeiro/contas', action: 'Pagar' })
    if (can('contas_a_pagar') && d.billsDueToday) out.push({ id: 'contas-hoje', level: 'warn', text: `${d.billsDueToday} conta(s) a pagar vencem hoje`, to: '/financeiro/contas', action: 'Pagar' })
    if (d.creditCustomers) out.push({ id: 'a-receber', level: 'info', text: `${money(d.creditOpenCents)} a receber de ${d.creditCustomers} cliente(s)`, to: '/fiado', action: 'Cobrar' })
  }
  out.push(...mod)
  if (backup?.warning) out.push({ id: 'backup', level: 'warn', text: backup.warning, to: '/configuracoes', action: 'Configurar' })
  if (licenseStatus.value?.warning) out.push({ id: 'licenca', level: licenseStatus.value.canWrite ? 'warn' : 'bad', text: licenseStatus.value.warning, to: '/licenca', action: 'Ver' })
  alerts.value = out.sort((a, b) => order[a.level] - order[b.level])
}

let timer: number | undefined
export function startAlerts() {
  refreshAlerts()
  window.clearInterval(timer)
  timer = window.setInterval(refreshAlerts, 120_000)
}
export function stopAlerts() {
  window.clearInterval(timer)
}
