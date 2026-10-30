<script setup lang="ts">
import ToastHost from '@/components/ToastHost.vue'
import { useTheme } from '@/stores/theme'
useTheme()
</script>

<template>
  <RouterView />
  <ToastHost />
</template>
