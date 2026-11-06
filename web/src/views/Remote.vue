<template>
  <div style="display:flex; flex-direction:column; gap:16px;">

    <!-- 远程访问 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>远程访问</span>
          <el-button size="small" type="primary" :loading="accessSaving" @click="saveAccess">保存</el-button>
        </div>
      </template>

      <el-form label-width="130px" label-position="left">
        <el-form-item label="SSH 服务">
          <el-switch v-model="access.ssh_enabled" />
        </el-form-item>
        <el-form-item label="SSH 端口">
          <el-input-number v-model="access.ssh_port" :min="1" :max="65535" />
          <span class="hint">端口变更不会自动改 sshd_config，需人工处理（防锁死）</span>
        </el-form-item>
        <el-form-item label="Web 管理">
          <el-switch v-model="access.web_enabled" />
        </el-form-item>
        <el-form-item label="Web 端口">
          <el-input-number v-model="access.web_port" :min="1" :max="65535" />
          <span class="hint">Web 端口变更需人工调整监听配置</span>
        </el-form-item>
        <el-form-item label="允许来源 (CIDR)">
          <el-select
            v-model="access.allow_sources"
            multiple
            filterable
            allow-create
            default-first-option
            placeholder="输入 CIDR 后回车"
            style="width:360px"
          />
        </el-form-item>
        <el-form-item label="Telnet">
          <el-switch v-model="access.telnet_enabled" />
          <span v-if="access.telnet_enabled" class="warn">⚠ Telnet 是明文协议，建议改用 SSH</span>
        </el-form-item>
      </el-form>

      <div v-if="pendingManual.length" class="manual-block">
        <h4>需人工处理的项（未自动执行，避免锁死远程访问）</h4>
        <ul>
          <li v-for="(p, i) in pendingManual" :key="i" class="manual-item">{{ p }}</li>
        </ul>
      </div>
    </el-card>

    <!-- 系统时间 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>系统时间</span>
          <el-button size="small" type="primary" :loading="timeSaving" @click="saveTime">保存</el-button>
        </div>
      </template>

      <el-form label-width="130px" label-position="left">
        <el-form-item label="时区">
          <el-select v-model="time.timezone" filterable placeholder="选择时区" style="width:280px">
            <el-option v-for="tz in timezones" :key="tz" :label="tz" :value="tz" />
          </el-select>
        </el-form-item>
        <el-form-item label="NTP 服务器">
          <el-select
            v-model="time.ntp_servers"
            multiple
            filterable
            allow-create
            default-first-option
            placeholder="输入 NTP 服务器后回车"
            style="width:360px"
          />
        </el-form-item>
        <el-form-item label="NTP 自动同步">
          <el-switch v-model="time.enabled" />
        </el-form-item>
        <el-form-item label="当前状态">
          <div class="status-line">
            <el-tag v-if="timeStatus.ntpsynchronized === 'yes'" type="success" size="small">时间已同步</el-tag>
            <el-tag v-else type="warning" size="small">未同步</el-tag>
            <span class="hint">Timezone={{ timeStatus.timezone }} · NTP={{ timeStatus.ntp }} · LocalRTC={{ timeStatus.localrtc }}</span>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 远程访问运行状态 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>远程访问运行状态</span>
          <el-button size="small" :loading="remoteStatusLoading" @click="loadRemoteStatus">刷新</el-button>
        </div>
      </template>
      <el-alert v-if="remoteStatusErr" type="error" :closable="false" show-icon :title="remoteStatusErr" style="margin-bottom:12px" />
      <el-descriptions :column="3" border size="small">
        <el-descriptions-item label="SSH 状态">
          <el-tag :type="remoteStatus.ssh_active === 'active' ? 'success' : 'info'" size="small">{{ remoteStatus.ssh_active || '-' }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="SSH 端口">{{ remoteStatus.ssh_port || '-' }}</el-descriptions-item>
        <el-descriptions-item label="SSH 监听">{{ remoteStatus.ssh_listen || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Telnet 状态">
          <el-tag :type="remoteStatus.telnet_active === 'active' ? 'success' : 'info'" size="small">{{ remoteStatus.telnet_active || '-' }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="FTP 状态">
          <el-tag :type="remoteStatus.ftp_active === 'active' ? 'success' : 'info'" size="small">{{ remoteStatus.ftp_active || '-' }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="FTP 端口">{{ remoteStatus.ftp_port || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <!-- 网络唤醒 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>网络唤醒 (WOL)</span>
          <el-button size="small" type="primary" :loading="targetSaving" @click="saveTargets">保存目标</el-button>
        </div>
      </template>

      <el-table :data="targets" size="small" stripe>
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="mac" label="MAC" min-width="160" />
        <el-table-column prop="iface" label="接口" min-width="120" />
        <el-table-column prop="broadcast" label="广播地址" min-width="140" />
        <el-table-column label="操作" width="140">
          <template #default="{ row }">
            <el-button size="small" type="primary" plain :loading="waking===row.name" @click="wakeTarget(row)">唤醒</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-form inline class="add-form" style="margin-top:12px">
        <el-form-item label="名称">
          <el-input v-model="newTarget.name" placeholder="如 NAS" style="width:120px" />
        </el-form-item>
        <el-form-item label="MAC">
          <el-input v-model="newTarget.mac" placeholder="aa:bb:cc:dd:ee:ff" style="width:160px" />
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="newTarget.iface" placeholder="eth0" style="width:100px" />
        </el-form-item>
        <el-form-item label="广播">
          <el-input v-model="newTarget.broadcast" placeholder="192.168.1.255" style="width:140px" />
        </el-form-item>
        <el-form-item>
          <el-button size="small" @click="addTarget">添加</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- FRP 内网穿透 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>FRP 内网穿透</span>
          <div style="display:flex; align-items:center; gap:8px">
            <el-tag :type="frpStatus.running ? 'success' : 'info'" size="small">{{ frpStatus.running ? '运行中' : '未运行' }}</el-tag>
            <el-button size="small" :loading="frpSaving" @click="saveFrp">保存</el-button>
            <el-button size="small" type="primary" :loading="frpApplying" @click="applyFrp">应用</el-button>
            <el-button size="small" danger :loading="frpStopping" @click="stopFrp">停止</el-button>
            <el-button size="small" @click="loadFrp">刷新</el-button>
          </div>
        </div>
      </template>

      <el-form label-width="110px" label-position="left">
        <el-form-item label="服务器地址">
          <el-input v-model="frp.server_addr" placeholder="frp.example.com" />
        </el-form-item>
        <el-form-item label="服务器端口">
          <el-input-number v-model="frp.server_port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="Token">
          <el-input v-model="frp.token" show-password />
        </el-form-item>

        <el-form-item label="隧道列表">
          <el-table :data="frp.tunnels" size="small" stripe>
            <el-table-column prop="name" label="名称" min-width="140" />
            <el-table-column prop="type" label="类型" min-width="80" />
            <el-table-column prop="local_ip" label="本地 IP" min-width="120" />
            <el-table-column prop="local_port" label="本地端口" min-width="100" />
            <el-table-column prop="remote_port" label="远程端口" min-width="100" />
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button size="small" danger @click="removeTunnel(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-form inline class="add-tunnel-form" style="margin-top:8px">
            <el-form-item label="名称">
              <el-input v-model="newTunnel.name" placeholder="如 SSH" style="width:120px" />
            </el-form-item>
            <el-form-item label="类型">
              <el-select v-model="newTunnel.type" style="width:100px">
                <el-option label="tcp" value="tcp" />
                <el-option label="udp" value="udp" />
              </el-select>
            </el-form-item>
            <el-form-item label="本地 IP">
              <el-input v-model="newTunnel.local_ip" placeholder="127.0.0.1" style="width:120px" />
            </el-form-item>
            <el-form-item label="本地端口">
              <el-input-number v-model="newTunnel.local_port" :min="1" :max="65535" style="width:120px" />
            </el-form-item>
            <el-form-item label="远程端口">
              <el-input-number v-model="newTunnel.remote_port" :min="1" :max="65535" style="width:120px" />
            </el-form-item>
            <el-form-item>
              <el-button size="small" @click="addTunnel">添加隧道</el-button>
            </el-form-item>
          </el-form>
        </el-form-item>

        <el-form-item label="状态">
          <div class="status-line">
            <span>运行={{ frpStatus.running ? '是' : '否' }}</span>
            <span v-if="frpStatus.pid">PID={{ frpStatus.pid }}</span>
            <span v-if="frpStatus.binary !== undefined">客户端={{ frpStatus.binary_exists ? frpStatus.binary : '未找到' }}</span>
            <span v-if="frpStatus.config">配置={{ frpStatus.config }}</span>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- NTP 服务（独立于 /system/time） -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>NTP 服务</span>
          <div style="display:flex; align-items:center; gap:8px">
            <el-button size="small" type="primary" :loading="ntpSaving" @click="saveNtp">保存</el-button>
            <el-button size="small" @click="refreshNtpStatus">刷新状态</el-button>
          </div>
        </div>
      </template>

      <el-form label-width="130px" label-position="left">
        <el-form-item label="启用">
          <el-switch v-model="ntp.enabled" />
        </el-form-item>
        <el-form-item label="NTP 服务器">
          <el-select
            v-model="ntp.servers"
            multiple
            filterable
            allow-create
            default-first-option
            placeholder="输入 NTP 服务器后回车，最多 16 条"
            style="width:360px"
          />
          <span class="hint">示例：ntp.aliyun.com、cn.pool.ntp.org、time.windows.com</span>
        </el-form-item>
        <el-form-item label="同步状态">
          <div class="status-line">
            <el-tag v-if="ntpStatus.ntpsynchronized === 'yes'" type="success" size="small">已同步</el-tag>
            <el-tag v-else type="warning" size="small">未同步</el-tag>
            <span class="hint">NTP={{ ntpStatus.ntp }} · 时区={{ ntpStatus.timezone }} · 本地RTC={{ ntpStatus.localrtc }}</span>
          </div>
        </el-form-item>
        <el-alert v-if="ntpError" :title="ntpError" type="error" show-icon :closable="false" style="margin-top: 12px;" />
      </el-form>
    </el-card>

  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface Access {
  ssh_enabled: boolean;
  ssh_port: number;
  web_enabled: boolean;
  web_port: number;
  allow_sources: string[];
  telnet_enabled: boolean;
}
interface TimeCfg {
  timezone: string;
  ntp_servers: string[];
  enabled: boolean;
}
interface TimeStatus { [k: string]: string }
interface WOLTarget { name: string; mac: string; iface: string; broadcast: string }
interface FrpTunnel { name: string; type: string; local_ip: string; local_port: number; remote_port: number }
interface FrpConfig { server_addr: string; server_port: number; token: string; tunnels: FrpTunnel[] }
interface FrpStatus { running: boolean; pid?: number; binary?: string; config?: string; binary_exists?: boolean }
interface NTPConfig { servers: string[]; enabled: boolean }
interface NTPStatus { [k: string]: string }

const access = ref<Access>({ ssh_enabled: false, ssh_port: 22, web_enabled: true, web_port: 443, allow_sources: [], telnet_enabled: false });
const accessSaving = ref(false);
const pendingManual = ref<string[]>([]);

const time = ref<TimeCfg>({ timezone: 'Asia/Shanghai', ntp_servers: ['ntp.aliyun.com'], enabled: true });
const timeSaving = ref(false);
const timeStatus = ref<TimeStatus>({});
const timezones = ref<string[]>([]);

const targets = ref<WOLTarget[]>([]);
const targetSaving = ref(false);
const waking = ref('');
const newTarget = ref<WOLTarget>({ name: '', mac: '', iface: 'eth0', broadcast: '' });

const frp = ref<FrpConfig>({ server_addr: '', server_port: 7000, token: '', tunnels: [] });
const frpStatus = ref<FrpStatus>({ running: false });
const frpSaving = ref(false);
const frpApplying = ref(false);
const frpStopping = ref(false);
const newTunnel = ref<FrpTunnel>({ name: '', type: 'tcp', local_ip: '127.0.0.1', local_port: 0, remote_port: 0 });

// ---- NTP 服务（独立 /api/v1/ntp/*，不与 /system/time 混用） ----
const ntp = ref<NTPConfig>({ servers: ['ntp.aliyun.com'], enabled: true });
const ntpSaving = ref(false);
const ntpError = ref('');
const ntpStatus = ref<NTPStatus>({});

const remoteStatus = ref<Record<string, string>>({});
const remoteStatusLoading = ref(false);
const remoteStatusErr = ref('');

async function loadRemoteStatus() {
  remoteStatusLoading.value = true;
  remoteStatusErr.value = '';
  try {
    const res = await api.get('/api/v1/remote/access/status');
    remoteStatus.value = res.data || {};
  } catch (e: any) {
    remoteStatusErr.value = e?.response?.data?.message || e?.response?.data?.detail || e?.message || '加载远程访问状态失败';
    showError(e);
  } finally {
    remoteStatusLoading.value = false;
  }
}

let timer: number | undefined;

async function loadAll() {
  try {
    const [acc, tm, tmz, wl] = await Promise.all([
      api.get('/api/v1/remote/access'),
      api.get('/api/v1/system/time'),
      api.get('/api/v1/system/timezones'),
      api.get('/api/v1/wol/targets'),
    ]);
    const accData = acc.data?.access;
    if (accData) access.value = { ssh_enabled: false, ssh_port: 22, web_enabled: true, web_port: 443, allow_sources: [], telnet_enabled: false, ...accData };
    const tData = tm.data?.config;
    if (tData) time.value = tData;
    timeStatus.value = tm.data?.status || {};
    if (tmz.data?.timezones) timezones.value = tmz.data.timezones;
    if (wl.data?.targets) targets.value = wl.data.targets;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function saveAccess() {
  accessSaving.value = true;
  try {
    const res = await api.post('/api/v1/remote/access', access.value);
    pendingManual.value = res.data?.pending_manual || [];
    if (res.data?.applied) ElMessage.success('远程访问配置已保存'); else ElMessage.warning('已保存，部分项需人工处理');
    if (res.data?.warnings?.length) ElMessage.warning(res.data.warnings[0]);
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    accessSaving.value = false;
  }
}

async function saveTime() {
  timeSaving.value = true;
  try {
    const res = await api.post('/api/v1/system/time', time.value);
    timeStatus.value = res.data?.status || {};
    if (res.data?.applied) ElMessage.success('时间配置已应用'); else ElMessage.error('应用失败: ' + (res.data?.reason || ''));
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    timeSaving.value = false;
  }
}

function addTarget() {
  if (!newTarget.value.name || !newTarget.value.mac) { ElMessage.warning('请填写名称和 MAC'); return; }
  targets.value.push({ ...newTarget.value });
  newTarget.value = { name: '', mac: '', iface: 'eth0', broadcast: '' };
}

async function saveTargets() {
  targetSaving.value = true;
  try {
    await api.post('/api/v1/wol/targets', { targets: targets.value });
    ElMessage.success('目标已保存');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    targetSaving.value = false;
  }
}

async function wakeTarget(t: WOLTarget) {
  waking.value = t.name;
  try {
    await api.post('/api/v1/wol/wake', { name: t.name });
    ElMessage.success(`已向 ${t.name} 发送唤醒包`);
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    waking.value = '';
  }
}

function showError(e: any) {
  const d = e?.response?.data || {};
  const parts: string[] = [];
  if (d.code !== undefined) parts.push(`code=${d.code}`);
  if (d.message) parts.push(d.message);
  else if (e.message) parts.push(e.message);
  if (d.detail !== undefined && d.detail !== null) parts.push(`detail=${String(d.detail)}`);
  ElMessage.error(parts.join(' | ') || '未知错误');
}

async function loadFrp() {
  try {
    const [cfg, st] = await Promise.all([
      api.get('/api/v1/frp/config'),
      api.get('/api/v1/frp/status'),
    ]);
    const c = cfg.data?.config;
    if (c) frp.value = { server_addr: '', server_port: 7000, token: '', tunnels: [], ...c };
    if (!Array.isArray(frp.value.tunnels)) frp.value.tunnels = [];
    frpStatus.value = st.data || {};
  } catch (e: any) {
    showError(e);
  }
}

async function saveFrp() {
  frpSaving.value = true;
  try {
    await api.post('/api/v1/frp/config', frp.value);
    ElMessage.success('FRP 配置已保存');
  } catch (e: any) {
    showError(e);
  } finally {
    frpSaving.value = false;
  }
}

async function applyFrp() {
  frpApplying.value = true;
  try {
    const res = await api.post('/api/v1/frp/apply');
    ElMessage.success('FRP 已应用并启动');
    frpStatus.value = res.data?.status || {};
  } catch (e: any) {
    showError(e);
  } finally {
    frpApplying.value = false;
  }
}

async function stopFrp() {
  frpStopping.value = true;
  try {
    await api.post('/api/v1/frp/stop');
    ElMessage.success('FRP 已停止');
    frpStatus.value = { running: false };
  } catch (e: any) {
    showError(e);
  } finally {
    frpStopping.value = false;
  }
}

async function loadNtp() {
  ntpError.value = '';
  try {
    const [cfg, st] = await Promise.all([
      api.get('/api/v1/ntp/config'),
      api.get('/api/v1/ntp/status'),
    ]);
    const c = cfg.data?.config;
    if (c) ntp.value = { servers: c.servers || [], enabled: c.enabled || false };
    ntpStatus.value = st.data || {};
  } catch (e: any) {
    ntpError.value = e?.response?.data?.message || e?.response?.data?.detail || e.message || '加载 NTP 配置失败';
    ElMessage.error(ntpError.value);
  }
}

async function saveNtp() {
  ntpSaving.value = true;
  ntpError.value = '';
  try {
    const res = await api.post('/api/v1/ntp/config', { servers: ntp.value.servers, enabled: ntp.value.enabled });
    if (res.data?.applied) ElMessage.success('NTP 配置已保存并应用');
    else ElMessage.warning(res.data?.reason || 'NTP 配置已保存，未应用');
  } catch (e: any) {
    ntpError.value = e?.response?.data?.message || e?.response?.data?.detail || e.message || '保存 NTP 配置失败';
    ElMessage.error(ntpError.value);
  } finally {
    ntpSaving.value = false;
  }
}

async function refreshNtpStatus() {
  try {
    const res = await api.get('/api/v1/ntp/status');
    ntpStatus.value = res.data || {};
    ElMessage.success('NTP 状态已刷新');
  } catch (e: any) {
    showError(e);
  }
}

function addTunnel() {
  if (!newTunnel.value.name || !newTunnel.value.local_port || !newTunnel.value.remote_port) {
    ElMessage.warning('请填写名称、本地端口和远程端口');
    return;
  }
  frp.value.tunnels.push({ ...newTunnel.value });
  newTunnel.value = { name: '', type: 'tcp', local_ip: '127.0.0.1', local_port: 0, remote_port: 0 };
}

function removeTunnel(row: FrpTunnel) {
  const idx = frp.value.tunnels.indexOf(row);
  if (idx >= 0) frp.value.tunnels.splice(idx, 1);
}

function startRefresh() { stopRefresh(); timer = window.setInterval(loadAll, 5000); }
function stopRefresh() { if (timer) { clearInterval(timer); timer = undefined; } }

onMounted(() => { loadAll(); loadFrp(); loadNtp(); loadRemoteStatus(); startRefresh(); });
onBeforeUnmount(stopRefresh);
</script>

<style scoped>
.hint { font-size: 12px; color: #909399; margin-left: 10px; }
.warn { font-size: 12px; color: #f56c6c; margin-left: 10px; font-weight: 600; }
.manual-block { margin-top: 8px; padding: 10px 14px; background: #fdf6ec; border: 1px solid #f3d19e; border-radius: 6px; }
.manual-block h4 { margin: 0 0 6px; color: #b88230; font-size: 13px; }
.manual-item { font-size: 12px; color: #7a5c2e; }
.status-line { display: flex; align-items: center; gap: 10px; }
.add-form { padding-top: 4px; }
</style>