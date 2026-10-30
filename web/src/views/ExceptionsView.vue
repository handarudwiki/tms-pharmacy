<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { useContext } from '@/stores/context'
import { useAction } from '@/lib/useAction'
import { dateTime, titleCase } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import Tabs from '@/components/Tabs.vue'
import StatusPill from '@/components/StatusPill.vue'
import Modal from '@/components/Modal.vue'
import EmptyState from '@/components/EmptyState.vue'

const auth = useAuth()
const ctx = useContext()
const router = useRouter()
const { busy, run } = useAction()
const tab = ref('OPEN')
const type = ref('')
const items = ref<any[]>([])
const resolving = ref<any>(null)
const resolution = ref('')

async function load() {
  items.value = (await api.get('/exceptions', { depot_id: ctx.depotId, status: tab.value === 'ALL' ? '' : tab.value, limit: 500 })).exceptions ?? []
}
onMounted(load)
const types = computed(() => [...new Set(items.value.map((e) => e.exception_type))].map((t) => ({ t, n: items.value.filter((e) => e.exception_type === t).length })))
const shown = computed(() => items.value.filter((e) => !type.value || e.exception_type === type.value))
const hints: Record<string, string> = {
  TEMPERATURE_EXCURSION: 'Quarantine the affected goods on return and ask QA for a stability assessment before re-delivery.',
  FAILED_DELIVERY: 'The shipment is back in the planning pool; confirm a delivery window with the consignee.',
  RETURNED_TO_DEPOT: 'Goods are coming back to the depot — register a return in the WMS.',
  LATE_DELIVERY: 'Check the cause (traffic, late release, waiting time) and inform customer service.',
}
async function resolve() {
  if (await run(() => api.post(`/exceptions/${resolving.value.id}/resolve`, { resolution: resolution.value }), 'Exception resolved')) {
    resolving.value = null
    resolution.value = ''
    load()
  }
}
</script>

<template>
  <div>
    <PageHeader title="Exceptions" subtitle="Failed deliveries, returns, late drops and cold-chain excursions, tracked until resolved" />
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <Tabs v-model="tab" :tabs="[{ key: 'OPEN', label: 'Open' }, { key: 'RESOLVED', label: 'Resolved' }, { key: 'ALL', label: 'All' }]" @update:model-value="load" />
      <div class="flex flex-wrap gap-1.5">
        <button :class="['rounded-lg border px-2.5 py-1 text-xs font-semibold', !type ? 'border-brand bg-brand-soft text-brand-ink' : 'border-line text-ink-2']" @click="type = ''">All types</button>
        <button v-for="s in types" :key="s.t" :class="['rounded-lg border px-2.5 py-1 text-xs font-semibold', type === s.t ? 'border-brand bg-brand-soft text-brand-ink' : 'border-line text-ink-2']" @click="type = s.t">{{ titleCase(s.t) }} · {{ s.n }}</button>
      </div>
    </div>
    <section class="card card-pad">
      <EmptyState v-if="!shown.length" message="No exceptions" />
      <div v-else class="table-wrap"><table class="tbl">
        <thead><tr><th>Severity</th><th>Exception</th><th>Shipment</th><th>Raised</th><th /></tr></thead>
        <tbody>
          <tr v-for="e in shown" :key="e.id">
            <td><StatusPill :status="e.severity" /></td>
            <td class="max-w-[460px]"><div class="font-bold">{{ titleCase(e.exception_type) }}</div><div class="text-xs text-ink-3">{{ e.details }}</div>
              <div v-if="e.resolution" class="mt-1 text-xs text-good">✓ {{ e.resolution }} — {{ e.resolved_by.split('@')[0] }}</div></td>
            <td><button class="font-semibold text-brand-ink hover:underline" @click="router.push({ name: 'shipments', query: { q: e.shipment_number } })">{{ e.shipment_number }}</button>
              <div v-if="e.trip_id" class="text-xs"><button class="text-ink-3 hover:underline" @click="router.push({ name: 'trips', query: { id: e.trip_id } })">view trip</button></div></td>
            <td class="whitespace-nowrap text-ink-3">{{ dateTime(e.created_at) }}<div class="text-[11px]">{{ e.raised_by.split('@')[0] }}</div></td>
            <td class="text-right"><button v-if="e.status === 'OPEN' && auth.can('TRANSPORT_MANAGER', 'DISPATCHER', 'CUSTOMER_SERVICE')" class="btn-secondary btn-sm" @click="resolving = e">Resolve</button></td>
          </tr>
        </tbody>
      </table></div>
    </section>
    <Modal v-if="resolving" :title="`Resolve ${titleCase(resolving.exception_type)}`" :subtitle="resolving.shipment_number" @close="resolving = null">
      <p class="mb-3 rounded-xl bg-muted/70 px-3 py-2 text-[13px] text-ink-2">{{ hints[resolving.exception_type] ?? resolving.details }}</p>
      <label class="label">Resolution</label><textarea v-model="resolution" rows="3" class="input" placeholder="What was done and why" />
      <template #footer><button class="btn-secondary" @click="resolving = null">Cancel</button><button class="btn-primary" :disabled="busy || !resolution" @click="resolve">Mark resolved</button></template>
    </Modal>
  </div>
</template>
