<script setup lang="ts">
defineProps<{ tabs: { key: string; label: string; count?: number }[] }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <div class="inline-flex flex-wrap gap-1 rounded-2xl bg-muted p-1" role="tablist">
    <button v-for="t in tabs" :key="t.key" role="tab" :aria-selected="model === t.key"
      :class="['tab', model === t.key && 'tab-active']" @click="model = t.key">
      {{ t.label }}
      <span v-if="t.count !== undefined" class="ml-1 rounded-md bg-line/70 px-1.5 text-[11px] text-ink-2">{{ t.count }}</span>
    </button>
  </div>
</template>
