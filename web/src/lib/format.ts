const nf = new Intl.NumberFormat('en-US')
const idr = new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 })

export const fmt = (n: number | string | undefined | null, digits = 0) =>
  new Intl.NumberFormat('en-US', { maximumFractionDigits: digits }).format(Number(n ?? 0))
export const rupiah = (n: number | undefined | null) => idr.format(Number(n ?? 0))
export const compactRupiah = (n: number | undefined | null) => {
  const v = Number(n ?? 0)
  if (v >= 1e9) return `Rp ${(v / 1e9).toFixed(1)} M`
  if (v >= 1e6) return `Rp ${(v / 1e6).toFixed(1)} jt`
  return `Rp ${nf.format(Math.round(v))}`
}

export function pct(n: number | undefined, digits = 1) {
  return `${(n ?? 0).toFixed(digits)}%`
}

export function dateTime(s?: string) {
  if (!s) return '—'
  return new Date(s).toLocaleString('en-GB', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })
}

export function time(s?: string) {
  if (!s) return '—'
  return new Date(s).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })
}

export function day(s?: string) {
  if (!s) return '—'
  return new Date(s.length === 10 ? s + 'T00:00:00' : s).toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' })
}

export function daysUntil(s?: string) {
  if (!s) return null
  const ms = new Date(s + 'T00:00:00').getTime() - new Date().setHours(0, 0, 0, 0)
  return Math.round(ms / 86400000)
}

export function relative(s?: string) {
  if (!s) return '—'
  const diff = (Date.now() - new Date(s).getTime()) / 1000
  if (diff < 60) return 'just now'
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

export const titleCase = (s?: string) =>
  (s ?? '').toLowerCase().replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())

export const today = (offset = 0) => {
  const d = new Date()
  d.setDate(d.getDate() + offset)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
