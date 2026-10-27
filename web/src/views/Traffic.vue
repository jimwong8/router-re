<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:16px; justify-content:space-between; flex-wrap:wrap">
          <span>流量统计</span>
          <div style="display:flex; align-items:center; gap:12px">
            <span style="font-size:12px; color:#888">
              每 3 秒自动刷新<template v-if="elapsedSec > 0">（采样间隔 {{ elapsedSec.toFixed(2) }}s）</template>
            </span>
            <el-button size="small" :loading="loading" @click="loadRates">刷新</el-button>
          </div>
        </div>
      </template>

      <el-empty v-if="!loading && !rates.length" description="暂无接口速率数据，等待采样…" />
      <el-table v-else v-loading="loading" :data="sortedRates" size="small" stripe>
        <el-table-column prop="iface" label="接口" min-width="140" />
        <el-table-column label="RX 速率" min-width="160" sortable :sort-method="(a: any, b: any) => a.rx_bps - b.rx_bps">
          <template #default="{ row }">
            <span style="color:#3fb950">{{ formatRate(row.rx_bps) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="TX 速率" min-width="160" sortable :sort-method="(a: any, b: any) => a.tx_bps - b.tx_bps">
          <template #default="{ row }">
            <span style="color:#f85149">{{ formatRate(row.tx_bps) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="总速率" min-width="160" sortable :sort-method="(a: any, b: any) => (a.rx_bps + a.tx_bps) - (b.rx_bps + b.tx_bps)">
          <template #default="{ row }">
            <span>{{ formatRate(row.rx_bps + row.tx_bps) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never" style="margin-top:16px">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>TopN 流量排行</span>
          <el-radio-group v-model="topN" size="small" @change="loadTop">
            <el-radio-button :value="5">Top 5</el-radio-button>
            <el-radio-button :value="10">Top 10</el-radio-button>
            <el-radio-button :value="20">Top 20</el-radio-button>
          </el-radio-group>
        </div>
      </template>

      <el-empty v-if="!topRates.length" description="暂无排行数据" />
      <div v-else class="topn-grid">
        <div v-for="(r, i) in topRates" :key="r.iface" class="topn-card">
          <div class="topn-rank">#{{ i + 1 }}</div>
          <div class="topn-iface">{{ r.iface }}</div>
          <div class="topn-total">{{ formatRate(r.rx_bps + r.tx_bps) }}</div>
          <div class="topn-detail">
            <span style="color:#3fb950">↓ {{ formatRate(r.rx_bps) }}</span>
            <span style="color:#f85149">↑ {{ formatRate(r.tx_bps) }}</span>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface Rate {
  iface: string;
  rx_bps: number;
  tx_bps: number;
}

const rates = ref<Rate[]>([]);
const topRates = ref<Rate[]>([]);
const topN = ref(10);
const elapsedSec = ref(0);
const loading = ref(false);
let timer: number | undefined;

// 用 B/s 输入，自动选 KB/s、MB/s、GB/s，避免大数值糊成一片。
function formatRate(bps: number): string {
  const v = Number(bps) || 0;
  if (v < 1024) return `${v.toFixed(0)} B/s`;
  if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KB/s`;
  if (v < 1024 * 1024 * 1024) return `${(v / 1024 / 1024).toFixed(2)} MB/s`;
  return `${(v / 1024 / 1024 / 1024).toFixed(2)} GB/s`;
}

const sortedRates = computed(() =>
  [...rates.value].sort((a, b) => (b.rx_bps + b.tx_bps) - (a.rx_bps + a.tx_bps)),
);

async function loadRates() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/traffic/rates');
    rates.value = res.data?.rates || [];
    elapsedSec.value = Number(res.data?.elapsed_sec) || 0;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}

async function loadTop() {
  try {
    const res = await api.get('/api/v1/traffic/top', { params: { n: topN.value } });
    topRates.value = res.data?.rates || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

function refresh() {
  loadRates();
  loadTop();
}

function startAutoRefresh() {
  stopAutoRefresh();
  timer = window.setInterval(refresh, 3000);
}
function stopAutoRefresh() {
  if (timer) {
    clearInterval(timer);
    timer = undefined;
  }
}

onMounted(() => {
  refresh();
  startAutoRefresh();
});
onBeforeUnmount(stopAutoRefresh);
</script>

<style scoped>
.topn-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}
.topn-card {
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 12px;
  background: #fafbfc;
}
.topn-rank {
  font-size: 12px;
  color: #909399;
}
.topn-iface {
  font-size: 15px;
  font-weight: 600;
  margin: 4px 0;
  word-break: break-all;
}
.topn-total {
  font-size: 18px;
  font-weight: 700;
  color: #303133;
}
.topn-detail {
  margin-top: 6px;
  display: flex;
  gap: 12px;
  font-size: 12px;
}
</style>
