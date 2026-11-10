// Šírka bočného panela (detail vpravo): dá sa ťahať za ľavý okraj,
// šípkami na úchyte a dvojklikom sa vráti na predvolenú. Pamätá sa
// v prehliadači (iba pohodlie, bez nej panel funguje).
import { onBeforeUnmount, ref } from 'vue'

export const PANEL_MIN = 360

/** Obmedzí šírku: najmenej PANEL_MIN, zoznamu vľavo ostane aspoň 320 px. */
export function clampPanel(w, viewport) {
  const max = Math.max(PANEL_MIN, viewport - 320)
  const n = Math.round(Number(w) || 0)
  return Math.min(max, Math.max(PANEL_MIN, n))
}

/** Šírka panela uložená pod kľúčom key, predvolená def. */
export function usePanelWidth(key, def = 460) {
  let start = def
  try {
    const v = Number(localStorage.getItem(key))
    if (v > 0) start = v
  } catch {
    // prehliadač bez úložiska
  }
  const width = ref(clampPanel(start, window.innerWidth))
  const save = () => {
    try {
      localStorage.setItem(key, String(width.value))
    } catch {
      // nevadí
    }
  }
  const set = (w) => {
    width.value = clampPanel(w, window.innerWidth)
  }
  let move = null
  let up = null
  const stop = () => {
    if (move) window.removeEventListener('pointermove', move)
    if (up) window.removeEventListener('pointerup', up)
    move = up = null
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
  }
  // ťahanie: panel je pri pravom okraji okna, šírka = okno − poloha myši
  function drag(e) {
    if (e.button !== 0) return
    e.preventDefault()
    const right = e.currentTarget.parentElement?.getBoundingClientRect().right ?? window.innerWidth
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    move = (ev) => set(right - ev.clientX)
    up = () => {
      stop()
      save()
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
  function key_(e) {
    const step = e.shiftKey ? 80 : 20
    if (e.key === 'ArrowLeft') set(width.value + step)
    else if (e.key === 'ArrowRight') set(width.value - step)
    else return
    e.preventDefault()
    save()
  }
  function reset() {
    set(def)
    save()
  }
  onBeforeUnmount(stop)
  return { width, drag, onKey: key_, reset }
}
