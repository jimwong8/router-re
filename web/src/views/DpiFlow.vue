<template>
  <div>
    <!-- 流表 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>DPI 流表（双向归一 + 应用识别）</span>
          <el-button type="primary" size="small" :loading="loading" @click="loadFlows">刷新</el-button>
        </div>
      </template>

      <!-- 统计 -->
      <el-row :gutter="16" style="margin-bottom:12px">
        <el-col :span="6"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold">{{ stats.tracked ?? 0 }}</div>
          <div style="color:#909399; font-size:12px">已跟踪</div>
        </div></el-card></el-col>
        <el-col :span="6"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold">{{ stats.total ?? 0 }}</div>
          <div style="color:#909399; font-size:12px">上限</div>
        </div></el-card></el-col>
        <el-col :span="6"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold; color:#e63000">{{ stats.evicted ?? 0 }}</div>
          <div style="color:#909399; font-size:12px">已淘汰</div>
        </div></el-card></el-col>
        <el-col :span="6"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold; color:#409EFC">{{ appCount }}</div>
          <div style="color:#909399; font-size:12px">应用数</div>
        </div></el-card></el-col>
      </el-row>

      <el-table :data="flows" style="width:100%" height="420">
        <el-table-column label="源" width="150">
          <template #default="s"><span>{{ s.row.key.SrcIP }}:{{ s.row.key.SrcPort }}</span></template>
        </el-table-column>
        <el-table-column label="目的" width="150">
          <template #default="s"><span>{{ s.row.key.DstIP }}:{{ s.row.key.DstPort }}</span></template>
        </el-table-column>
        <el-table-column prop="key.Proto" label="协议" width="80" />
        <el-table-column label="应用" width="160">
          <template #default="s">
            <el-tag v-if="s.row.detected" type="success" size="small">{{ s.row.app }}</el-tag>
            <el-tag v-else type="info" size="small">未识别</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="分类" width="100"><template #default="s">{{ s.row.category }}</template></el-table-column>
        <el-table-column label="上行/下行" width="140">
          <template #default="s"><span>{{ s.row.bytes_out }}/{{ s.row.bytes_in }}</span></template>
        </el-table-column>
        <el-table-column label="首次/最近" width="200">
          <template #default="s">
            <span>{{ fmtTime(s.row.first_seen) }}<br/>{{ fmtTime(s.row.last_seen) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 应用统计 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header><span>按应用分组统计</span></template>
      <el-row v-for="(cnt, app) in appStats" :key="app" style="margin-bottom:6px">
        <el-col :span="6"><el-tag size="small">{{ app }}</el-tag></el-col>
        <el-col :span="18"><el-progress :percentage="pct(cnt)" :stroke-width="14" /></el-col>
      </el-row>
      <el-empty v-if="Object.keys(appStats).length === 0" description="暂无流" />
    </el-card>

    <!-- payload 测试器 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header><span>载荷识别测试器</span></template>
      <el-form :inline="true" label-width="90px">
        <el-form-item label="payload_hex"><el-input v-model="testHex" placeholder="如 1603030045..." style="width:260px" /></el-form-item>
        <el-form-item label="payload_ascii"><el-input v-model="testAscii" placeholder="如 GET / HTTP/1.1..." style="width:220px" /></el-form-item>
        <el-form-item label="端口"><el-input-number v-model="testPort" :min="0" :max="65535" /></el-form-item>
        <el-form-item><el-button type="primary" :loading="testing" @click="runTest">识别</el-button></el-form-item>
      </el-form>
      <el-alert v-if="testResult" :title="testResult" type="info" :closable="false" style="margin-top:6px" />
    </el-card>

    <!-- mark 规则配置 -->
    <el-card shadow="never">
      <template #header><span>应用→端口 Mark 规则</span></template>
      <el-form :inline="true" label-width="70px">
        <el-form-item label="应用"><el-input v-model="mkApp" placeholder="如 http" style="width:120px" /></el-form-item>
        <el-form-item label="端口"><el-input v-model="mkPorts" placeholder="80,443,8080" style="width:200px" /></el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addMark">添加</el-button>
          <el-button type="success" :loading="savingMark" @click="applyMarks">下发</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="markRows" style="width:100%" row-key="app">
        <el-table-column prop="app" label="应用" width="160" />
        <el-table-column prop="portsText" label="端口" />
        <el-table-column label="操作" width="90">
          <template #default="s"><el-button type="danger" link size="small" @click="delMark(s.row.app)">删除</el-button></template>
        </el-table-column>
      </el-table>
      <div v-if="nftListing" style="margin-top:12px">
        <div style="font-weight:bold; margin-bottom:6px">nft 回读：</div>
        <pre>{{ nftListing }}</pre>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const loading = ref(false);
const testing = ref(false);
const savingMark = ref(false);
const flows = ref<any[]>([]);
const stats = ref<Record<string, number>>({});
const appStats = ref<Record<string, number>>({});
const testHex = ref('');
const testAscii = ref('');
const testPort = ref(80);
const testResult = ref('');
const mkApp = ref('');
const mkPorts = ref('');
const markRows = ref<any[]>([]);
const nftListing = ref('');

const appCount = computed(() => Object.keys(appStats.value).length);

function fmtTime(ms: number) {
  if (!ms) return '-';
  const d = new Date(ms);
  return d.toLocaleString('zh-CN', { hour12: false });
}

function pct(cnt: number) {
  const total = Object.values(appStats.value).reduce((a, b) => a + b, 0);
  if (!total) return 0;
  return Math.round((cnt / total) * 100);
}

async function loadFlows() {
  loading.value = true;
  try {
    const [f, s, a] = await Promise.all([
      api.get('/api/v1/dpi/flows'),
      api.get('/api/v1/dpi/tracker-stats'),
      api.get('/api/v1/dpi/apps'),
    ]);
    flows.value = f.data?.flows ?? [];
    stats.value = s.data?.stats ?? {};
    appStats.value = a.data?.apps ?? {};
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}

async function runTest() {
  testing.value = true;
  try {
    const body: any = { dst_port: testPort.value };
    if (testHex.value) body.payload_hex = testHex.value;
    else body.payload_ascii = testAscii.value;
    const res = await api.post('/api/v1/dpi/classify-payload', body);
    const d = res.data || {};
    testResult.value = `识别结果：应用=${d.app} 分类=${d.category} (载荷 ${d.payload_len} 字节)`;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    testing.value = false;
  }
}

function portsToArray(s: string): number[] {
  return s.split(/[,\s]+/).filter(Boolean).map(Number).filter(p => p >= 1 && p <= 65535);
}

async function loadMarks() {
  try {
    const res = await api.get('/api/v1/dpi/mark-rules');
    const ap = res.data?.app_ports ?? {};
    markRows.value = Object.entries(ap).map(([app, ports]: any) => ({ app, portsText: ports.join(',') }));
    nftListing.value = res.data?.listing ?? '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

function addMark() {
  if (!mkApp.value) { ElMessage.warning('请输入应用名'); return; }
  const ports = portsToArray(mkPorts.value);
  if (!ports.length) { ElMessage.warning('请输入有效端口'); return; }
  const existing = markRows.value.find(r => r.app === mkApp.value);
  if (existing) { existing.portsText = ports.join(','); }
  else { markRows.value.push({ app: mkApp.value, portsText: ports.join(',') }); }
  mkApp.value = '';
  mkPorts.value = '';
}

function delMark(app: string) {
  markRows.value = markRows.value.filter(r => r.app !== app);
}

async function applyMarks() {
  savingMark.value = true;
  try {
    const appPorts: Record<string, number[]> = {};
    markRows.value.forEach(r => {
      appPorts[r.app] = r.portsText.split(',').map(Number).filter((p: number) => p >= 1 && p <= 65535);
    });
    const res = await api.post('/api/v1/dpi/mark-rules', { app_ports: appPorts });
    const d = res.data || {};
    if (d.applied) {
      ElMessage.success('mark 规则已下发');
      nftListing.value = d.listing ?? '';
    } else {
      ElMessage.error(d.detail || '下发失败');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    savingMark.value = false;
  }
}

onMounted(() => { loadFlows(); loadMarks(); });
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; background:#1e1e1e; color:#d4d4d4; padding:8px; border-radius:4px; overflow-x:auto; }
</style>
