import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface Toast {
  id: number
  kind: 'success' | 'error' | 'info'
  message: string
}

export const useToast = defineStore('toast', () => {
  const items = ref<Toast[]>([])
  let seq = 0
  function push(kind: Toast['kind'], message: string) {
    const id = ++seq
    items.value.push({ id, kind, message })
    setTimeout(() => (items.value = items.value.filter((t) => t.id !== id)), kind === 'error' ? 6500 : 3500)
  }
  return {
    items,
    success: (m: string) => push('success', m),
    error: (e: unknown) => push('error', e instanceof Error ? e.message : String(e)),
    info: (m: string) => push('info', m),
  }
})
