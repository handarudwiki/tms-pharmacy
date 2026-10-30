<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Wand2, Plus, Snowflake, Lock, AlertTriangle, Handshake } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useContext } from '@/stores/context'
import { useAction } from '@/lib/useAction'
import { useToast } from '@/stores/toast'
import { fmt, day, today, titleCase } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import StatusPill from '@/components/StatusPill.vue'
import EmptyState from '@/components/EmptyState.vue'
import RouteMap from '@/components/RouteMap.vue'

const auth = useAuth()
const ctx = useContext()
const router = useRouter()
const { busy, run } = useAction()
const date = ref(today())
const pool = ref<any[]>([])
const trips = ref<any[]>([])
const picked = ref<Set<string>>(new Set())
const vehicleId = ref('')
const driverId = ref('')
const unplanned = ref<any[]>([])
const canPlan = computed(() => auth.can('DISPATCHER', 'TRANSPORT_MANAGER'))

function plusDays(d: string, n: number) {
  const x = new Date(d + 'T00:00:00')
  x.setDate(x.getDate() + n)
  return `${x.getFullYear()}-${String(x.getMonth() + 1).padStart(2, '0')}-${String(x.getDate()).padStart(2, '0')}`
}
async function load() {
  const [p, t] = await Promise.all([
    api.get('/shipments', { depot_id: ctx.depotId, status: 'PENDING', required_by: plusDays(date.value, 1) }),
    api.get('/trips', { depot_id: ctx.depotId, plan_date: date.value }),
  ])
  pool.value = p.shipments ?? []
  trips.value = (t.trips ?? []).filter((x: any) => x.status !== 'CANCELLED')
  picked.value = new Set()
}
onMounted(load)
watch(date, () => { unplanned.value = []; load() })

const selected = computed(() => pool.value.filter((s) => picked.value.has(s.id)))
const load_ = computed(() => selected.value.reduce((a, s) => ({ kg: a.kg + s.weight_kg, vol: a.vol + s.volume_l }), { kg: 0, vol: 0 }))
const vehicle = computed(() => ctx.vehicle(vehicleId.value))
const booked = computed(() => new Set(trips.value.flatMap((t) => [t.vehicle_id, t.driver_id])))
const needsCold = computed(() => selected.value.some((s) => s.temperature_class === 'COLD'))
const needsControlled = computed(() => selected.value.some((s) => s.controlled))
const totalKg = computed(() => pool.value.reduce((a, s) => a + s.weight_kg, 0))

function toggle(id: string) {
  const s = new Set(picked.value)
  s.has(id) ? s.delete(id) : s.add(id)
  picked.value = s
}
async function autoPlan() {
  const res = await run(() => api.post('/planning/auto', { depot_id: ctx.depotId, plan_date: date.value }))
  if (res) {
    unplanned.value = res.unplanned ?? []
    useToast().success(`${res.trips?.length ?? 0} trip(s) planned${unplanned.value.length ? ` · ${unplanned.value.length} shipment(s) could not be placed` : ''}`)
    load()
  }
}
async function createTrip() {
  const t = await run(() => api.post('/trips', { depot_id: ctx.depotId, plan_date: date.value, vehicle_id: vehicleId.value, driver_id: driverId.value,
    shipment_ids: [...picked.value] }), 'Trip created with an optimized stop sequence')
  if (t) { vehicleId.value = driverId.value = ''; load() }
}
</script>

<template>
  <div>
    <PageHeader title="Planning" subtitle="Turn the shipment pool into trips — automatically or by hand. Capacity, cold chain, controlled-drug and licence rules are always enforced.">
      <input v-model="date" type="date" class="input h-10 w-44" aria-label="Plan date" />
      <button v-if="canPlan" class="btn-primary" :disabled="busy || !pool.length" @click="autoPlan"><Wand2 class="h-4 w-4" />Auto-plan {{ day(date) }}</button>
    </PageHeader>

    <div v-if="unplanned.length" class="card mb-5 border-warn/40 p-4">
      <div class="mb-2 flex items-center gap-2 text-[13px] font-bold text-warn"><AlertTriangle class="h-4 w-4" />{{ unplanned.length }} shipment(s) could not be planned</div>
      <ul class="space-y-1 text-[13px]">
        <li v-for="u in unplanned" :key="u.shipment_id"><b>{{ u.shipment_number }}</b> — {{ u.reason }}</li>
      </ul>
      <RouterLink :to="{ name: 'freight', query: { tender: unplanned.map((u) => u.shipment_id).join(',') } }" class="btn-secondary btn-sm mt-3"><Handshake class="h-4 w-4" />Tender these to carriers</RouterLink>
    </div>

    <div class="grid gap-5 xl:grid-cols-5">
      <section class="card card-pad xl:col-span-3">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div><h2 class="card-title">Shipment pool</h2><p class="card-sub">{{ pool.length }} pending shipments due by {{ day(plusDays(date, 1)) }} · {{ fmt(totalKg) }} kg</p></div>
          <span v-if="picked.size" class="text-xs font-semibold text-brand-ink">{{ picked.size }} selected · {{ fmt(load_.kg, 1) }} kg</span>
        </div>
        <EmptyState v-if="!pool.length" message="Nothing waiting to be planned for this date" />
        <div v-else class="table-wrap max-h-[520px] overflow-y-auto">
          <table class="tbl">
            <thead><tr><th class="w-8" /><th>Shipment</th><th>Consignee</th><th>Window</th><th class="num">Kg</th><th>Handling</th><th>Due</th></tr></thead>
            <tbody>
              <tr v-for="s in pool" :key="s.id" class="clickable" @click="toggle(s.id)">
                <td><input type="checkbox" :checked="picked.has(s.id)" class="accent-[rgb(var(--brand))]" :aria-label="`Select ${s.shipment_number}`" @click.stop="toggle(s.id)" /></td>
                <td class="font-semibold">{{ s.shipment_number }}<div v-if="s.attempts" class="text-[11px] text-warn">retry · attempt {{ s.attempts + 1 }}</div></td>
                <td class="max-w-[220px] truncate">{{ s.consignee_name }}</td>
                <td class="whitespace-nowrap text-ink-2">{{ s.window_start }}–{{ s.window_end }}</td>
                <td class="num">{{ fmt(s.weight_kg, 1) }}</td>
                <td class="whitespace-nowrap"><Snowflake v-if="s.temperature_class === 'COLD'" class="inline h-4 w-4 text-info" aria-label="Cold chain" /><Lock v-if="s.controlled" class="inline h-4 w-4 text-serious" aria-label="Controlled" /></td>
                <td class="whitespace-nowrap text-ink-2">{{ day(s.required_date) }}<span v-if="s.priority === 1" class="ml-1 text-[11px] font-bold text-bad">urgent</span></td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div class="space-y-5 xl:col-span-2">
        <section v-if="canPlan" class="card card-pad">
          <h2 class="card-title">Build a trip manually</h2>
          <p class="card-sub">Select shipments in the pool, then a vehicle and driver</p>
          <div class="mt-4 grid gap-3">
            <div><label class="label">Vehicle</label>
              <select v-model="vehicleId" class="input"><option value="" disabled>Choose vehicle</option>
                <option v-for="v in ctx.vehicles" :key="v.id" :value="v.id" :disabled="v.status !== 'AVAILABLE' || booked.has(v.id)">
                  {{ v.plate }} · {{ titleCase(v.vehicle_type) }} · {{ fmt(v.capacity_kg) }} kg{{ v.controlled_certified ? ' · controlled' : '' }}{{ v.status !== 'AVAILABLE' ? ` — ${titleCase(v.status)}` : booked.has(v.id) ? ' — booked' : '' }}
                </option></select></div>
            <div><label class="label">Driver</label>
              <select v-model="driverId" class="input"><option value="" disabled>Choose driver</option>
                <option v-for="d in ctx.drivers" :key="d.id" :value="d.id" :disabled="d.status !== 'ACTIVE' || booked.has(d.id) || d.license_expiry < date">
                  {{ d.name }}{{ d.cold_chain_certified ? ' · cold' : '' }}{{ d.controlled_certified ? ' · controlled' : '' }}{{ d.license_expiry < date ? ' — licence expired' : d.status !== 'ACTIVE' ? ` — ${titleCase(d.status)}` : booked.has(d.id) ? ' — booked' : '' }}
                </option></select></div>
            <div v-if="vehicle && picked.size" class="rounded-xl bg-muted/70 p-3 text-xs">
              <div class="mb-1 flex justify-between"><span>Load</span><b>{{ fmt(load_.kg, 1) }} / {{ fmt(vehicle.capacity_kg) }} kg</b></div>
              <div class="h-1.5 rounded-full bg-line"><div :class="['h-1.5 rounded-full', load_.kg > vehicle.capacity_kg ? 'bg-bad' : 'bg-brand']" :style="{ width: Math.min(100, (load_.kg / vehicle.capacity_kg) * 100) + '%' }" /></div>
              <p v-if="needsCold && vehicle.temperature_class !== 'COLD'" class="mt-2 font-semibold text-bad">Cold-chain shipments need a refrigerated vehicle.</p>
              <p v-if="needsControlled && !vehicle.controlled_certified" class="mt-2 font-semibold text-bad">Controlled drugs need a certified vehicle.</p>
            </div>
            <button class="btn-primary" :disabled="busy || !picked.size || !vehicleId || !driverId" @click="createTrip"><Plus class="h-4 w-4" />Create trip</button>
          </div>
        </section>

        <section class="card card-pad">
          <h2 class="card-title">Trips on {{ day(date) }}</h2>
          <EmptyState v-if="!trips.length" message="No trips yet" />
          <div v-else class="mt-3 h-[220px]"><RouteMap :trips="trips" :depots="ctx.depot ? [ctx.depot] : []" /></div>
          <ul class="mt-3 space-y-2">
            <li v-for="t in trips" :key="t.id" class="flex cursor-pointer items-center gap-3 rounded-xl border border-line px-3 py-2.5 hover:border-brand/40" @click="router.push({ name: 'trips', query: { id: t.id } })">
              <div class="flex-1"><div class="text-[13px] font-bold">{{ t.trip_number }} · {{ t.vehicle_plate }}</div>
                <div class="text-xs text-ink-3">{{ t.driver_name }} · {{ t.stops.length }} stops · {{ fmt(t.distance_km) }} km · {{ fmt(t.capacity_util_pct) }}% full</div></div>
              <StatusPill :status="t.status" />
            </li>
          </ul>
        </section>
      </div>
    </div>
  </div>
</template>
