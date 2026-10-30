<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '@/api/client'
import { dateTime } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'

const events = ref<any[]>([])
const entity = ref('')
async function load() {
  events.value = (await api.get('/audit', { entity_type: entity.value, limit: 300 })).events ?? []
}
onMounted(load)
const types = ['', 'trip', 'stop', 'shipment', 'tender', 'carrier', 'vehicle', 'driver', 'maintenance', 'depot', 'user', 'exception', 'integration']
</script>

<template>
  <div>
    <PageHeader title="Audit Trail" subtitle="Every sensitive action, attributable to an actor and traceable across services by correlation ID">
      <select v-model="entity" class="input h-10 w-48" @change="load"><option v-for="t in types" :key="t" :value="t">{{ t || 'All entities' }}</option></select>
    </PageHeader>
    <section class="card card-pad">
      <EmptyState v-if="!events.length" message="No audit events" />
      <div v-else class="table-wrap">
        <table class="tbl">
          <thead><tr><th>When</th><th>Actor</th><th>Action</th><th>Entity</th><th>Correlation ID</th></tr></thead>
          <tbody>
            <tr v-for="e in events" :key="e.id">
              <td class="whitespace-nowrap text-ink-2">{{ dateTime(e.created_at) }}</td>
              <td class="font-semibold">{{ e.actor_email }}</td>
              <td><code class="rounded-md bg-muted px-1.5 py-0.5 text-[12px]">{{ e.action }}</code></td>
              <td class="text-ink-2">{{ e.entity_type }} <span class="text-[11px] text-ink-3">{{ e.entity_id.slice(0, 8) }}</span></td>
              <td class="font-mono text-[11px] text-ink-3">{{ e.correlation_id }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
