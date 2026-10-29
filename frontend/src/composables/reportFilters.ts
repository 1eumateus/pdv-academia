// Caminho: pdv/frontend/src/composables/reportFilters.ts

export type AnalyticsKind = 'lucro' | 'abc' | 'parados' | 'fiado' | 'compras' | 'estoque' | 'descontos'

// Filtros da barra única de Relatórios, repassados ao relatório escolhido.
export interface ReportFilters {
  start: string
  end: string
  userId: number // 0 = todos (o backend restringe ao próprio usuário sem permissão)
  methods: string[] // formas de pagamento marcadas (vazio = todas)
  days: number // produtos parados: sem venda há N dias
}
