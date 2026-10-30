import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '@/api/client'

export interface Depot { id: string; code: string; name: string; address: string; lat: number; lng: number }
export interface Vehicle {
  id: string; depot_id: string; plate: string; vehicle_type: string; capacity_kg: number; capacity_volume_l: number
  temperature_class: string; controlled_certified: boolean; status: string; status_reason: string; odometer_km: number
  fuel_km_per_liter: number; next_service_date: string
}
export interface Driver {
  id: string; depot_id: string; name: string; phone: string; license_number: string; license_class: string; license_expiry: string
  cold_chain_certified: boolean; controlled_certified: boolean; status: string; user_email: string
}

/** Shared working context: the selected depot and its fleet, used to resolve ids across screens. */
export const useContext = defineStore('context', () => {
  const depots = ref<Depot[]>([])
  const depotId = ref(localStorage.getItem('tms.depot') ?? '')
  const vehicles = ref<Vehicle[]>([])
  const drivers = ref<Driver[]>([])
  const loaded = ref(false)

  const depot = computed(() => depots.value.find((d) => d.id === depotId.value))
  const vehicle = (id?: string) => vehicles.value.find((v) => v.id === id)
  const driver = (id?: string) => drivers.value.find((d) => d.id === id)

  async function load() {
    depots.value = (await api.get('/depots')).depots ?? []
    if (!depots.value.some((d) => d.id === depotId.value)) depotId.value = depots.value[0]?.id ?? ''
    await refreshFleet()
    loaded.value = true
  }

  async function refreshFleet() {
    if (!depotId.value) return
    const [v, d] = await Promise.all([api.get('/vehicles', { depot_id: depotId.value }), api.get('/drivers', { depot_id: depotId.value })])
    vehicles.value = v.vehicles ?? []
    drivers.value = d.drivers ?? []
  }

  async function selectDepot(id: string) {
    depotId.value = id
    localStorage.setItem('tms.depot', id)
    await refreshFleet()
  }

  return { depots, depotId, depot, vehicles, drivers, loaded, load, refreshFleet, selectDepot, vehicle, driver }
})
