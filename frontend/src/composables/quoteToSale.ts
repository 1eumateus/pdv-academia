// Caminho: pdv/frontend/src/composables/quoteToSale.ts

// Orçamento levado para a tela de Vendas: os itens entram no carrinho e, ao
// finalizar, a venda leva o nº do orçamento (que fica "convertido").
import { ref } from 'vue'

export interface QuoteForSale {
  quoteId: number
  customerId: number | null
  saleDiscountCents: number
  items: { productId: number; qty: number; discountCents: number }[]
}

export const pendingQuote = ref<QuoteForSale | null>(null)
