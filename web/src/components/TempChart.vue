<script setup lang="ts">
// Reefer temperature over time with the 2–8 °C compliance band; excursions are marked red.
import { computed } from 'vue'
import { Line } from 'vue-chartjs'
import { CategoryScale, Chart, LineElement, LinearScale, PointElement, Tooltip, type ChartOptions, type Plugin } from 'chart.js'
import { useTheme } from '@/stores/theme'
import { time } from '@/lib/format'

Chart.register(LineElement, PointElement, LinearScale, CategoryScale, Tooltip)

const props = defineProps<{ readings: { recorded_at: string; temperature_c: number; excursion: boolean }[] }>()
const theme = useTheme()
const css = (n: string) => getComputedStyle(document.documentElement).getPropertyValue(n).trim()

const band: Plugin<'line'> = {
  id: 'band',
  beforeDatasetsDraw(chart) {
    const { ctx, chartArea: a, scales: { y } } = chart
    ctx.save()
    ctx.fillStyle = theme.dark ? 'rgba(64,196,84,.10)' : 'rgba(12,140,12,.07)'
    const top = y.getPixelForValue(8), bottom = y.getPixelForValue(2)
    ctx.fillRect(a.left, top, a.right - a.left, bottom - top)
    ctx.restore()
  },
}
const data = computed(() => {
  void theme.version
  const color = css('--series-1')
  return {
    labels: props.readings.map((r) => time(r.recorded_at)),
    datasets: [{
      label: 'Temperature', data: props.readings.map((r) => r.temperature_c), borderColor: color, borderWidth: 2, tension: 0.3,
      pointRadius: props.readings.map((r) => (r.excursion ? 5 : 2)), pointHoverRadius: 6,
      pointBackgroundColor: props.readings.map((r) => (r.excursion ? '#d03b3b' : color)), pointBorderColor: props.readings.map((r) => (r.excursion ? '#d03b3b' : color)),
    }],
  }
})
const options = computed<ChartOptions<'line'>>(() => {
  void theme.version
  const text = css('--chart-text')
  return {
    responsive: true, maintainAspectRatio: false, animation: false, interaction: { mode: 'index', intersect: false },
    plugins: { legend: { display: false }, tooltip: { callbacks: { label: (c) => ` ${Number(c.parsed.y).toFixed(1)} °C${props.readings[c.dataIndex]?.excursion ? ' — outside 2–8 °C' : ''}` } } },
    scales: {
      x: { grid: { display: false }, ticks: { color: text, maxTicksLimit: 8, font: { size: 11 } }, border: { display: false } },
      y: { suggestedMin: 0, suggestedMax: 12, grid: { color: css('--chart-grid') }, ticks: { color: text, callback: (v) => `${v}°`, font: { size: 11 } }, border: { display: false } },
    },
  }
})
const summary = computed(() => {
  const t = props.readings.map((r) => r.temperature_c)
  return t.length ? { min: Math.min(...t), max: Math.max(...t), exc: props.readings.filter((r) => r.excursion).length } : null
})
</script>

<template>
  <div>
    <div class="h-[180px]"><Line :data="data" :options="options" :plugins="[band]" /></div>
    <p v-if="summary" class="mt-2 text-xs text-ink-3">
      Shaded band = allowed 2–8 °C · min {{ summary.min.toFixed(1) }} °C · max {{ summary.max.toFixed(1) }} °C ·
      <span :class="summary.exc ? 'font-bold text-bad' : ''">{{ summary.exc }} excursion reading(s)</span>
    </p>
  </div>
</template>
