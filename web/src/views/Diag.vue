<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>Ping</template>
      <el-form inline @submit.prevent="runPing">
        <el-form-item label="主机">
          <el-input v-model="ping.host" placeholder="example.com 或 1.1.1.1" style="width: 220px;" @keyup.enter="runPing" />
        </el-form-item>
        <el-form-item label="次数">
          <el-input-number v-model="ping.count" :min="1" :max="10" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="pingLoading" :disabled="pingLoading" @click="runPing">Ping</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="pingResult && pingResult.avg_ms" :title="`丢包 ${pingResult.loss_pct}% · 平均RTT ${pingResult.avg_ms}ms (min ${pingResult.min_ms} / max ${pingResult.max_ms})`" type="success" show-icon :closable="false" style="margin-bottom: 8px;">
        <div v-if="pingResult.resolved_ip">解析到 {{ pingResult.resolved_ip }}</div>
      </el-alert>
      <el-alert v-if="pingError" :title="pingError" type="error" show-icon :closable="false" style="margin-bottom: 8px;" />
      <pre v-if="pingResult && pingResult.raw" class="diag-pre">{{ pingResult.raw }}</pre>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>Traceroute</template>
      <el-form inline @submit.prevent="runTrace">
        <el-form-item label="主机">
          <el-input v-model="trace.host" placeholder="example.com" style="width: 220px;" @keyup.enter="runTrace" />
        </el-form-item>
        <el-form-item label="最大跳数">
          <el-input-number v-model="trace.max_hops" :min="1" :max="30" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="traceLoading" :disabled="traceLoading" @click="runTrace">追踪</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="traceResult && traceResult.hop_count" :title="`探测到 ${traceResult.hop_count} 跳（工具：${traceResult.extra?.tool || 'traceroute'}）`" type="info" show-icon :closable="false" style="margin-bottom: 8px;" />
      <el-alert v-if="traceError" :title="traceError" type="error" show-icon :closable="false" style="margin-bottom: 8px;" />
      <pre v-if="traceResult && traceResult.raw" class="diag-pre">{{ traceResult.raw }}</pre>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>DNS 查询</template>
      <el-form inline @submit.prevent="runDNS">
        <el-form-item label="域名">
          <el-input v-model="dns.name" placeholder="example.com" style="width: 200px;" @keyup.enter="runDNS" />
        </el-form-item>
        <el-form-item label="DNS 服务器(可选)">
          <el-input v-model="dns.server" placeholder="留空用系统DNS" style="width: 160px;" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="dnsLoading" :disabled="dnsLoading" @click="runDNS">解析</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="dnsError" :title="dnsError" type="error" show-icon :closable="false" style="margin-bottom: 8px;" />
      <el-table v-if="dnsResult && dnsResult.records && dnsResult.records.length" :data="dnsRows" border size="small" style="margin-bottom: 8px;">
        <el-table-column prop="i" label="#" width="40" />
        <el-table-column prop="val" label="记录" />
      </el-table>
      <pre v-if="dnsResult && dnsResult.raw && dnsResult.raw.trim()" class="diag-pre">{{ dnsResult.raw }}</pre>
    </el-card>

    <el-card shadow="never">
      <template #header>接口统计</template>
      <el-button size="small" style="margin-bottom: 8px;" :loading="ifaceLoading" @click="loadIfaces">刷新</el-button>
      <el-table v-if="ifaces.length" :data="ifaces" border>
        <el-table-column prop="iface" label="接口" width="110" />
        <el-table-column label="RX 字节" width="110">
          <template #default="{ row }">{{ fmtSize(row.rx_bytes) }}</template>
        </el-table-column>
        <el-table-column label="RX 包" width="100">
          <template #default="{ row }">{{ row.rx_packets }}</template>
        </el-table-column>
        <el-table-column label="RX 错误" width="90">
          <template #default="{ row }">{{ row.rx_errors }}</template>
        </el-table-column>
        <el-table-column label="RX 丢弃" width="90">
          <template #default="{ row }">{{ row.rx_dropped }}</template>
        </el-table-column>
        <el-table-column label="TX 字节" width="110">
          <template #default="{ row }">{{ fmtSize(row.tx_bytes) }}</template>
        </el-table-column>
        <el-table-column label="TX 包" width="100">
          <template #default="{ row }">{{ row.tx_packets }}</template>
        </el-table-column>
        <el-table-column label="TX 错误" width="90">
          <template #default="{ row }">{{ row.tx_errors }}</template>
        </el-table-column>
        <el-table-column label="TX 丢弃" width="90">
          <template #default="{ row }">{{ row.tx_dropped }}</template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const ping = ref<any>({ host: '', count: 4 });
const pingResult = ref<any>(null);
const pingError = ref('');
const pingLoading = ref(false);

const trace = ref<any>({ host: '', max_hops: 15 });
const traceResult = ref<any>(null);
const traceError = ref('');
const traceLoading = ref(false);

const dns = ref<any>({ name: '', server: '' });
const dnsResult = ref<any>(null);
const dnsError = ref('');
const dnsLoading = ref(false);

const ifaces = ref<any[]>([]);
const ifaceLoading = ref(false);

const dnsRows = computed(() => (dnsResult.value?.records || []).map((v: string, i: number) => ({ i: i + 1, val: v })));

function fmtSize(n: string): string {
  const v = Number(n);
  if (isNaN(v)) return n;
  if (v >= 1e9) return (v / 1e9).toFixed(2) + ' GB';
  if (v >= 1e6) return (v / 1e6).toFixed(2) + ' MB';
  if (v >= 1e3) return (v / 1e3).toFixed(1) + ' KB';
  return String(v);
}

async function runPing() {
  if (!ping.value.host) { ElMessage.warning('请输入主机'); return; }
  pingLoading.value = true; pingError.value = ''; pingResult.value = null;
  try {
    const res = await api.post('/api/v1/diag/ping', { host: ping.value.host, count: ping.value.count });
    pingResult.value = res.data;
  } catch (e: any) {
    pingError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || 'Ping 失败';
  } finally {
    pingLoading.value = false;
  }
}

async function runTrace() {
  if (!trace.value.host) { ElMessage.warning('请输入主机'); return; }
  traceLoading.value = true; traceError.value = ''; traceResult.value = null;
  try {
    const res = await api.post('/api/v1/diag/traceroute', { host: trace.value.host, max_hops: trace.value.max_hops });
    traceResult.value = res.data;
  } catch (e: any) {
    traceError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || 'Traceroute 失败';
  } finally {
    traceLoading.value = false;
  }
}

async function runDNS() {
  if (!dns.value.name) { ElMessage.warning('请输入域名'); return; }
  dnsLoading.value = true; dnsError.value = ''; dnsResult.value = null;
  try {
    const res = await api.post('/api/v1/diag/dns', { name: dns.value.name, server: dns.value.server });
    dnsResult.value = res.data;
  } catch (e: any) {
    dnsError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || 'DNS 查询失败';
  } finally {
    dnsLoading.value = false;
  }
}

async function loadIfaces() {
  ifaceLoading.value = true;
  try {
    const res = await api.get('/api/v1/diag/interfaces');
    ifaces.value = res.data?.interfaces || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '接口统计失败');
  } finally {
    ifaceLoading.value = false;
  }
}

onMounted(() => { loadIfaces(); });
</script>

<style scoped>
.diag-pre {
  background: #f6f6f6;
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  padding: 10px;
  font-size: 12px;
  max-height: 360px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>