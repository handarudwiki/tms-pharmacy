import { ref } from 'vue'
import { useToast } from '@/stores/toast'

/** Wraps an async mutation with a busy flag and success / error toasts. */
export function useAction() {
  const busy = ref(false)
  const toast = useToast()
  async function run<T>(fn: () => Promise<T>, success?: string): Promise<T | undefined> {
    busy.value = true
    try {
      const res = await fn()
      if (success) toast.success(success)
      return res
    } catch (e) {
      toast.error(e)
      return undefined
    } finally {
      busy.value = false
    }
  }
  return { busy, run }
}
