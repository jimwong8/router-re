<template>
  <div>
    <el-tabs v-model="activeTab">
      <el-tab-pane label="系统日志" name="logs">
        <el-form inline @submit.prevent>
          <el-form-item label="单元">
            <el-select v-model="unitFilter" placeholder="全部" clearable style="width: 180px">
              <el-option v-for="u in sources" :key="u" :label="u" :value="u" />
            </el-select>
          </el-form-item>
          <el-form-item label="关键词">
            <el-input v-model="grepFilter" placeholder="grep message" />
          </el-form-item>
          <el-form-item label="条数">
            <el-select v-model="logLimit" style="width: 120px">
              <el-option v-for="n in [50, 100, 200, 500]" :key="n" :label="n" :value="n" />
            </el-select>
          </el-form-item>
          <el-form-item label="自动刷新">
            <el-switch v-model="autoRefresh" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="loadLogs">刷新</el-button>
          </el-form-item>
        </el-form>
        <el-table :data="logEntries" border style="width: 100%; margin-top: 12px">
          <el-table-column prop="ts" label="时间" width="160" />
          <el-table-column prop="unit" label="单元" width="180" />
          <el-table-column prop="priority" label="优先级" width="100">
            <template #default="{ row }">
              <el-tag :type="priorityTagType(row.priority)">{{ row.priority }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="message" label="消息" show-overflow-tooltip />
        </el-table>
        <div style="margin-top: 8px; display: flex; align-items: center; gap: 12px">
          <el-button size="small" :disabled="logOffset === 0" @click="prevLogs">上一页</el-button>
          <el-button size="small" :disabled="!logHasMore" @click="nextLogs">下一页</el-button>
          <span style="color: #666">
            已加载 {{ logEntries.length }} 条
            <template v-if="logTotal >= 0">/ 共 {{ logTotal }} 条</template>
            <template v-else>（还有更多）</template>
            · offset {{ logOffset }}
          </span>
        </div>
      </el-tab-pane>

      <el-tab-pane label="连接跟踪" name="conntrack">
        <div style="margin-bottom: 12px">
          <el-tag v-for="(cnt, proto) in connSummary" :key="proto" style="margin-right: 8px">
            {{ proto }}: {{ cnt }}
          </el-tag>
        </div>
        <el-table :data="connEntries" border style="width: 100%" :default-sort="{ prop: 'bytes', order: 'descending' }">
          <el-table-column prop="proto" label="协议" width="80" />
          <el-table-column prop="src" label="源地址" width="140" />
          <el-table-column prop="src_port" label="源端口" width="80" />
          <el-table-column prop="dst" label="目的地址" width="140" />
          <el-table-column prop="dst_port" label="目的端口" width="80" />
          <el-table-column prop="state" label="状态" width="110" />
          <el-table-column prop="bytes" label="字节" sortable width="100" />
          <el-table-column prop="packets" label="包数" width="80" />
          <el-table-column prop="timeout_sec" label="超时(s)" width="90" />
        </el-table>
        <el-button type="primary" style="margin-top: 12px" @click="loadConntrack">刷新</el-button>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const activeTab = ref('logs');

// ---- 日志 Tab ----
const logEntries = ref<any[]>([]);
const sources = ref<string[]>([]);
const unitFilter = ref('');
const grepFilter = ref('');
const logLimit = ref(100);
const autoRefresh = ref(false);
const logOffset = ref(0);
const logHasMore = ref(false);
const logTotal = ref(-1);
let refreshTimer: any = null;

function priorityTagType(p: string) {
  switch (p) {
    case 'emerg':
    case 'alert':
    case 'crit':
    case 'err':
      return 'danger';
    case 'warning':
      return 'warning';
    default:
      return 'info';
  }
}

async function loadSources() {
  try {
    const res = await api.get('/api/v1/logs/sources');
    sources.value = res.data?.sources || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function loadLogs(offset?: number) {
  if (offset !== undefined) logOffset.value = offset;
  try {
    const params: any = { limit: logLimit.value, offset: logOffset.value };
    if (unitFilter.value) params.unit = unitFilter.value;
    if (grepFilter.value) params.grep = grepFilter.value;
    const res = await api.get('/api/v1/logs', { params });
    logEntries.value = res.data?.entries || [];
    logHasMore.value = !!res.data?.has_more;
    logTotal.value = (typeof res.data?.total === 'number') ? res.data.total : -1;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

function nextLogs() {
  if (!logHasMore.value) return;
  loadLogs(logOffset.value + logLimit.value);
}

function prevLogs() {
  const newOffset = Math.max(0, logOffset.value - logLimit.value);
  loadLogs(newOffset);
}

// ---- 连接跟踪 Tab ----
const connEntries = ref<any[]>([]);
const connSummary = ref<Record<string, number>>({});

async function loadConntrack() {
  try {
    const res = await api.get('/api/v1/conntrack', { params: { limit: 1000 } });
    connEntries.value = res.data?.entries || [];
    connSummary.value = res.data?.summary || {};
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

watch([unitFilter, grepFilter, logLimit], () => {
  loadLogs(0);
});

watch(autoRefresh, (v) => {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
  if (v) {
    refreshTimer = setInterval(() => {
      if (activeTab.value === 'logs') loadLogs();
      else loadConntrack();
    }, 5000);
  }
});

onMounted(() => {
  loadSources();
  loadLogs();
  loadConntrack();
});

onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer);
});
</script>

<style scoped>
.cmd-output {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
}
</style>