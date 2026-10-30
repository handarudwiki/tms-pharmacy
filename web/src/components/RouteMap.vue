<script setup lang="ts">
// Leaflet map of depots, trip routes (stop sequence), stop status and live vehicle positions.
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useTheme } from '@/stores/theme'
import { seriesColor } from '@/lib/palette'
import { time, titleCase } from '@/lib/format'

export interface MapTrip {
  id: string; trip_number: string; vehicle_plate: string; status: string; depot_lat: number; depot_lng: number
  last_lat: number; last_lng: number; last_ping_at?: string
  stops: { id: string; sequence: number; consignee_name: string; lat: number; lng: number; status: string; eta: string; eta_outside_window?: boolean }[]
}
const props = defineProps<{
  trips?: MapTrip[]
  depots?: { code: string; name: string; lat: number; lng: number }[]
  pins?: { lat: number; lng: number; label: string; tone?: 'brand' | 'bad' | 'muted' }[]
  colorIndex?: Record<string, number>
  focus?: string
}>()
const emit = defineEmits<{ select: [tripId: string] }>()

const el = ref<HTMLElement>()
const theme = useTheme()
let map: L.Map | undefined
let layer: L.LayerGroup | undefined

function draw() {
  if (!map || !layer) return
  layer.clearLayers()
  const bounds: L.LatLngExpression[] = []
  const dark = theme.dark
  for (const d of props.depots ?? []) {
    L.marker([d.lat, d.lng], {
      icon: L.divIcon({ className: '', html: `<div style="display:grid;place-items:center;width:30px;height:30px;border-radius:9px;background:rgb(var(--ink));color:rgb(var(--surface));font:700 10px/1 inherit;box-shadow:0 2px 6px rgba(0,0,0,.3)">DC</div>`, iconSize: [30, 30], iconAnchor: [15, 15] }),
    }).bindTooltip(`<b>${d.code}</b> · ${d.name}`).addTo(layer)
    bounds.push([d.lat, d.lng])
  }
  ;(props.trips ?? []).forEach((t, i) => {
    const color = seriesColor(props.colorIndex?.[t.id] ?? i, dark)
    const faded = props.focus && props.focus !== t.id
    const path: L.LatLngExpression[] = [[t.depot_lat, t.depot_lng], ...t.stops.map((s) => [s.lat, s.lng] as L.LatLngExpression), [t.depot_lat, t.depot_lng]]
    L.polyline(path, { color, weight: faded ? 2 : 3.5, opacity: faded ? 0.25 : 0.9, dashArray: t.status === 'PLANNED' ? '6 6' : undefined })
      .on('click', () => emit('select', t.id)).addTo(layer!)
    for (const s of t.stops) {
      const failed = s.status === 'FAILED'
      const done = s.status === 'DELIVERED'
      L.circleMarker([s.lat, s.lng], {
        radius: 7, weight: 2, color: failed ? '#d03b3b' : color, opacity: faded ? 0.3 : 1,
        fillColor: failed ? '#d03b3b' : done ? color : '#ffffff', fillOpacity: faded ? 0.3 : 1,
      }).bindTooltip(`<b>${t.trip_number} · stop ${s.sequence}</b><br>${s.consignee_name}<br>${titleCase(s.status)} · ETA ${time(s.eta)}${s.eta_outside_window ? ' ⚠ outside window' : ''}`)
        .on('click', () => emit('select', t.id)).addTo(layer!)
      bounds.push([s.lat, s.lng])
    }
    if (t.status === 'IN_PROGRESS' && t.last_lat) {
      L.marker([t.last_lat, t.last_lng], {
        zIndexOffset: 1000,
        icon: L.divIcon({ className: '', iconSize: [96, 26], iconAnchor: [12, 13],
          html: `<div style="display:inline-flex;align-items:center;gap:5px;padding:3px 8px 3px 3px;border-radius:999px;background:${color};color:#fff;font:700 11px/1 'Plus Jakarta Sans',sans-serif;white-space:nowrap;box-shadow:0 3px 10px rgba(0,0,0,.3);border:2px solid #fff"><span style="display:grid;place-items:center;width:18px;height:18px;border-radius:999px;background:rgba(255,255,255,.25)">🚚</span>${t.vehicle_plate}</div>` }),
      }).bindTooltip(`${t.trip_number} · last ping ${time(t.last_ping_at)}`).on('click', () => emit('select', t.id)).addTo(layer!)
    }
  })
  for (const p of props.pins ?? []) {
    const c = p.tone === 'bad' ? '#d03b3b' : p.tone === 'muted' ? '#8c91a3' : `rgb(${getComputedStyle(document.documentElement).getPropertyValue('--brand')})`
    L.circleMarker([p.lat, p.lng], { radius: 6, weight: 2, color: '#fff', fillColor: c, fillOpacity: 1 }).bindTooltip(p.label).addTo(layer)
    bounds.push([p.lat, p.lng])
  }
  if (bounds.length) map.fitBounds(L.latLngBounds(bounds), { padding: [28, 28], maxZoom: 14 })
}

onMounted(() => {
  map = L.map(el.value!, { zoomControl: true, attributionControl: true, scrollWheelZoom: false }).setView([-6.22, 106.9], 11)
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 18, attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
  }).addTo(map)
  layer = L.layerGroup().addTo(map)
  draw()
})
watch(() => [props.trips, props.depots, props.pins, props.focus, theme.dark], draw, { deep: true })
onBeforeUnmount(() => map?.remove())
</script>

<template>
  <div ref="el" class="relative z-0 h-full min-h-[200px] w-full overflow-hidden rounded-xl" />
</template>
