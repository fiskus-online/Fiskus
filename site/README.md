# fiskus.online

Statický web (HTML, CSS a trochu JavaScriptu, bez zostavovania). Písmo Source Sans 3 je uložené v `fonts/`, takže web nenačítava nič od tretích strán.

Web nemá cenník, predaj ani formuláre.

- `config.js` — verzia na stiahnutie a adresa inštalátorov. Pri nasadení ich workflow `pages.yml` doplní z posledného vydania na GitHube.
- `ochrana-osobnych-udajov.html` — **návrh**, údaje v hranatých zátvorkách treba doplniť.
- `obrazky/` — snímky obrazovky z ukážkových dát; obnovia sa spolu so snímkami návodu príkazom `make snimky`.
- `robots.txt`, `sitemap.xml` — pre vyhľadávače; novú stránku treba pridať do `sitemap.xml` a dať jej `<link rel="canonical">` a `<meta name="description">`.
- `obrazky/og.jpg` — náhľad pri zdieľaní odkazu (1200 × 630).
- `404.html` — stránka pre neexistujúcu adresu (Cloudflare Pages ju vráti sama), neindexuje sa.
- `node site/check.mjs` — kontrola odkazov a údajov pre vyhľadávače (titulok, popis, kanonická adresa, sitemap, JSON-LD); beží v CI.

## Návštevnosť a vyhľadávače

- Návštevnosť meria Cloudflare Web Analytics (bez cookies). Zapína sa v Cloudflare: Workers & Pages → projekt `fiskus` (a `fiskus-docs`) → Metrics → Web Analytics → Enable. Cloudflare potom skript vkladá do stránok sám, v kóde nič nie je.
- Google Search Console: doména `fiskus.online` overená záznamom TXT v DNS (pokrýva aj docs a app), sitemapy `https://fiskus.online/sitemap.xml` a `https://docs.fiskus.online/sitemap.xml`.

## Nasadenie

Web sa nasadzuje na Cloudflare Pages workflowom `.github/workflows/pages.yml` (projekt `fiskus`, doména fiskus.online). Návod (`docs/`) ide na docs.fiskus.online a Fiškus v prehliadači na app.fiskus.online.
