<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus } from 'lucide-vue-next'
import { api } from '@/api/client'
import { useContext } from '@/stores/context'
import { useAction } from '@/lib/useAction'
import { titleCase, day } from '@/lib/format'
import PageHeader from '@/components/PageHeader.vue'
import StatusPill from '@/components/StatusPill.vue'
import Modal from '@/components/Modal.vue'

const ctx = useContext()
const { busy, run } = useAction()
const users = ref<any[]>([])
const roles = ref<string[]>([])
const editing = ref<any>(null)
const form = reactive({ id: '', email: '', name: '', password: '', roles: [] as string[], depot_ids: [] as string[], active: true })

async function load() {
  const [u, r] = await Promise.all([api.get('/users'), api.get('/roles')])
  users.value = u.users ?? []
  roles.value = r.roles
}
onMounted(load)
function edit(u?: any) {
  Object.assign(form, { id: '', email: '', name: '', password: '', roles: [], depot_ids: [], active: true }, u ? { ...u, password: '' } : {})
  editing.value = u ?? {}
}
async function save() {
  const ok = await run(() => (form.id ? api.put(`/users/${form.id}`, form) : api.post('/users', form)), 'User saved')
  if (ok) { editing.value = null; load() }
}
</script>

<template>
  <div>
    <PageHeader title="Users & Roles" subtitle="Role-based access — one person can hold several roles; sensitive actions still need the right one">
      <button class="btn-primary" @click="edit()"><Plus class="h-4 w-4" />New user</button>
    </PageHeader>
    <section class="card card-pad">
      <div class="table-wrap">
        <table class="tbl">
          <thead><tr><th>User</th><th>Roles</th><th>Depots</th><th>Status</th><th>Since</th></tr></thead>
          <tbody>
            <tr v-for="u in users" :key="u.id" class="clickable" @click="edit(u)">
              <td><div class="font-semibold">{{ u.name }}</div><div class="text-xs text-ink-3">{{ u.email }}</div></td>
              <td><div class="flex flex-wrap gap-1"><span v-for="r in u.roles" :key="r" class="rounded-md bg-brand-soft px-2 py-0.5 text-[11px] font-semibold text-brand-ink">{{ titleCase(r) }}</span></div></td>
              <td class="text-ink-2">{{ u.depot_ids.length ? u.depot_ids.map((id: string) => ctx.depots.find((w) => w.id === id)?.code ?? id).join(', ') : 'All' }}</td>
              <td><StatusPill :status="u.active ? 'ACTIVE' : 'CANCELLED'" :label="u.active ? 'Active' : 'Deactivated'" /></td>
              <td class="text-ink-3">{{ day(u.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <Modal v-if="editing" :title="form.id ? form.name : 'New user'" wide @close="editing = null">
      <div class="grid gap-4 sm:grid-cols-2">
        <div><label class="label">Name</label><input v-model="form.name" class="input" /></div>
        <div><label class="label">Email</label><input v-model="form.email" class="input" :disabled="!!form.id" /></div>
        <div><label class="label">{{ form.id ? 'New password (optional)' : 'Password' }}</label><input v-model="form.password" type="password" class="input" autocomplete="new-password" /></div>
        <label v-if="form.id" class="mt-6 flex items-center gap-2 text-[13px] font-semibold"><input v-model="form.active" type="checkbox" class="accent-[rgb(var(--brand))]" />Active</label>
      </div>
      <div class="mt-5"><div class="label">Roles</div>
        <div class="grid gap-2 sm:grid-cols-3">
          <label v-for="r in roles" :key="r" class="flex items-center gap-2 rounded-lg border border-line px-3 py-2 text-[13px]">
            <input v-model="form.roles" type="checkbox" :value="r" class="accent-[rgb(var(--brand))]" />{{ titleCase(r) }}</label>
        </div></div>
      <div class="mt-5"><div class="label">Depot access (none selected = all depots)</div>
        <div class="flex flex-wrap gap-2">
          <label v-for="w in ctx.depots" :key="w.id" class="flex items-center gap-2 rounded-lg border border-line px-3 py-2 text-[13px]">
            <input v-model="form.depot_ids" type="checkbox" :value="w.id" class="accent-[rgb(var(--brand))]" />{{ w.code }}</label>
        </div></div>
      <template #footer><button class="btn-secondary" @click="editing = null">Cancel</button>
        <button class="btn-primary" :disabled="busy || !form.name || !form.email || !form.roles.length || (!form.id && form.password.length < 6)" @click="save">Save</button></template>
    </Modal>
  </div>
</template>
