import { defineStore } from 'pinia'
import { ref, watchEffect } from 'vue'

export const useTheme = defineStore('theme', () => {
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  const saved = (() => {
    try { return localStorage.getItem('tms.theme') } catch { return null }
  })()
  const dark = ref(saved ? saved === 'dark' : media.matches)
  /** Bumped whenever the palette changes so canvas charts can re-read CSS variables. */
  const version = ref(0)
  watchEffect(() => {
    document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'
    version.value++
  })
  function toggle() {
    dark.value = !dark.value
    try { localStorage.setItem('tms.theme', dark.value ? 'dark' : 'light') } catch { /* private mode */ }
  }
  return { dark, version, toggle }
})
