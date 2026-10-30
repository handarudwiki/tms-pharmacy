<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Plus, Snowflake, Lock, Wrench, Pencil } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useContext } from '@/stores/context'
import { useAction } from '@/lib/useAction'
import { fmt, day, daysUntil, titleCase, rupiah, today } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import Tabs from '@/components/Tabs.vue'
import StatusPill from '@/components/StatusPill.vue'
import Modal from '@/components/Modal.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const auth = useAuth()
const ctx = useContext()
const { busy, run } = useAction()
const tab = ref((route.query.tab as string) || 'vehicles')
const maintenance = ref<any[]>([])
const modal = ref<'' | 'vehicle' | 'driver' | 'maintenance' | 'depot' | 'status'>('')
const isFleet = computed(() => auth.can('FLEET_MANAGER', 'TRANSPORT_MANAGER'))

async function loadMaintenance() {
  maintenance.value = (await api.get('/maintenance', { limit: 100 })).records ?? []
}
onMounted(loadMaintenance)

const vBlank = () => ({ id: '', plate: '', vehicle_type: 'VAN', capacity_kg: 850, capacity_volume_l: 4500, controlled_certified: false, odometer_km: 0, fuel_km_per_liter: 9, next_service_date: today(90) })
const vForm = reactive(vBlank())
const dBlank = () => ({ id: '', name: '', phone: '', license_number: '', license_class: 'B1', license_expiry: today(365), cold_chain_certified: false, controlled_certified: false, user_email: '' })
const dForm = reactive(dBlank())
const mForm = reactive({ vehicle_id: '', maintenance_type: 'SERVICE', description: '', performed_on: today(), cost: 0, odometer_km: 0, next_due_date: today(90) })
const depotForm = reactive({ code: '', name: '', address: '', lat: -6.2, lng: 106.85 })
const statusTarget = ref<{ kind: 'vehicles' | 'drivers'; item: any; status: string; reason: string } | null>(null)

function editVehicle(v?: any) { Object.assign(vForm, vBlank(), v ?? {}); modal.value = 'vehicle' }
function editDriver(d?: any) { Object.assign(dForm, dBlank(), d ?? {}); modal.value = 'driver' }
async function saveVehicle() {
  const body = { ...vForm, depot_id: ctx.depotId }
  if (await run(() => (vForm.id ? api.put(`/vehicles/${vForm.id}`, body) : api.post('/vehicles', body)), 'Vehicle saved')) { modal.value = ''; ctx.refreshFleet() }
}
async function saveDriver() {
  const body = { ...dForm, depot_id: ctx.depotId }
  if (await run(() => (dForm.id ? api.put(`/drivers/${dForm.id}`, body) : api.post('/drivers', body)), 'Driver saved')) { modal.value = ''; ctx.refreshFleet() }
}
async function saveMaintenance() {
  if (await run(() => api.post('/maintenance', mForm), 'Maintenance recorded')) { modal.value = ''; loadMaintenance(); ctx.refreshFleet() }
}
async function saveDepot() {
  const d = await run(() => api.post('/depots', depotForm), 'Depot created')
  if (d) { modal.value = ''; await ctx.load(); ctx.selectDepot(d.id) }
}
async function saveStatus() {
  const s = statusTarget.value!
  if (await run(() => api.post(`/${s.kind}/${s.item.id}/status`, { status: s.status, reason: s.reason }), 'Status updated')) { modal.value = ''; ctx.refreshFleet() }
}
function changeStatus(kind: 'vehicles' | 'drivers', item: any) {
  statusTarget.value = { kind, item, status: item.status, reason: item.status_reason ?? '' }
  modal.value = 'status'
}
const licenceTone = (d: string) => { const n = daysUntil(d) ?? 0; return n < 0 ? ['EXPIRED', 'Expired'] : n <= 30 ? ['EXPIRING', `${n} days left`] : ['GOOD', day(d)] }
const serviceTone = (d: string) => { const n = daysUntil(d); return n === null ? null : n < 0 ? ['EXPIRED', 'Overdue'] : n <= 14 ? ['EXPIRING', `in ${n} days`] : ['GOOD', day(d)] }
</script>

<template>
  <div>
    <PageHeader title="Fleet & Drivers" subtitle="Vehicles, temperature capability, certifications, licences and maintenance">
      <template v-if="isFleet">
        <button v-if="tab === 'vehicles'" class="btn-primary" @click="editVehicle()"><Plus class="h-4 w-4" />Add vehicle</button>
        <button v-if="tab === 'drivers'" class="btn-primary" @click="editDriver()"><Plus class="h-4 w-4" />Add driver</button>
        <button v-if="tab === 'maintenance'" class="btn-primary" @click="mForm.vehicle_id = ctx.vehicles[0]?.id ?? ''; modal = 'maintenance'"><Wrench class="h-4 w-4" />Record maintenance</button>
      </template>
      <button v-if="tab === 'depots' && auth.can('ORG_ADMIN')" class="btn-primary" @click="modal = 'depot'"><Plus class="h-4 w-4" />Add depot</button>
    </PageHeader>
    <Tabs v-model="tab" class="mb-4" :tabs="[{ key: 'vehicles', label: 'Vehicles', count: ctx.vehicles.length }, { key: 'drivers', label: 'Drivers', count: ctx.drivers.length }, { key: 'maintenance', label: 'Maintenance' }, { key: 'depots', label: 'Depots' }]" />

    <section class="card card-pad">
      <template v-if="tab === 'vehicles'">
        <EmptyState v-if="!ctx.vehicles.length" message="No vehicles at this depot" />
        <div v-else class="table-wrap"><table class="tbl">
          <thead><tr><th>Vehicle</th><th>Type</th><th class="num">Capacity</th><th>Capability</th><th class="num">Odometer</th><th class="num">km/L</th><th>Next service</th><th>Status</th><th /></tr></thead>
          <tbody>
            <tr v-for="v in ctx.vehicles" :key="v.id">
              <td class="text-[14px] font-extrabold">{{ v.plate }}</td>
              <td class="text-ink-2">{{ titleCase(v.vehicle_type) }}</td>
              <td class="num">{{ fmt(v.capacity_kg) }} kg<div class="text-[11px] text-ink-3">{{ fmt(v.capacity_volume_l) }} L</div></td>
              <td><span class="inline-flex flex-wrap gap-1 text-xs">
                <span v-if="v.temperature_class === 'COLD'" class="inline-flex items-center gap-1 rounded-md bg-info-soft px-1.5 py-0.5 font-semibold text-info"><Snowflake class="h-3 w-3" />2–8 °C</span>
                <span v-if="v.controlled_certified" class="inline-flex items-center gap-1 rounded-md bg-serious-soft px-1.5 py-0.5 font-semibold text-serious"><Lock class="h-3 w-3" />Controlled</span>
                <span v-if="v.temperature_class !== 'COLD' && !v.controlled_certified" class="text-ink-3">Ambient</span></span></td>
              <td class="num text-ink-2">{{ fmt(v.odometer_km) }} km</td>
              <td class="num text-ink-2">{{ v.fuel_km_per_liter }}</td>
              <td><StatusPill v-if="serviceTone(v.next_service_date)" :status="serviceTone(v.next_service_date)![0]" :label="serviceTone(v.next_service_date)![1]" /></td>
              <td><StatusPill :status="v.status" /><div v-if="v.status_reason" class="max-w-[180px] truncate text-[11px] text-ink-3">{{ v.status_reason }}</div></td>
              <td class="whitespace-nowrap text-right"><template v-if="isFleet">
                <button class="btn-ghost btn-sm" aria-label="Edit vehicle" @click="editVehicle(v)"><Pencil class="h-4 w-4" /></button>
                <button class="btn-ghost btn-sm" @click="changeStatus('vehicles', v)">Status</button></template></td>
            </tr>
          </tbody>
        </table></div>
      </template>

      <template v-else-if="tab === 'drivers'">
        <div class="table-wrap"><table class="tbl">
          <thead><tr><th>Driver</th><th>Licence</th><th>Licence expiry</th><th>Certifications</th><th>App login</th><th>Status</th><th /></tr></thead>
          <tbody>
            <tr v-for="d in ctx.drivers" :key="d.id">
              <td><div class="font-semibold">{{ d.name }}</div><div class="text-xs text-ink-3">{{ d.phone }}</div></td>
              <td class="text-ink-2">{{ d.license_number }} · {{ d.license_class }}</td>
              <td><StatusPill :status="licenceTone(d.license_expiry)[0]" :label="licenceTone(d.license_expiry)[1]" /></td>
              <td><span class="inline-flex flex-wrap gap-1 text-xs">
                <span v-if="d.cold_chain_certified" class="inline-flex items-center gap-1 rounded-md bg-info-soft px-1.5 py-0.5 font-semibold text-info"><Snowflake class="h-3 w-3" />Cold chain</span>
                <span v-if="d.controlled_certified" class="inline-flex items-center gap-1 rounded-md bg-serious-soft px-1.5 py-0.5 font-semibold text-serious"><Lock class="h-3 w-3" />Controlled</span>
                <span v-if="!d.cold_chain_certified && !d.controlled_certified" class="text-ink-3">—</span></span></td>
              <td class="text-ink-2">{{ d.user_email || '—' }}</td>
              <td><StatusPill :status="d.status" /></td>
              <td class="whitespace-nowrap text-right"><template v-if="isFleet">
                <button class="btn-ghost btn-sm" aria-label="Edit driver" @click="editDriver(d)"><Pencil class="h-4 w-4" /></button>
                <button class="btn-ghost btn-sm" @click="changeStatus('drivers', d)">Status</button></template></td>
            </tr>
          </tbody>
        </table></div>
      </template>

      <template v-else-if="tab === 'maintenance'">
        <EmptyState v-if="!maintenance.length" message="No maintenance recorded" />
        <div v-else class="table-wrap"><table class="tbl">
          <thead><tr><th>Date</th><th>Vehicle</th><th>Type</th><th>Work done</th><th class="num">Cost</th><th class="num">Odometer</th><th>Next due</th><th>By</th></tr></thead>
          <tbody>
            <tr v-for="m in maintenance" :key="m.id">
              <td class="whitespace-nowrap text-ink-2">{{ day(m.performed_on) }}</td>
              <td class="font-bold">{{ ctx.vehicle(m.vehicle_id)?.plate ?? '—' }}</td>
              <td><StatusPill status="NEW" :label="titleCase(m.maintenance_type)" /></td>
              <td class="max-w-[320px] text-ink-2">{{ m.description }}</td>
              <td class="num">{{ rupiah(m.cost) }}</td>
              <td class="num text-ink-2">{{ fmt(m.odometer_km) }}</td>
              <td class="text-ink-2">{{ day(m.next_due_date) }}</td>
              <td class="text-ink-3">{{ m.recorded_by.split('@')[0] }}</td>
            </tr>
          </tbody>
        </table></div>
      </template>

      <template v-else>
        <div class="grid gap-3 sm:grid-cols-2">
          <div v-for="d in ctx.depots" :key="d.id" :class="['rounded-xl border p-4', d.id === ctx.depotId ? 'border-brand bg-brand-soft/40' : 'border-line']">
            <div class="flex items-center justify-between"><span class="font-extrabold">{{ d.code }}</span><span class="text-xs text-ink-3">{{ d.lat.toFixed(4) }}, {{ d.lng.toFixed(4) }}</span></div>
            <div class="text-[13px] font-semibold">{{ d.name }}</div><div class="text-xs text-ink-3">{{ d.address }}</div>
          </div>
        </div>
      </template>
    </section>

    <Modal v-if="modal === 'vehicle'" :title="vForm.id ? `Edit ${vForm.plate}` : 'Add vehicle'" wide @close="modal = ''">
      <div class="grid gap-4 sm:grid-cols-3">
        <div><label class="label">Plate</label><input v-model="vForm.plate" class="input" /></div>
        <div><label class="label">Type</label><select v-model="vForm.vehicle_type" class="input"><option v-for="t in ['MOTORBIKE', 'VAN', 'BOX_TRUCK', 'REEFER_VAN', 'REEFER_TRUCK']" :key="t" :value="t">{{ titleCase(t) }}</option></select></div>
        <label class="mt-6 flex items-center gap-2 text-[13px] font-semibold"><input v-model="vForm.controlled_certified" type="checkbox" class="accent-[rgb(var(--brand))]" />Certified for controlled drugs</label>
        <div><label class="label">Capacity (kg)</label><input v-model.number="vForm.capacity_kg" type="number" class="input" /></div>
        <div><label class="label">Capacity (L)</label><input v-model.number="vForm.capacity_volume_l" type="number" class="input" /></div>
        <div><label class="label">Fuel efficiency (km/L)</label><input v-model.number="vForm.fuel_km_per_liter" type="number" step="0.5" class="input" /></div>
        <div><label class="label">Odometer (km)</label><input v-model.number="vForm.odometer_km" type="number" class="input" /></div>
        <div><label class="label">Next service</label><input v-model="vForm.next_service_date" type="date" class="input" /></div>
      </div>
      <p class="mt-3 text-xs text-ink-3">Reefer types are automatically treated as 2–8 °C cold-chain vehicles.</p>
      <template #footer><button class="btn-secondary" @click="modal = ''">Cancel</button><button class="btn-primary" :disabled="busy || !vForm.plate" @click="saveVehicle">Save</button></template>
    </Modal>

    <Modal v-if="modal === 'driver'" :title="dForm.id ? `Edit ${dForm.name}` : 'Add driver'" wide @close="modal = ''">
      <div class="grid gap-4 sm:grid-cols-3">
        <div><label class="label">Name</label><input v-model="dForm.name" class="input" /></div>
        <div><label class="label">Phone</label><input v-model="dForm.phone" class="input" /></div>
        <div><label class="label">Driver app login (email)</label><input v-model="dForm.user_email" class="input" /></div>
        <div><label class="label">Licence number</label><input v-model="dForm.license_number" class="input" /></div>
        <div><label class="label">Class</label><input v-model="dForm.license_class" class="input" /></div>
        <div><label class="label">Licence expiry</label><input v-model="dForm.license_expiry" type="date" class="input" /></div>
        <label class="flex items-center gap-2 text-[13px] font-semibold"><input v-model="dForm.cold_chain_certified" type="checkbox" class="accent-[rgb(var(--brand))]" />Cold-chain certified</label>
        <label class="flex items-center gap-2 text-[13px] font-semibold"><input v-model="dForm.controlled_certified" type="checkbox" class="accent-[rgb(var(--brand))]" />Controlled-drug certified</label>
      </div>
      <template #footer><button class="btn-secondary" @click="modal = ''">Cancel</button><button class="btn-primary" :disabled="busy || !dForm.name || !dForm.license_number" @click="saveDriver">Save</button></template>
    </Modal>

    <Modal v-if="modal === 'maintenance'" title="Record maintenance" @close="modal = ''">
      <div class="grid gap-4 sm:grid-cols-2">
        <div><label class="label">Vehicle</label><select v-model="mForm.vehicle_id" class="input"><option v-for="v in ctx.vehicles" :key="v.id" :value="v.id">{{ v.plate }}</option></select></div>
        <div><label class="label">Type</label><select v-model="mForm.maintenance_type" class="input"><option v-for="t in ['SERVICE', 'REPAIR', 'INSPECTION', 'TIRES']" :key="t">{{ t }}</option></select></div>
        <div class="sm:col-span-2"><label class="label">Work done</label><input v-model="mForm.description" class="input" /></div>
        <div><label class="label">Date</label><input v-model="mForm.performed_on" type="date" class="input" /></div>
        <div><label class="label">Cost (IDR)</label><input v-model.number="mForm.cost" type="number" class="input" /></div>
        <div><label class="label">Odometer (km)</label><input v-model.number="mForm.odometer_km" type="number" class="input" /></div>
        <div><label class="label">Next due</label><input v-model="mForm.next_due_date" type="date" class="input" /></div>
      </div>
      <template #footer><button class="btn-secondary" @click="modal = ''">Cancel</button><button class="btn-primary" :disabled="busy || !mForm.vehicle_id" @click="saveMaintenance">Save</button></template>
    </Modal>

    <Modal v-if="modal === 'depot'" title="Add depot" @close="modal = ''">
      <div class="grid gap-4 sm:grid-cols-2">
        <div><label class="label">Code</label><input v-model="depotForm.code" class="input" /></div>
        <div><label class="label">Name</label><input v-model="depotForm.name" class="input" /></div>
        <div class="sm:col-span-2"><label class="label">Address</label><input v-model="depotForm.address" class="input" /></div>
        <div><label class="label">Latitude</label><input v-model.number="depotForm.lat" type="number" step="0.0001" class="input" /></div>
        <div><label class="label">Longitude</label><input v-model.number="depotForm.lng" type="number" step="0.0001" class="input" /></div>
      </div>
      <template #footer><button class="btn-secondary" @click="modal = ''">Cancel</button><button class="btn-primary" :disabled="busy || !depotForm.code || !depotForm.name" @click="saveDepot">Create</button></template>
    </Modal>

    <Modal v-if="modal === 'status' && statusTarget" :title="`Status · ${statusTarget.item.plate ?? statusTarget.item.name}`" @close="modal = ''">
      <div class="space-y-4">
        <div><label class="label">Status</label><select v-model="statusTarget.status" class="input">
          <option v-for="s in statusTarget.kind === 'vehicles' ? ['AVAILABLE', 'MAINTENANCE', 'INACTIVE'] : ['ACTIVE', 'ON_LEAVE', 'INACTIVE']" :key="s" :value="s">{{ titleCase(s) }}</option></select></div>
        <div v-if="statusTarget.kind === 'vehicles'"><label class="label">Reason</label><input v-model="statusTarget.reason" class="input" /></div>
        <p class="text-xs text-ink-3">Unavailable vehicles and drivers are excluded from planning.</p>
      </div>
      <template #footer><button class="btn-secondary" @click="modal = ''">Cancel</button><button class="btn-primary" :disabled="busy" @click="saveStatus">Save</button></template>
    </Modal>
  </div>
</template>
