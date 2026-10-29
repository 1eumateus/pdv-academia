// Caminho: pdv/frontend/src/composables/barcode.ts

// Desenho de código de barras sem biblioteca: devolve a sequência de módulos
// (true = barra). EAN-13 quando o código é um EAN-13 válido; senão Code128 (B).

const L = ['0001101', '0011001', '0010011', '0111101', '0100011', '0110001', '0101111', '0111011', '0110111', '0001011']
const G = ['0100111', '0110011', '0011011', '0100001', '0011101', '0111001', '0000101', '0010001', '0001001', '0010111']
const R = ['1110010', '1100110', '1101100', '1000010', '1011100', '1001110', '1010000', '1000100', '1001000', '1110100']
const PARITY = ['LLLLLL', 'LLGLGG', 'LLGGLG', 'LLGGGL', 'LGLLGG', 'LGGLLG', 'LGGGLL', 'LGLGLG', 'LGLGGL', 'LGGLGL']

export function isEAN13(code: string): boolean {
  if (!/^\d{13}$/.test(code)) return false
  const sum = [...code.slice(0, 12)].reduce((a, c, i) => a + Number(c) * (i % 2 ? 3 : 1), 0)
  return (10 - (sum % 10)) % 10 === Number(code[12])
}

function ean13(code: string): string {
  const d = [...code].map(Number)
  let bits = '101'
  for (let i = 1; i <= 6; i++) bits += (PARITY[d[0]][i - 1] === 'L' ? L : G)[d[i]]
  bits += '01010'
  for (let i = 7; i <= 12; i++) bits += R[d[i]]
  return bits + '101'
}

// Larguras (barra, espaço, ...) dos símbolos 0..105 e do stop.
const C128 = ('212222 222122 222221 121223 121322 131222 122213 122312 132212 221213 221312 231212 112232 122132 122231 113222 ' +
  '123122 123221 223211 221132 221231 213212 223112 312131 311222 321122 321221 312212 322112 322211 212123 212321 232121 111323 ' +
  '131123 131321 112313 132113 132311 211313 231113 231311 112133 112331 132131 113123 113321 133121 313121 211331 231131 213113 ' +
  '213311 213131 311123 311321 331121 312113 312311 332111 314111 221411 431111 111224 111422 121124 121421 141122 141221 112214 ' +
  '112412 122114 122411 142112 142211 241211 221114 413111 241112 134111 111242 121142 121241 114212 124112 124211 411212 421112 ' +
  '421211 212141 214121 412121 111143 111341 131141 114113 114311 411113 411311 113141 114131 311141 411131 211412 211214 211232').split(' ')
const STOP = '2331112'
const START_B = 104

function code128(text: string): string | null {
  const codes = [...text].map((c) => c.charCodeAt(0) - 32)
  if (!codes.length || codes.some((c) => c < 0 || c > 94)) return null
  const check = codes.reduce((a, c, i) => a + c * (i + 1), START_B) % 103
  const widths = [START_B, ...codes, check].map((c) => C128[c]).join('') + STOP
  let bits = ''
  ;[...widths].forEach((w, i) => (bits += (i % 2 ? '0' : '1').repeat(Number(w))))
  return bits
}

export interface Bars {
  kind: 'ean13' | 'code128'
  bits: string // '1' = barra, '0' = espaço, um caractere por módulo
}

export function encodeBarcode(code: string): Bars | null {
  if (isEAN13(code)) return { kind: 'ean13', bits: ean13(code) }
  const bits = code128(code)
  return bits ? { kind: 'code128', bits } : null
}
