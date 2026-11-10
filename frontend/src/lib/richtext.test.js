import { describe, expect, it } from 'vitest'
import cases from '../../../backend/internal/richtext/testdata/cases.json'
import { escapeHTML, isRich, plain, sanitize, toHTML } from './richtext.js'

// rovnaké vzorky ako server (backend/internal/richtext)
describe('richtext — spoločné vzorky so serverom', () => {
  it.each(cases.map((c) => [c.name, c]))('%s', (_, c) => {
    const got = sanitize(c.in)
    expect(got).toBe(c.out)
    expect(sanitize(got)).toBe(got)
    expect(plain(got)).toBe(c.plain)
  })
})

describe('richtext', () => {
  it('obyčajný text na zobrazenie', () => {
    expect(toHTML('Riadok <1>\n\nRiadok 2')).toBe('<p>Riadok &lt;1&gt;</p><p><br></p><p>Riadok 2</p>')
    expect(toHTML('')).toBe('')
    expect(toHTML('<p><b>x</b></p>')).toBe('<p><strong>x</strong></p>')
    expect(isRich(' <p>x</p>')).toBe(true)
    expect(isRich('a < b')).toBe(false)
    expect(escapeHTML(`a&<>'"`)).toBe('a&amp;&lt;&gt;&#39;&#34;')
  })
})
