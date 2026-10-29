// Caminho: pdv/frontend/src/composables/format.ts

// Formatação e conversão para exibição. Todo valor monetário é inteiro em
// centavos; a conversão de texto para centavos é feita só com inteiros,
// sem float. O backend continua sendo a autoridade sobre valores e regras.

const brl = new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' })
const dateTime = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short' })
const dateOnly = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short' })

export function money(cents: number | null | undefined): string {
  return brl.format((cents ?? 0) / 100)
}

const MAX_CENTS = 10_000_000_000 // R$ 100 milhões, igual ao backend

// parseMoney aceita "10", "10,5", "10,50", "1.234,56" e "10.50". Devolve null
// para entradas inválidas ou fora do limite.
export function parseMoney(input: string): number | null {
  let s = input.trim().replace(/^R\$\s*/i, '').replace(/\s/g, '')
  if (s === '') return null
  let intPart: string
  let decPart = ''
  if (/^\d{1,3}(\.\d{3})+(,\d{1,2})?$/.test(s) || /^\d+(,\d{1,2})?$/.test(s)) {
    s = s.replace(/\./g, '')
    ;[intPart, decPart = ''] = s.split(',')
  } else if (/^\d+\.\d{1,2}$/.test(s)) {
    ;[intPart, decPart] = s.split('.')
  } else {
    return null
  }
  if (intPart.length > 9) return null
  const cents = Number(intPart) * 100 + Number(decPart.padEnd(2, '0'))
  return Number.isSafeInteger(cents) && cents <= MAX_CENTS ? cents : null
}

// parseDiscount aceita "5,00" (reais) ou "10%" / "2,5%" (sobre base). Vazio = 0;
// inválido = null. O backend confere de novo (não passa do valor, limite do operador).
export function parseDiscount(input: string, base: number): number | null {
  const s = input.trim()
  if (s === '') return 0
  if (s.endsWith('%')) {
    const pct = Number(s.slice(0, -1).trim().replace(',', '.'))
    if (!Number.isFinite(pct) || pct < 0 || pct > 100) return null
    return Math.round((base * pct) / 100)
  }
  return parseMoney(s)
}

// centsToInput mostra centavos no formato aceito por parseMoney ("10,50").
export function centsToInput(cents: number): string {
  const sign = cents < 0 ? '-' : ''
  const abs = Math.abs(cents)
  return `${sign}${Math.floor(abs / 100)},${String(abs % 100).padStart(2, '0')}`
}

// parseQty aceita apenas inteiros positivos até o limite do backend.
export function parseQty(input: string | number, allowNegative = false): number | null {
  const s = String(input).trim()
  if (!(allowNegative ? /^-?\d{1,6}$/ : /^\d{1,6}$/).test(s)) return null
  const n = Number(s)
  if (n === 0 || n > 100_000 || n < -100_000) return null
  return n
}

export function formatDateTime(v: string | null | undefined): string {
  if (!v) return '—'
  const d = new Date(v)
  return isNaN(d.getTime()) ? '—' : dateTime.format(d)
}

export function formatDate(v: string | null | undefined): string {
  if (!v) return '—'
  const d = new Date(v)
  return isNaN(d.getTime()) ? '—' : dateOnly.format(d)
}

// today devolve a data local no formato AAAA-MM-DD esperado pelo backend.
// stockLevel segue a regra do backend (StockAlerts): o mínimo do produto ou,
// sem ele, o padrão das Configurações.
export function stockLevel(qty: number, minStock: number, lowDefault: number): 'zerado' | 'baixo' | 'ok' {
  if (qty <= 0) return 'zerado'
  return qty <= (minStock > 0 ? minStock : lowDefault) ? 'baixo' : 'ok'
}

export function today(offsetDays = 0): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

export const paymentLabels: Record<string, string> = {
  dinheiro: 'Dinheiro',
  pix: 'Pix',
  cartao: 'Cartão',
  debito: 'Débito',
  credito: 'Crédito',
  fiado: 'A prazo',
  vale_troca: 'Vale-troca',
  misto: 'Misto',
}

export const stockReasonLabels: Record<string, string> = {
  entrada: 'Entrada',
  venda: 'Venda',
  ajuste: 'Ajuste',
  devolucao: 'Devolução',
  cancelamento: 'Cancelamento',
}

export const cashTypeLabels: Record<string, string> = {
  suprimento: 'Suprimento',
  sangria: 'Sangria',
  venda: 'Venda',
  recebimento_fiado: 'Recebimento a prazo',
  estorno_venda: 'Estorno de venda',
  estorno_recebimento: 'Estorno de recebimento',
}

// Documento gravado sem pontuação: 11 = CPF, 14 = CNPJ (numérico ou alfanumérico).
export function formatDocument(d: string | null | undefined): string {
  if (!d) return ''
  if (d.length === 11) return `${d.slice(0, 3)}.${d.slice(3, 6)}.${d.slice(6, 9)}-${d.slice(9)}`
  if (d.length === 14) return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5, 8)}/${d.slice(8, 12)}-${d.slice(12)}`
  return d
}

export function formatZip(z: string | null | undefined): string {
  return z && z.length === 8 ? `${z.slice(0, 5)}-${z.slice(5)}` : z ?? ''
}

interface AddressLike {
  street?: string | null
  number?: string | null
  complement?: string | null
  district?: string | null
  city?: string | null
  state?: string | null
  zipCode?: string | null
}

// formatAddress monta as linhas do endereço, omitindo o que estiver vazio.
export function formatAddress(a: AddressLike | null | undefined): string[] {
  if (!a) return []
  const line1 = [a.street, a.number].filter(Boolean).join(', ') + (a.complement ? ` — ${a.complement}` : '')
  const cityUf = [a.city, a.state].filter(Boolean).join('/')
  const line2 = [a.district, cityUf].filter(Boolean).join(' · ')
  const zip = a.zipCode ? `CEP ${formatZip(a.zipCode)}` : ''
  return [line1.trim(), line2, zip].filter((l) => l !== '')
}
