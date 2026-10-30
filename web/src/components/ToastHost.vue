<script setup lang="ts">
import { CheckCircle2, AlertTriangle, Info } from 'lucide-vue-next'
import { useToast } from '@/stores/toast'
const toast = useToast()
const icon = { success: CheckCircle2, error: AlertTriangle, info: Info }
const tone = { success: 'text-good', error: 'text-bad', info: 'text-info' }
</script>

<template>
  <div class="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-[min(380px,calc(100vw-2rem))] flex-col gap-2" aria-live="polite">
    <TransitionGroup enter-from-class="opacity-0 translate-y-2" leave-to-class="opacity-0" enter-active-class="transition" leave-active-class="transition">
      <div v-for="t in toast.items" :key="t.id" class="card pointer-events-auto flex items-start gap-3 px-4 py-3">
        <component :is="icon[t.kind]" :class="['mt-0.5 h-4 w-4 shrink-0', tone[t.kind]]" />
        <p class="text-[13px] text-ink">{{ t.message }}</p>
      </div>
    </TransitionGroup>
  </div>
</template>
