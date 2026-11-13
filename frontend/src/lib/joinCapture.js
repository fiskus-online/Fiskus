// Fiškus v prehliadači: odkaz z QR kódu (telefón mimo domu) sa zapamätá
// a adresa sa vyčistí — kľúč nesmie ostať v histórii prehliadača. Musí sa
// načítať pred smerovačom (main.js ho importuje ako prvý), inak by si
// smerovač zapamätal pôvodnú adresu.
import { joinFragment, rememberJoin } from './sync.js'

if (import.meta.env.MODE === 'browser') {
  const join = joinFragment(location.hash)
  if (join) {
    rememberJoin(join)
    history.replaceState(null, '', '/pripojit')
  }
}
