import { copyFileSync, existsSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// npm run build:browser (make browser) zostaví Fiškus v prehliadači: rozhranie,
// service worker (src/browser/sw.js) a program v Go ako WebAssembly
// (src/browser/fiskus.wasm.gz a wasm_exec.js pripraví make browser).

// browserFiles: service worker do koreňa webu a manifest aplikácie, ktorá
// sa otvára na úvodnej stránke (nie na mobilných bločkoch /m).
const browserFiles = () => ({
  name: 'fiskus-browser',
  apply: 'build',
  writeBundle({ dir }) {
    copyFileSync(new URL('./src/browser/sw.js', import.meta.url), `${dir}/sw.js`)
    const m = JSON.parse(readFileSync(new URL('./public/manifest.webmanifest', import.meta.url), 'utf8'))
    Object.assign(m, { description: 'Účtovníctvo Fiškus v prehliadači — údaje iba vo vašom počítači', start_url: '/' })
    writeFileSync(`${dir}/manifest.webmanifest`, JSON.stringify(m, null, 2) + '\n')
    // hosting bez presmerovania adries aplikácie (GitHub Pages): /faktury/12 → index.html
    copyFileSync(`${dir}/index.html`, `${dir}/404.html`)
    // Cloudflare Pages a Netlify: súbory s odtlačkom navždy v cache, service worker nie
    writeFileSync(`${dir}/_headers`, [
      '/assets/*', '  Cache-Control: public, max-age=31536000, immutable',
      '/sw.js', '  Cache-Control: no-cache',
      '/index.html', '  Cache-Control: no-cache',
      '',
    ].join('\n'))
  },
})

export default defineConfig(({ mode }) => {
  const browser = mode === 'browser'
  const wasm = new URL('./src/browser/fiskus.wasm.gz', import.meta.url)
  return {
    plugins: browser ? [vue(), browserFiles()] : [vue()],
    server: {
      port: 5173,
      // pri vývoji beží Go API zvlášť: fiskus -demo
      proxy: browser ? undefined : { '/api': 'http://127.0.0.1:8080' },
    },
    define: browser ? { __WASM_SIZE__: existsSync(wasm) ? statSync(wasm).size : 0 } : {},
    optimizeDeps: { exclude: ['@sqlite.org/sqlite-wasm'] },
    worker: { format: 'es' },
    build: {
      outDir: browser ? 'dist-browser' : 'dist',
      chunkSizeWarningLimit: 2000,
    },
    test: {
      environment: 'jsdom',
    },
  }
})
