// Bloček z mobilu: zmenšenie fotky a prečítanie QR kódu (jsQR) v prehliadači.

/** Rozmery zmenšené tak, aby dlhšia strana mala najviac max (nikdy nezväčšuje). */
export function fitSize(w, h, max) {
  const k = Math.min(1, max / Math.max(w, h))
  return { w: Math.max(1, Math.round(w * k)), h: Math.max(1, Math.round(h * k)) }
}

/**
 * Prečíta QR kód z obrázka. read(w, h) vráti ImageData v danej veľkosti;
 * skúša viac veľkostí (malý kód na veľkej fotke sa lepšie číta vo väčšej,
 * rozmazaná fotka v menšej). Vráti text kódu alebo ''.
 */
export function findQR(jsQR, width, height, read, sizes = [1600, 1000, 2400, 700]) {
  const tried = new Set()
  for (const max of sizes) {
    const { w, h } = fitSize(width, height, max)
    if (tried.has(w)) continue
    tried.add(w)
    const img = read(w, h)
    const code = jsQR(img.data, w, h, { inversionAttempts: 'attemptBoth' })
    if (code?.data) return code.data
  }
  return ''
}

/** Je text identifikátor online bločku eKasa (O-…)? Rovnaké pravidlo ako server. */
export function receiptId(text) {
  const m = /\bO-[0-9A-F]{32}\b/i.exec(String(text ?? ''))
  return m ? 'O-' + m[0].slice(2).toUpperCase() : ''
}

export const payments = [
  { label: 'Hotovosť', value: 'cash' },
  { label: 'Karta', value: 'card' },
]

/** Načíta obrázok zo súboru (v prehliadači). Vráti { img, url } — url treba uvoľniť. */
export function loadImage(file) {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => resolve({ img, url })
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('Fotka sa nedá otvoriť'))
    }
    img.src = url
  })
}

/** Zmenší fotku na JPEG (dlhšia strana najviac max px). */
export function shrinkImage(img, max = 1600) {
  const { w, h } = fitSize(img.naturalWidth, img.naturalHeight, max)
  const c = document.createElement('canvas')
  c.width = w
  c.height = h
  c.getContext('2d').drawImage(img, 0, 0, w, h)
  return new Promise((resolve) => c.toBlob(resolve, 'image/jpeg', 0.82))
}

/** Čítačka pixelov obrázka pre findQR. */
export function imageReader(img) {
  const c = document.createElement('canvas')
  const ctx = c.getContext('2d', { willReadFrequently: true })
  return (w, h) => {
    c.width = w
    c.height = h
    ctx.drawImage(img, 0, 0, w, h)
    return ctx.getImageData(0, 0, w, h)
  }
}
