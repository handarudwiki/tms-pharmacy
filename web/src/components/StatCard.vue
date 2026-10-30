<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  label: string
  value: string | number
  icon: Component
  tone?: 'brand' | 'info' | 'good' | 'warn' | 'serious' | 'bad'
  hint?: string
}>()
const soft: Record<string, string> = {
  brand: 'bg-brand-soft text-brand-ink',
  info: 'bg-info-soft text-info',
  good: 'bg-good-soft text-good',
  warn: 'bg-warn-soft text-warn',
  serious: 'bg-serious-soft text-serious',
  bad: 'bg-bad-soft text-bad',
}
</script>

<template>
  <div class="card card-pad flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <span class="text-[13px] font-semibold text-ink-2">{{ label }}</span>
      <span :class="['grid h-10 w-10 place-items-center rounded-xl', soft[tone ?? 'brand']]">
        <component :is="icon" class="h-[18px] w-[18px]" />
      </span>
    </div>
    <div>
      <div class="text-[28px] font-extrabold leading-none tracking-tight text-ink tabular-nums">{{ value }}</div>
      <div v-if="hint" class="mt-2 text-xs text-ink-3"><slot name="hint">{{ hint }}</slot></div>
    </div>
  </div>
</template>
