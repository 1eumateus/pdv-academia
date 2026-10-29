// Caminho: pdv/frontend/src/composables/messages.ts

// Mensagens prontas do WhatsApp (textos em Configurações): troca {nome}, {plano},
// {vencimento}, {valor} e {loja} e abre a conversa; quem envia é o operador.
import { settings, storeName } from './cashState'
import { openWhatsapp } from './charge'

export type MessageKind = 'msgBirthday' | 'msgDue' | 'msgOverdue' | 'msgMissing' | 'msgWinback'

export function sendMessage(kind: MessageKind, phone: string | null | undefined, vars: { nome: string; plano?: string; vencimento?: string; valor?: string }) {
  const text = (settings.value?.[kind] ?? '')
    .replaceAll('{nome}', vars.nome.trim().split(/\s+/)[0] ?? '')
    .replaceAll('{plano}', vars.plano ?? '')
    .replaceAll('{vencimento}', vars.vencimento ?? '')
    .replaceAll('{valor}', vars.valor ?? '')
    .replaceAll('{loja}', storeName.value)
  openWhatsapp(phone ?? '', text)
}

// Aniversário: dias até o próximo (0 = hoje) e a idade que a pessoa faz.
export function birthdayInfo(birth: string, today = new Date()): { days: number; age: number } {
  const [y, m, d] = birth.split('-').map(Number)
  const base = new Date(today.getFullYear(), today.getMonth(), today.getDate())
  let next = new Date(base.getFullYear(), m - 1, d)
  if (next < base) next = new Date(base.getFullYear() + 1, m - 1, d)
  return { days: Math.round((next.getTime() - base.getTime()) / 86_400_000), age: next.getFullYear() - y }
}
