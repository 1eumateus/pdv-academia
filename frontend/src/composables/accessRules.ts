// Caminho: catraca/frontend/src/composables/accessRules.ts
// Regras de acesso da catraca (plano e exceção da assinatura).
import { models } from '../../wailsjs/go/models'

const dayNames = ['', 'seg', 'ter', 'qua', 'qui', 'sex', 'sáb', 'dom']

export const noRules = () => models.AccessRules.createFrom({ weekdays: [], weeklyLimit: 0, dailyLimit: 0 })

export const copyRules = (r?: models.AccessRules | null) => models.AccessRules.createFrom({ ...noRules(), ...r, weekdays: [...(r?.weekdays ?? [])] })

// "3x/semana · 2/dia · seg qua sex" ("" = sem restrição).
export const describeRules = (r?: models.AccessRules | null) => [
  r?.weeklyLimit ? `${r.weeklyLimit}x/semana` : '',
  r?.dailyLimit ? `${r.dailyLimit}/dia` : '',
  (r?.weekdays ?? []).map((d) => dayNames[d]).join(' '),
].filter(Boolean).join(' · ')

export const rulesKey = (r?: models.AccessRules | null) => (r ? JSON.stringify([r.weekdays ?? [], r.weeklyLimit ?? 0, r.dailyLimit ?? 0]) : 'plano')

// O Go recebe *AccessRules: null = a assinatura segue as regras do plano.
export const ruleException = (own: boolean, r: models.AccessRules) => (own ? r : null) as unknown as models.AccessRules
