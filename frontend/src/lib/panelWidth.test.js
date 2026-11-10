import { describe, expect, it } from 'vitest'
import { clampPanel, PANEL_MIN } from './panelWidth.js'

describe('clampPanel', () => {
  it.each([
    ['v rozsahu', 600, 1600, 600],
    ['pod minimom', 200, 1600, PANEL_MIN],
    ['zoznamu ostane 320 px', 1500, 1600, 1280],
    ['úzke okno', 900, 600, PANEL_MIN],
    ['nečíslo', 'x', 1600, PANEL_MIN],
    ['zaokrúhli', 500.6, 1600, 501],
  ])('%s', (_, w, vp, want) => {
    expect(clampPanel(w, vp)).toBe(want)
  })
})
