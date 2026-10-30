<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Sparkles, Send, Play, Flag, XCircle, Snowflake, AlertTriangle } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useContext } from '@/stores/context'
import { useAction } from '@/lib/useAction'
import { fmt, day, time, titleCase, rupiah, today } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import Tabs from '@/components/Tabs.vue'
import StatusPill from '@/components/StatusPill.vue'
import Modal from '@/components/Modal.vue'
import EmptyState from '@/components/EmptyState.vue'
import RouteMap from '@/components/RouteMap.vue'
import TempChart from '@/components/TempChart.vue'

const route = useRoute()
const auth = useAuth()
const ctx = useContext()
const { busy, run } = useAction()
const date = ref(today())
const tab = ref('all')
const trips = ref<any[]>([])
const selected = ref<any>(null)
const telemetry = ref<any[]>([])
const cancelReason = ref('')
const cancelling = ref(false)

async function load() {
  trips.value = (await api.get('/trips', { depot_id: ctx.depotId, plan_date: date.value })).trips ?? []
}
async function open(id: string) {
  selected.value = await api.get(`/trips/${id}`)
  telemetry.value = (await api.get(`/trips/${id}/telemetry`)).readings ?? []
}
onMounted(async () => {
  if (route.query.id) {
    await open(route.query.id as string)
    date.value = selected.value.plan_date
  }
  await load()
})
watch(date, load)
const shown = computed(() => trips.value.filter((t) => tab.value === 'all' || t.status === tab.value))
const isPlanner = computed(() => auth.can('DISPATCHER', 'TRANSPORT_MANAGER'))
const temps = computed(() => telemetry.value.filter((r) => r.has_temperature))
const late = computed(() => selected.value?.stops.filter((s: any) => s.eta_outside_window && ['PENDING', 'ARRIVED'].includes(s.status)).length ?? 0)

async function act(path: string, msg: string, body: any = {}) {
  const t = await run(() => api.post(`/trips/${selected.value.id}/${path}`, body), msg)
  if (t) { selected.value = t; cancelling.value = false; load() }
}
</script>

<template>
  <div>
    <PageHeader title="Trips" subtitle="Optimized routes with ETAs per stop. Release a planned trip to send it to the driver's app.">
      <input v-model="date" type="date" class="input h-10 w-44" aria-label="Plan date" />
    </PageHeader>
    <Tabs v-model="tab" class="mb-4" :tabs="[{ key: 'all', label: 'All', count: trips.length }, { key: 'PLANNED', label: 'Planned' }, { key: 'RELEASED', label: 'Released' },
      { key: 'IN_PROGRESS', label: 'On the road' }, { key: 'COMPLETED', label: 'Completed' }, { key: 'CANCELLED', label: 'Cancelled' }]" />
    <section class="card card-pad">
      <EmptyState v-if="!shown.length" :message="`No trips on ${day(date)}`" />
      <div v-else class="table-wrap">
        <table class="tbl">
          <thead><tr><th>Trip</th><th>Vehicle</th><th>Driver</th><th class="num">Stops</th><th class="num">Distance</th><th class="num">Duration</th><th class="num">Load</th><th class="num">Est. cost</th><th>Status</th></tr></thead>
          <tbody>
            <tr v-for="t in shown" :key="t.id" class="clickable" @click="open(t.id)">
              <td class="font-bold">{{ t.trip_number }}</td>
              <td><span class="font-semibold">{{ t.vehicle_plate }}</span> <Snowflake v-if="t.temperature_class === 'COLD'" class="inline h-3.5 w-3.5 text-info" aria-label="Refrigerated" /></td>
              <td class="text-ink-2">{{ t.driver_name }}</td>
              <td class="num">{{ t.delivered_stops + t.failed_stops }}/{{ t.stops.length }}</td>
              <td class="num">{{ fmt(t.distance_km) }} km</td>
              <td class="num text-ink-2">{{ Math.floor(t.duration_min / 60) }}h {{ t.duration_min % 60 }}m</td>
              <td class="num">{{ fmt(t.capacity_util_pct) }}%</td>
              <td class="num">{{ rupiah(t.estimated_cost) }}</td>
              <td><StatusPill :status="t.status" /></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <Modal v-if="selected" :title="`${selected.trip_number} · ${selected.vehicle_plate}`" :subtitle="`${selected.driver_name} · ${day(selected.plan_date)} · departs ${time(selected.planned_start)}`" wide @close="selected = null; cancelling = false">
      <div class="grid gap-5 lg:grid-cols-[1.1fr_1fr]">
        <div class="h-[340px]"><RouteMap :trips="[selected]" :depots="ctx.depot ? [ctx.depot] : []" /></div>
        <div>
          <div class="mb-3 flex flex-wrap items-center gap-2"><StatusPill :status="selected.status" /><span v-if="selected.temperature_class === 'COLD'" class="inline-flex items-center gap-1 text-xs font-semibold text-info"><Snowflake class="h-3.5 w-3.5" />Refrigerated 2–8 °C</span></div>
          <dl class="grid grid-cols-2 gap-3">
            <div class="rounded-xl bg-muted/70 p-3"><dt class="text-xs text-ink-3">Distance</dt><dd class="text-[18px] font-extrabold">{{ fmt(selected.distance_km, 1) }} km</dd></div>
            <div class="rounded-xl bg-muted/70 p-3"><dt class="text-xs text-ink-3">Duration</dt><dd class="text-[18px] font-extrabold">{{ Math.floor(selected.duration_min / 60) }}h {{ selected.duration_min % 60 }}m</dd></div>
            <div class="rounded-xl bg-muted/70 p-3"><dt class="text-xs text-ink-3">Load</dt><dd class="text-[18px] font-extrabold">{{ fmt(selected.weight_kg) }} kg <span class="text-xs font-semibold text-ink-3">{{ fmt(selected.capacity_util_pct) }}%</span></dd></div>
            <div class="rounded-xl bg-muted/70 p-3"><dt class="text-xs text-ink-3">Estimated cost</dt><dd class="text-[18px] font-extrabold">{{ rupiah(selected.estimated_cost) }}</dd></div>
          </dl>
          <p v-if="late" class="mt-3 flex items-center gap-2 rounded-xl bg-warn-soft px-3 py-2 text-xs text-warn"><AlertTriangle class="h-4 w-4" />{{ late }} stop(s) are expected outside their delivery window.</p>
          <div v-if="temps.length" class="mt-4"><div class="mb-1 text-[13px] font-bold">Reefer temperature</div><TempChart :readings="temps" /></div>
        </div>
      </div>
      <div class="table-wrap mt-5">
        <table class="tbl">
          <thead><tr><th>#</th><th>Consignee</th><th>Shipment</th><th>Window</th><th>ETA</th><th class="num">Leg</th><th class="num">Kg</th><th>Status</th></tr></thead>
          <tbody>
            <tr v-for="s in selected.stops" :key="s.id">
              <td><span class="grid h-7 w-7 place-items-center rounded-lg bg-brand-soft text-xs font-bold text-brand-ink">{{ s.sequence }}</span></td>
              <td class="max-w-[260px]"><div class="truncate font-semibold">{{ s.consignee_name }}</div><div class="truncate text-xs text-ink-3">{{ s.address }}</div></td>
              <td class="whitespace-nowrap text-ink-2">{{ s.shipment_number }} <Snowflake v-if="s.temperature_class === 'COLD'" class="inline h-3.5 w-3.5 text-info" /></td>
              <td class="whitespace-nowrap text-ink-2">{{ s.window_start }}–{{ s.window_end }}</td>
              <td class="whitespace-nowrap font-semibold" :class="s.eta_outside_window ? 'text-warn' : ''">{{ time(s.eta) }}<span v-if="s.eta_outside_window" class="ml-1 text-[11px]">late</span></td>
              <td class="num text-ink-2">{{ fmt(s.distance_from_prev_km, 1) }} km</td>
              <td class="num">{{ fmt(s.weight_kg, 1) }}</td>
              <td><StatusPill :status="s.status" /><div v-if="s.failure_reason" class="text-[11px] text-bad">{{ titleCase(s.failure_reason) }}</div><div v-if="s.completed_at" class="text-[11px] text-ink-3">{{ time(s.completed_at) }}</div></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="cancelling" class="mt-4 flex gap-2 rounded-xl border border-bad/30 bg-bad-soft/50 p-3">
        <input v-model="cancelReason" class="input" placeholder="Why cancel? Shipments return to the pool" /><button class="btn-danger" :disabled="busy || !cancelReason" @click="act('cancel', 'Trip cancelled', { reason: cancelReason })">Confirm</button>
      </div>
      <template v-if="isPlanner" #footer>
        <button v-if="['PLANNED', 'RELEASED'].includes(selected.status)" class="btn-ghost text-bad" @click="cancelling = true"><XCircle class="h-4 w-4" />Cancel trip</button>
        <button v-if="selected.status === 'PLANNED'" class="btn-secondary" :disabled="busy" @click="act('optimize', 'Route re-optimized')"><Sparkles class="h-4 w-4" />Re-optimize</button>
        <button v-if="selected.status === 'PLANNED'" class="btn-primary" :disabled="busy" @click="act('release', 'Trip released to the driver')"><Send class="h-4 w-4" />Release to driver</button>
        <button v-if="selected.status === 'RELEASED'" class="btn-primary" :disabled="busy" @click="act('start', 'Trip started')"><Play class="h-4 w-4" />Start trip</button>
        <button v-if="selected.status === 'IN_PROGRESS'" class="btn-primary" :disabled="busy" @click="act('complete', 'Trip completed')"><Flag class="h-4 w-4" />Complete trip</button>
      </template>
    </Modal>
  </div>
</template>
