// Caminho: pdv/frontend/src/api.ts

// Contrato de erro Go → JS: todo método do App rejeita com um JSON
// {"code": "...", "message": "..."} (ver ErrorPayload em app.go). Aqui ele vira
// ApiError, e as telas decidem o fluxo pelo code, nunca pelo texto.

export type ErrorCode =
  | 'ESTOQUE_INSUFICIENTE'
  | 'LIMITE_CREDITO_EXCEDIDO'
  | 'CLIENTE_INEXISTENTE'
  | 'CLIENTE_INATIVO'
  | 'PRODUTO_INEXISTENTE'
  | 'PRODUTO_INATIVO'
  | 'COMANDA_INEXISTENTE'
  | 'COMANDA_FECHADA'
  | 'VENDA_INEXISTENTE'
  | 'VENDA_JA_CANCELADA'
  | 'CAIXA_NAO_ABERTO'
  | 'CAIXA_JA_ABERTO'
  | 'OPERACAO_INVALIDA'
  | 'CONFLITO_DE_ESTADO'
  | 'RECURSO_DUPLICADO'
  | 'NAO_ENCONTRADO'
  | 'BANCO_OCUPADO'
  | 'ERRO_ARMAZENAMENTO'
  | 'ERRO_INTERNO'
  | 'LOGIN_NECESSARIO'
  | 'USUARIO_JA_LOGADO'
  | 'AUTORIZACAO_NECESSARIA'
  | 'SEM_PERMISSAO'
  | 'SOMENTE_CONSULTA'

export class ApiError extends Error {
  // permission acompanha AUTORIZACAO_NECESSARIA: o que o gerente deve liberar.
  constructor(public code: ErrorCode, message: string, public permission = '') {
    super(message)
  }
}

// Chamado quando o backend responde que não há usuário logado (sessão encerrada).
let onLoginRequired: (() => void) | null = null
export function setLoginRequiredHandler(fn: () => void) {
  onLoginRequired = fn
}

function toApiError(raw: unknown): ApiError {
  if (raw instanceof ApiError) return raw
  const text = typeof raw === 'string' ? raw : raw instanceof Error ? raw.message : String(raw)
  try {
    const p = JSON.parse(text)
    if (p && typeof p.code === 'string' && typeof p.message === 'string') {
      return new ApiError(p.code as ErrorCode, p.message, typeof p.permission === 'string' ? p.permission : '')
    }
  } catch {
    // não é o contrato: erro do próprio Wails ou do navegador
  }
  return new ApiError('ERRO_INTERNO', 'Não foi possível comunicar com o sistema. Tente novamente.')
}

// call executa uma chamada ao backend convertendo o erro para ApiError.
export async function call<T>(p: Promise<T>): Promise<T> {
  try {
    return await p
  } catch (e) {
    const err = toApiError(e)
    if (err.code === 'LOGIN_NECESSARIO') onLoginRequired?.()
    throw err
  }
}

export function errorMessage(e: unknown): string {
  return toApiError(e).message
}

export function isCode(e: unknown, code: ErrorCode): boolean {
  return e instanceof ApiError && e.code === code
}

// requestId gera a chave de idempotência de uma venda/recebimento.
export function newRequestId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}
