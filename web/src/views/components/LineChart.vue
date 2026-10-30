<template>
  <div class="chart-card">
    <div class="chart-head">
      <span class="chart-title">{{ title }}</span>
      <span v-if="latestText" class="chart-value">{{ latestText }}</span>
    </div>

    <div v-if="series.length > 1" class="legend">
      <span v-for="s in series" :key="s.label" class="lg">
        <i :style="{ background: s.color }"></i>{{ s.label }}<b>{{ s.lastText }}</b>
      </span>
    </div>

    <div ref="bodyRef" class="chart-body" @mousemove="onMove" @mouseleave="leave">
      <svg :viewBox="`0 0 ${w} ${h}`" :width="w" :height="h" class="chart-svg">
        <!-- 横向网格 + Y 轴刻度值 -->
        <g v-for="(t, i) in yTicks" :key="'y' + i">
          <line :x1="padL" :x2="w - padR" :y1="t.y" :y2="t.y" stroke="#232a36" stroke-width="1" />
          <text :x="padL - 6" :y="t.y + 3.5" fill="#6b7280" font-size="10" text-anchor="end">{{ t.label }}</text>
        </g>

        <!-- 折线 -->
        <polyline
          v-for="(s, i) in series"
          :key="'p' + i"
          :points="s.points"
          fill="none"
          :stroke="s.color"
          stroke-width="1.6"
          stroke-linejoin="round"
          stroke-linecap="round"
        />

        <!-- 悬停游标 -->
        <g v-if="hover">
          <line :x1="hover.x" :x2="hover.x" :y1="padT" :y2="h - padB" stroke="#4b5563" stroke-dasharray="3 3" />
          <circle
            v-for="(hp, i) in hoverPoints"
            :key="'c' + i"
            :cx="hover.x"
            :cy="hp.y"
            r="3"
            :fill="hp.color"
          />
        </g>

        <!-- X 轴时间 -->
        <text
          v-for="(t, i) in xTicks"
          :key="'x' + i"
          :x="t.x"
          :y="h - 3"
          fill="#6b7280"
          font-size="10"
          :text-anchor="t.anchor"
        >{{ t.label }}</text>

        <text v-if="!series.length" :x="w / 2" :y="h / 2" fill="#6b7280" font-size="12" text-anchor="middle">
          暂无数据
        </text>
      </svg>

      <!-- 悬停读数 -->
      <div v-if="hover" class="tip" :style="tipStyle">
        <div class="tip-time">{{ hover.time }}</div>
        <div v-for="(hp, i) in hoverPoints" :key="i" class="tip-row">
          <i :style="{ background: hp.color }"></i>
          <span class="tip-label">{{ hp.label }}</span>
          <b>{{ hp.text }}</b>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';

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
  /** 数值单位，决定刻度与读数的格式 */
  unit?: 'percent' | 'mb' | 'load' | 'count' | 'bps';
}>();

const bodyRef = ref<HTMLElement | null>(null);
const w = ref(600);
const h = ref(170);

// 左右留白给 Y 轴刻度，上下留白给时间轴与最高点
const padL = 42;
const padR = 10;
const padT = 10;
const padB = 18;

let ro: ResizeObserver | null = null;

onMounted(() => {
  if (!bodyRef.value) return;
  const el = bodyRef.value;
  const apply = () => {
    const rect = el.getBoundingClientRect();
    if (rect.width > 80) w.value = Math.round(rect.width);
    if (rect.height > 60) h.value = Math.round(rect.height);
  };
  apply();
  // 用实际像素做坐标系（viewBox 与像素 1:1），这样文字不会被拉伸变形——
  // 之前用 preserveAspectRatio="none" 固定 600x200，窄屏上刻度数字会被压扁
  ro = new ResizeObserver(apply);
  ro.observe(el);
});

onBeforeUnmount(() => ro?.disconnect());

// ---------- 数值格式化 ----------

function fmt(v: number, unit?: string): string {
  if (!isFinite(v)) return '-';
  switch (unit) {
    case 'percent':
      return v.toFixed(1) + '%';
    case 'mb':
      // 内存到 GB 量级就换算，一味显示 8192 MB 不好读
      return v >= 1024 ? (v / 1024).toFixed(2) + ' GB' : Math.round(v) + ' MB';
    case 'load':
      return v.toFixed(2);
    case 'count':
      return Math.round(v).toLocaleString();
    case 'bps':
      return fmtBps(v);
    default:
      return Math.abs(v) >= 100 ? v.toFixed(0) : v.toFixed(2);
  }
}

function fmtBps(v: number): string {
  if (!isFinite(v) || v <= 0) return '0';
  const units = ['bps', 'Kbps', 'Mbps', 'Gbps'];
  let i = 0;
  let x = v;
  while (x >= 1000 && i < units.length - 1) {
    x /= 1000;
    i++;
  }
  return (x >= 100 ? x.toFixed(0) : x.toFixed(1)) + ' ' + units[i];
}

// ---------- 序列 ----------

const series = computed(() => {
  const data = props.data || [];
  if (!data.length) return [] as any[];

  type Raw = { label: string; color: string; values: number[] };
  let raws: Raw[] = [];

  if (props.multi && props.keys?.length) {
    const colors = ['#f85149', '#3fb950', '#d29922'];
    const names: Record<string, string> = { load1: '1 分钟', load5: '5 分钟', load15: '15 分钟' };
    raws = props.keys.map((k, i) => ({
      label: names[k] || k,
      color: colors[i % colors.length],
      values: data.map((s) => Number(s[k]) || 0),
    }));
  } else if (props.traffic) {
    // 累计字节差分换算成速率；首点没有前一帧，记 0
    const rate = (key: string) =>
      data.map((s, i) => {
        if (i === 0) return 0;
        const dt = (data[i].ts || 0) - (data[i - 1].ts || 0);
        if (dt <= 0) return 0;
        const d = Number(s[key]) - Number(data[i - 1][key]);
        return Math.max(0, (d * 8) / dt);
      });
    raws = [
      { label: '接收', color: '#3fb950', values: rate('rx_bytes') },
      { label: '发送', color: '#f85149', values: rate('tx_bytes') },
    ];
  } else {
    const key = props.keyFn || 'cpu_percent';
    raws = [{ label: '', color: props.color || '#58a6ff', values: data.map((s) => Number(s[key]) || 0) }];
  }

  // 统一的纵轴范围：所有序列共用一个尺度，否则"负载 0.5 和 15 看起来一样高"
  const all = raws.flatMap((r) => r.values).filter((v) => isFinite(v));
  const dom = domain(all);

  const stepX = data.length > 1 ? (w.value - padL - padR) / (data.length - 1) : 0;
  const yOf = (v: number) => {
    const span = dom.max - dom.min || 1;
    return h.value - padB - ((v - dom.min) / span) * (h.value - padT - padB);
  };

  return raws.map((r) => {
    const points = r.values
      .map((v, i) => `${(padL + i * stepX).toFixed(1)},${yOf(v).toFixed(1)}`)
      .join(' ');
    const last = r.values[r.values.length - 1];
    return {
      ...r,
      points,
      lastValue: last,
      lastText: fmt(last, props.unit),
      yOf,
    };
  });
});

// domain 决定刻度范围。百分比固定 0-100（否则 5% 和 21% 的差别会被放大成满屏波动）；
// 其它量从数据最小值起步，但避免 min==max 时把线画在正中间看不出水平。
function domain(vals: number[]): { min: number; max: number } {
  if (props.unit === 'percent') return { min: 0, max: 100 };
  if (!vals.length) return { min: 0, max: 1 };
  let mn = Math.min(...vals);
  let mx = Math.max(...vals);
  if (mn === mx) {
    if (mn === 0) {
      // 全 0（例如容器内读不到 conntrack）也要有可读刻度，
      // 否则会算出 -1..1 并显示成 "-1, -1, 0, 1, 1"
      return { min: 0, max: props.unit === 'count' ? 10 : 1 };
    }
    const pad = Math.abs(mx) * 0.1;
    mn -= pad;
    mx += pad;
  }
  if (mn > 0) mn = 0; // 从 0 起，避免小波动被放大成大起伏
  const span = mx - mn;
  const mag = Math.pow(10, Math.floor(Math.log10(span || 1)));
  const step = mag / 2 || 1;
  return { min: Math.floor(mn / step) * step, max: Math.ceil(mx / step) * step };
}

// ---------- 刻度 ----------

const yTicks = computed(() => {
  const data = props.data || [];
  if (!data.length) return [];
  const all = series.value.flatMap((s: any) => s.values as number[]).filter((v) => isFinite(v));
  const dom = domain(all);
  const n = 4;
  const out: { y: number; label: string }[] = [];
  for (let i = 0; i <= n; i++) {
    const v = dom.min + ((dom.max - dom.min) * i) / n;
    const y = h.value - padB - ((v - dom.min) / (dom.max - dom.min || 1)) * (h.value - padT - padB);
    out.push({ y, label: fmt(v, props.unit) });
  }
  return out;
});

const xTicks = computed(() => {
  const data = props.data || [];
  if (data.length < 2) return [];
  const idxs = [0, Math.floor((data.length - 1) / 2), data.length - 1];
  // 跨度短时只显示分钟会出现三个 "12:52"，读数没有区分度
  const first = data[0]?.ts || 0;
  const lastTs = data[data.length - 1]?.ts || 0;
  const shortSpan = lastTs - first < 7200;
  return idxs.map((i, k) => {
    const x = padL + ((w.value - padL - padR) * i) / (data.length - 1);
    return {
      x,
      label: shortSpan ? clockFull(data[i]?.ts) : clock(data[i]?.ts),
      anchor: k === 0 ? 'start' : k === idxs.length - 1 ? 'end' : 'middle',
    };
  });
});

function clock(ts?: number): string {
  if (!ts) return '';
  const d = new Date(ts * 1000);
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  return `${hh}:${mm}`;
}

function clockFull(ts?: number): string {
  if (!ts) return '';
  const d = new Date(ts * 1000);
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  const ss = String(d.getSeconds()).padStart(2, '0');
  return `${hh}:${mm}:${ss}`;
}

// 标题右侧显示最新一个采样值——不悬停也能看到"现在是多少"
const latestText = computed(() => {
  const s = series.value;
  if (!s.length) return '';
  if (s.length === 1) return s[0].lastText;
  return '';
});

// ---------- 悬停 ----------

const hover = ref<null | { x: number; index: number; time: string }>(null);
const hoverPoints = ref<{ y: number; color: string; label: string; text: string }[]>([]);

function onMove(e: MouseEvent) {
  const el = bodyRef.value;
  const data = props.data || [];
  if (!el || data.length < 1 || !series.value.length) return;
  const rect = el.getBoundingClientRect();
  const px = e.clientX - rect.left;
  const plotW = w.value - padL - padR;
  if (plotW <= 0) return;
  const ratio = (px - padL) / plotW;
  const i = Math.max(0, Math.min(data.length - 1, Math.round(ratio * (data.length - 1))));
  const x = padL + (plotW * i) / Math.max(1, data.length - 1);

  hoverPoints.value = series.value.map((s: any) => {
    const v = s.values[i];
    return { y: s.yOf(v), color: s.color, label: s.label, text: fmt(v, props.unit) };
  });
  hover.value = { x, index: i, time: clockFull(data[i]?.ts) };
}

function leave() {
  hover.value = null;
  hoverPoints.value = [];
}

const tipStyle = computed(() => {
  if (!hover.value) return {};
  // tooltip 靠近右边界时改为向左展开，避免被裁掉
  const flip = hover.value.x > w.value * 0.65;
  return {
    left: (flip ? hover.value.x - 8 : hover.value.x + 8) + 'px',
    transform: flip ? 'translateX(-100%)' : 'none',
    top: padT + 'px',
  } as Record<string, string>;
});
</script>

<style scoped>
.chart-card {
  background: #151922;
  border: 1px solid #232a36;
  border-radius: 8px;
  padding: 12px 12px 8px;
}
.chart-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 6px;
}
.chart-title {
  font-size: 13px;
  color: #cfd3da;
}
.chart-value {
  font-size: 18px;
  font-weight: 600;
  color: #e6e9ef;
  font-variant-numeric: tabular-nums;
}
.legend {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
  margin-bottom: 4px;
  font-size: 11px;
  color: #8b91a5;
}
.legend .lg {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.legend .lg b {
  color: #cfd3da;
  font-variant-numeric: tabular-nums;
}
.legend i,
.tip-row i {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  display: inline-block;
}
.chart-body {
  position: relative;
  width: 100%;
  height: 170px;
}
.chart-svg {
  display: block;
}
.tip {
  position: absolute;
  z-index: 5;
  min-width: 116px;
  padding: 6px 8px;
  background: #0d1117;
  border: 1px solid #30363d;
  border-radius: 6px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
  pointer-events: none;
  font-size: 11px;
}
.tip-time {
  color: #8b91a5;
  margin-bottom: 3px;
  font-variant-numeric: tabular-nums;
}
.tip-row {
  display: flex;
  align-items: center;
  gap: 5px;
  line-height: 1.6;
}
.tip-label {
  color: #8b91a5;
}
.tip-row b {
  margin-left: auto;
  color: #e6e9ef;
  font-variant-numeric: tabular-nums;
}
</style>
