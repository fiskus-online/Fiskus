# Schránka synchronizácie Fiškusa

Telefón mimo domu (Nastavenia → Telefón mimo domu) si vymieňa údaje
s Fiškusom v počítači cez túto schránku. Schránka drží iba **zašifrované**
dáta — kópiu databázy z počítača a frontu nových záznamov z telefónu. Kľúč
na rozšifrovanie majú iba počítač a telefón (je v QR kóde za znakom `#`,
ktorý prehliadač na server neposiela).

- `src/core.js` — logika jedného kanála (spárovania), testy `npm test`
- `src/index.js` — Cloudflare Worker, každý kanál je jeden Durable Object
- rovnaké rozhranie má schránka v pamäti v Go: `backend/internal/relay/memserver.go`

| Požiadavka | Čo robí |
|---|---|
| `GET /v1/c/{kanál}/state` | stav: kópia (verzia, veľkosť), zoznam záznamov, či telefón chce čerstvú kópiu |
| `PUT /v1/c/{kanál}/snapshot` | počítač uloží kópiu (hlavička `X-Version`, najviac 32 MB) |
| `GET /v1/c/{kanál}/snapshot` | telefón stiahne kópiu |
| `POST /v1/c/{kanál}/inbox` | telefón pridá záznam (najviac 512 kB, najviac 500 čakajúcich) |
| `GET`/`DELETE /v1/c/{kanál}/inbox/{id}` | počítač si záznam vyzdvihne a zmaže |
| `POST /v1/c/{kanál}/want` | telefón si pýta čerstvú kópiu |
| `DELETE /v1/c/{kanál}` | zrušenie spárovania |

Prístup: hlavička `Authorization: Bearer <kľúč>`. Prvá požiadavka kanál
založí a schránka si zapamätá iba odtlačok kľúča. Nepoužívaná schránka sa
po 60 dňoch zmaže sama.

## Vlastná schránka

Kto nechce používať schránku Fiškusa, nasadí si vlastnú na bezplatný
Cloudflare (bezplatný plán nič nefakturuje):

1. Účet na [dash.cloudflare.com](https://dash.cloudflare.com) (stačí bezplatný).
2. V tomto priečinku: `npx wrangler@3 login` a `npx wrangler@3 deploy`.
3. Adresu, ktorú wrangler vypíše (`https://fiskus-sync.<účet>.workers.dev`),
   zadajte vo Fiškuse v počítači: Nastavenia → Telefón mimo domu →
   Vlastná schránka. Potom telefón spárujte znova.
