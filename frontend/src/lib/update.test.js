import { describe, expect, it } from 'vitest'
import { pending, percent, progressText, restarted, statusLine, updating } from './update.js'

describe('aktualizácia', () => {
  it.each([
    ['downloading', true],
    ['installing', true],
    ['restarting', true],
    ['available', false],
    ['current', false],
    ['error', false],
  ])('prebieha %s', (state, want) => expect(updating({ state })).toBe(want))

  it('čaká na kontrolu', () => {
    expect(pending(null)).toBe(true)
    expect(pending({ state: '' })).toBe(true)
    expect(pending({ state: 'checking' })).toBe(true)
    expect(pending({ state: 'current' })).toBe(false)
  })

  it.each([
    [0.456, 46],
    [1.2, 100],
    [-1, 0],
    [undefined, 0],
  ])('percentá %s', (progress, want) => expect(percent({ progress })).toBe(want))

  it('texty priebehu', () => {
    expect(progressText({ state: 'downloading', latest: '0.8.0', progress: 0.5 }).title).toBe('Sťahujem Fiškus 0.8.0… 50 %')
    expect(progressText({ state: 'installing', latest: '0.8.0' }).title).toBe('Inštalujem Fiškus 0.8.0…')
    expect(progressText({ state: 'current' }).title).toBe('')
  })

  it.each([
    [{ state: 'current', current: '0.7.0' }, 'Fiškus 0.7.0 je aktuálny.'],
    [{ state: 'available', current: '0.7.0', latest: '0.8.0' }, 'Je k dispozícii Fiškus 0.8.0 (máte 0.7.0).'],
    [{ state: 'error', error: 'internet?' }, 'Aktualizácia zlyhala: internet?'],
    [{ state: '', current: '0.7.0' }, 'Fiškus 0.7.0. Nová verzia sa ešte nezisťovala.'],
  ])('stav %#', (st, want) => expect(statusLine(st)).toBe(want))

  it('nový program beží', () => {
    expect(restarted('0.7.0', { version: 'v0.8.0' })).toBe(true)
    expect(restarted('0.7.0', { version: '0.7.0' })).toBe(false)
    expect(restarted('0.7.0', null)).toBe(false)
  })
})
