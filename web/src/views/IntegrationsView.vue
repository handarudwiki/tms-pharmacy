<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RefreshCw, ArrowRight, Warehouse, Truck } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useContext } from '@/stores/context'
import { useAction } from '@/lib/useAction'
import { fmt, dateTime } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import StatusPill from '@/components/StatusPill.vue'
import EmptyState from '@/components/EmptyState.vue'

const ctx = useContext()
const { busy, run } = useAction()
const result = ref<any>(null)
const imported = ref<any[]>([])
async function load() {
  imported.value = ((await api.get('/shipments', { depot_id: ctx.depotId, limit: 1000 })).shipments ?? []).filter((s: any) => s.source === 'WMS')
}
onMounted(load)
async function sync() {
  const r = await run(() => api.post('/integrations/wms/sync', { depot_id: ctx.depotId }))
  if (r) { result.value = r; load() }
}
</script>

<template>
  <div>
    <PageHeader title="Integrations" subtitle="Shipment readiness flows in from the WMS; delivery status flows back through shipment events">
      <button class="btn-primary" :disabled="busy" @click="sync"><RefreshCw class="h-4 w-4" />Sync from WMS</button>
    </PageHeader>
    <section class="card card-pad mb-5">
      <div class="flex flex-wrap items-center gap-4">
        <div class="flex items-center gap-3 rounded-xl bg-muted/70 px-4 py-3"><Warehouse class="h-5 w-5 text-ink-2" /><div><div class="text-[13px] font-bold">Pharmacy WMS</div><div class="text-xs text-ink-3">shipment.ready events (packed orders)</div></div></div>
        <ArrowRight class="h-5 w-5 text-ink-3" />
        <div class="flex items-center gap-3 rounded-xl bg-brand-soft px-4 py-3"><Truck class="h-5 w-5 text-brand-ink" /><div><div class="text-[13px] font-bold">Pharmacy TMS</div><div class="text-xs text-ink-3">shipments in {{ ctx.depot?.code }} planning pool</div></div></div>
      </div>
      <p class="mt-4 text-[13px] text-ink-2">Imports are keyed by the WMS order number, so syncing repeatedly never creates duplicates. Weight, temperature class and controlled-drug flags come from the WMS packing data; known customers are geocoded automatically, others are flagged as approximate for the dispatcher to confirm.</p>
      <div v-if="result" class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div class="rounded-xl bg-muted/70 p-3"><div class="text-xs text-ink-3">WMS events read</div><div class="text-[20px] font-extrabold">{{ result.events_seen }}</div></div>
        <div class="rounded-xl bg-good-soft p-3"><div class="text-xs text-good">Imported</div><div class="text-[20px] font-extrabold text-good">{{ result.imported }}</div></div>
        <div class="rounded-xl bg-muted/70 p-3"><div class="text-xs text-ink-3">Already imported</div><div class="text-[20px] font-extrabold">{{ result.already_imported }}</div></div>
        <div class="rounded-xl bg-muted/70 p-3"><div class="text-xs text-ink-3">Skipped (already shipped)</div><div class="text-[20px] font-extrabold">{{ result.skipped }}</div></div>
      </div>
    </section>
    <section class="card card-pad">
      <h2 class="card-title">Shipments received from the WMS</h2>
      <EmptyState v-if="!imported.length" message="Nothing imported yet — pack an order in the WMS, then sync" />
      <div v-else class="table-wrap mt-3"><table class="tbl">
        <thead><tr><th>Shipment</th><th>WMS order</th><th>Consignee</th><th class="num">Weight</th><th>Handling</th><th>Imported</th><th>Status</th></tr></thead>
        <tbody><tr v-for="s in imported" :key="s.id">
          <td class="font-bold">{{ s.shipment_number }}</td><td class="text-ink-2">{{ s.external_ref.replace('WMS:', '') }}</td>
          <td>{{ s.consignee_name }}<div v-if="s.location_approximate" class="text-[11px] text-warn">approximate location</div></td>
          <td class="num">{{ fmt(s.weight_kg, 1) }} kg</td><td class="text-xs">{{ s.temperature_class === 'COLD' ? '2–8 °C' : 'Ambient' }}{{ s.controlled ? ' · controlled' : '' }}</td>
          <td class="text-ink-3">{{ dateTime(s.created_at) }}</td><td><StatusPill :status="s.status" /></td>
        </tr></tbody>
      </table></div>
    </section>
  </div>
</template>
