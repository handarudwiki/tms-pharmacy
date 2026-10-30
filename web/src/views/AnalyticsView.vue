<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '@/api/client'
import { useContext } from '@/stores/context'
import { fmt, pct, rupiah, compactRupiah, titleCase } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import TrendChart from '@/components/TrendChart.vue'
import EmptyState from '@/components/EmptyState.vue'

const ctx = useContext()
const trips = ref<any[]>([])
const freight = ref<any>(null)
const ship = ref<any>(null)

onMounted(async () => {
  const [t, f, s] = await Promise.all([
    api.get('/trips', { depot_id: ctx.depotId, limit: 500 }),
    api.get('/dashboard', { depot_id: ctx.depotId }),
    Promise.resolve(null),
  ])
  trips.value = (t.trips ?? []).filter((x: any) => x.status !== 'CANCELLED')
  freight.value = f.freight
  ship.value = f.shipments
  void s
})

const days = computed(() => {
  const out: { date: string; km: number; cost: number; trips: number; stops: number }[] = []
  for (let i = 13; i >= 0; i--) {
    const d = new Date(); d.setDate(d.getDate() - i)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    const list = trips.value.filter((t) => t.plan_date === key)
    out.push({ date: key, km: list.reduce((a, t) => a + t.distance_km, 0), cost: list.reduce((a, t) => a + t.estimated_cost, 0), trips: list.length, stops: list.reduce((a, t) => a + t.stops.length, 0) })
  }
  return out
})
const labels = computed(() => days.value.map((d) => new Date(d.date + 'T00:00:00').toLocaleDateString('en-GB', { day: '2-digit', month: 'short' })))
const byVehicle = computed(() => {
  const m = new Map<string, any>()
  for (const t of trips.value) {
    const r = m.get(t.vehicle_plate) ?? { plate: t.vehicle_plate, trips: 0, km: 0, cost: 0, util: 0, stops: 0, delivered: 0, failed: 0, cold: t.temperature_class === 'COLD' }
    r.trips++; r.km += t.distance_km; r.cost += t.estimated_cost; r.util += t.capacity_util_pct; r.stops += t.stops.length; r.delivered += t.delivered_stops; r.failed += t.failed_stops
    m.set(t.vehicle_plate, r)
  }
  return [...m.values()].map((r) => ({ ...r, util: r.util / r.trips, costPerKm: r.km ? r.cost / r.km : 0 })).sort((a, b) => b.km - a.km)
})
const failures = computed(() => {
  const m = new Map<string, number>()
  for (const t of trips.value) for (const s of t.stops) if (s.status === 'FAILED') m.set(s.failure_reason, (m.get(s.failure_reason) ?? 0) + 1)
  const total = [...m.values()].reduce((a, b) => a + b, 0)
  return [...m.entries()].map(([reason, n]) => ({ reason, n, share: total ? (n / total) * 100 : 0 })).sort((a, b) => b.n - a.n)
})
const totals = computed(() => {
  const t = trips.value
  const km = t.reduce((a, x) => a + x.distance_km, 0), cost = t.reduce((a, x) => a + x.estimated_cost, 0)
  const stops = t.reduce((a, x) => a + x.stops.length, 0)
  return { km, cost, stops, trips: t.length, costPerStop: stops ? cost / stops : 0, costPerKm: km ? cost / km : 0 }
})
const maxSpend = computed(() => Math.max(1, ...(freight.value?.by_carrier ?? []).map((c: any) => c.spend)))
</script>

<template>
  <div>
    <PageHeader title="Analytics" subtitle="Distance, cost, utilization, delivery performance and freight spend" />
    <div class="mb-5 grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
      <div class="card card-pad"><div class="text-[13px] font-semibold text-ink-2">Own-fleet cost</div><div class="mt-2 text-[28px] font-extrabold">{{ compactRupiah(totals.cost) }}</div><div class="text-xs text-ink-3">{{ totals.trips }} trips · {{ fmt(totals.km) }} km</div></div>
      <div class="card card-pad"><div class="text-[13px] font-semibold text-ink-2">Cost per drop</div><div class="mt-2 text-[28px] font-extrabold">{{ compactRupiah(totals.costPerStop) }}</div><div class="text-xs text-ink-3">{{ fmt(totals.stops) }} stops · {{ rupiah(totals.costPerKm) }}/km</div></div>
      <div class="card card-pad"><div class="text-[13px] font-semibold text-ink-2">On-time / OTIF</div><div class="mt-2 text-[28px] font-extrabold">{{ pct(ship?.on_time_pct, 0) }} <span class="text-[16px] text-ink-3">/ {{ pct(ship?.otif_pct, 0) }}</span></div><div class="text-xs text-ink-3">last 30 days</div></div>
      <div class="card card-pad"><div class="text-[13px] font-semibold text-ink-2">Carrier spend</div><div class="mt-2 text-[28px] font-extrabold">{{ compactRupiah(freight?.spend_30d) }}</div><div class="text-xs text-ink-3">{{ pct(ship?.outsourced_pct, 0) }} of deliveries outsourced</div></div>
    </div>

    <div class="grid gap-5 xl:grid-cols-2">
      <section class="card card-pad"><h2 class="card-title">Distance driven per day</h2><p class="card-sub mb-3">Own fleet, kilometres, last 14 days</p>
        <TrendChart :labels="labels" unit="km" :series="[{ name: 'Distance (km)', values: days.map((d) => Math.round(d.km)), color: 1 }]" /></section>
      <section class="card card-pad"><h2 class="card-title">Estimated trip cost per day</h2><p class="card-sub mb-3">Fuel, driver time and fixed cost, IDR</p>
        <TrendChart :labels="labels" unit="IDR" :series="[{ name: 'Cost (IDR)', values: days.map((d) => Math.round(d.cost)), color: 2 }]" /></section>
    </div>

    <div class="mt-5 grid gap-5 xl:grid-cols-3">
      <section class="card card-pad xl:col-span-2">
        <h2 class="card-title">Vehicle performance</h2>
        <p class="card-sub">All recorded trips</p>
        <EmptyState v-if="!byVehicle.length" message="No trips yet" />
        <div v-else class="table-wrap mt-3"><table class="tbl">
          <thead><tr><th>Vehicle</th><th class="num">Trips</th><th class="num">Km</th><th class="w-[28%]">Avg utilization</th><th class="num">Cost/km</th><th class="num">Delivered</th><th class="num">Failed</th></tr></thead>
          <tbody><tr v-for="v in byVehicle" :key="v.plate">
            <td class="font-bold">{{ v.plate }}<span v-if="v.cold" class="ml-1 text-[11px] font-semibold text-info">reefer</span></td>
            <td class="num">{{ v.trips }}</td><td class="num">{{ fmt(v.km) }}</td>
            <td><div class="flex items-center gap-2"><div class="h-2 flex-1 rounded-full bg-muted"><div class="h-2 rounded-full bg-brand" :style="{ width: Math.min(100, v.util) + '%' }" /></div><span class="w-10 text-right text-xs tabular-nums">{{ fmt(v.util) }}%</span></div></td>
            <td class="num">{{ rupiah(v.costPerKm) }}</td><td class="num">{{ v.delivered }}</td><td class="num" :class="v.failed ? 'font-bold text-bad' : ''">{{ v.failed }}</td>
          </tr></tbody>
        </table></div>
      </section>
      <div class="space-y-5">
        <section class="card card-pad">
          <h2 class="card-title">Carrier spend</h2><p class="card-sub">Awarded tenders</p>
          <ul class="mt-4 space-y-3">
            <li v-for="c in freight?.by_carrier ?? []" :key="c.carrier_id">
              <div class="mb-1 flex justify-between text-[12.5px]"><span class="font-semibold">{{ c.carrier_name }}</span><span class="tabular-nums">{{ compactRupiah(c.spend) }}</span></div>
              <div class="h-2 rounded-full bg-muted"><div class="h-2 rounded-full bg-brand" :style="{ width: (c.spend / maxSpend) * 100 + '%' }" /></div>
            </li>
          </ul>
        </section>
        <section class="card card-pad">
          <h2 class="card-title">Failed delivery reasons</h2>
          <EmptyState v-if="!failures.length" message="No failed deliveries" />
          <ul class="mt-4 space-y-3">
            <li v-for="f in failures" :key="f.reason">
              <div class="mb-1 flex justify-between text-[12.5px]"><span class="font-semibold">{{ titleCase(f.reason) }}</span><span class="tabular-nums">{{ f.n }} · {{ pct(f.share, 0) }}</span></div>
              <div class="h-2 rounded-full bg-muted"><div class="h-2 rounded-full bg-serious" :style="{ width: f.share + '%' }" /></div>
            </li>
          </ul>
        </section>
      </div>
    </div>
  </div>
</template>
