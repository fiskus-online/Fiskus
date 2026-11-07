# fiskus.online

Statický web (HTML, CSS a trochu JavaScriptu, bez zostavovania). Písmo Source Sans 3 je uložené v `fonts/`, takže web nenačítava nič od tretích strán.

Web nemá cenník, predaj ani formuláre.

- `config.js` — verzia na stiahnutie a adresa inštalátorov. Pri nasadení ich workflow `pages.yml` doplní z posledného vydania na GitHube.
- `ochrana-osobnych-udajov.html` — **návrh**, údaje v hranatých zátvorkách treba doplniť.
- `node site/check.mjs` — kontrola odkazov (beží v CI).

## Nasadenie

Web sa nasadzuje na Cloudflare Pages workflowom `.github/workflows/pages.yml` (projekt `fiskus`, doména fiskus.online). Návod (`docs/`) ide na docs.fiskus.online a Fiškus v prehliadači na app.fiskus.online.
