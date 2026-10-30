<script setup lang="ts">
import { X } from 'lucide-vue-next'
import { onMounted, onUnmounted } from 'vue'

defineProps<{ title: string; subtitle?: string; wide?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const onKey = (e: KeyboardEvent) => e.key === 'Escape' && emit('close')
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4 backdrop-blur-[2px] sm:p-8" @mousedown.self="emit('close')">
      <div :class="['card w-full my-auto', wide ? 'max-w-4xl' : 'max-w-lg']" role="dialog" aria-modal="true">
        <div class="flex items-start justify-between gap-4 border-b border-line px-6 py-4">
          <div>
            <h2 class="text-[16px] font-bold text-ink">{{ title }}</h2>
            <p v-if="subtitle" class="mt-0.5 text-xs text-ink-3">{{ subtitle }}</p>
          </div>
          <button class="btn-ghost btn-sm -mr-2" aria-label="Close" @click="emit('close')"><X class="h-4 w-4" /></button>
        </div>
        <div class="px-6 py-5"><slot /></div>
        <div v-if="$slots.footer" class="flex flex-wrap justify-end gap-2 border-t border-line px-6 py-4"><slot name="footer" /></div>
      </div>
    </div>
  </Teleport>
</template>
