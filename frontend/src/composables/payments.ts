// Caminho: pdv/frontend/src/composables/payments.ts

// Partes do pagamento como o operador digita (texto) e a conversão para o que
// o backend recebe. O backend confere de novo soma, troco e limite do fiado.
import { models } from '../types'
import { money, parseMoney } from './format'

export interface PayPart {
  method: string
  amount: string // só no pagamento dividido; na forma única o valor é o total
  received: string // dinheiro: valor entregue pelo cliente
  brand: string
  installments: string
}

export const payMethods = ['dinheiro', 'pix', 'debito', 'credito', 'fiado'] as const

export function newPart(method = 'dinheiro', amount = ''): PayPart {
  return { method, amount, received: '', brand: '', installments: '1' }
}

export interface PayResult {
  payments: models.PaymentInput[]
  error: string
  hasFiado: boolean
  remaining: number // quanto falta (negativo = passou do total)
  change: number
}

// evaluate monta os pagamentos. split=false: uma forma só, com o valor total.
export function evaluate(parts: PayPart[], total: number, split: boolean): PayResult {
  const out: PayResult = { payments: [], error: '', hasFiado: false, remaining: 0, change: 0 }
  if (total <= 0) return out
  const list = split ? parts : parts.slice(0, 1)
  let sum = 0
  for (const p of list) {
    const amount = split ? parseMoney(p.amount) : total
    if (amount === null || amount <= 0) {
      out.error = 'Informe o valor de cada forma de pagamento.'
      continue
    }
    sum += amount
    const pay = models.PaymentInput.createFrom({ method: p.method, amountCents: amount, receivedCents: 0, cardBrand: '', installments: 0 })
    if (p.method === 'dinheiro' && p.received.trim()) {
      const rec = parseMoney(p.received)
      if (rec === null || rec < amount) out.error ||= 'O valor recebido em dinheiro é menor que a parte em dinheiro.'
      else {
        pay.receivedCents = rec
        out.change += rec - amount
      }
    }
    if (p.method === 'debito' || p.method === 'credito') pay.cardBrand = p.brand.trim()
    if (p.method === 'credito') pay.installments = Math.max(1, Math.min(24, Number(p.installments) || 1))
    out.hasFiado ||= p.method === 'fiado'
    out.payments.push(pay)
  }
  out.remaining = total - sum
  if (!out.error && out.remaining > 0) out.error = `Falta ${money(out.remaining)} para fechar o total.`
  if (!out.error && out.remaining < 0) out.error = `As formas passam do total em ${money(-out.remaining)}.`
  return out
}
