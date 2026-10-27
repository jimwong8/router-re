<template>
  <div class="chart-card">
    <div class="chart-title">{{ title }}</div>
    <svg
      viewBox="0 0 600 200"
      preserveAspectRatio="none"
      class="chart-svg"
    >
      <g v-if="!pointsGeom.length">
        <text
          x="300" y="100"
          fill="#bbb" text-anchor="middle" font-size="14"
        >暂无数据</text>
      </g>
      <g v-else>
        <polyline
          v-for="(g, gi) in pointsGeom"
          :key="gi"
          :points="g.points"
          fill="none"
          :stroke="g.color"
          stroke-width="1.5"
        />
        <text v-if="traffic" x="10" y="16" fill="#3fb950" font-size="10">RX</text>
        <text v-if="traffic" x="60" y="16" fill="#f85149" font-size="10">TX</text>
      </g>
    </svg>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

interface AnySample {
  ts?: number;
  [k: string]: any;
}

const props = defineProps<{
  title: string;
  data: AnySample[];
  keyFn?: string;
  keys?: string[];
  color?: string;
  multi?: boolean;
  traffic?: boolean;
}>();

const W = 600;
const H = 200;
const PAD = 10;

function scale(vals: number[]): { min: number; span: number } {
  const cleaned = vals.filter((v) => !isNaN(v));
  const max = cleaned.length ? Math.max(...cleaned, 1) : 1;
  const min = cleaned.length ? Math.min(...cleaned, 0) : 0;
  const span = max - min || 1;
  return { min, span };
}

function toPoints(series: number[], min: number, span: number): string {
  const stepX = props.data.length > 1 ? (W - 2 * PAD) / (props.data.length - 1) : 0;
  return series
    .map((v, i) => {
      const x = PAD + i * stepX;
      const y = H - PAD - ((v - min) / span) * (H - 2 * PAD);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
}

const pointsGeom = computed(() => {
  const data = props.data || [];
  const out: { points: string; color: string }[] = [];
  if (!data.length) return out;

  if (props.multi && props.keys?.length) {
    const colors = ['#f85149', '#3fb950', '#d29922'];
    props.keys.forEach((k, i) => {
      const series = data.map((s) => Number(s[k]) || 0);
      const { min, span } = scale(series);
      out.push({ points: toPoints(series, min, span), color: colors[i % colors.length] });
    });
    return out;
  }

  if (props.traffic) {
    const bps = (key: string) =>
      data.map((s, i) => {
        if (i === 0) return 0;
        const dt = (data[i].ts || 0) - (data[i - 1].ts || 0);
        if (dt <= 0) return 0;
        const d = Number(s[key]) - Number(data[i - 1][key]);
        return Math.max(0, (d * 8) / dt);
      });
    const rx = bps('rx_bytes');
    const tx = bps('tx_bytes');
    const { min, span } = scale([...rx, ...tx]);
    out.push({ points: toPoints(rx, min, span), color: '#3fb950' });
    out.push({ points: toPoints(tx, min, span), color: '#f85149' });
    return out;
  }

  const key = props.keyFn || 'cpu_percent';
  const series = data.map((s) => Number(s[key]) || 0);
  const { min, span } = scale(series);
  out.push({ points: toPoints(series, min, span), color: props.color || '#58a6ff' });
  return out;
});
</script>

<style scoped>
.chart-card {
  background: #fafbfc;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 12px;
}
.chart-title {
  font-size: 13px;
  color: #333;
  margin-bottom: 8px;
}
.chart-svg {
  width: 100%;
  height: 160px;
  display: block;
}
</style>