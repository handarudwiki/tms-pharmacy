import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, setToken } from '@/api/client'

export interface User {
  id: string
  email: string
  name: string
  roles: string[]
  depot_ids: string[]
  active: boolean
}

export const useAuth = defineStore('auth', () => {
  const stored = localStorage.getItem('tms.session')
  const session = ref<{ user: User; org_name: string } | null>(stored ? JSON.parse(stored) : null)

  const user = computed(() => session.value?.user ?? null)
  const orgName = computed(() => session.value?.org_name ?? '')
  const isAuthenticated = computed(() => !!session.value)

  async function login(email: string, password: string) {
    const res = await api.post('/auth/login', { email, password })
    setToken(res.token)
    session.value = { user: res.user, org_name: res.org_name }
    localStorage.setItem('tms.session', JSON.stringify(session.value))
  }

  function logout() {
    setToken('')
    session.value = null
    localStorage.removeItem('tms.session')
  }

  /** Mirrors the gateway's rule: org admins pass every role check. */
  function can(...roles: string[]) {
    const mine = user.value?.roles ?? []
    return mine.includes('ORG_ADMIN') || roles.some((r) => mine.includes(r))
  }

  /** Drivers without office roles land straight in the driver app. */
  const isDriverOnly = computed(() => !!user.value && user.value.roles.every((r) => r === 'DRIVER'))

  return { user, orgName, isAuthenticated, isDriverOnly, login, logout, can }
})
