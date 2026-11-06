import { describe, expect, it } from 'vitest'
import QRCode from 'qrcode'
import { decodeQR, pickCode, regions } from './qrscan.js'

// „fotka“ dlhého bločku: biela plocha a QR kód v dolnej časti
function photo(text, { width = 900, height = 2400, scale = 6, top = 1900 } = {}) {
  const qr = QRCode.create(text, { errorCorrectionLevel: 'L' })
  const n = qr.modules.size
  const px = new Uint8ClampedArray(width * height * 4).fill(255)
  const left = Math.round((width - n * scale) / 2)
  for (let y = 0; y < n; y++) {
    for (let x = 0; x < n; x++) {
      if (!qr.modules.get(y, x)) continue
      for (let dy = 0; dy < scale; dy++) {
        for (let dx = 0; dx < scale; dx++) {
          const i = ((top + y * scale + dy) * width + left + x * scale + dx) * 4
          px[i] = px[i + 1] = px[i + 2] = 0
        }
      }
    }
  }
  // čítačka výrezu bez zmenšovania (najbližší pixel)
  return (w, h, sx = 0, sy = 0, sw = width, sh = height) => {
    const out = new Uint8ClampedArray(w * h * 4)
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const s = ((sy + Math.floor((y * sh) / h)) * width + sx + Math.floor((x * sw) / w)) * 4
        out.set(px.subarray(s, s + 4), (y * w + x) * 4)
      }
    }
    return { data: out, width: w, height: h }
  }
}

describe('čítanie QR kódu bločku', () => {
  it('uprednostní kód bločku eKasa', () => {
    const id = 'O-0123456789ABCDEF0123456789ABCDEF'
    expect(pickCode(['https://reklama.sk', id])).toBe(id)
    expect(pickCode(['', 'iný'])).toBe('iný')
    expect(pickCode([])).toBe('')
  })

  it('oblasti: celá fotka, tretiny po dĺžke bločku a stred', () => {
    const tall = regions(1000, 3000)
    expect(tall[0]).toEqual({ x: 0, y: 0, w: 1, h: 1 })
    expect(tall.slice(1, 4).map((r) => r.y)).toEqual([0, 0.275, 0.55])
    expect(regions(3000, 1000)[1].x).toBe(0)
    expect(tall).toHaveLength(5)
  })

  it('nájde malý kód v dolnej časti dlhej fotky (jsQR)', async () => {
    const id = 'O-' + 'A1B2C3D4'.repeat(4)
    expect(await decodeQR(900, 2400, photo(id), { useZXing: false })).toBe(id)
  })

  it('prázdna fotka', async () => {
    const blank = (w, h) => ({ data: new Uint8ClampedArray(w * h * 4).fill(255), width: w, height: h })
    expect(await decodeQR(600, 800, blank, { useZXing: false })).toBe('')
  })
})
