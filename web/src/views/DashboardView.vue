<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Timer, PackageSearch, Truck, CarFront, Plus, Wand2, Thermometer, IdCard, Wrench, ArrowRight } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useContext } from '@/stores/context'
import { useTheme } from '@/stores/theme'
import { seriesColor } from '@/lib/palette'
import { fmt, pct, compactRupiah, titleCase, time } from '@/lib/format'
import StatCard from '@/components/StatCard.vue'
import TrendChart from '@/components/TrendChart.vue'
import StatusPill from '@/components/StatusPill.vue'
import RouteMap from '@/components/RouteMap.vue'
import EmptyState from '@/components/EmptyState.vue'

const auth = useAuth()
const ctx = useContext()
const theme = useTheme()
const router = useRouter()
const d = ref<any>(null)
const error = ref('')
const focus = ref('')

onMounted(async () => {
  try {
    d.value = await api.get('/dashboard', { depot_id: ctx.depotId })
  } catch (e) {
    error.value = (e as Error).message
  }
})

const greeting = computed(() => {
  const h = new Date().getHours()
  return h < 11 ? 'Good morning' : h < 15 ? 'Good afternoon' : 'Good evening'
})
const todayLabel = new Date().toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })
const trips = computed<any[]>(() => d.value?.trips ?? [])
const colorIndex = computed(() => Object.fromEntries(trips.value.map((t, i) => [t.id, i])))
const trend = computed(() => {
  const rows = d.value?.shipments.trend ?? []
  return {
    labels: rows.map((r: any) => new Date(r.date).toLocaleDateString('en-GB', { day: '2-digit', month: 'short' })),
    created: rows.map((r: any) => Number(r.created)),
    delivered: rows.map((r: any) => Number(r.delivered)),
  }
})
const pipelineOrder = ['PENDING', 'TENDERED', 'PLANNED', 'DISPATCHED', 'DELIVERED', 'RETURNED']
const pipeline = computed(() => {
  const counts = new Map<string, number>((d.value?.shipments.status_counts ?? []).map((s: any) => [s.status, Number(s.count)]))
  const rows = pipelineOrder.map((s) => ({ status: s, count: counts.get(s) ?? 0 }))
  const max = Math.max(1, ...rows.map((r) => r.count))
  return rows.map((r) => ({ ...r, width: (r.count / max) * 100 }))
})
const activeTrips = computed(() => trips.value.filter((t) => ['RELEASED', 'IN_PROGRESS'].includes(t.status)).length)
const metrics = computed(() => {
  if (!d.value) return []
  const s = d.value.shipments, p = d.value.planning, f = d.value.fleet
  return [
    { label: 'On-time delivery', value: pct(s.on_time_pct), sub: 'delivered by required date, 30 d' },
    { label: 'OTIF', value: pct(s.otif_pct), sub: 'on time, first attempt' },
    { label: 'Failed delivery rate', value: pct(s.failed_rate_pct), sub: 'returned to depot, 30 d' },
    { label: 'Vehicle utilization', value: pct(p.avg_utilization_pct), sub: 'avg load vs capacity, 7 d' },
    { label: 'Cost per km', value: compactRupiah(p.cost_per_km_7d), sub: `${fmt(p.distance_km_7d)} km in 7 d` },
    { label: 'Stops per trip', value: fmt(p.avg_stops_per_trip, 1), sub: 'average, 7 d' },
    { label: 'Fleet availability', value: pct(f.vehicles ? (Number(f.vehicles_available) / Number(f.vehicles)) * 100 : 0, 0), sub: `${f.vehicles_maintenance} in maintenance` },
    { label: 'Cold-chain excursions', value: fmt(s.cold_chain_excursions_30d), sub: 'readings outside 2–8 °C, 30 d' },
  ]
})
const progress = (t: any) => (t.stops.length ? ((t.delivered_stops + t.failed_stops) / t.stops.length) * 100 : 0)
</script>

<template>
  <div>
    <div class="mb-7 flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="text-[13px] font-semibold text-ink-3">{{ todayLabel }}</p>
        <h1 class="mt-1 text-[26px] font-extrabold tracking-tight">{{ greeting }}, {{ auth.user?.name.split(' ')[0] }} 👋</h1>
        <p class="mt-1 text-[13px] text-ink-3">Deliveries from {{ ctx.depot?.name }} at a glance.</p>
      </div>
      <div class="flex gap-2">
        <RouterLink :to="{ name: 'shipments', query: { new: '1' } }" class="btn-secondary"><Plus class="h-4 w-4" />New shipment</RouterLink>
        <RouterLink :to="{ name: 'planning' }" class="btn-primary"><Wand2 class="h-4 w-4" />Plan deliveries</RouterLink>
      </div>
    </div>

    <p v-if="error" class="card card-pad text-bad">{{ error }}</p>
    <div v-else-if="!d" class="grid gap-5 sm:grid-cols-2 xl:grid-cols-4"><div v-for="i in 8" :key="i" class="card h-36 animate-pulse" /></div>

    <template v-else>
      <div class="grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="On-time delivery" :value="pct(d.shipments.on_time_pct)" :icon="Timer" tone="good" :hint="`${fmt(d.shipments.delivered_today)} delivered today · OTIF ${pct(d.shipments.otif_pct, 0)}`" />
        <StatCard label="Awaiting planning" :value="fmt(d.pending_shipments)" :icon="PackageSearch" tone="warn" :hint="`${fmt(d.pending_weight_kg)} kg in the pool · ${fmt(d.shipments.created_today)} new today`" />
        <StatCard label="Trips on the road" :value="fmt(activeTrips)" :icon="Truck" tone="brand" :hint="`${fmt(d.planning.trips_today)} trips planned today · ${fmt(d.planning.completed_today)} completed`" />
        <StatCard label="Fleet available" :value="`${d.fleet.vehicles_available}/${d.fleet.vehicles}`" :icon="CarFront" tone="info" :hint="`${d.fleet.reefer_vehicles} refrigerated · ${d.fleet.drivers_active} active drivers`" />
      </div>

      <div class="mt-5 grid gap-5 xl:grid-cols-3">
        <section class="card card-pad xl:col-span-2">
          <div class="mb-4 flex items-start justify-between">
            <div><h2 class="card-title">Today's routes</h2><p class="card-sub">Planned (dashed) and running trips · filled stops are delivered</p></div>
            <RouterLink :to="{ name: 'tracking' }" class="text-xs font-semibold text-brand-ink hover:underline">Live tracking</RouterLink>
          </div>
          <div class="h-[520px]"><RouteMap :trips="trips" :depots="ctx.depot ? [ctx.depot] : []" :color-index="colorIndex" :focus="focus" @select="(id) => router.push({ name: 'trips', query: { id } })" /></div>
        </section>
        <section class="card card-pad">
          <div class="mb-3 flex items-start justify-between">
            <div><h2 class="card-title">Trips today</h2><p class="card-sub">Hover to highlight on the map</p></div>
            <RouterLink :to="{ name: 'trips' }" class="text-xs font-semibold text-brand-ink hover:underline">All trips</RouterLink>
          </div>
          <EmptyState v-if="!trips.length" message="No trips planned for today yet" />
          <ul class="space-y-2.5">
            <li v-for="t in trips" :key="t.id" class="cursor-pointer rounded-xl border border-line px-3.5 py-3 transition hover:border-brand/40"
              @mouseenter="focus = t.id" @mouseleave="focus = ''" @click="router.push({ name: 'trips', query: { id: t.id } })">
              <div class="flex items-center gap-2">
                <span class="h-2.5 w-2.5 rounded-full" :style="{ background: seriesColor(colorIndex[t.id], theme.dark) }" aria-hidden="true" />
                <span class="text-[13px] font-bold">{{ t.vehicle_plate }}</span>
                <Thermometer v-if="t.temperature_class === 'COLD'" class="h-3.5 w-3.5 text-info" aria-label="Refrigerated" />
                <StatusPill class="ml-auto" :status="t.status" />
              </div>
              <div class="mt-1 text-xs text-ink-3">{{ t.driver_name }} · {{ t.stops.length }} stops · {{ fmt(t.distance_km) }} km</div>
              <div class="mt-2 flex items-center gap-2">
                <div class="h-1.5 flex-1 rounded-full bg-muted"><div class="h-1.5 rounded-full bg-brand" :style="{ width: progress(t) + '%' }" /></div>
                <span class="text-[11px] tabular-nums text-ink-2">{{ t.delivered_stops + t.failed_stops }}/{{ t.stops.length }}</span>
              </div>
              <div v-if="t.temperature_class === 'COLD' && t.last_ping_at" class="mt-1.5 text-[11px]"
                :class="t.last_temperature_c < 2 || t.last_temperature_c > 8 ? 'font-bold text-bad' : 'text-ink-3'">
                {{ t.last_temperature_c.toFixed(1) }} °C at {{ time(t.last_ping_at) }}
              </div>
            </li>
          </ul>
        </section>
      </div>

      <div class="mt-5 grid gap-5 xl:grid-cols-3">
        <section class="card card-pad xl:col-span-2">
          <h2 class="card-title">Shipments created vs delivered</h2>
          <p class="card-sub mb-4">Per day, last 14 days</p>
          <TrendChart :labels="trend.labels" unit="shipments" :series="[{ name: 'Created', values: trend.created, color: 1 }, { name: 'Delivered', values: trend.delivered, color: 2 }]" />
        </section>
        <section class="card card-pad">
          <div class="mb-4 flex items-start justify-between">
            <div><h2 class="card-title">Shipment pipeline</h2><p class="card-sub">Shipments by status</p></div>
            <RouterLink :to="{ name: 'shipments' }" class="text-xs font-semibold text-brand-ink hover:underline">View all</RouterLink>
          </div>
          <ul class="space-y-3.5">
            <li v-for="p in pipeline" :key="p.status" :title="`${titleCase(p.status)}: ${p.count}`">
              <div class="mb-1 flex items-center justify-between text-[12.5px]"><span class="font-semibold text-ink-2">{{ titleCase(p.status) }}</span><span class="font-bold tabular-nums">{{ p.count }}</span></div>
              <div class="h-2 rounded-full bg-muted"><div class="h-2 rounded-full bg-brand" :style="{ width: `${Math.max(p.width, p.count ? 3 : 0)}%` }" /></div>
            </li>
          </ul>
        </section>
      </div>

      <div class="mt-5 grid gap-5 xl:grid-cols-3">
        <section class="card card-pad">
          <h2 class="card-title">Compliance watch</h2>
          <p class="card-sub">Cold chain, licences and maintenance</p>
          <ul class="mt-4 space-y-2.5">
            <li class="flex items-center gap-3 rounded-xl bg-muted/70 px-3 py-2.5"><Thermometer class="h-4 w-4 text-bad" /><span class="flex-1 text-[13px]">Temperature excursions (30 d)</span><b class="tabular-nums">{{ d.shipments.cold_chain_excursions_30d }}</b></li>
            <li class="flex items-center gap-3 rounded-xl bg-muted/70 px-3 py-2.5"><IdCard class="h-4 w-4 text-warn" /><span class="flex-1 text-[13px]">Licences expiring ≤ 30 days</span><b class="tabular-nums">{{ d.fleet.licenses_expiring_30d }}</b></li>
            <li class="flex items-center gap-3 rounded-xl bg-muted/70 px-3 py-2.5"><IdCard class="h-4 w-4 text-bad" /><span class="flex-1 text-[13px]">Expired licences (blocked from trips)</span><b class="tabular-nums">{{ d.fleet.licenses_expired }}</b></li>
            <li class="flex items-center gap-3 rounded-xl bg-muted/70 px-3 py-2.5"><Wrench class="h-4 w-4 text-info" /><span class="flex-1 text-[13px]">Services due ≤ 14 days</span><b class="tabular-nums">{{ d.fleet.services_due_14d }}</b></li>
          </ul>
        </section>
        <section class="card card-pad">
          <div class="flex items-start justify-between">
            <div><h2 class="card-title">Open exceptions</h2><p class="card-sub">Failed deliveries, excursions, late drops</p></div>
            <RouterLink :to="{ name: 'exceptions' }" class="text-xs font-semibold text-brand-ink hover:underline">Resolve</RouterLink>
          </div>
          <EmptyState v-if="!d.exceptions.exceptions?.length" message="All clear — no open exceptions" />
          <ul v-else class="mt-4 space-y-2.5">
            <li v-for="e in d.exceptions.exceptions" :key="e.id" class="rounded-xl bg-muted/70 px-3 py-2.5">
              <div class="flex items-center justify-between gap-2"><span class="text-[13px] font-bold">{{ titleCase(e.exception_type) }}</span><StatusPill :status="e.severity" /></div>
              <p class="mt-0.5 line-clamp-1 text-xs text-ink-3">{{ e.shipment_number }} · {{ e.details }}</p>
            </li>
          </ul>
        </section>
        <section class="card card-pad">
          <div class="flex items-start justify-between">
            <div><h2 class="card-title">Freight & outsourcing</h2><p class="card-sub">Third-party carriers, last 30 days</p></div>
            <RouterLink :to="{ name: 'freight' }" class="text-xs font-semibold text-brand-ink hover:underline">Tenders</RouterLink>
          </div>
          <dl class="mt-4 grid grid-cols-2 gap-3">
            <div class="rounded-xl bg-muted/70 p-3"><dt class="text-[11.5px] font-semibold text-ink-3">Carrier spend</dt><dd class="mt-1 text-[18px] font-extrabold">{{ compactRupiah(d.freight.spend_30d) }}</dd></div>
            <div class="rounded-xl bg-muted/70 p-3"><dt class="text-[11.5px] font-semibold text-ink-3">Open tenders</dt><dd class="mt-1 text-[18px] font-extrabold">{{ d.freight.open_tenders }}</dd></div>
            <div class="rounded-xl bg-muted/70 p-3"><dt class="text-[11.5px] font-semibold text-ink-3">Savings vs avg bid</dt><dd class="mt-1 text-[18px] font-extrabold">{{ pct(d.freight.avg_savings_pct, 0) }}</dd></div>
            <div class="rounded-xl bg-muted/70 p-3"><dt class="text-[11.5px] font-semibold text-ink-3">Outsourced share</dt><dd class="mt-1 text-[18px] font-extrabold">{{ pct(d.shipments.outsourced_pct, 0) }}</dd></div>
          </dl>
          <RouterLink :to="{ name: 'analytics' }" class="mt-3 inline-flex items-center gap-1 text-xs font-semibold text-brand-ink hover:underline">Cost analytics <ArrowRight class="h-3.5 w-3.5" /></RouterLink>
        </section>
      </div>

      <section class="card card-pad mt-5">
        <h2 class="card-title">Success metrics</h2>
        <p class="card-sub">Baseline KPIs from the BRD (§9)</p>
        <dl class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-4">
          <div v-for="m in metrics" :key="m.label" class="rounded-xl bg-muted/70 p-3">
            <dt class="text-[11.5px] font-semibold text-ink-3">{{ m.label }}</dt>
            <dd class="mt-1 text-[20px] font-extrabold tabular-nums">{{ m.value }}</dd>
            <dd class="text-[11px] text-ink-3">{{ m.sub }}</dd>
          </div>
        </dl>
      </section>
    </template>
  </div>
</template>
