<script setup lang="ts">
import { computed, ref } from 'vue'
import { Bar } from 'vue-chartjs'
import { BarElement, CategoryScale, Chart, LinearScale, Tooltip, type ChartOptions } from 'chart.js'
import { useTheme } from '@/stores/theme'
import { fmt } from '@/lib/format'

Chart.register(BarElement, CategoryScale, LinearScale, Tooltip)

const props = defineProps<{
  labels: string[]
  series: { name: string; values: number[]; color: 1 | 2 }[]
  unit?: string
}>()
const theme = useTheme()
const showTable = ref(false)

const css = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim()

const data = computed(() => {
  void theme.version
  return {
    labels: props.labels,
    datasets: props.series.map((s) => ({
      label: s.name,
      data: s.values,
      backgroundColor: css(`--series-${s.color}`),
      hoverBackgroundColor: css(`--series-${s.color}`),
      borderRadius: { topLeft: 4, topRight: 4 },
      borderSkipped: 'bottom' as const,
      borderColor: 'transparent',
      borderWidth: { left: 1, right: 1 },
      maxBarThickness: 14,
      categoryPercentage: 0.62,
      barPercentage: 0.9,
    })),
  }
})

const options = computed<ChartOptions<'bar'>>(() => {
  void theme.version
  const text = css('--chart-text')
  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: { duration: 300 },
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        backgroundColor: css('--surface') ? `rgb(${css('--ink')})` : '#111',
        titleColor: `rgb(${css('--surface')})`,
        bodyColor: `rgb(${css('--surface')})`,
        padding: 10,
        cornerRadius: 10,
        boxPadding: 4,
        usePointStyle: true,
        callbacks: { label: (c) => ` ${c.dataset.label}: ${fmt(c.parsed.y)} ${props.unit ?? ''}` },
      },
    },
    scales: {
      x: { grid: { display: false }, border: { display: false }, ticks: { color: text, font: { size: 11 } } },
      y: {
        beginAtZero: true,
        border: { display: false },
        grid: { color: css('--chart-grid') },
        ticks: { color: text, font: { size: 11 }, maxTicksLimit: 5, callback: (v) => fmt(Number(v)) },
      },
    },
  }
})
</script>

<template>
  <div>
    <div class="mb-3 flex flex-wrap items-center gap-4 text-xs text-ink-2">
      <span v-for="s in series" :key="s.name" class="inline-flex items-center gap-1.5">
        <span class="h-2.5 w-2.5 rounded-[3px]" :style="{ background: `var(--series-${s.color})` }" aria-hidden="true" />
        {{ s.name }}
      </span>
      <button class="ml-auto text-xs font-semibold text-brand-ink hover:underline" @click="showTable = !showTable">
        {{ showTable ? 'Show chart' : 'Show table' }}
      </button>
    </div>
    <div v-if="!showTable" class="h-[300px]"><Bar :data="data" :options="options" /></div>
    <div v-else class="table-wrap max-h-[300px] overflow-y-auto">
      <table class="tbl">
        <thead><tr><th>Day</th><th v-for="s in series" :key="s.name" class="num">{{ s.name }}</th></tr></thead>
        <tbody>
          <tr v-for="(l, i) in labels" :key="l">
            <td>{{ l }}</td>
            <td v-for="s in series" :key="s.name" class="num">{{ fmt(s.values[i]) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
