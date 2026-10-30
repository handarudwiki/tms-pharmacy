<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Plus, Snowflake, Lock, Trophy, Pencil, Ban } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useContext } from '@/stores/context'
import { useAction } from '@/lib/useAction'
import { fmt, dateTime, titleCase, rupiah, pct, day } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import Tabs from '@/components/Tabs.vue'
import StatusPill from '@/components/StatusPill.vue'
import Modal from '@/components/Modal.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const auth = useAuth()
const ctx = useContext()
const { busy, run } = useAction()
const tab = ref('tenders')
const tenders = ref<any[]>([])
const carriers = ref<any[]>([])
const pending = ref<any[]>([])
const selected = ref<any>(null)
const creating = ref(false)
const picked = ref<Set<string>>(new Set())
const editing = ref<any>(null)
const bid = reactive({ carrier_id: '', amount: 0, transit_days: 1, note: '' })
const cancelReason = ref('')
const canManage = computed(() => auth.can('CARRIER_MANAGER', 'TRANSPORT_MANAGER'))

async function load() {
  const [t, c] = await Promise.all([api.get('/tenders', { depot_id: ctx.depotId }), api.get('/carriers', { include_inactive: true })])
  tenders.value = t.tenders ?? []
  carriers.value = c.carriers ?? []
  if (selected.value) selected.value = tenders.value.find((x) => x.id === selected.value.id) ?? null
}
onMounted(async () => {
  await load()
  if (route.query.tender) {
    await openCreate()
    picked.value = new Set((route.query.tender as string).split(','))
  }
})
async function openCreate() {
  pending.value = (await api.get('/shipments', { depot_id: ctx.depotId, status: 'PENDING' })).shipments ?? []
  picked.value = new Set()
  creating.value = true
}
function toggle(id: string) {
  const s = new Set(picked.value)
  s.has(id) ? s.delete(id) : s.add(id)
  picked.value = s
}
async function createTender() {
  const t = await run(() => api.post('/tenders', { depot_id: ctx.depotId, shipment_ids: [...picked.value] }), 'Tender opened — carriers quoted from their rate cards')
  if (t) { creating.value = false; await load(); selected.value = t }
}
async function award(b: any) {
  if (await run(() => api.post(`/tenders/${selected.value.id}/award`, { bid_id: b.id }), `Awarded to ${b.carrier_name} — shipments handed over`)) load()
}
async function submitBid() {
  if (await run(() => api.post(`/tenders/${selected.value.id}/bids`, bid), 'Bid recorded')) { bid.amount = 0; bid.note = ''; load() }
}
async function cancelTender() {
  if (await run(() => api.post(`/tenders/${selected.value.id}/cancel`, { reason: cancelReason.value }), 'Tender cancelled, shipments back in planning')) { cancelReason.value = ''; load() }
}
const best = (t: any) => t.bids.filter((b: any) => b.compliant).sort((a: any, b: any) => a.amount - b.amount)[0]
const awarded = (t: any) => t.bids.find((b: any) => b.id === t.awarded_bid_id)

const cBlank = () => ({ id: '', code: '', name: '', service_level: 'NEXT_DAY', supports_cold: false, supports_controlled: false, base_rate: 25000, rate_per_kg: 3000,
  rate_per_km: 500, cold_surcharge_pct: 0, on_time_pct: 95, transit_days: 1, contact: '', active: true })
const cForm = reactive(cBlank())
function editCarrier(c?: any) { Object.assign(cForm, cBlank(), c ?? {}); editing.value = c ?? {} }
async function saveCarrier() {
  if (await run(() => (cForm.id ? api.put(`/carriers/${cForm.id}`, cForm) : api.post('/carriers', cForm)), 'Carrier saved')) { editing.value = null; load() }
}
</script>

<template>
  <div>
    <PageHeader title="Carriers & Tenders" subtitle="Outsource overflow or specialist shipments — every carrier is quoted from its contracted rate card, non-compliant bids cannot win">
      <button v-if="canManage && tab === 'tenders'" class="btn-primary" @click="openCreate"><Plus class="h-4 w-4" />New tender</button>
      <button v-if="canManage && tab === 'carriers'" class="btn-primary" @click="editCarrier()"><Plus class="h-4 w-4" />Add carrier</button>
    </PageHeader>
    <Tabs v-model="tab" class="mb-4" :tabs="[{ key: 'tenders', label: 'Tenders', count: tenders.length }, { key: 'carriers', label: 'Carriers', count: carriers.length }]" />

    <section v-if="tab === 'tenders'" class="card card-pad">
      <EmptyState v-if="!tenders.length" message="No tenders yet" />
      <div v-else class="table-wrap"><table class="tbl">
        <thead><tr><th>Tender</th><th class="num">Shipments</th><th class="num">Weight</th><th>Requires</th><th>Best compliant bid</th><th>Awarded to</th><th>Status</th></tr></thead>
        <tbody>
          <tr v-for="t in tenders" :key="t.id" class="clickable" @click="selected = t">
            <td><div class="font-bold">{{ t.tender_number }}</div><div class="text-xs text-ink-3">{{ dateTime(t.created_at) }}</div></td>
            <td class="num">{{ t.shipments.length }}</td>
            <td class="num">{{ fmt(t.total_weight_kg, 1) }} kg</td>
            <td><span class="inline-flex gap-1 text-xs"><span v-if="t.requires_cold" class="inline-flex items-center gap-1 rounded-md bg-info-soft px-1.5 py-0.5 font-semibold text-info"><Snowflake class="h-3 w-3" />Cold</span>
              <span v-if="t.requires_controlled" class="inline-flex items-center gap-1 rounded-md bg-serious-soft px-1.5 py-0.5 font-semibold text-serious"><Lock class="h-3 w-3" />Controlled</span>
              <span v-if="!t.requires_cold && !t.requires_controlled" class="text-ink-3">Standard</span></span></td>
            <td>{{ best(t) ? `${best(t).carrier_name} · ${rupiah(best(t).amount)}` : '—' }}</td>
            <td class="font-semibold">{{ awarded(t) ? `${awarded(t).carrier_name} · ${rupiah(awarded(t).amount)}` : '—' }}</td>
            <td><StatusPill :status="t.status" /></td>
          </tr>
        </tbody>
      </table></div>
    </section>

    <section v-else class="card card-pad">
      <div class="table-wrap"><table class="tbl">
        <thead><tr><th>Carrier</th><th>Service</th><th>Capabilities</th><th class="num">Base</th><th class="num">Per kg</th><th class="num">Per km</th><th class="num">Cold surcharge</th><th class="num">On-time</th><th /></tr></thead>
        <tbody>
          <tr v-for="c in carriers" :key="c.id" :class="!c.active && 'opacity-60'">
            <td><div class="font-semibold">{{ c.name }}</div><div class="text-xs text-ink-3">{{ c.code }} · {{ c.contact }}</div></td>
            <td class="text-ink-2">{{ titleCase(c.service_level) }}</td>
            <td><span class="inline-flex gap-1 text-xs"><span v-if="c.supports_cold" class="inline-flex items-center gap-1 rounded-md bg-info-soft px-1.5 py-0.5 font-semibold text-info"><Snowflake class="h-3 w-3" />Cold</span>
              <span v-if="c.supports_controlled" class="inline-flex items-center gap-1 rounded-md bg-serious-soft px-1.5 py-0.5 font-semibold text-serious"><Lock class="h-3 w-3" />Controlled</span>
              <span v-if="!c.supports_cold && !c.supports_controlled" class="text-ink-3">Ambient only</span></span></td>
            <td class="num">{{ rupiah(c.base_rate) }}</td><td class="num">{{ rupiah(c.rate_per_kg) }}</td><td class="num">{{ rupiah(c.rate_per_km) }}</td>
            <td class="num">{{ c.cold_surcharge_pct ? pct(c.cold_surcharge_pct, 0) : '—' }}</td><td class="num">{{ pct(c.on_time_pct, 0) }}</td>
            <td class="text-right"><button v-if="canManage" class="btn-ghost btn-sm" aria-label="Edit carrier" @click="editCarrier(c)"><Pencil class="h-4 w-4" /></button></td>
          </tr>
        </tbody>
      </table></div>
    </section>

    <Modal v-if="selected" :title="selected.tender_number" :subtitle="`${selected.shipments.length} shipment(s) · ${fmt(selected.total_weight_kg, 1)} kg · deadline ${dateTime(selected.deadline)}`" wide @close="selected = null">
      <div class="mb-4 flex items-center gap-2"><StatusPill :status="selected.status" /><span v-if="selected.cancel_reason" class="text-xs text-bad">{{ selected.cancel_reason }}</span></div>
      <div class="table-wrap"><table class="tbl">
        <thead><tr><th>Shipment</th><th>Consignee</th><th class="num">Kg</th><th class="num">Distance</th><th>Handling</th></tr></thead>
        <tbody><tr v-for="s in selected.shipments" :key="s.shipment_id"><td class="font-semibold">{{ s.shipment_number }}</td><td>{{ s.consignee_name }}</td>
          <td class="num">{{ fmt(s.weight_kg, 1) }}</td><td class="num">{{ fmt(s.distance_km, 1) }} km</td>
          <td><Snowflake v-if="s.temperature_class === 'COLD'" class="inline h-4 w-4 text-info" /><Lock v-if="s.controlled" class="inline h-4 w-4 text-serious" /></td></tr></tbody>
      </table></div>
      <h3 class="mb-2 mt-5 text-[13px] font-bold">Bids</h3>
      <div class="table-wrap"><table class="tbl">
        <thead><tr><th>Carrier</th><th class="num">Amount</th><th class="num">Transit</th><th>Source</th><th>Compliance</th><th /></tr></thead>
        <tbody>
          <tr v-for="(b, i) in selected.bids" :key="b.id" :class="!b.compliant && 'opacity-60'">
            <td class="font-semibold">{{ b.carrier_name }} <Trophy v-if="b.id === selected.awarded_bid_id" class="inline h-4 w-4 text-warn" aria-label="Awarded" /></td>
            <td class="num text-[14px] font-extrabold">{{ rupiah(b.amount) }}<div v-if="i === 0 && b.compliant" class="text-[11px] font-semibold text-good">lowest compliant</div></td>
            <td class="num text-ink-2">{{ b.transit_days ? `${b.transit_days} d` : 'same day' }}</td>
            <td class="text-xs text-ink-2">{{ titleCase(b.source) }}</td>
            <td><StatusPill :status="b.compliant ? 'GOOD' : 'FAILED'" :label="b.compliant ? 'Compliant' : b.non_compliance_reason" /></td>
            <td class="text-right"><button v-if="canManage && selected.status === 'OPEN' && b.compliant" class="btn-primary btn-sm" :disabled="busy" @click="award(b)">Award</button></td>
          </tr>
        </tbody>
      </table></div>
      <form v-if="canManage && selected.status === 'OPEN'" class="mt-4 grid gap-2 rounded-2xl bg-muted/60 p-3 sm:grid-cols-[1fr_150px_100px_auto]" @submit.prevent="submitBid">
        <select v-model="bid.carrier_id" class="input" aria-label="Carrier"><option value="" disabled>Manual bid from…</option><option v-for="c in carriers.filter((c) => c.active)" :key="c.id" :value="c.id">{{ c.name }}</option></select>
        <input v-model.number="bid.amount" type="number" min="0" class="input" aria-label="Amount" placeholder="IDR" />
        <input v-model.number="bid.transit_days" type="number" min="0" class="input" aria-label="Transit days" />
        <button class="btn-secondary" :disabled="busy || !bid.carrier_id || bid.amount <= 0">Add bid</button>
      </form>
      <template v-if="canManage && selected.status === 'OPEN'" #footer>
        <input v-model="cancelReason" class="input max-w-xs" placeholder="Reason to cancel" />
        <button class="btn-ghost text-bad" :disabled="busy || !cancelReason" @click="cancelTender"><Ban class="h-4 w-4" />Cancel tender</button>
      </template>
    </Modal>

    <Modal v-if="creating" title="New tender" subtitle="Select pending shipments to offer to carriers" wide @close="creating = false">
      <EmptyState v-if="!pending.length" message="No pending shipments to tender" />
      <div v-else class="table-wrap max-h-[420px] overflow-y-auto"><table class="tbl">
        <thead><tr><th class="w-8" /><th>Shipment</th><th>Consignee</th><th class="num">Kg</th><th>Due</th><th>Handling</th></tr></thead>
        <tbody><tr v-for="s in pending" :key="s.id" class="clickable" @click="toggle(s.id)">
          <td><input type="checkbox" :checked="picked.has(s.id)" class="accent-[rgb(var(--brand))]" @click.stop="toggle(s.id)" :aria-label="`Select ${s.shipment_number}`" /></td>
          <td class="font-semibold">{{ s.shipment_number }}</td><td>{{ s.consignee_name }}</td><td class="num">{{ fmt(s.weight_kg, 1) }}</td><td class="text-ink-2">{{ day(s.required_date) }}</td>
          <td><Snowflake v-if="s.temperature_class === 'COLD'" class="inline h-4 w-4 text-info" /><Lock v-if="s.controlled" class="inline h-4 w-4 text-serious" /></td></tr></tbody>
      </table></div>
      <template #footer><button class="btn-secondary" @click="creating = false">Cancel</button><button class="btn-primary" :disabled="busy || !picked.size" @click="createTender">Open tender ({{ picked.size }})</button></template>
    </Modal>

    <Modal v-if="editing" :title="cForm.id ? `Edit ${cForm.name}` : 'Add carrier'" wide @close="editing = null">
      <div class="grid gap-4 sm:grid-cols-3">
        <div><label class="label">Code</label><input v-model="cForm.code" class="input" /></div>
        <div><label class="label">Name</label><input v-model="cForm.name" class="input" /></div>
        <div><label class="label">Service level</label><select v-model="cForm.service_level" class="input"><option v-for="s in ['SAME_DAY', 'NEXT_DAY', 'REGULAR']" :key="s" :value="s">{{ titleCase(s) }}</option></select></div>
        <div><label class="label">Base rate (IDR / shipment)</label><input v-model.number="cForm.base_rate" type="number" class="input" /></div>
        <div><label class="label">Rate per kg</label><input v-model.number="cForm.rate_per_kg" type="number" class="input" /></div>
        <div><label class="label">Rate per km</label><input v-model.number="cForm.rate_per_km" type="number" class="input" /></div>
        <div><label class="label">Cold surcharge %</label><input v-model.number="cForm.cold_surcharge_pct" type="number" class="input" /></div>
        <div><label class="label">On-time %</label><input v-model.number="cForm.on_time_pct" type="number" class="input" /></div>
        <div><label class="label">Transit days</label><input v-model.number="cForm.transit_days" type="number" class="input" /></div>
        <div class="sm:col-span-2"><label class="label">Contact</label><input v-model="cForm.contact" class="input" /></div>
        <div class="flex flex-col gap-2 pt-5 text-[13px] font-semibold">
          <label class="flex items-center gap-2"><input v-model="cForm.supports_cold" type="checkbox" class="accent-[rgb(var(--brand))]" />Cold chain</label>
          <label class="flex items-center gap-2"><input v-model="cForm.supports_controlled" type="checkbox" class="accent-[rgb(var(--brand))]" />Controlled drugs</label>
          <label v-if="cForm.id" class="flex items-center gap-2"><input v-model="cForm.active" type="checkbox" class="accent-[rgb(var(--brand))]" />Active</label>
        </div>
      </div>
      <template #footer><button class="btn-secondary" @click="editing = null">Cancel</button><button class="btn-primary" :disabled="busy || !cForm.code || !cForm.name" @click="saveCarrier">Save</button></template>
    </Modal>
  </div>
</template>
