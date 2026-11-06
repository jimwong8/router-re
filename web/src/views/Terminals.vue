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

      <div style="margin-top:12px; display:flex; justify-content:flex-end">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="pageTotal"
          :page-sizes="[10, 20, 50, 100]"
          :layout="pageLayout"
          :disabled="loading"
          @size-change="onPageSizeChange"
          @current-change="load"
        />
        <el-button size="small" style="margin-left:8px" :disabled="!hasMore || loading" @click="nextPage">下一页</el-button>
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
const page = ref(1);
const pageSize = ref(20);
const total = ref(-1);          // -1 表示后端未提供真总数，不显示假总数
const hasMore = ref(false);
const count = ref(0);
const terminals = ref<any[]>([]);
const sessions = ref<any[]>([]);

const pageTotal = computed(() => {
  // total=-1 时给 el-pagination 一个仅够当前页的值，避免显示假总数与假页码
  if (total.value >= 0) return total.value;
  if (hasMore.value) return page.value * pageSize.value + 1;
  return (page.value - 1) * pageSize.value + terminals.value.length;
});

const pageLayout = computed(() => {
  return total.value >= 0 ? 'total, sizes, prev, pager, next' : 'sizes, prev, pager, next';
});

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

function onPageSizeChange() {
  page.value = 1;
  load();
}

function nextPage() {
  if (!hasMore.value) return;
  page.value += 1;
  load();
}

// 后端分页端点（终端列表）。
const TERMINALS_URL = '/api/v1/conntrack/terminals';

async function fetchTerminalsPage(limit: number, offset: number) {
  const res = await api.get(TERMINALS_URL, { params: { limit, offset } });
  return res;
}

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const offset = (page.value - 1) * pageSize.value;
    const res = await fetchTerminalsPage(pageSize.value, offset);
    const d = res.data || {};
    terminals.value = d.terminals || d.items || [];
    count.value = d.count ?? terminals.value.length;
    if (typeof d.total === 'number') total.value = d.total; else total.value = -1;
    if (typeof d.has_more === 'boolean') hasMore.value = d.has_more;
    else hasMore.value = false;
    sessions.value = [];
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