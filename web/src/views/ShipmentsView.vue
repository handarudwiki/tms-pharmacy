<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Plus, Snowflake, Lock, MapPin, FileSignature, XCircle } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useContext } from '@/stores/context'
import { useAction } from '@/lib/useAction'
import { fmt, day, dateTime, titleCase, rupiah, today } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import Tabs from '@/components/Tabs.vue'
import StatusPill from '@/components/StatusPill.vue'
import Modal from '@/components/Modal.vue'
import EmptyState from '@/components/EmptyState.vue'
import RouteMap from '@/components/RouteMap.vue'

const route = useRoute()
const auth = useAuth()
const ctx = useContext()
const { busy, run } = useAction()
const tab = ref('open')
const q = ref((route.query.q as string) || '')
const items = ref<any[]>([])
const selected = ref<any>(null)
const events = ref<any[]>([])
const creating = ref(route.query.new === '1')
const delivering = ref(false)
const cancelling = ref(false)
const pod = reactive({ receiver_name: '', signature_captured: true, note: '' })
const cancelReason = ref('')

const groups: Record<string, string[]> = {
  open: ['PENDING', 'TENDERED', 'PLANNED', 'DISPATCHED'],
  pending: ['PENDING'],
  transit: ['DISPATCHED'],
  delivered: ['DELIVERED'],
  closed: ['RETURNED', 'CANCELLED'],
}
async function load() {
  items.value = (await api.get('/shipments', { depot_id: ctx.depotId, query: q.value, limit: 1000 })).shipments ?? []
}
onMounted(load)
const shown = computed(() => items.value.filter((s) => tab.value === 'all' || groups[tab.value].includes(s.status)))
const count = (k: string) => items.value.filter((s) => groups[k].includes(s.status)).length

async function open(s: any) {
  selected.value = s
  events.value = (await api.get(`/shipments/${s.id}/events`)).events ?? []
}
async function refresh() {
  await load()
  const s = items.value.find((x) => x.id === selected.value?.id)
  if (s) await open(s)
}
async function deliver() {
  if (await run(() => api.post(`/shipments/${selected.value.id}/deliver`, { pod: { ...pod } }), 'Proof of delivery recorded')) {
    delivering.value = false
    refresh()
  }
}
async function cancel() {
  if (await run(() => api.post(`/shipments/${selected.value.id}/cancel`, { reason: cancelReason.value }), 'Shipment cancelled')) {
    cancelling.value = false
    refresh()
  }
}

const presets = [
  { name: 'Apotek K-24 Kemang', address: 'Jl. Kemang Raya 12, Jakarta Selatan', lat: -6.2607, lng: 106.8136 },
  { name: 'RSCM Kencana', address: 'Jl. Diponegoro 71, Jakarta Pusat', lat: -6.1966, lng: 106.8473 },
  { name: 'RS Mitra Keluarga Bekasi', address: 'Jl. Ahmad Yani, Bekasi', lat: -6.2471, lng: 107.0003 },
  { name: 'RS Hermina Depok', address: 'Jl. Siliwangi 50, Depok', lat: -6.391, lng: 106.825 },
  { name: 'Watsons Pondok Indah', address: 'Jl. Metro Pondok Indah, Jakarta Selatan', lat: -6.2655, lng: 106.7838 },
]
const blank = () => ({ consignee_name: '', consignee_phone: '', address: '', city: 'Jakarta', lat: 0, lng: 0, window_start: '08:00', window_end: '17:00',
  required_date: today(1), weight_kg: 25, packages: 2, temperature_class: 'AMBIENT', controlled: false, priority: 3, notes: '', external_ref: '' })
const form = reactive(blank())
function usePreset(i: string) {
  const p = presets[Number(i)]
  if (p) Object.assign(form, { consignee_name: p.name, address: p.address, lat: p.lat, lng: p.lng })
}
async function create() {
  const s = await run(() => api.post('/shipments', { ...form, depot_id: ctx.depotId }), 'Shipment created and added to the planning pool')
  if (s) {
    creating.value = false
    Object.assign(form, blank())
    await load()
    open(s)
  }
}
const prio = ['', 'Urgent', 'High', 'Normal', 'Low', 'Lowest']
</script>

<template>
  <div>
    <PageHeader title="Shipments" subtitle="Deliveries imported from the WMS or entered manually, from planning through proof of delivery">
      <form @submit.prevent="load"><input v-model="q" class="input h-10 w-60" placeholder="Shipment no., consignee, WMS order" /></form>
      <button v-if="auth.can('DISPATCHER', 'TRANSPORT_MANAGER', 'CUSTOMER_SERVICE')" class="btn-primary" @click="creating = true"><Plus class="h-4 w-4" />New shipment</button>
    </PageHeader>
    <Tabs v-model="tab" class="mb-4" :tabs="[{ key: 'open', label: 'Open', count: count('open') }, { key: 'pending', label: 'Awaiting planning', count: count('pending') },
      { key: 'transit', label: 'Out for delivery', count: count('transit') }, { key: 'delivered', label: 'Delivered' }, { key: 'closed', label: 'Returned / cancelled' }, { key: 'all', label: 'All' }]" />
    <section class="card card-pad">
      <EmptyState v-if="!shown.length" message="No shipments in this view" />
      <div v-else class="table-wrap">
        <table class="tbl">
          <thead><tr><th>Shipment</th><th>Consignee</th><th>Window</th><th>Required</th><th class="num">Weight</th><th>Handling</th><th>Carrier / trip</th><th>Status</th></tr></thead>
          <tbody>
            <tr v-for="s in shown" :key="s.id" class="clickable" @click="open(s)">
              <td><div class="font-bold">{{ s.shipment_number }}</div><div class="text-xs text-ink-3">{{ s.source === 'WMS' ? s.external_ref : 'Manual' }}</div></td>
              <td class="max-w-[240px]"><div class="truncate font-semibold">{{ s.consignee_name }}</div><div class="truncate text-xs text-ink-3">{{ s.address }}</div></td>
              <td class="whitespace-nowrap text-ink-2">{{ s.window_start }}–{{ s.window_end }}</td>
              <td class="whitespace-nowrap text-ink-2">{{ day(s.required_date) }}<div v-if="s.priority === 1" class="text-[11px] font-bold text-bad">Urgent</div></td>
              <td class="num font-semibold">{{ fmt(s.weight_kg, 1) }} kg</td>
              <td><span class="inline-flex items-center gap-1.5 text-xs">
                <span v-if="s.temperature_class === 'COLD'" class="inline-flex items-center gap-1 rounded-md bg-info-soft px-1.5 py-0.5 font-semibold text-info"><Snowflake class="h-3 w-3" />2–8 °C</span>
                <span v-if="s.controlled" class="inline-flex items-center gap-1 rounded-md bg-serious-soft px-1.5 py-0.5 font-semibold text-serious"><Lock class="h-3 w-3" />Controlled</span>
                <span v-if="s.temperature_class !== 'COLD' && !s.controlled" class="text-ink-3">Ambient</span></span></td>
              <td class="text-ink-2">{{ s.carrier_name || (s.trip_id ? 'Own fleet' : '—') }}</td>
              <td><StatusPill :status="s.status" /></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <Modal v-if="selected" :title="selected.shipment_number" :subtitle="`${selected.consignee_name} · ${selected.address}`" wide @close="selected = null; delivering = false; cancelling = false">
      <div class="grid gap-5 md:grid-cols-[1fr_300px]">
        <div>
          <div class="mb-4 flex flex-wrap items-center gap-2 text-[13px]">
            <StatusPill :status="selected.status" />
            <span class="text-ink-3">{{ selected.source === 'WMS' ? `From WMS ${selected.external_ref}` : 'Manual entry' }}</span>
            <span v-if="selected.location_approximate" class="inline-flex items-center gap-1 text-xs text-warn"><MapPin class="h-3.5 w-3.5" />approximate location</span>
          </div>
          <dl class="grid grid-cols-2 gap-3 text-[13px] sm:grid-cols-3">
            <div><dt class="text-xs text-ink-3">Required</dt><dd class="font-semibold">{{ day(selected.required_date) }}</dd></div>
            <div><dt class="text-xs text-ink-3">Window</dt><dd class="font-semibold">{{ selected.window_start }}–{{ selected.window_end }}</dd></div>
            <div><dt class="text-xs text-ink-3">Priority</dt><dd class="font-semibold">{{ prio[selected.priority] }}</dd></div>
            <div><dt class="text-xs text-ink-3">Weight / volume</dt><dd class="font-semibold">{{ fmt(selected.weight_kg, 1) }} kg · {{ fmt(selected.volume_l) }} L</dd></div>
            <div><dt class="text-xs text-ink-3">Packages</dt><dd class="font-semibold">{{ selected.packages }}</dd></div>
            <div><dt class="text-xs text-ink-3">Handling</dt><dd class="font-semibold">{{ selected.temperature_class === 'COLD' ? '2–8 °C' : 'Ambient' }}{{ selected.controlled ? ' · controlled' : '' }}</dd></div>
            <div v-if="selected.carrier_name"><dt class="text-xs text-ink-3">Carrier</dt><dd class="font-semibold">{{ selected.carrier_name }} · {{ rupiah(selected.freight_cost) }}</dd></div>
            <div><dt class="text-xs text-ink-3">Attempts</dt><dd class="font-semibold">{{ selected.attempts }}</dd></div>
            <div><dt class="text-xs text-ink-3">Contact</dt><dd class="font-semibold">{{ selected.consignee_phone || '—' }}</dd></div>
          </dl>
          <div v-if="selected.pod" class="mt-4 rounded-xl bg-good-soft/60 p-3 text-[13px]">
            <div class="flex items-center gap-2 font-bold text-good"><FileSignature class="h-4 w-4" />Proof of delivery</div>
            <div class="mt-1">Received by <b>{{ selected.pod.receiver_name }}</b> {{ selected.pod.signature_captured ? '(signed)' : '' }} · {{ dateTime(selected.delivered_at) }}
              <StatusPill class="ml-1" :status="selected.delivered_on_time ? 'ON_TIME' : 'LATE'" :label="selected.delivered_on_time ? 'On time' : 'Late'" /></div>
            <div v-if="selected.pod.note" class="text-ink-2">“{{ selected.pod.note }}” — {{ selected.pod.recorded_by }}</div>
          </div>
          <h3 class="mb-2 mt-5 text-[13px] font-bold">Timeline</h3>
          <ol class="relative ml-2 border-l border-line pl-5">
            <li v-for="e in events" :key="e.id" class="mb-3">
              <span class="absolute -left-[5px] mt-1.5 h-2.5 w-2.5 rounded-full bg-brand" />
              <div class="text-[13px] font-semibold">{{ titleCase(e.event_type) }} <span class="font-normal text-ink-3">· {{ dateTime(e.created_at) }}</span></div>
              <div class="text-xs text-ink-2">{{ e.description }} <span class="text-ink-3">— {{ e.actor.split('@')[0] }}</span></div>
            </li>
          </ol>
        </div>
        <div class="h-[300px]"><RouteMap :depots="ctx.depot ? [ctx.depot] : []" :pins="[{ lat: selected.lat, lng: selected.lng, label: selected.consignee_name }]" /></div>
      </div>
      <form v-if="delivering" class="mt-4 grid gap-3 rounded-2xl border border-line bg-muted/50 p-4 sm:grid-cols-[1fr_1fr_auto]" @submit.prevent="deliver">
        <div><label class="label">Received by</label><input v-model="pod.receiver_name" class="input" required /></div>
        <div><label class="label">Note</label><input v-model="pod.note" class="input" placeholder="Carrier waybill / remarks" /></div>
        <label class="mt-6 flex items-center gap-2 text-[13px] font-semibold"><input v-model="pod.signature_captured" type="checkbox" class="accent-[rgb(var(--brand))]" />Signed</label>
        <div class="sm:col-span-3 flex justify-end"><button class="btn-primary" :disabled="busy || !pod.receiver_name || !pod.signature_captured">Record delivery</button></div>
      </form>
      <div v-if="cancelling" class="mt-4 flex gap-2 rounded-xl border border-bad/30 bg-bad-soft/50 p-3">
        <input v-model="cancelReason" class="input" placeholder="Cancellation reason" /><button class="btn-danger" :disabled="busy || !cancelReason" @click="cancel">Confirm</button>
      </div>
      <template #footer>
        <button v-if="selected.status === 'PENDING' && auth.can('DISPATCHER', 'TRANSPORT_MANAGER')" class="btn-ghost text-bad" @click="cancelling = true"><XCircle class="h-4 w-4" />Cancel shipment</button>
        <button v-if="selected.status === 'DISPATCHED' && selected.carrier_id && auth.can('CUSTOMER_SERVICE', 'DISPATCHER', 'TRANSPORT_MANAGER')" class="btn-primary" @click="delivering = true">
          <FileSignature class="h-4 w-4" />Record carrier POD</button>
      </template>
    </Modal>

    <Modal v-if="creating" title="New shipment" subtitle="Added to the planning pool for the selected depot" wide @close="creating = false">
      <div class="grid gap-4 sm:grid-cols-3">
        <div class="sm:col-span-3"><label class="label">Quick fill from a known customer</label>
          <select class="input" @change="usePreset(($event.target as HTMLSelectElement).value)"><option value="">Choose…</option>
            <option v-for="(p, i) in presets" :key="p.name" :value="i">{{ p.name }}</option></select></div>
        <div><label class="label">Consignee</label><input v-model="form.consignee_name" class="input" /></div>
        <div class="sm:col-span-2"><label class="label">Address</label><input v-model="form.address" class="input" /></div>
        <div><label class="label">Phone</label><input v-model="form.consignee_phone" class="input" /></div>
        <div><label class="label">Latitude</label><input v-model.number="form.lat" type="number" step="0.0001" class="input" /></div>
        <div><label class="label">Longitude</label><input v-model.number="form.lng" type="number" step="0.0001" class="input" /></div>
        <div><label class="label">Required date</label><input v-model="form.required_date" type="date" class="input" /></div>
        <div class="grid grid-cols-2 gap-2"><div><label class="label">Window from</label><input v-model="form.window_start" type="time" class="input" /></div>
          <div><label class="label">to</label><input v-model="form.window_end" type="time" class="input" /></div></div>
        <div><label class="label">Priority</label><select v-model.number="form.priority" class="input"><option v-for="p in 5" :key="p" :value="p">{{ p }} · {{ prio[p] }}</option></select></div>
        <div><label class="label">Weight (kg)</label><input v-model.number="form.weight_kg" type="number" min="0.1" step="0.1" class="input" /></div>
        <div><label class="label">Packages</label><input v-model.number="form.packages" type="number" min="1" class="input" /></div>
        <div><label class="label">Temperature</label><select v-model="form.temperature_class" class="input"><option value="AMBIENT">Ambient</option><option value="COLD">Cold chain 2–8 °C</option></select></div>
        <label class="flex items-center gap-2 text-[13px] font-semibold"><input v-model="form.controlled" type="checkbox" class="accent-[rgb(var(--brand))]" />Controlled drugs</label>
        <div class="sm:col-span-2"><label class="label">Notes</label><input v-model="form.notes" class="input" /></div>
      </div>
      <template #footer>
        <button class="btn-secondary" @click="creating = false">Cancel</button>
        <button class="btn-primary" :disabled="busy || !form.consignee_name || !form.address || !form.lat" @click="create">Create shipment</button>
      </template>
    </Modal>
  </div>
</template>
