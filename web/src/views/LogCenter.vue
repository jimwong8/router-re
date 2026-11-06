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

      <!-- 日志轮转 -->
      <el-tab-pane label="日志轮转" name="rotate">
        <el-form label-width="110px" label-position="left" size="small" style="margin-bottom:12px">
          <el-form-item label="日志路径">
            <el-input v-model="rotateConfig.path" placeholder="/var/log/router-re/app.log" />
          </el-form-item>
          <el-form-item label="最大大小(MB)">
            <el-input-number v-model="rotateConfig.max_size_mb" :min="1" :max="10240" style="width:140px" />
          </el-form-item>
          <el-form-item label="保留份数">
            <el-input-number v-model="rotateConfig.keep_files" :min="1" :max="100" style="width:140px" />
          </el-form-item>
          <el-form-item label="间隔">
            <el-select v-model="rotateConfig.interval" style="width:140px">
              <el-option label="daily" value="daily" />
              <el-option label="weekly" value="weekly" />
              <el-option label="none (size-based)" value="none" />
            </el-select>
          </el-form-item>
          <el-form-item label="压缩">
            <el-switch v-model="rotateConfig.compress" />
          </el-form-item>
        </el-form>
        <div style="display:flex; gap:8px; margin-bottom:12px; flex-wrap:wrap">
          <el-button :loading="rotateDryRun" @click="dryRunRotate">dry-run 预览</el-button>
          <el-button type="primary" :loading="rotateApplying" @click="applyRotate">确认应用</el-button>
          <el-button :loading="rotateLoading" @click="loadRotate">刷新</el-button>
        </div>
        <el-alert v-if="rotateDetail" type="error" :closable="false" show-icon :title="rotateDetail" style="margin-bottom:8px" />
        <el-alert v-if="rotateDryResult" type="info" :closable="false" show-icon :title="rotateDryResult" style="margin-bottom:8px" />
        <div v-if="rotateRendered" style="margin-top:10px">
          <div style="font-size:12px; color:#909399; margin-bottom:4px">渲染的 logrotate 配置</div>
          <pre class="cmd-output">{{ rotateRendered }}</pre>
        </div>
      </el-tab-pane>

      <!-- Syslog 外发 -->
      <el-tab-pane label="Syslog 外发" name="syslog">
        <el-form label-width="110px" label-position="left" size="small" style="margin-bottom:12px">
          <el-form-item label="名称">
            <el-input v-model="syslogTarget.name" placeholder="远程 syslog" />
          </el-form-item>
          <el-form-item label="地址 (host:port)">
            <el-input v-model="syslogTarget.address" placeholder="192.168.1.100:514" />
          </el-form-item>
          <el-form-item label="协议">
            <el-select v-model="syslogTarget.protocol" style="width:140px">
              <el-option label="udp" value="udp" />
              <el-option label="tcp" value="tcp" />
            </el-select>
          </el-form-item>
          <el-form-item label="格式">
            <el-select v-model="syslogTarget.format" style="width:140px">
              <el-option label="rfc5424" value="rfc5424" />
              <el-option label="rfc3164" value="rfc3164" />
            </el-select>
          </el-form-item>
          <el-form-item label="设施">
            <el-select v-model="syslogTarget.facility" style="width:140px">
              <el-option v-for="f in ['local0','local1','local2','local3','local4','local5','local6','local7']" :key="f" :label="f" :value="f" />
            </el-select>
          </el-form-item>
          <el-form-item label="启用">
            <el-switch v-model="syslogTarget.enabled" />
          </el-form-item>
        </el-form>
        <div style="display:flex; gap:8px; margin-bottom:12px; flex-wrap:wrap">
          <el-button type="primary" :loading="syslogSaving" @click="saveSyslog">保存</el-button>
          <el-button :loading="syslogTesting" @click="testSyslog">测试连通</el-button>
          <el-button :loading="syslogLoading" @click="loadSyslog">刷新</el-button>
        </div>
        <el-alert v-if="syslogDetail" type="error" :closable="false" show-icon :title="syslogDetail" style="margin-bottom:8px" />
        <el-alert v-if="syslogTestOk" type="success" :closable="false" show-icon :title="syslogTestDetail" style="margin-bottom:8px" />
        <el-alert v-if="syslogTestFail" type="error" :closable="false" show-icon :title="syslogTestDetail" style="margin-bottom:8px" />
        <div v-if="syslogRendered" style="margin-top:10px">
          <div style="font-size:12px; color:#909399; margin-bottom:4px">渲染的 rsyslog 行</div>
          <pre class="cmd-output">{{ syslogRendered }}</pre>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface RotateConfig {
  path: string; max_size_mb: number; keep_files: number; compress: boolean; interval: string;
}
interface SyslogTarget {
  name: string; address: string; protocol: string; format: string; facility: string; enabled: boolean;
}

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

// ---- 日志轮转 ----
const rotateConfig = ref<RotateConfig>({ path: '/var/log/router-re/app.log', max_size_mb: 100, keep_files: 5, compress: true, interval: 'daily' });
const rotateRendered = ref('');
const rotateDetail = ref('');
const rotateDryResult = ref('');
const rotateLoading = ref(false);
const rotateDryRun = ref(false);
const rotateApplying = ref(false);

// ---- Syslog 外发 ----
const syslogTarget = ref<SyslogTarget>({ name: '', address: '', protocol: 'udp', format: 'rfc5424', facility: 'local0', enabled: false });
const syslogRendered = ref('');
const syslogDetail = ref('');
const syslogTestOk = ref(false);
const syslogTestFail = ref(false);
const syslogTestDetail = ref('');
const syslogLoading = ref(false);
const syslogSaving = ref(false);
const syslogTesting = ref(false);

function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

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

async function loadRotate() {
  rotateLoading.value = true;
  rotateDetail.value = '';
  try {
    const res = await api.get('/api/v1/logs/rotate');
    const d = res.data || {};
    const c = d.config || {};
    rotateConfig.value = { path: '', max_size_mb: 100, keep_files: 5, compress: true, interval: 'daily', ...c };
    rotateRendered.value = d.logrotate || '';
  } catch (e: any) {
    showError(e, '加载日志轮转配置失败');
  } finally {
    rotateLoading.value = false;
  }
}

async function dryRunRotate() {
  rotateDryRun.value = true;
  rotateDetail.value = '';
  rotateDryResult.value = '';
  try {
    const res = await api.post('/api/v1/logs/rotate', { confirm: false, config: rotateConfig.value });
    const d = res.data || {};
    rotateRendered.value = d.logrotate || '';
    rotateDryResult.value = d.detail || 'dry-run：未写入，配置语法已通过';
    ElMessage.success('dry-run 通过，未写入文件');
  } catch (e: any) {
    showError(e, 'dry-run 失败');
  } finally {
    rotateDryRun.value = false;
  }
}

async function applyRotate() {
  rotateApplying.value = true;
  rotateDetail.value = '';
  try {
    const res = await api.post('/api/v1/logs/rotate', { confirm: true, config: rotateConfig.value });
    const d = res.data || {};
    rotateRendered.value = d.logrotate || '';
    ElMessage.success(d.detail || '日志轮转已应用');
    await loadRotate();
  } catch (e: any) {
    showError(e, '应用日志轮转失败');
  } finally {
    rotateApplying.value = false;
  }
}

async function loadSyslog() {
  syslogLoading.value = true;
  syslogDetail.value = '';
  try {
    const res = await api.get('/api/v1/logs/syslog');
    const d = res.data || {};
    const t = d.target || {};
    syslogTarget.value = { name: '', address: '', protocol: 'udp', format: 'rfc5424', facility: 'local0', enabled: false, ...t };
    syslogRendered.value = d.rsyslog || '';
  } catch (e: any) {
    showError(e, '加载 Syslog 配置失败');
  } finally {
    syslogLoading.value = false;
  }
}

async function saveSyslog() {
  syslogSaving.value = true;
  syslogDetail.value = '';
  try {
    const res = await api.post('/api/v1/logs/syslog', syslogTarget.value);
    const d = res.data || {};
    syslogRendered.value = d.rsyslog || '';
    ElMessage.success('Syslog 目标已保存');
  } catch (e: any) {
    showError(e, '保存 Syslog 失败');
  } finally {
    syslogSaving.value = false;
  }
}

async function testSyslog() {
  syslogTesting.value = true;
  syslogTestOk.value = false;
  syslogTestFail.value = false;
  syslogTestDetail.value = '';
  try {
    const res = await api.post('/api/v1/logs/syslog/test', syslogTarget.value);
    const d = res.data || {};
    if (d.ok) {
      syslogTestOk.value = true;
      syslogTestDetail.value = d.detail || '测试消息已发送';
      ElMessage.success(syslogTestDetail.value);
    } else {
      syslogTestFail.value = true;
      syslogTestDetail.value = d.detail || '测试失败';
      ElMessage.error(syslogTestDetail.value);
    }
  } catch (e: any) {
    syslogTestFail.value = true;
    showError(e, 'Syslog 测试失败');
  } finally {
    syslogTesting.value = false;
  }
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
  loadRotate();
  loadSyslog();
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