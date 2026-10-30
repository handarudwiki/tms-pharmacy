<script setup lang="ts">
import { Inbox } from 'lucide-vue-next'
defineProps<{ message: string }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center gap-2 py-12 text-center text-ink-3">
    <span class="grid h-11 w-11 place-items-center rounded-2xl bg-muted"><Inbox class="h-5 w-5" /></span>
    <p class="text-[13px]">{{ message }}</p>
  </div>
</template>
