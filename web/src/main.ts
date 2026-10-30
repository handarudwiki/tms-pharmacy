import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { handleUnauthorized } from './api/client'
import { useAuth } from './stores/auth'
import './style.css'

const app = createApp(App).use(createPinia()).use(router)
handleUnauthorized(() => {
  useAuth().logout()
  router.push({ name: 'login' })
})
app.mount('#app')
