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

function startRefresh() { stopRefresh(); timer = window.setInterval(loadAll, 5000); }
function stopRefresh() { if (timer) { clearInterval(timer); timer = undefined; } }

onMounted(() => { loadAll(); startRefresh(); });
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