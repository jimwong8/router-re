<template>
  <div style="display:flex; flex-direction:column; gap:16px;">

    <!-- 统计卡片 -->
    <el-row :gutter="16">
      <el-col :span="6" v-for="card in statCards" :key="card.label">
        <el-card shadow="never" class="stat-card">
          <div class="stat-label">{{ card.label }}</div>
          <div class="stat-value" :style="{ color: card.color }">{{ card.value }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 封禁操作区 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>封禁 / 导入 / 情报源</span>
          <div style="display:flex; gap:8px">
            <el-button size="small" type="danger" plain :loading="applying" @click="applyNft">应用下发 nft</el-button>
            <el-button size="small" :disabled="stats.expired===0" @click="pruneExpired">一键清理过期</el-button>
          </div>
        </div>
      </template>

      <el-form label-width="80px" label-position="left">
        <el-form-item label="IP / CIDR">
          <el-input v-model="blockForm.ip" placeholder="如 203.0.113.5 或 10.0.0.0/8" style="width:260px" />
        </el-form-item>
        <el-form-item label="原因">
          <el-input v-model="blockForm.reason" placeholder="封禁原因" style="width:360px" />
        </el-form-item>
        <el-form-item label="时长">
          <el-select v-model="blockForm.duration" style="width:160px">
            <el-option v-for="d in durations" :key="d.value" :label="d.label" :value="d.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <el-button type="danger" size="small" :loading="blocking" @click="doBlock">封禁</el-button>

      <el-divider />

      <div style="display:flex; gap:16px; flex-wrap:wrap">
        <div style="flex:1; min-width:280px">
          <div class="subtitle">批量导入（每行一个 IP/CIDR，支持 # 注释）</div>
          <el-input v-model="importText" type="textarea" :rows="4" placeholder="203.0.113.5&#10;198.51.100.0/24&#10;# 注释行" />
          <el-button size="small" style="margin-top:8px" :loading="importing" @click="doImport">导入</el-button>
        </div>
        <div style="flex:1; min-width:280px">
          <div class="subtitle">从情报源拉取（仅 http/https）</div>
          <el-input v-model="feedUrl" placeholder="https://feeds.example.com/threat.txt" />
          <el-button size="small" style="margin-top:8px" :loading="fetching" @click="doFetch">拉取并导入</el-button>
        </div>
      </div>
    </el-card>

    <!-- nft 回读展示 -->
    <el-card v-if="nftResult" shadow="never">
      <template #header><span>nft 下发回读</span></template>
      <pre class="nft-pre">{{ nftResult }}</pre>
    </el-card>

    <!-- 封禁列表 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between">
          <span>封禁列表</span>
          <el-switch v-model="includeExpired" active-text="显示已过期" @change="load" />
        </div>
      </template>

      <el-table :data="entries" size="small" stripe v-loading="loading">
        <el-table-column prop="ip" label="IP / CIDR" min-width="150" />
        <el-table-column prop="reason" label="原因" min-width="160" show-overflow-tooltip />
        <el-table-column prop="source" label="来源" min-width="120" />
        <el-table-column label="创建时间" width="150">
          <template #default="{ row }">{{ fmtTs(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="过期时间" width="150">
          <template #default="{ row }">
            <span v-if="row.expires_at===0" style="color:#409eff">永久</span>
            <span v-else :class="{ 'expired-row': isExpired(row) }">{{ fmtTs(row.expires_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="hits" label="命中数" width="80" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag v-if="isExpired(row)" type="info" size="small">已过期</el-tag>
            <el-tag v-else type="danger" size="small">生效中</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain @click="doUnblock(row.ip)">解封</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface Entry {
  ip: string; reason: string; source: string;
  created_at: number; expires_at: number; hits: number;
}
interface Stats { total: number; active: number; expired: number; permanent: number }

const entries = ref<Entry[]>([]);
const stats = ref<Stats>({ total: 0, active: 0, expired: 0, permanent: 0 });
const includeExpired = ref(false);
const loading = ref(false);

const blockForm = ref({ ip: '', reason: '', duration: 86400 });
const durations = [
  { label: '1 小时', value: 3600 },
  { label: '1 天', value: 86400 },
  { label: '7 天', value: 604800 },
  { label: '永久', value: 0 },
];
const blocking = ref(false);
const importText = ref('');
const importing = ref(false);
const feedUrl = ref('');
const fetching = ref(false);
const applying = ref(false);
const nftResult = ref('');

const statCards = computed(() => [
  { label: '总计', value: stats.value.total, color: '#409eff' },
  { label: '生效中', value: stats.value.active, color: '#f56c6c' },
  { label: '已过期', value: stats.value.expired, color: '#909399' },
  { label: '永久', value: stats.value.permanent, color: '#67c23a' },
]);

function isExpired(row: Entry): boolean {
  if (row.expires_at === 0) return false;
  return Date.now() / 1000 >= row.expires_at;
}
function fmtTs(ts: number): string {
  if (!ts) return '—';
  const d = new Date(ts * 1000);
  return d.toLocaleString();
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/threat/list', { params: { include_expired: includeExpired.value } });
    entries.value = res.data?.entries || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}

async function loadStats() {
  try {
    const res = await api.get('/api/v1/threat/stats');
    stats.value = res.data || stats.value;
  } catch { /* 统计失败不阻断 */ }
}

async function doBlock() {
  if (!blockForm.value.ip) { ElMessage.warning('请输入 IP/CIDR'); return; }
  blocking.value = true;
  try {
    const res = await api.post('/api/v1/threat/block', {
      ip: blockForm.value.ip,
      reason: blockForm.value.reason,
      duration_seconds: blockForm.value.duration,
    });
    ElMessage.success(`已封禁 ${res.data?.entry?.ip}`);
    blockForm.value.ip = '';
    blockForm.value.reason = '';
    await refresh();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    blocking.value = false;
  }
}

async function doUnblock(ip: string) {
  try {
    await api.delete('/api/v1/threat/block', { data: { ip } });
    ElMessage.success(`已解封 ${ip}`);
    await refresh();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function pruneExpired() {
  try {
    // 用当前列表里已过期的逐个解封。
    const now = Date.now() / 1000;
    const expired = entries.value.filter((r) => r.expires_at !== 0 && now >= r.expires_at);
    for (const r of expired) {
      await api.delete('/api/v1/threat/block', { data: { ip: r.ip } });
    }
    ElMessage.success(`已清理 ${expired.length} 条过期`);
    await refresh();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function doImport() {
  if (!importText.value.trim()) { ElMessage.warning('粘贴要导入的文本'); return; }
  importing.value = true;
  try {
    const res = await api.post('/api/v1/threat/import', { text: importText.value });
    ElMessage.success(`导入成功 ${res.data?.added} 条，跳过 ${res.data?.skipped} 条`);
    if (res.data?.parse_note) ElMessage.warning(res.data.parse_note);
    importText.value = '';
    await refresh();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    importing.value = false;
  }
}

async function doFetch() {
  if (!feedUrl.value.trim()) { ElMessage.warning('请输入情报源 URL'); return; }
  fetching.value = true;
  try {
    const res = await api.post('/api/v1/threat/fetch', { url: feedUrl.value });
    ElMessage.success(`拉取并导入 ${res.data?.added} 条，跳过 ${res.data?.skipped} 条`);
    await refresh();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    fetching.value = false;
  }
}

async function applyNft() {
  applying.value = true;
  try {
    const res = await api.post('/api/v1/threat/apply');
    nftResult.value = `已下发 ${res.data?.count} 条；nft 回读 v4=${res.data?.v4_count} v6=${res.data?.v6_count}\n\n${res.data?.script || ''}`;
    ElMessage.success('nft 已下发并回读确认');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    applying.value = false;
  }
}

async function refresh() {
  await Promise.all([load(), loadStats()]);
}

let timer: number | undefined;
function start() { stop(); timer = window.setInterval(loadStats, 10000); }
function stop() { if (timer) { clearInterval(timer); timer = undefined; } }

onMounted(() => { refresh(); start(); });
onBeforeUnmount(stop);
</script>

<style scoped>
.stat-card { text-align: center; }
.stat-label { font-size: 12px; color: #909399; }
.stat-value { font-size: 28px; font-weight: 700; margin-top: 4px; }
.subtitle { font-size: 13px; color: #606266; margin-bottom: 8px; }
.nft-pre { background: #0d1117; color: #c9d1d9; padding: 12px; border-radius: 6px; font-size: 12px; overflow: auto; white-space: pre-wrap; }
.expired-row { color: #909399; text-decoration: line-through; }
</style>