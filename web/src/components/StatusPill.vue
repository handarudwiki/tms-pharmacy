<script setup lang="ts">
import { computed } from 'vue'
import { titleCase } from '@/lib/format'

const props = defineProps<{ status: string; label?: string }>()

const tones: Record<string, string[]> = {
  good: ['COMPLETED', 'DELIVERED', 'ACTIVE', 'AVAILABLE', 'AWARDED', 'RESOLVED', 'GOOD', 'ON_TIME', 'AMBIENT_OK'],
  info: ['IN_PROGRESS', 'DISPATCHED', 'RELEASED', 'ARRIVED', 'IN_TRANSIT'],
  brand: ['PLANNED', 'OPEN', 'NEW', 'TENDERED'],
  warn: ['PENDING', 'MEDIUM', 'MAINTENANCE', 'ON_LEAVE', 'EXPIRING', 'LOW'],
  serious: ['RETURNED', 'LATE'],
  bad: ['FAILED', 'CANCELLED', 'HIGH', 'EXPIRED', 'INACTIVE', 'EXCURSION', 'CRITICAL'],
}
const styles: Record<string, string> = {
  good: 'bg-good-soft text-good',
  info: 'bg-info-soft text-info',
  brand: 'bg-brand-soft text-brand-ink',
  warn: 'bg-warn-soft text-warn',
  serious: 'bg-serious-soft text-serious',
  bad: 'bg-bad-soft text-bad',
  neutral: 'bg-muted text-ink-2',
}
const tone = computed(() => Object.keys(tones).find((t) => tones[t].includes(props.status)) ?? 'neutral')
</script>

<template>
  <span :class="['inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11.5px] font-semibold whitespace-nowrap', styles[tone]]">
    <span class="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
    {{ label ?? titleCase(status) }}
  </span>
</template>
