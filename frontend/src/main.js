import './lib/joinCapture.js' // ako prvý — pred smerovačom
import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import Tooltip from 'primevue/tooltip'
import { ModuleRegistry, AllCommunityModule } from 'ag-grid-community'

import '@fontsource/source-sans-3/400.css'
import '@fontsource/source-sans-3/500.css'
import '@fontsource/source-sans-3/600.css'
import '@fontsource/source-sans-3/700.css'
import 'primeicons/primeicons.css'
import './styles.css'

import App from './App.vue'
import { router } from './router.js'
import { FiskusPreset, skLocale } from './theme.js'

ModuleRegistry.registerModules([AllCommunityModule])

function mount() {
  createApp(App)
    .use(router)
    .use(PrimeVue, {
      theme: { preset: FiskusPreset, options: { darkModeSelector: '.fiskus-dark' } },
      locale: skLocale,
    })
    .use(ToastService)
    .use(ConfirmationService)
    .directive('tooltip', Tooltip)
    .mount('#app')
}

if (import.meta.env.MODE === 'browser') {
  // Fiškus v prehliadači: najprv spustiť program (WebAssembly) a databázu
  import('./browser/boot.js').then(({ boot }) => boot(document.getElementById('app')).then((ok) => ok && mount()))
} else {
  mount()
}
