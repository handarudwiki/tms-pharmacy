<script setup lang="ts">
// Driver application (FR-08): today's trip, ordered stops, arrival, proof of delivery and failures.
import { computed, onMounted, reactive, ref } from 'vue'
import { Play, MapPinned, FileSignature, XCircle, Flag, Navigation, Snowflake, Lock, Phone, CheckCircle2 } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useAction } from '@/lib/useAction'
import { fmt, time, day } from '@/lib/format'
import StatusPill from '@/components/StatusPill.vue'
import EmptyState from '@/components/EmptyState.vue'
import RouteMap from '@/components/RouteMap.vue'
import Modal from '@/components/Modal.vue'

const auth = useAuth()
const { busy, run } = useAction()
const trips = ref<any[]>([])
const tripId = ref('')
const delivering = ref<any>(null)
const failing = ref<any>(null)
const pod = reactive({ receiver_name: '', signature_captured: false, note: '' })
const fail = reactive({ reason: 'CUSTOMER_ABSENT', note: '', reschedule: true })

async function load() {
  trips.value = (await api.get('/driver/trips')).trips ?? []
  if (!trips.value.some((t) => t.id === tripId.value)) tripId.value = (trips.value.find((t) => t.status !== 'COMPLETED') ?? trips.value[0])?.id ?? ''
}
onMounted(load)
const trip = computed(() => trips.value.find((t) => t.id === tripId.value))
const next = computed(() => trip.value?.stops.find((s: any) => s.status === 'PENDING' || s.status === 'ARRIVED'))

async function call(path: string, body: any, msg: string) {
  const t = await run(() => api.post(`/trips/${trip.value.id}/${path}`, body), msg)
  if (t) { await load(); return true }
  return false
}
async function sendLocation() {
  // Moves the vehicle a step toward the next stop (stand-in for the phone's GPS).
  const t = trip.value, s = next.value
  const from = [t.last_lat || t.depot_lat, t.last_lng || t.depot_lng], to = s ? [s.lat, s.lng] : [t.depot_lat, t.depot_lng]
  const f = 0.5
  const reading: any = { lat: from[0] + (to[0] - from[0]) * f, lng: from[1] + (to[1] - from[1]) * f, speed_kmh: 26 }
  if (t.temperature_class === 'COLD') Object.assign(reading, { has_temperature: true, temperature_c: 4 + Math.random() * 2 })
  await call('telemetry', reading, 'Location sent')
}
async function deliver() {
  if (await call(`stops/${delivering.value.id}/deliver`, { ...pod }, 'Delivered — proof of delivery saved')) {
    delivering.value = null
    Object.assign(pod, { receiver_name: '', signature_captured: false, note: '' })
  }
}
async function failStop() {
  if (await call(`stops/${failing.value.id}/fail`, { ...fail }, fail.reschedule ? 'Failed attempt recorded — shipment back in planning' : 'Returned to depot')) {
    failing.value = null
    Object.assign(fail, { reason: 'CUSTOMER_ABSENT', note: '', reschedule: true })
  }
}
const reasons = [['CUSTOMER_ABSENT', 'Nobody to receive'], ['CLOSED', 'Premises closed'], ['ADDRESS_NOT_FOUND', 'Address not found'], ['REFUSED', 'Refused by customer'], ['DAMAGED', 'Goods damaged'], ['OTHER', 'Other']]
</script>

<template>
  <div class="mx-auto max-w-md">
    <div class="mb-4">
      <h1 class="text-[22px] font-extrabold tracking-tight">Hi {{ auth.user?.name.split(' ')[0] }} 👋</h1>
      <p class="text-[13px] text-ink-3">{{ day(new Date().toISOString()) }}</p>
    </div>
    <select v-if="trips.length > 1" v-model="tripId" class="input mb-3" aria-label="Trip">
      <option v-for="t in trips" :key="t.id" :value="t.id">{{ t.trip_number }} · {{ t.vehicle_plate }} · {{ t.status }}</option>
    </select>
    <div v-if="!trip" class="card card-pad"><EmptyState message="No trip assigned to you right now. Check with your dispatcher." /></div>

    <template v-else>
      <section class="card overflow-hidden">
        <div class="h-[250px]"><RouteMap :trips="[trip]" :depots="[{ code: 'DC', name: 'Depot', lat: trip.depot_lat, lng: trip.depot_lng }]" /></div>
        <div class="p-4">
          <div class="flex items-center gap-2"><span class="text-[15px] font-extrabold">{{ trip.vehicle_plate }}</span><span class="text-xs text-ink-3">{{ trip.trip_number }}</span><StatusPill class="ml-auto" :status="trip.status" /></div>
          <div class="mt-1 text-xs text-ink-3">{{ trip.stops.length }} stops · {{ fmt(trip.distance_km) }} km · {{ trip.delivered_stops }} delivered · {{ trip.failed_stops }} failed</div>
          <div v-if="trip.temperature_class === 'COLD'" class="mt-2 inline-flex items-center gap-1.5 rounded-lg bg-info-soft px-2 py-1 text-xs font-semibold text-info"><Snowflake class="h-3.5 w-3.5" />Keep reefer at 2–8 °C<span v-if="trip.last_ping_at"> · now {{ trip.last_temperature_c.toFixed(1) }} °C</span></div>
          <div class="mt-3 grid grid-cols-2 gap-2">
            <button v-if="trip.status === 'RELEASED'" class="btn-primary col-span-2 h-11" :disabled="busy" @click="call('start', {}, 'Trip started — drive safely')"><Play class="h-4 w-4" />Start trip</button>
            <button v-if="trip.status === 'IN_PROGRESS'" class="btn-secondary h-11" :disabled="busy" @click="sendLocation"><Navigation class="h-4 w-4" />Send location</button>
            <button v-if="trip.status === 'IN_PROGRESS'" class="btn-secondary h-11" :disabled="busy || !!next" @click="call('complete', {}, 'Trip completed')"><Flag class="h-4 w-4" />Finish trip</button>
          </div>
        </div>
      </section>

      <ol class="mt-4 space-y-3">
        <li v-for="s in trip.stops" :key="s.id" :class="['card p-4', next && s.id === next.id ? 'ring-2 ring-brand/40' : '', ['DELIVERED', 'FAILED'].includes(s.status) ? 'opacity-70' : '']">
          <div class="flex items-start gap-3">
            <span :class="['grid h-8 w-8 shrink-0 place-items-center rounded-xl text-[13px] font-bold', s.status === 'DELIVERED' ? 'bg-good-soft text-good' : s.status === 'FAILED' ? 'bg-bad-soft text-bad' : 'bg-brand-soft text-brand-ink']">
              <CheckCircle2 v-if="s.status === 'DELIVERED'" class="h-4 w-4" /><XCircle v-else-if="s.status === 'FAILED'" class="h-4 w-4" /><template v-else>{{ s.sequence }}</template></span>
            <div class="min-w-0 flex-1">
              <div class="font-bold">{{ s.consignee_name }}</div>
              <div class="text-xs text-ink-3">{{ s.address }}</div>
              <div class="mt-1 flex flex-wrap items-center gap-2 text-xs">
                <span class="font-semibold">ETA {{ time(s.eta) }}</span><span class="text-ink-3">window {{ s.window_start }}–{{ s.window_end }}</span>
                <span class="text-ink-3">{{ s.packages }} pkg · {{ fmt(s.weight_kg, 1) }} kg</span>
                <Snowflake v-if="s.temperature_class === 'COLD'" class="h-3.5 w-3.5 text-info" aria-label="Cold chain" />
                <span v-if="s.controlled" class="inline-flex items-center gap-1 font-semibold text-serious"><Lock class="h-3 w-3" />double signature</span>
              </div>
              <StatusPill class="mt-2" :status="s.status" />
            </div>
          </div>
          <div v-if="trip.status === 'IN_PROGRESS' && ['PENDING', 'ARRIVED'].includes(s.status)" class="mt-3 grid grid-cols-3 gap-2">
            <button class="btn-secondary btn-sm" :disabled="busy || s.status === 'ARRIVED'" @click="call(`stops/${s.id}/arrive`, {}, 'Arrival recorded')"><MapPinned class="h-4 w-4" />Arrived</button>
            <button class="btn-primary btn-sm" :disabled="busy" @click="delivering = s"><FileSignature class="h-4 w-4" />Deliver</button>
            <button class="btn-ghost btn-sm text-bad" :disabled="busy" @click="failing = s"><XCircle class="h-4 w-4" />Failed</button>
          </div>
        </li>
      </ol>
    </template>

    <Modal v-if="delivering" title="Proof of delivery" :subtitle="delivering.consignee_name" @close="delivering = null">
      <div class="space-y-4">
        <div><label class="label">Receiver's name</label><input v-model="pod.receiver_name" class="input h-11" placeholder="e.g. apoteker on duty" /></div>
        <label class="flex items-center gap-3 rounded-xl border border-line p-3 text-[13px] font-semibold">
          <input v-model="pod.signature_captured" type="checkbox" class="h-5 w-5 accent-[rgb(var(--brand))]" />Receiver signed on the device
        </label>
        <div><label class="label">Note (condition, seal number…)</label><input v-model="pod.note" class="input h-11" /></div>
        <p class="flex items-center gap-1.5 text-xs text-ink-3"><Phone class="h-3.5 w-3.5" />Location is attached automatically.</p>
      </div>
      <template #footer>
        <button class="btn-secondary" @click="delivering = null">Cancel</button>
        <button class="btn-primary" :disabled="busy || !pod.receiver_name || !pod.signature_captured" @click="deliver">Confirm delivery</button>
      </template>
    </Modal>

    <Modal v-if="failing" title="Delivery failed" :subtitle="failing.consignee_name" @close="failing = null">
      <div class="space-y-4">
        <div><label class="label">Reason</label><select v-model="fail.reason" class="input h-11"><option v-for="r in reasons" :key="r[0]" :value="r[0]">{{ r[1] }}</option></select></div>
        <div><label class="label">Note</label><input v-model="fail.note" class="input h-11" /></div>
        <label class="flex items-center gap-3 text-[13px] font-semibold"><input v-model="fail.reschedule" type="checkbox" class="h-5 w-5 accent-[rgb(var(--brand))]" />Reschedule another attempt</label>
        <p class="text-xs text-ink-3">Refusals, or a third failed attempt, return the goods to the depot.</p>
      </div>
      <template #footer>
        <button class="btn-secondary" @click="failing = null">Cancel</button>
        <button class="btn-danger" :disabled="busy" @click="failStop">Record failure</button>
      </template>
    </Modal>
  </div>
</template>
