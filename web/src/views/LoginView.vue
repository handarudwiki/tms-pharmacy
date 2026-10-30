<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Truck, Route, Thermometer, FileSignature } from 'lucide-vue-next'
import { useAuth } from '@/stores/auth'

const auth = useAuth()
const router = useRouter()
const route = useRoute()
const email = ref('transport@tms.local')
const password = ref('password123')
const error = ref('')
const busy = ref(false)

const demo = [
  ['admin@tms.local', 'admin123', 'Org admin'],
  ['transport@tms.local', 'password123', 'Transport manager'],
  ['dispatcher@tms.local', 'password123', 'Dispatcher'],
  ['fleet@tms.local', 'password123', 'Fleet manager'],
  ['carrier@tms.local', 'password123', 'Carrier manager'],
  ['budi@tms.local', 'password123', 'Driver (cold chain)'],
  ['andi@tms.local', 'password123', 'Driver'],
]

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await auth.login(email.value, password.value)
    router.push((route.query.next as string) || (auth.isDriverOnly ? '/driver' : '/'))
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="grid min-h-screen lg:grid-cols-[1.05fr_1fr]">
    <section class="relative hidden overflow-hidden bg-brand p-12 text-white lg:flex lg:flex-col">
      <div class="flex items-center gap-3">
        <span class="grid h-10 w-10 place-items-center rounded-xl bg-white/15"><Truck class="h-5 w-5" /></span>
        <span class="text-lg font-extrabold">PharmaTMS</span>
      </div>
      <div class="my-auto max-w-md">
        <h1 class="text-[40px] font-extrabold leading-[1.1] tracking-tight">Every delivery planned, tracked and proven.</h1>
        <p class="mt-4 text-white/75">Transportation management for pharmaceutical distribution — optimized routes, compliant cold-chain trips, proof of delivery and carrier tendering.</p>
        <ul class="mt-10 space-y-4 text-[14px]">
          <li class="flex items-center gap-3"><span class="grid h-9 w-9 place-items-center rounded-xl bg-white/15"><Route class="h-4 w-4" /></span>Capacity- and compliance-aware route planning</li>
          <li class="flex items-center gap-3"><span class="grid h-9 w-9 place-items-center rounded-xl bg-white/15"><Thermometer class="h-4 w-4" /></span>Live 2–8 °C monitoring on refrigerated trips</li>
          <li class="flex items-center gap-3"><span class="grid h-9 w-9 place-items-center rounded-xl bg-white/15"><FileSignature class="h-4 w-4" /></span>Proof of delivery captured by the driver app</li>
        </ul>
      </div>
      <div class="pointer-events-none absolute -bottom-32 -right-32 h-96 w-96 rounded-full bg-white/10" />
      <div class="pointer-events-none absolute -bottom-10 right-40 h-40 w-40 rounded-full bg-white/10" />
    </section>
    <section class="flex items-center justify-center p-6">
      <div class="w-full max-w-sm">
        <h2 class="text-[26px] font-extrabold tracking-tight">Sign in</h2>
        <p class="mt-1 text-[13px] text-ink-3">Use your transport account to continue.</p>
        <form class="mt-8 space-y-4" @submit.prevent="submit">
          <div><label class="label" for="email">Email</label><input id="email" v-model="email" class="input h-11" type="email" autocomplete="username" required /></div>
          <div><label class="label" for="pw">Password</label><input id="pw" v-model="password" class="input h-11" type="password" autocomplete="current-password" required /></div>
          <p v-if="error" class="rounded-xl bg-bad-soft px-3 py-2 text-[13px] text-bad">{{ error }}</p>
          <button class="btn-primary h-11 w-full" :disabled="busy">{{ busy ? 'Signing in…' : 'Sign in' }}</button>
        </form>
        <div class="mt-8">
          <div class="mb-2 text-[11px] font-bold uppercase tracking-wider text-ink-3">Demo accounts</div>
          <div class="flex flex-wrap gap-1.5">
            <button v-for="d in demo" :key="d[0]" type="button" class="rounded-lg border border-line bg-surface px-2.5 py-1 text-xs font-semibold text-ink-2 hover:border-brand/50 hover:text-brand-ink"
              @click="email = d[0]; password = d[1]">{{ d[2] }}</button>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
