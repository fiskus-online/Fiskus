// Tlač zostavy: obsah sa vloží do stránky (písmo Fiškusa je už načítané),
// pri tlači sa skryje všetko ostatné a orientácia strany sa nastaví cez
// @page. „Uložiť ako PDF“ ponúka dialóg tlače prehliadača.
import { reportCSS } from './report.js'

export function printReport(html, orientation) {
  document.getElementById('report-print')?.remove()
  document.getElementById('report-print-style')?.remove()
  const box = document.createElement('div')
  box.id = 'report-print'
  box.innerHTML = html
  const style = document.createElement('style')
  style.id = 'report-print-style'
  style.textContent = `${reportCSS(orientation)}
@media screen { #report-print { display: none; } }
@media print { html.printing-report body > *:not(#report-print) { display: none !important; } html.printing-report body > #report-print { display: block !important; } }`
  document.head.append(style)
  document.body.append(box)
  document.documentElement.classList.add('printing-report')
  const done = () => {
    document.documentElement.classList.remove('printing-report')
    box.remove()
    style.remove()
    window.removeEventListener('afterprint', done)
  }
  window.addEventListener('afterprint', done)
  window.print()
}
