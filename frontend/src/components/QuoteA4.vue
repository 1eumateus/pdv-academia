<!-- Caminho: pdv/frontend/src/components/QuoteA4.vue -->
<script setup lang="ts">
// Orçamento em página A4 (impressão/PDF pelo diálogo do Windows). A térmica
// continua usando as linhas do backend; este é só o layout de folha.
import { computed } from 'vue'
import { settings } from '../composables/cashState'
import { formatAddress, formatDateTime, formatDocument, money } from '../composables/format'
import type { models } from '../types'

const props = defineProps<{ data: models.QuoteDetails }>()
const q = computed(() => props.data.quote)
const st = computed(() => settings.value)
const c = computed(() => props.data.customer)
const br = (d: string) => d.split('-').reverse().join('/')
const itemDisc = (it: models.QuoteItem) => it.promoDiscountCents + it.discountCents
const itemsDisc = computed(() => props.data.items.reduce((a, it) => a + itemDisc(it), 0))
const gross = computed(() => props.data.items.reduce((a, it) => a + it.qty * it.unitPriceCents, 0))
const storeContact = computed(() => [st.value?.phone && `Tel. ${st.value.phone}`, st.value?.whatsapp && `WhatsApp ${st.value.whatsapp}`, st.value?.email].filter(Boolean).join(' · '))
const storeDoc = computed(() => [formatDocument(st.value?.document), st.value?.stateRegistration && `IE ${st.value.stateRegistration}`].filter(Boolean).join(' · '))
</script>

<template>
  <div class="quote-a4">
    <header class="qa-head">
      <div class="qa-store">
        <img v-if="st?.logo" :src="st.logo" alt="Logo" class="qa-logo" />
        <div>
          <div class="qa-store-name">{{ st?.storeName }}</div>
          <div v-if="st?.legalName && st.legalName !== st.storeName">{{ st.legalName }}</div>
          <div v-if="storeDoc">{{ storeDoc }}</div>
          <div v-for="l in formatAddress(st)" :key="l">{{ l }}</div>
          <div v-if="storeContact">{{ storeContact }}</div>
        </div>
      </div>
      <div class="qa-title">
        <div class="qa-doc">ORÇAMENTO</div>
        <div class="qa-num">nº {{ q.id }}</div>
        <div>Emitido em {{ formatDateTime(q.createdAt) }}</div>
        <div><strong>Válido até {{ br(q.validUntil) }}</strong></div>
        <div v-if="q.userName">Operador: {{ q.userName }}</div>
      </div>
    </header>

    <section v-if="c" class="qa-box">
      <div class="qa-label">Cliente</div>
      <div class="qa-cust-name">{{ c.name }}</div>
      <div class="qa-cols">
        <span v-if="c.document">{{ formatDocument(c.document) }}</span>
        <span v-if="c.phone">Tel. {{ c.phone }}</span>
        <span v-if="c.email">{{ c.email }}</span>
      </div>
      <div v-for="l in formatAddress(c)" :key="l">{{ l }}</div>
      <div v-if="c.reference">Ref.: {{ c.reference }}</div>
    </section>

    <div class="qa-section">DADOS DOS PRODUTOS</div>
    <table class="qa-items">
      <thead>
        <tr>
          <th class="c">CÓDIGO</th><th>DESCRIÇÃO DO PRODUTO</th><th class="c">UN</th><th class="r">QTD.</th>
          <th class="r">V. UNIT.</th><th class="r">V. DESC.</th><th class="r">V. TOTAL</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="it in data.items" :key="it.id">
          <td class="c">{{ it.barcode || it.productId }}</td>
          <td>{{ it.productName.toUpperCase() }}<span v-if="it.promoLabel" class="qa-promo"> — {{ it.promoLabel }}</span></td>
          <td class="c">UN</td>
          <td class="r">{{ it.qty }}</td>
          <td class="r">{{ money(it.unitPriceCents) }}</td>
          <td class="r">{{ itemDisc(it) ? money(itemDisc(it)) : '0,00' }}</td>
          <td class="r">{{ money(it.qty * it.unitPriceCents - itemDisc(it)) }}</td>
        </tr>
      </tbody>
    </table>

    <div class="qa-bottom">
      <div class="qa-notes">
        <template v-if="q.notes"><div class="qa-label">Observações</div><div class="qa-pre">{{ q.notes }}</div></template>
      </div>
      <div class="qa-totals">
        <div><span>Subtotal</span><span>{{ money(gross) }}</span></div>
        <div v-if="q.totalCents !== gross"><span>Descontos</span><span>-{{ money(gross - q.totalCents) }}</span></div>
        <div v-if="itemsDisc && gross - q.totalCents !== itemsDisc" class="qa-sub"><span>nos itens {{ money(itemsDisc) }} · no orçamento {{ money(gross - q.totalCents - itemsDisc) }}</span></div>
        <div class="qa-grand"><span>TOTAL</span><span>{{ money(q.totalCents) }}</span></div>
      </div>
    </div>

    <div class="qa-sign"><div class="qa-line" />Aceite do cliente</div>

    <footer class="qa-foot">
      <strong>ORÇAMENTO — NÃO É DOCUMENTO FISCAL.</strong>
      Preços e estoque sujeitos a alteração após a validade.
    </footer>
  </div>
</template>

<style scoped>
.quote-a4 { page: a4; width: 186mm; max-width: 100%; margin: 0 auto; padding: 4mm; background: #fff; color: #111; font: 11pt/1.4 'Segoe UI', Arial, sans-serif; }
.qa-head { display: flex; justify-content: space-between; gap: 16px; padding-bottom: 10px; border-bottom: 3px solid #111; font-size: 9.5pt; }
.qa-store { display: flex; gap: 12px; align-items: flex-start; }
.qa-logo { max-width: 110px; max-height: 80px; object-fit: contain; }
.qa-store-name { font-size: 15pt; font-weight: 700; }
.qa-title { text-align: right; white-space: nowrap; }
.qa-doc { font-size: 20pt; font-weight: 800; letter-spacing: 2px; }
.qa-num { font-size: 13pt; font-weight: 700; margin-bottom: 4px; }
.qa-box { margin-top: 12px; padding: 8px 12px; border: 1px solid #bbb; border-radius: 6px; font-size: 10pt; }
.qa-label { font-size: 8pt; text-transform: uppercase; letter-spacing: 1px; color: #555; }
.qa-cust-name { font-size: 12pt; font-weight: 700; }
.qa-cols { display: flex; gap: 18px; flex-wrap: wrap; }
.qa-section { margin-top: 14px; font-size: 8pt; font-weight: 700; letter-spacing: .5px; }
.qa-items { width: 100%; margin-top: 2px; border-collapse: collapse; font-size: 8.5pt; line-height: 1.25; }
.qa-items th, .qa-items td { border: 1px solid #000; padding: 2px 4px; vertical-align: top; }
.qa-items th { font-size: 6.5pt; font-weight: 700; text-align: left; background: #fff; }
.qa-items tr { break-inside: avoid; }
.qa-items thead { display: table-header-group; }
.qa-items .r { text-align: right; white-space: nowrap; }
.qa-items .c { text-align: center; white-space: nowrap; }
.qa-promo { font-size: 7.5pt; }
.qa-bottom { display: flex; justify-content: space-between; gap: 20px; margin-top: 12px; break-inside: avoid; }
.qa-notes { flex: 1; font-size: 10pt; }
.qa-pre { white-space: pre-wrap; }
.qa-totals { min-width: 240px; font-size: 10.5pt; }
.qa-totals > div { display: flex; justify-content: space-between; gap: 12px; padding: 2px 0; }
.qa-sub { font-size: 8.5pt; color: #555; justify-content: flex-end !important; }
.qa-grand { margin-top: 4px; padding: 6px 8px !important; background: #111; color: #fff; font-size: 14pt; font-weight: 800; border-radius: 4px; }
.qa-sign { width: 260px; margin: 48px auto 0; text-align: center; font-size: 9.5pt; break-inside: avoid; }
.qa-line { border-top: 1px solid #111; margin-bottom: 3px; }
.qa-foot { margin-top: 24px; padding-top: 6px; border-top: 1px solid #bbb; text-align: center; font-size: 8.5pt; color: #444; }
@media print {
  .quote-a4 { width: auto; padding: 0; }
  .qa-grand { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
}
</style>
