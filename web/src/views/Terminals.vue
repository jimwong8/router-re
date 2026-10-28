<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>终端列表</span>
          <el-button size="small" :loading="loading" @click="load">刷新</el-button>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon :closable="true" @close="error=''" style="margin-bottom:12px" />

      <el-row :gutter="12" style="margin-bottom:16px">
        <el-col :span="6">
          <el-card shadow="hover" class="stat"><div class="n">{{ count }}</div><div class="t">终端总数</div></el-card>
        </el-col>
        <el-col :span="6">
          <el-card shadow="hover" class="stat"><div class="n">{{ activeSessionCount }}</div><div class="t">有活跃会话</div></el-card>
        </el-col>
      </el-row>

      <el-table :data="terminals" v-loading="loading" size="small" style="width:100%" empty-text="暂无终端">
        <el-table-column prop="ip" label="IP" min-width="130" />
        <el-table-column label="MAC" min-width="140">
          <template #default="{ row }">{{ row.mac || '—' }}</template>
        </el-table-column>
        <el-table-column label="主机名" min-width="140">
          <template #default="{ row }">{{ row.hostname || '—' }}</template>
        </el-table-column>
        <el-table-column label="接口" width="100">
          <template #default="{ row }">{{ row.iface || '—' }}</template>
        </el-table-column>
        <el-table-column label="来源" min-width="140">
          <template #default="{ row }">
            <template v-if="row.source">
              <el-tag v-for="s in String(row.source).split(',')" :key="s" size="small" style="margin-right:4px">{{ String(s).trim() }}</el-tag>
            </template>
            <span v-else>—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <span style="display:inline-flex; align-items:center; gap:6px;">
              <span :style="{ display:'inline-block', width:'8px', height:'8px', borderRadius:'50%', background: row.online ? '#67c23a' : '#c0c4cc' }" />
              {{ row.online ? '在线' : '离线' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="在线会话" min-width="150">
          <template #default="{ row }">
            <template v-if="sessionOf(row.ip)">
              <div style="font-size:12px">
                <div>rx {{ formatBytes(sessionOf(row.ip).rx_bytes) }}</div>
                <div>tx {{ formatBytes(sessionOf(row.ip).tx_bytes) }}</div>
              </div>
            </template>
            <span v-else>—</span>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="!loading && terminals.length === 0" style="margin-top:12px">
        <el-empty description="暂无终端数据" />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const loading = ref(false);
const error = ref('');
const count = ref(0);
const terminals = ref<any[]>([]);
const sessions = ref<any[]>([]);

const sessionByIp = computed(() => {
  const m: Record<string, any> = {};
  for (const s of sessions.value) {
    if (s && s.ip) m[String(s.ip)] = s;
  }
  return m;
});

function sessionOf(ip: any) {
  return ip ? sessionByIp.value[String(ip)] : undefined;
}

const activeSessionCount = computed(() => {
  let n = 0;
  for (const t of terminals.value) {
    const s = sessionOf(t.ip);
    if (s && s.online) n++;
  }
  return n;
});

function formatBytes(b: any) {
  const n = Number(b) || 0;
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + ' MB';
  return (n / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
}

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const res = await api.get('/api/v1/terminals');
    const d = res.data || {};
    terminals.value = d.terminals || [];
    count.value = d.count ?? terminals.value.length;
    try {
      const r2 = await api.get('/api/v1/online');
      const d2 = r2.data || {};
      sessions.value = d2.sessions || [];
    } catch (e2: any) {
      sessions.value = [];
    }
  } catch (e: any) {
    const d = e?.response?.data || {};
    const msg = d.message || d.detail || e?.message || '请求失败';
    error.value = msg;
    ElMessage.error(msg);
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<style scoped>
.stat { text-align: center; }
.stat .n { font-size: 26px; font-weight: 600; }
.stat .t { color: #909399; font-size: 12px; margin-top: 4px; }
</style>