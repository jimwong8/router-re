<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>服务与进程</span>
          <div>
            <el-button size="small" :loading="loading" @click="loadServices">刷新服务</el-button>
            <el-button size="small" :loading="procLoading" @click="loadProcesses">刷新进程</el-button>
          </div>
        </div>
      </template>

      <el-tabs v-model="activeTab">
        <!-- Tab 1: systemd 服务 -->
        <el-tab-pane label="服务 (systemd)" name="services">
          <el-alert v-if="svcError" :title="svcError" type="error" show-icon :closable="false" style="margin-bottom:12px" />

          <div v-if="svcAvailable === false" style="padding:12px 0">
            <el-alert type="warning" show-icon :closable="false"
              :title="svcReason || '当前系统不支持 systemd'"
              :description="svcHint || '本机可能是 OpenWrt，使用 procd 管理服务'" />
          </div>

          <div v-else>
            <el-input v-model="svcFilter" placeholder="按名称过滤" size="small" style="width:260px; margin-bottom:12px" clearable />

            <el-table :data="filteredServices" v-loading="svcLoading" size="small" style="width:100%" empty-text="没有服务">
              <el-table-column prop="name" label="服务名" min-width="200" show-overflow-tooltip />
              <el-table-column prop="load" label="加载" width="90" />
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag :type="activeTag(row.active)" size="small">{{ row.active }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="sub" label="子状态" width="110" />
              <el-table-column prop="description" label="描述" min-width="240" show-overflow-tooltip />
              <el-table-column label="操作" width="180">
                <template #default="{ row }">
                  <el-button link size="small" type="success" @click="doAction(row, 'start')">启动</el-button>
                  <el-button link size="small" type="primary" @click="doAction(row, 'restart')">重启</el-button>
                  <el-button link size="small" type="danger" @click="doAction(row, 'stop')">停止</el-button>
                </template>
              </el-table-column>
            </el-table>

            <div v-if="lastActionResult" style="margin-top:12px; font-size:13px; color:#606266">
              上次操作：{{ lastActionResult.unit }} / {{ lastActionResult.action }} →
              applied={{ lastActionResult.applied }}，当前状态={{ lastActionResult.active_after }}
              <span v-if="lastActionResult.error" style="color:#f56c6c">（{{ lastActionResult.error }}）</span>
            </div>
          </div>
        </el-tab-pane>

        <!-- Tab 2: 进程 -->
        <el-tab-pane label="进程" name="procs">
          <el-alert v-if="procError" :title="procError" type="error" show-icon :closable="false" style="margin-bottom:12px" />

          <div style="margin-bottom:12px; font-size:13px; color:#606266">
            进程总数：{{ procTotal }}
          </div>

          <el-table :data="processes" v-loading="procLoading" size="small" style="width:100%" empty-text="没有进程">
            <el-table-column prop="pid" label="PID" width="80" />
            <el-table-column prop="user" label="用户" width="110" />
            <el-table-column label="CPU%" width="90">
              <template #default="{ row }">
                <span :style="{ color: cpuColor(row.cpu) }">{{ Number(row.cpu).toFixed(1) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="内存%" width="90">
              <template #default="{ row }">{{ Number(row.mem).toFixed(1) }}</template>
            </el-table-column>
            <el-table-column prop="rss" label="RSS(KB)" width="110" />
            <el-table-column prop="cmd" label="命令" min-width="300" show-overflow-tooltip />
            <el-table-column label="操作" width="110">
              <template #default="{ row }">
                <el-button link size="small" type="danger"
                  :disabled="row.pid === 1"
                  :title="row.pid === 1 ? 'PID 1 是 init，不能被杀死' : ''"
                  @click="killProc(row)">结束进程</el-button>
              </template>
            </el-table-column>
          </el-table>

          <div v-if="lastKillResult" style="margin-top:12px; font-size:13px; color:#606266">
            上次结束：PID {{ lastKillResult.pid }} → applied={{ lastKillResult.applied }}
            <span v-if="lastKillResult.error" style="color:#f56c6c">（{{ lastKillResult.error }}）</span>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

interface Svc { name: string; load: string; active: string; sub: string; description: string }
interface Proc { pid: number; user: string; cpu: number | string; mem: number | string; rss: number | string; cmd: string }

const activeTab = ref('services');
const loading = ref(false);
const procLoading = ref(false);

// services
const svcAvailable = ref<boolean | null>(null);
const svcReason = ref('');
const svcHint = ref('');
const services = ref<Svc[]>([]);
const svcFilter = ref('');
const svcLoading = ref(false);
const svcError = ref('');
const lastActionResult = ref<any>(null);

const filteredServices = computed(() => {
  const q = svcFilter.value.trim().toLowerCase();
  if (!q) return services.value;
  return services.value.filter(s => (s.name || '').toLowerCase().includes(q));
});

// processes
const processes = ref<Proc[]>([]);
const procTotal = ref(0);
const procError = ref('');
const lastKillResult = ref<any>(null);

function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

function activeTag(a: string) {
  if (a === 'active') return 'success';
  if (a === 'failed') return 'danger';
  return 'info';
}

function cpuColor(v: any) {
  const n = Number(v);
  if (n >= 50) return 'var(--el-color-danger)';
  if (n >= 20) return 'var(--el-color-warning)';
  return '';
}

const CRITICAL = ['sshd', 'networking', 'mgmtd', 'ssh.service'];
function isCritical(name: string) {
  return CRITICAL.some(k => (name || '').includes(k));
}

async function loadServices() {
  svcLoading.value = true;
  svcError.value = '';
  try {
    const res = await api.get('/api/v1/system/services');
    const d = res.data || {};
    svcAvailable.value = d.available !== false;
    svcReason.value = d.reason || '';
    svcHint.value = d.hint || '';
    services.value = d.services || d.items || [];
  } catch (e: any) {
    showError(e, '加载服务失败');
    svcError.value = e?.response?.data?.detail || e?.response?.data?.message || e?.message || '加载失败';
  } finally {
    svcLoading.value = false;
  }
}

async function doAction(row: Svc, action: string) {
  const needConfirm = (action === 'stop' || action === 'restart') && isCritical(row.name);
  let confirmed = false;
  if (needConfirm) {
    try {
      await ElMessageBox.confirm(
        `服务 ${row.name} 是关键服务，${action === 'stop' ? '停止' : '重启'}它会中断远程管理，确认继续？`,
        '二次确认', { confirmButtonText: '确认', cancelButtonText: '取消', type: 'warning' });
      confirmed = true;
    } catch {
      return;
    }
  } else {
    confirmed = false;
  }
  try {
    const res = await api.post('/api/v1/system/services/action', {
      unit: row.name, action, confirm: needConfirm ? true : false,
    });
    const d = res.data || {};
    lastActionResult.value = {
      unit: row.name, action,
      applied: d.applied, active_after: d.active_after, error: d.error,
    };
    if (d.applied) ElMessage.success(`${row.name} ${action} 已执行`);
    else ElMessage.warning(d.error || '未执行');
    await loadServices();
  } catch (e: any) {
    showError(e, '操作失败');
  }
}

async function loadProcesses() {
  procLoading.value = true;
  procError.value = '';
  try {
    const res = await api.get('/api/v1/system/processes', { params: { n: 20 } });
    const d = res.data || {};
    processes.value = d.processes || d.items || [];
    procTotal.value = d.total || processes.value.length;
  } catch (e: any) {
    showError(e, '加载进程失败');
    procError.value = e?.response?.data?.detail || e?.response?.data?.message || e?.message || '加载失败';
  } finally {
    procLoading.value = false;
  }
}

async function killProc(row: Proc) {
  if (row.pid === 1) {
    ElMessage.warning('PID 1 是 init，不能被杀死');
    return;
  }
  try {
    await ElMessageBox.confirm(`确认结束进程 PID ${row.pid}（${row.cmd}）？`, '确认', {
      confirmButtonText: '结束', cancelButtonText: '取消', type: 'warning',
    });
  } catch {
    return;
  }
  try {
    const res = await api.post('/api/v1/system/processes/kill', { pid: row.pid, signal: 15, confirm: true });
    const d = res.data || {};
    lastKillResult.value = { pid: row.pid, applied: d.applied, error: d.error };
    if (d.applied) ElMessage.success(`PID ${row.pid} 已结束`);
    else ElMessage.warning(d.error || '未执行');
    await loadProcesses();
  } catch (e: any) {
    showError(e, '结束进程失败');
  }
}

onMounted(() => { loadServices(); loadProcesses(); });
</script>
