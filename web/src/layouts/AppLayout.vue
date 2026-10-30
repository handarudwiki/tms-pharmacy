<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  LayoutDashboard, Radar, Package, Route, Truck, Smartphone, Handshake, CarFront, AlertOctagon, BarChart3, Plug, Users,
  ScrollText, Menu, X, Moon, Sun, LogOut, Search, ChevronDown, Building2,
} from 'lucide-vue-next'
import { useAuth } from '@/stores/auth'
import { useContext } from '@/stores/context'
import { useTheme } from '@/stores/theme'
import { api } from '@/api/client'
import { titleCase } from '@/lib/format'

const auth = useAuth()
const ctx = useContext()
const theme = useTheme()
const route = useRoute()
const router = useRouter()
const mobileOpen = ref(false)
const openExceptions = ref(0)
const search = ref('')

const office = ['TRANSPORT_MANAGER', 'DISPATCHER', 'FLEET_MANAGER', 'CARRIER_MANAGER', 'CUSTOMER_SERVICE', 'TRANSPORT_ANALYST', 'INTEGRATION_ADMIN']
const nav = [
  { group: 'Overview', items: [
    { to: 'dashboard', label: 'Dashboard', icon: LayoutDashboard, roles: office },
    { to: 'tracking', label: 'Live Tracking', icon: Radar, roles: office },
  ] },
  { group: 'Operations', items: [
    { to: 'shipments', label: 'Shipments', icon: Package, roles: office },
    { to: 'planning', label: 'Planning', icon: Route, roles: office },
    { to: 'trips', label: 'Trips', icon: Truck, roles: office },
    { to: 'driver', label: 'Driver App', icon: Smartphone, roles: ['DRIVER', 'DISPATCHER', 'TRANSPORT_MANAGER'] },
  ] },
  { group: 'Resources', items: [
    { to: 'fleet', label: 'Fleet & Drivers', icon: CarFront, roles: office },
    { to: 'freight', label: 'Carriers & Tenders', icon: Handshake, roles: office },
  ] },
  { group: 'Control', items: [
    { to: 'exceptions', label: 'Exceptions', icon: AlertOctagon, roles: office, badge: true },
    { to: 'analytics', label: 'Analytics', icon: BarChart3, roles: office },
  ] },
  { group: 'Setup', items: [
    { to: 'integrations', label: 'Integrations', icon: Plug, roles: ['INTEGRATION_ADMIN', 'TRANSPORT_MANAGER'] },
    { to: 'users', label: 'Users & Roles', icon: Users, roles: ['ORG_ADMIN'] },
    { to: 'audit', label: 'Audit Trail', icon: ScrollText, roles: ['TRANSPORT_MANAGER', 'TRANSPORT_ANALYST'] },
  ] },
]
const visibleNav = computed(() => nav.map((g) => ({ ...g, items: g.items.filter((i) => auth.can(...i.roles)) })).filter((g) => g.items.length))
const initials = computed(() => (auth.user?.name ?? '?').split(' ').map((p) => p[0]).slice(0, 2).join('').toUpperCase())

async function loadBadge() {
  if (!ctx.depotId || auth.isDriverOnly) return
  try {
    openExceptions.value = ((await api.get('/exceptions', { depot_id: ctx.depotId, status: 'OPEN' })).exceptions ?? []).length
  } catch { /* badge is non-critical */ }
}
onMounted(async () => {
  if (!ctx.loaded) await ctx.load()
  loadBadge()
})
watch(() => route.fullPath, () => { mobileOpen.value = false; loadBadge() })

function onSearch() {
  const q = search.value.trim()
  if (!q) return
  router.push({ name: q.toUpperCase().startsWith('TRP') ? 'trips' : 'shipments', query: { q } })
  search.value = ''
}
function logout() {
  auth.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="min-h-screen lg:pl-[264px]">
    <aside :class="['fixed inset-y-0 left-0 z-40 flex w-[264px] flex-col border-r border-line bg-surface transition-transform lg:translate-x-0',
      mobileOpen ? 'translate-x-0' : '-translate-x-full']">
      <div class="flex h-[72px] items-center gap-3 px-6">
        <span class="grid h-9 w-9 place-items-center rounded-xl bg-brand text-white"><Truck class="h-[18px] w-[18px]" /></span>
        <div class="leading-tight">
          <div class="text-[15px] font-extrabold tracking-tight">PharmaTMS</div>
          <div class="max-w-[160px] truncate text-[11px] text-ink-3">{{ auth.orgName }}</div>
        </div>
        <button class="btn-ghost btn-sm ml-auto lg:hidden" aria-label="Close menu" @click="mobileOpen = false"><X class="h-4 w-4" /></button>
      </div>
      <nav class="flex-1 overflow-y-auto px-4 pb-6">
        <div v-for="g in visibleNav" :key="g.group" class="mt-4">
          <div class="px-3 pb-1.5 text-[10.5px] font-bold uppercase tracking-[0.12em] text-ink-3">{{ g.group }}</div>
          <RouterLink v-for="item in g.items" :key="item.to" :to="{ name: item.to }"
            :class="['group flex items-center gap-3 rounded-xl px-3 py-2 text-[13.5px] font-semibold transition',
              route.name === item.to ? 'bg-brand text-white shadow-[0_6px_16px_-6px_rgb(var(--brand)/.6)]' : 'text-ink-2 hover:bg-muted hover:text-ink']">
            <component :is="item.icon" class="h-[18px] w-[18px]" />
            <span class="flex-1">{{ item.label }}</span>
            <span v-if="'badge' in item && openExceptions"
              :class="['rounded-full px-2 text-[11px] font-bold', route.name === item.to ? 'bg-white/25 text-white' : 'bg-bad-soft text-bad']">{{ openExceptions }}</span>
          </RouterLink>
        </div>
      </nav>
      <div class="m-4 rounded-2xl bg-muted p-4">
        <div class="flex items-center gap-3">
          <span class="grid h-9 w-9 place-items-center rounded-full bg-brand-soft text-[12px] font-bold text-brand-ink">{{ initials }}</span>
          <div class="min-w-0 flex-1 leading-tight">
            <div class="truncate text-[13px] font-bold">{{ auth.user?.name }}</div>
            <div class="truncate text-[11px] text-ink-3">{{ titleCase(auth.user?.roles[0]) }}</div>
          </div>
          <button class="btn-ghost btn-sm px-2" aria-label="Sign out" title="Sign out" @click="logout"><LogOut class="h-4 w-4" /></button>
        </div>
      </div>
    </aside>
    <div v-if="mobileOpen" class="fixed inset-0 z-30 bg-black/30 lg:hidden" @click="mobileOpen = false" />

    <header class="sticky top-0 z-20 border-b border-line/70 bg-page/85 backdrop-blur">
      <div class="flex h-[72px] items-center gap-3 px-4 sm:px-8">
        <button class="btn-secondary btn-sm px-2 lg:hidden" aria-label="Open menu" @click="mobileOpen = true"><Menu class="h-4 w-4" /></button>
        <form v-if="!auth.isDriverOnly" class="relative hidden max-w-md flex-1 sm:block" @submit.prevent="onSearch">
          <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-3" />
          <input v-model="search" class="input h-10 rounded-xl pl-9" placeholder="Search shipment, consignee or trip (TRP-…)" />
        </form>
        <div class="ml-auto flex items-center gap-2">
          <label v-if="!auth.isDriverOnly" class="relative">
            <span class="sr-only">Depot</span>
            <Building2 class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-3" />
            <select :value="ctx.depotId" class="input h-10 w-[150px] appearance-none truncate pl-9 pr-8 font-semibold sm:w-[290px]"
              @change="ctx.selectDepot(($event.target as HTMLSelectElement).value)">
              <option v-for="d in ctx.depots" :key="d.id" :value="d.id">{{ d.code }} · {{ d.name }}</option>
            </select>
            <ChevronDown class="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-3" />
          </label>
          <button class="btn-secondary h-10 w-10 px-0" :aria-label="theme.dark ? 'Light mode' : 'Dark mode'" @click="theme.toggle()">
            <Sun v-if="theme.dark" class="h-4 w-4" /><Moon v-else class="h-4 w-4" />
          </button>
          <RouterLink v-if="!auth.isDriverOnly" :to="{ name: 'exceptions' }" class="btn-secondary relative h-10 w-10 px-0" aria-label="Open exceptions">
            <AlertOctagon class="h-4 w-4" />
            <span v-if="openExceptions" class="absolute -right-1 -top-1 grid h-5 min-w-5 place-items-center rounded-full bg-bad px-1 text-[10px] font-bold text-white">{{ openExceptions }}</span>
          </RouterLink>
        </div>
      </div>
    </header>

    <main class="px-4 py-6 sm:px-8 sm:py-8">
      <div v-if="!ctx.loaded" class="py-24 text-center text-ink-3">Loading workspace…</div>
      <div v-else-if="!ctx.depotId && route.name !== 'fleet'" class="card card-pad mx-auto max-w-lg text-center">
        <h2 class="card-title">No depot yet</h2>
        <p class="card-sub">Create a depot under Fleet & Drivers to start planning deliveries.</p>
        <RouterLink :to="{ name: 'fleet', query: { tab: 'depots' } }" class="btn-primary mt-4">Set up a depot</RouterLink>
      </div>
      <RouterView v-else :key="ctx.depotId" />
    </main>
  </div>
</template>
