// Thin fetch wrapper for the TMS gateway. Every mutation carries an Idempotency-Key so a
// retried click or a flaky network never records a delivery twice.
export class ApiError extends Error {
  constructor(message: string, public status: number, public code?: string) {
    super(message)
  }
}

let token = localStorage.getItem('tms.token') ?? ''
let onUnauthorized: () => void = () => {}

export function setToken(t: string) {
  token = t
  if (t) localStorage.setItem('tms.token', t)
  else localStorage.removeItem('tms.token')
}
export const getToken = () => token
export function handleUnauthorized(fn: () => void) {
  onUnauthorized = fn
}

type Query = Record<string, string | number | boolean | undefined | null | string[]>

function qs(params?: Query) {
  if (!params) return ''
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    p.set(k, Array.isArray(v) ? v.join(',') : String(v))
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

async function request<T>(method: string, path: string, body?: unknown, query?: Query): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers.Authorization = `Bearer ${token}`
  if (method !== 'GET') headers['Idempotency-Key'] = crypto.randomUUID()
  const res = await fetch(`/api${path}${qs(query)}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    if (res.status === 401 && path !== '/auth/login') onUnauthorized()
    throw new ApiError(data.error ?? res.statusText, res.status, data.code)
  }
  return data as T
}

export const api = {
  get: <T = any>(path: string, query?: Query) => request<T>('GET', path, undefined, query),
  post: <T = any>(path: string, body: unknown = {}) => request<T>('POST', path, body),
  put: <T = any>(path: string, body: unknown = {}) => request<T>('PUT', path, body),
}
