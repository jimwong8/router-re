<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:16px; justify-content:space-between; flex-wrap:wrap">
          <span>系统监控</span>
          <div style="display:flex; align-items:center; gap:12px">
            <span style="font-size:12px; color:#888">每 30 秒自动刷新</span>
            <el-radio-group v-model="rangeMin" size="small">
              <el-radio-button :value="15">15 分钟</el-radio-button>
              <el-radio-button :value="60">60 分钟</el-radio-button>
              <el-radio-button :value="360">360 分钟</el-radio-button>
            </el-radio-group>
            <el-button size="small" :loading="loading" @click="loadHistory">刷新</el-button>
          </div>
        </div>
      </template>

      <el-alert v-if="conntrackHint" type="info" :closable="false" show-icon
        :title="conntrackHint" style="margin-bottom:12px" />

      <el-empty v-if="!loading && !samples.length" description="暂无监控数据，等待采集器产出样本…" />
      <div v-else class="grid">
        <LineChart title="CPU 使用率" unit="percent" :data="samples" key-fn="cpu_percent" color="#f85149" />
        <LineChart title="内存使用" unit="mb" :data="samples" key-fn="mem_used_mb" color="#3fb950" />
        <LineChart title="负载" unit="load" :data="samples" multi :keys="['load1','load5','load15']" />
        <LineChart title="设备连接数" unit="count" :data="samples" key-fn="conntrack_count" color="#d29922" />
        <LineChart title="网络流量" unit="bps" :data="samples" traffic />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';
import LineChart from './components/LineChart.vue';

interface Sample {
  ts: number;
  cpu_percent: number;
  mem_used_mb: number;
  load1: number;
  load5: number;
  load15: number;
  conntrack_count: number;
  rx_bytes: number;
  tx_bytes: number;
}

const samples = ref<Sample[]>([]);

// 连接数全是 0 时给出解释：容器里读不到 conntrack 表，
// 不说明的话用户会以为"设备一个都没连"
const conntrackHint = computed(() => {
  const s = samples.value;
  if (s.length < 3) return '';
  if (s.every((x) => !x.conntrack_count)) {
    return '设备连接数全为 0：读不到 conntrack 表（需要 /proc/net/nf_conntrack，容器内通常不可见）。这一项在容器里不具参考价值。';
  }
  return '';
});
const rangeMin = ref(60);
const loading = ref(false);
let timer: number | undefined;

async function loadHistory() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/stats/history', { params: { minutes: rangeMin.value } });
    samples.value = res.data?.samples || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}

function startAutoRefresh() {
  stopAutoRefresh();
  timer = window.setInterval(loadHistory, 30000);
}
function stopAutoRefresh() {
  if (timer) {
    clearInterval(timer);
    timer = undefined;
  }
}

watch(rangeMin, loadHistory);
onMounted(() => {
  loadHistory();
  startAutoRefresh();
});
onBeforeUnmount(stopAutoRefresh);
</script>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 16px;
}
</style>