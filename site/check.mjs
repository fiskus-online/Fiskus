// Kontrola webu: každý lokálny odkaz (href, src, url()) vedie na existujúci
// súbor alebo kotvu na stránke. Spustenie: node site/check.mjs
import { readFileSync, existsSync, readdirSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const dir = dirname(fileURLToPath(import.meta.url))
const errors = []
const pages = readdirSync(dir).filter((f) => f.endsWith('.html'))
const ids = Object.fromEntries(pages.map((p) => [p, new Set([...readFileSync(join(dir, p), 'utf8').matchAll(/id="([^"]+)"/g)].map((m) => m[1]))]))

for (const file of [...pages, 'style.css']) {
  const text = readFileSync(join(dir, file), 'utf8')
  const refs = [...text.matchAll(/(?:href|src)="([^"]+)"|url\('([^']+)'\)/g)].map((m) => m[1] ?? m[2])
  for (const ref of refs) {
    if (/^(https?:|mailto:|data:)/.test(ref)) continue
    const [path, hash] = ref.split('#')
    const target = path === '' || path === './' ? (path === '' ? file : 'index.html') : path
    if (!existsSync(join(dir, target))) errors.push(`${file}: chýba ${ref}`)
    else if (hash && target.endsWith('.html') && !ids[target]?.has(hash)) errors.push(`${file}: chýba kotva ${ref}`)
  }
  if (file.endsWith('.html') && !/<html lang="sk">/.test(text)) errors.push(`${file}: chýba lang="sk"`)
}

// vyhľadávače: titulok, popis a kanonická adresa na každej stránke, stránky
// v sitemap.xml, robots.txt so sitemapou, 404 sa neindexuje, JSON-LD je platný JSON
const sitemap = readFileSync(join(dir, 'sitemap.xml'), 'utf8')
const robots = readFileSync(join(dir, 'robots.txt'), 'utf8')
if (!robots.includes('Sitemap: https://fiskus.online/sitemap.xml')) errors.push('robots.txt: chýba odkaz na sitemap.xml')
for (const file of pages) {
  const text = readFileSync(join(dir, file), 'utf8')
  const title = text.match(/<title>([^<]+)<\/title>/)?.[1] ?? ''
  if (!title) errors.push(`${file}: chýba <title>`)
  else if (title.length > 70) errors.push(`${file}: titulok má ${title.length} znakov (vyhľadávač ukáže okolo 60)`)
  for (const m of text.matchAll(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/g)) {
    try {
      JSON.parse(m[1])
    } catch (e) {
      errors.push(`${file}: neplatné JSON-LD (${e.message})`)
    }
  }
  if (file === '404.html') {
    if (!/<meta name="robots" content="noindex">/.test(text)) errors.push('404.html: chýba noindex')
    continue
  }
  const desc = text.match(/<meta name="description" content="([^"]+)">/)?.[1] ?? ''
  if (desc.length < 50 || desc.length > 170) errors.push(`${file}: popis (meta description) má ${desc.length} znakov, má mať 50 – 170`)
  const url = file === 'index.html' ? 'https://fiskus.online/' : `https://fiskus.online/${file}`
  if (!text.includes(`<link rel="canonical" href="${url}">`)) errors.push(`${file}: chýba kanonická adresa ${url}`)
  if (!sitemap.includes(`<loc>${url}</loc>`)) errors.push(`sitemap.xml: chýba ${url}`)
}
if (errors.length) {
  console.error(errors.join('\n'))
  process.exit(1)
}
console.log(`web v poriadku (stránky: ${pages.length})`)
