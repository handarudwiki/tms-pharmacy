<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { RefreshCw, Thermometer, MapPin } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useContext } from '@/stores/context'
import { useTheme } from '@/stores/theme'
import { seriesColor } from '@/lib/palette'
import { time, relative } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import StatusPill from '@/components/StatusPill.vue'
import EmptyState from '@/components/EmptyState.vue'
import RouteMap from '@/components/RouteMap.vue'

const ctx = useContext()
const theme = useTheme()
const router = useRouter()
const trips = ref<any[]>([])
const focus = ref('')
let timer: number | undefined

async function load() {
  trips.value = (await api.get('/trips', { depot_id: ctx.depotId, statuses: ['RELEASED', 'IN_PROGRESS'] })).trips ?? []
}
onMounted(() => {
  load()
  timer = window.setInterval(load, 30000)
})
onUnmounted(() => window.clearInterval(timer))
const colorIndex = computed(() => Object.fromEntries(trips.value.map((t, i) => [t.id, i])))
const next = (t: any) => t.stops.find((s: any) => s.status === 'PENDING' || s.status === 'ARRIVED')
const excursion = (t: any) => t.temperature_class === 'COLD' && t.last_ping_at && (t.last_temperature_c < 2 || t.last_temperature_c > 8)
</script>

<template>
  <div>
    <PageHeader title="Live Tracking" subtitle="Vehicles on the road, refreshed every 30 seconds">
      <button class="btn-secondary" @click="load"><RefreshCw class="h-4 w-4" />Refresh</button>
    </PageHeader>
    <div class="grid gap-5 xl:grid-cols-[1fr_360px]">
      <section class="card p-3"><div class="h-[640px]"><RouteMap :trips="trips" :depots="ctx.depot ? [ctx.depot] : []" :color-index="colorIndex" :focus="focus" @select="(id) => router.push({ name: 'trips', query: { id } })" /></div></section>
      <section class="card card-pad">
        <h2 class="card-title">{{ trips.length }} active trip(s)</h2>
        <EmptyState v-if="!trips.length" message="No vehicles on the road" />
        <ul class="mt-3 space-y-2.5">
          <li v-for="t in trips" :key="t.id" class="cursor-pointer rounded-xl border px-3.5 py-3 transition" :class="excursion(t) ? 'border-bad/50 bg-bad-soft/30' : 'border-line hover:border-brand/40'"
            @mouseenter="focus = t.id" @mouseleave="focus = ''" @click="router.push({ name: 'trips', query: { id: t.id } })">
            <div class="flex items-center gap-2">
              <span class="h-2.5 w-2.5 rounded-full" :style="{ background: seriesColor(colorIndex[t.id], theme.dark) }" aria-hidden="true" />
              <span class="text-[13px] font-bold">{{ t.vehicle_plate }}</span><span class="text-xs text-ink-3">{{ t.trip_number }}</span>
              <StatusPill class="ml-auto" :status="t.status" />
            </div>
            <div class="mt-1 text-xs text-ink-3">{{ t.driver_name }} · {{ t.delivered_stops + t.failed_stops }}/{{ t.stops.length }} stops done</div>
            <div v-if="next(t)" class="mt-2 flex items-start gap-1.5 text-xs"><MapPin class="mt-0.5 h-3.5 w-3.5 shrink-0 text-brand-ink" />
              <span>Next: <b>{{ next(t).consignee_name }}</b> · ETA {{ time(next(t).eta) }}</span></div>
            <div class="mt-1.5 flex items-center gap-3 text-[11px] text-ink-3">
              <span>Last ping {{ relative(t.last_ping_at) }}</span>
              <span v-if="t.temperature_class === 'COLD' && t.last_ping_at" class="inline-flex items-center gap-1" :class="excursion(t) ? 'font-bold text-bad' : ''">
                <Thermometer class="h-3.5 w-3.5" />{{ t.last_temperature_c.toFixed(1) }} °C{{ excursion(t) ? ' — excursion' : '' }}</span>
            </div>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>
