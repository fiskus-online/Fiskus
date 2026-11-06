// Čítanie QR kódu bločku z fotky v telefóne. Prvý pokus je ZXing (zxing-cpp
// vo WebAssembly — číta aj rozmazané, slabé, natočené a malé kódy), potom
// jsQR. Skúša sa celá fotka aj výrezy (QR je na dlhom bločku malý).
import jsQR from 'jsqr'
import { fitSize, receiptId } from './receipt.js'

let zxing = null

/** Načíta ZXing (iba keď treba; súbor .wasm je v programe, nie z internetu). */
async function loadZXing() {
  zxing ??= (async () => {
    const [{ prepareZXingModule, readBarcodes }, { default: wasmUrl }] = await Promise.all([
      import('zxing-wasm/reader'),
      import('zxing-wasm/reader/zxing_reader.wasm?url'),
    ])
    await prepareZXingModule({
      overrides: { locateFile: (path, prefix) => (path.endsWith('.wasm') ? wasmUrl : prefix + path) },
      fireImmediately: true,
    })
    return readBarcodes
  })().catch(() => null) // bez WebAssembly zostane jsQR
  return zxing
}

// QR je nezávislý od natočenia a bloček je čierny na bielom — otáčanie
// a inverzia by čítanie iba spomalili
const zxingOptions = {
  formats: ['QRCode'],
  tryHarder: true,
  tryRotate: false,
  tryInvert: false,
  tryDownscale: true,
  maxNumberOfSymbols: 4,
}

/** Najdlhší čas na čítanie jednej fotky (potom už iba rýchly jsQR). */
export const BUDGET_MS = 6000

/** Z viacerých nájdených kódov uprednostní kód bločku eKasa (O-…). */
export function pickCode(texts) {
  const list = texts.filter(Boolean)
  return list.find((t) => receiptId(t)) ?? list[0] ?? ''
}

/**
 * Oblasti fotky na skúšanie (v podieloch šírky a výšky): celá, potom
 * prekrývajúce sa tretiny po dĺžke bločku a stred.
 */
export function regions(width, height) {
  const tall = height >= width
  const parts = [[0, 0.45], [0.275, 0.725], [0.55, 1]]
  const out = [{ x: 0, y: 0, w: 1, h: 1 }]
  for (const [a, b] of parts) out.push(tall ? { x: 0, y: a, w: 1, h: b - a } : { x: a, y: 0, w: b - a, h: 1 })
  out.push({ x: 0.2, y: 0.2, w: 0.6, h: 0.6 })
  return out
}

/**
 * Prečíta QR kód z obrázka. read(w, h, sx, sy, sw, sh) vráti ImageData
 * výrezu (sx … v pixeloch fotky) zmenšeného na w × h. Vráti text alebo ''.
 */
export async function decodeQR(width, height, read, { useZXing = true, now = () => performance.now() } = {}) {
  const box = (r, max) => {
    const sw = Math.round(width * r.w)
    const sh = Math.round(height * r.h)
    return { sx: Math.round(width * r.x), sy: Math.round(height * r.y), sw, sh, ...fitSize(sw, sh, max) }
  }
  const [whole, ...parts] = regions(width, height)
  // najprv rýchlo (celá fotka zmenšená, výrezy), potom v plnom rozlíšení
  const tries = [box(whole, 1600), ...parts.map((r) => box(r, 1400)), box(whole, 2600), ...parts.slice(0, 3).map((r) => box(r, 2600))]
  const seen = new Set()
  const unique = tries.filter((t) => {
    const k = `${t.sx},${t.sy},${t.w}`
    return seen.has(k) ? false : seen.add(k)
  })
  const start = now()
  const readBarcodes = useZXing ? await loadZXing() : null
  if (readBarcodes) {
    for (const t of unique) {
      if (now() - start > BUDGET_MS) break
      const img = read(t.w, t.h, t.sx, t.sy, t.sw, t.sh)
      const res = await readBarcodes(img, zxingOptions).catch(() => [])
      const code = pickCode(res.filter((x) => x.isValid).map((x) => x.text))
      if (code) return code
    }
  }
  for (const t of unique.slice(0, 5)) {
    const img = read(t.w, t.h, t.sx, t.sy, t.sw, t.sh)
    const code = jsQR(img.data, t.w, t.h, { inversionAttempts: 'attemptBoth' })
    if (code?.data) return code.data
  }
  return ''
}
