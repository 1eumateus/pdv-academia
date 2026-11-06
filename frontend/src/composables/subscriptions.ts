// Caminho: catraca/frontend/src/composables/subscriptions.ts

// Textos curtos das assinaturas, usados na tela Assinaturas e na ficha do aluno.
// Vencimento de quem tem isenção permanente (models.PermanentExemptDue): não é uma data de verdade.
export const permanentExemptDue = '2099-12-31'
export const brDate = (d: string) => (d === permanentExemptDue ? 'sem vencimento' : d.length === 10 ? `${d.slice(8, 10)}/${d.slice(5, 7)}/${d.slice(0, 4)}` : d)
export const exemptLabel = (s: { exemptUntil: string }) => (s.exemptUntil ? `Isenta até ${brDate(s.exemptUntil)}` : 'Isenta (permanente)')
export const planLabel = (days: number) => ({ 30: 'mensal', 60: 'bimestral', 90: 'trimestral', 180: 'semestral', 365: 'anual' } as Record<number, string>)[days] ?? `${days} dias`
