// Caminho: pdv/frontend/src/composables/charge.ts

import { ChargeCustomer } from '../../wailsjs/go/main/App'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { call, errorMessage } from '../api'
import { useToast } from './toast'

// Abre o WhatsApp com a cobrança do a prazo (mensagem das Configurações).
// O backend monta o link e registra a data; quem envia é o operador.
export async function chargeByWhatsapp(customerId: number): Promise<boolean> {
  const toast = useToast()
  try {
    await call(ChargeCustomer(customerId))
    toast.info('WhatsApp aberto com a mensagem de cobrança. Confira e envie.')
    return true
  } catch (e) {
    toast.error(errorMessage(e))
    return false
  }
}

// Mesma regra do whatsappPhone (Go): aceita DDD + número (10 ou 11 dígitos),
// com ou sem 55 na frente e com ou sem o 0 de interurbano. O 55 só sai quando
// sobra um número completo: (55) 99123-4567 é DDD 55 (RS), não o código do país.
// Telefone inválido = '' (o WhatsApp abre para escolher o contato).
export function whatsappPhone(phone: string): string {
  let d = phone.replace(/\D/g, '')
  if (d.length >= 12 && d.startsWith('55')) d = d.slice(2)
  d = d.replace(/^0/, '')
  return d.length === 10 || d.length === 11 ? '55' + d : ''
}

// firstPhone: o primeiro número que serve para o WhatsApp ('' se nenhum serve).
export function firstPhone(...phones: (string | null | undefined)[]): string {
  return phones.find((p) => whatsappPhone(p ?? '')) ?? ''
}

// Abre o WhatsApp com o texto pronto; quem confere e envia é o operador.
// phone null = nenhum cliente informado (a mensagem explica o motivo certo).
export function openWhatsapp(phone: string | null, text: string) {
  const to = whatsappPhone(phone ?? '')
  // O Wails recusa (sem avisar) link com ! ' ( ) * ~, que o encodeURIComponent não codifica.
  const q = encodeURIComponent(text).replace(/[!'()*~]/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase())
  BrowserOpenURL(`https://wa.me/${to}?text=${q}`)
  if (!to) useToast().info(phone === null ? 'Nenhum cliente informado: escolha o contato no WhatsApp.' : 'Cliente sem celular com DDD no cadastro: escolha o contato no WhatsApp.')
}
