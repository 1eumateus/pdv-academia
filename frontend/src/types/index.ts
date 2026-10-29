// Caminho: pdv/frontend/src/types/index.ts

// Tipos gerados pelo Wails a partir dos structs Go (internal/models).
import { models } from '../../wailsjs/go/models'

export type Product = models.Product
export type Category = models.Category
export type Supplier = models.Supplier
export type Customer = models.Customer
export type Sale = models.Sale
export type SaleDetails = models.SaleDetails
export type SaleItem = models.SaleItem
export type StockMovement = models.StockMovement
export type StockAuditEntry = models.StockAuditEntry
export type Comanda = models.Comanda
export type ComandaDetails = models.ComandaDetails
export type CreditPayment = models.CreditPayment
export type CustomerHistory = models.CustomerHistory
export type CashSummary = models.CashSummary
export type CashSession = models.CashSession
export type SalesReport = models.SalesReport
export type Settings = models.Settings
export type AppInfo = models.AppInfo
export type Receipt = models.Receipt
export type User = models.User
export type Session = models.Session
export type AuditEntry = models.AuditEntry

export { models }

// Item do carrinho na tela de vendas: o preço é só para exibição; o total
// confiável é calculado pelo backend.
export interface CartLine {
  product: Product
  qty: number
  discountText: string // "5,00" ou "10%"
}

export type PaymentMethod = 'dinheiro' | 'pix' | 'cartao' | 'debito' | 'credito' | 'fiado' | 'vale_troca' | 'misto'
