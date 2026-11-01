<template>
  <div class="dashboard">
    <header class="dashboard-header">
      <h1>系统概览</h1>
      <div class="header-actions">
        <el-button size="small" @click="loadAll" :loading="loading">刷新</el-button>
        <el-tag :type="systemOnline ? 'success' : 'danger'" size="small">
          {{ systemOnline ? '系统运行中' : '连接异常' }}
        </el-tag>
      </div>
    </header>

    <el-alert
      v-if="loadError"
      :title="loadError"
      type="error"
      show-icon
      :closable="false"
      style="margin-bottom: 12px"
    />

    <!-- 概览卡片 -->
    <el-row :gutter="16" class="stat-row">
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">CPU</div>
          <div class="stat-value">{{ cpuPercent }}%</div>
          <div class="stat-sub">{{ cpuModel }} · {{ cpuCores }} 核</div>
          <el-progress :percentage="cpuPercent" :show-text="false" :stroke-width="6" style="margin-top:8px" />
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">内存</div>
          <div class="stat-value">{{ memUsed }} MB</div>
          <div class="stat-sub">已用 / 共 {{ memTotal }} MB · 可用 {{ memAvail }} MB</div>
          <el-progress :percentage="memPercent" :show-text="false" :stroke-width="6" style="margin-top:8px" />
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">负载</div>
          <div class="stat-value">{{ load1 }}</div>
          <div class="stat-sub">1分 {{ load1 }} · 5分 {{ load5 }} · 15分 {{ load15 }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">连接数</div>
          <div class="stat-value">{{ conntrack }}</div>
          <div class="stat-sub">conntrack</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">LAN 客户端</div>
          <div class="stat-value">{{ clientCount }}</div>
          <div class="stat-sub">活跃终端</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">系统</div>
          <div class="stat-value">{{ osVersion }}</div>
          <div class="stat-sub">{{ hostname }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- WAN 状态 + 流量 -->
    <el-row :gutter="16" class="stat-row">
      <el-col :span="12">
        <el-card shadow="never" class="placeholder-card">
          <template #header>
            <div class="card-header">
              <span>WAN 状态</span>
              <el-button size="small" @click="loadWan" :loading="wanLoading">刷新</el-button>
            </div>
          </template>
          <el-table v-if="wanStatus.length" :data="wanStatus" border style="width: 100%">
            <el-table-column label="名称" prop="name" width="120" />
            <el-table-column label="接口" prop="interface" width="100" />
            <el-table-column label="网关" prop="gateway" width="140" />
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="row.alive ? 'success' : 'danger'" size="small">
                  {{ row.alive ? '在线' : '离线' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="延迟" width="90">
              <template #default="{ row }">
                {{ row.alive ? row.latency_ms + ' ms' : '—' }}
              </template>
            </el-table-column>
            <el-table-column label="丢包" width="80">
              <template #default="{ row }">
                {{ row.loss_pct != null ? row.loss_pct + '%' : '—' }}
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-else description="未配置 WAN 链路" :image-size="60" />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card shadow="never" class="placeholder-card">
          <template #header>实时流量</template>
          <div class="traffic-row">
            <div class="traffic-item">
              <div class="traffic-label">↓ 接收</div>
              <div class="traffic-value">{{ formatRate(rxRate) }}</div>
            </div>
            <div class="traffic-item">
              <div class="traffic-label">↑ 发送</div>
              <div class="traffic-value">{{ formatRate(txRate) }}</div>
            </div>
          </div>
          <el-divider />
          <el-table :data="interfaces" border style="width: 100%" size="small">
            <el-table-column prop="name" label="接口" width="120" />
            <el-table-column prop="ipaddr" label="IP 地址" width="160" />
            <el-table-column label="接收" width="120">
              <template #default="{ row }">{{ formatBytes(row.rx_bytes) }}</template>
            </el-table-column>
            <el-table-column label="发送" width="120">
              <template #default="{ row }">{{ formatBytes(row.tx_bytes) }}</template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <!-- 快捷入口 -->
    <el-card shadow="never" class="placeholder-card" style="margin-top: 4px">
      <template #header>快捷入口</template>
      <div class="quick-links">
        <el-button v-for="q in quickLinks" :key="q.path" type="primary" plain size="small" @click="$router.push(q.path)">
          {{ q.label }}
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import api from '../api';

const hostname = ref('-');
const cpuModel = ref('-');
const cpuCores = ref(0);
const memUsed = ref(0);
const memAvail = ref(0);
const memTotal = ref(0);
const osVersion = ref('-');
const loadError = ref('');
const loading = ref(false);
const systemOnline = ref(true);

const cpuPercent = ref(0);
const load1 = ref(0);
const load5 = ref(0);
const load15 = ref(0);
const conntrack = ref(0);
const clientCount = ref(0);
const rxRate = ref(0);
const txRate = ref(0);

const interfaces = ref<Array<{ name: string; ipaddr: string; rx_bytes: number; tx_bytes: number }>>([]);
const wanStatus = ref<any[]>([]);
const wanLoading = ref(false);

const memPercent = ref(0);

const quickLinks = [
  { path: '/wan', label: 'WAN 链路' },
  { path: '/interfaces', label: '接口管理' },
  { path: '/clients', label: '终端管理' },
  { path: '/dhcp', label: 'DHCP' },
  { path: '/dns', label: 'DNS' },
  { path: '/threat', label: '威胁情报' },
  { path: '/l7', label: 'L7 过滤' },
  { path: '/stats', label: '流量统计' },
  { path: '/alerts', label: '告警' },
  { path: '/system', label: '系统设置' },
];

let refreshTimer: any = null;
let lastRx = 0;
let lastTx = 0;
let lastTs = 0;

function formatBytes(b: number): string {
  if (!b) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = b;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return v.toFixed(1) + ' ' + units[i];
}

function formatRate(bps: number): string {
  if (!bps) return '0 B/s';
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
  let i = 0;
  let v = bps;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return v.toFixed(1) + ' ' + units[i];
}

async function loadSystem() {
  try {
    const info = await api.get('/api/v1/system/info');
    const d = info.data;
    hostname.value = d.hostname || '-';
    cpuModel.value = d.cpu_model || '-';
    cpuCores.value = d.cpu_cores || 0;
    memAvail.value = Math.round((d.mem_avail_kb || 0) / 1024);
    memTotal.value = Math.round((d.mem_total_kb || 0) / 1024);
    memUsed.value = Math.round(((d.mem_total_kb || 0) - (d.mem_avail_kb || 0)) / 1024);
    osVersion.value = d.os_version || '-';
    memPercent.value = memTotal.value > 0 ? Math.round((memUsed.value / memTotal.value) * 100) : 0;
    loadError.value = '';
    systemOnline.value = true;
  } catch (e: any) {
    loadError.value = e?.response?.data?.message || e.message || '加载失败';
    systemOnline.value = false;
  }
}

async function loadStats() {
  try {
    const r = await api.get('/api/v1/stats/current');
    const s = r.data;
    cpuPercent.value = Math.round(s.cpu_percent || 0);
    load1.value = (s.load1 || 0).toFixed(2);
    load5.value = (s.load5 || 0).toFixed(2);
    load15.value = (s.load15 || 0).toFixed(2);
    conntrack.value = s.conntrack_count || 0;
    // 计算流量速率
    const now = Date.now();
    if (lastTs > 0) {
      const dt = (now - lastTs) / 1000;
      if (dt > 0) {
        rxRate.value = Math.round((s.rx_bytes - lastRx) / dt);
        txRate.value = Math.round((s.tx_bytes - lastTx) / dt);
      }
    }
    lastRx = s.rx_bytes || 0;
    lastTx = s.tx_bytes || 0;
    lastTs = now;
  } catch { /* ignore */ }
}

async function loadInterfaces() {
  try {
    const ifs = await api.get('/api/v1/interfaces');
    interfaces.value = ifs.data.interfaces || [];
  } catch { /* ignore */ }
}

async function loadClients() {
  try {
    const r = await api.get('/api/v1/clients');
    clientCount.value = (r.data.clients || r.data || []).length || 0;
  } catch { /* ignore */ }
}

async function loadWan() {
  wanLoading.value = true;
  try {
    const r = await api.get('/api/v1/wan/status');
    wanStatus.value = r.data.status || [];
  } catch { /* ignore */ }
  finally { wanLoading.value = false; }
}

async function loadAll() {
  loading.value = true;
  await Promise.all([loadSystem(), loadStats(), loadInterfaces(), loadClients(), loadWan()]);
  loading.value = false;
}

onMounted(() => {
  loadAll();
  refreshTimer = setInterval(() => {
    loadStats();
    loadInterfaces();
  }, 5000);
});

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer);
});
</script>

<style scoped>
.dashboard {
  padding: 24px;
  background: #f5f7fa;
  min-height: 100vh;
}
.dashboard-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}
.dashboard-header h1 {
  font-size: 24px;
  margin: 0;
}
.header-actions {
  display: flex;
  gap: 10px;
  align-items: center;
}
.stat-row {
  margin-bottom: 16px;
}
.stat-card {
  text-align: center;
}
.stat-title {
  color: #909399;
  font-size: 14px;
}
.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
  margin: 8px 0 4px;
}
.stat-sub {
  color: #909399;
  font-size: 12px;
}
.placeholder-card {
  margin-top: 4px;
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.traffic-row {
  display: flex;
  gap: 40px;
  justify-content: center;
  padding: 12px 0;
}
.traffic-item {
  text-align: center;
}
.traffic-label {
  color: #909399;
  font-size: 14px;
  margin-bottom: 4px;
}
.traffic-value {
  font-size: 28px;
  font-weight: 700;
  color: #303133;
}
.quick-links {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
</style>
