<template>
  <div>
    <el-card shadow="never">
      <template #header>流量控制 (QoS)</template>
      <el-form inline @submit.prevent>
        <el-form-item label="接口">
          <el-input v-model="newLimit.interface" placeholder="如 eth0" />
        </el-form-item>
        <el-form-item label="速率 (Mbps)">
          <el-input-number v-model="newLimit.rate_mbps" :min="1" :max="10000" />
        </el-form-item>
        <el-form-item label="突发 (KB)">
          <el-input-number v-model="newLimit.burst_kb" :min="10" />
        </el-form-item>
        <el-form-item label="方向">
          <el-select v-model="newLimit.direction" style="width: 100px;">
            <el-option label="egress" value="egress" />
            <el-option label="ingress" value="ingress" />
          </el-select>
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="newLimit.priority" :min="0" :max="65535" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addLimit">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="limits" border style="width: 100%; margin-top: 12px;">
        <el-table-column prop="interface" label="接口" width="120" />
        <el-table-column prop="rate_mbps" label="速率 (Mbps)" />
        <el-table-column prop="burst_kb" label="突发 (KB)" width="120" />
        <el-table-column prop="direction" label="方向" width="100" />
        <el-table-column prop="priority" label="优先级" width="100" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeLimit(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button
        type="primary"
        style="margin-top: 16px"
        :loading="submitting"
        @click="submit"
      >
        提交
      </el-button>

      <el-divider>TC 命令（返回）</el-divider>
      <pre v-if="tcOutput.length" class="cmd-output">{{ cmdText }}</pre>
      <el-empty v-else description="尚未提交" />
    </el-card>

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>按 IP/MAC 限速</template>
      <el-form inline @submit.prevent>
        <el-form-item label="选择器">
          <el-input v-model="newClient.selector" placeholder="ip:192.168.1.100 或 mac:aa:bb:cc:dd:ee:ff" style="width: 260px" />
        </el-form-item>
        <el-form-item label="上行 (kbps)">
          <el-input-number v-model="newClient.upload_kbps" :min="0" :max="1000000" />
        </el-form-item>
        <el-form-item label="下行 (kbps)">
          <el-input-number v-model="newClient.download_kbps" :min="0" :max="1000000" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="newClient.comment" placeholder="如 客厅电视" />
        </el-form-item>
        <el-form-item label="设备">
          <el-input v-model="clientDevice" placeholder="eth0" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addClientLimit">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="clientLimits" border style="width: 100%; margin-top: 12px;">
        <el-table-column prop="selector" label="选择器" width="220" />
        <el-table-column prop="upload_kbps" label="上行 (kbps)" />
        <el-table-column prop="download_kbps" label="下行 (kbps)" />
        <el-table-column prop="comment" label="备注" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeClientLimit(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button
        type="primary"
        style="margin-top: 16px"
        :loading="clientSubmitting"
        @click="submitClientLimits"
      >
        提交限速
      </el-button>

      <el-divider>nft 规则（返回）</el-divider>
      <pre v-if="clientNftOutput.length" class="cmd-output">{{ clientNftOutput }}</pre>
      <el-empty v-else description="尚未提交" />
    </el-card>
  </div>
<el-card shadow="never" style="margin-top:16px">
      <template #header>智能流控 (Smart QoS)</template>
      <el-form inline @submit.prevent>
        <el-form-item label="总带宽 (Mbps)">
          <el-input-number v-model="smartRoot" :min="1" :max="10000" />
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="smartIface" placeholder="如 eth0" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="smartLoading" @click="previewSmart">预览</el-button>
        </el-form-item>
      </el-form>
      <el-table v-if="smartPlan.length" :data="smartPlan" border style="margin-top:12px">
        <el-table-column prop="terminal" label="终端" />
        <el-table-column prop="rate_mbps" label="分配速率 (Mbps)" />
      </el-table>
      <el-button v-if="smartPlan.length" type="success" style="margin-top:12px" :loading="smartApplying" @click="applySmart">确认下发</el-button>
    </el-card>

    <!-- HTB 配置 -->
    <el-card shadow="never" style="margin-top:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>HTB 层级带宽保证</span>
          <el-tag :type="htbConfig.applied ? 'success' : 'info'" size="small">
            {{ htbConfig.applied ? '已应用' : '未应用' }}
          </el-tag>
        </div>
      </template>

      <el-form :model="htbCfg" label-width="110px" size="small">
        <el-form-item label="接口">
          <el-input v-model="htbCfg.iface" placeholder="如 eth0" style="width:140px" />
        </el-form-item>
        <el-form-item label="根带宽 (Mbit)">
          <el-input-number v-model="htbCfg.root_rate_mbit" :min="1" :max="100000" style="width:140px" />
        </el-form-item>
        <el-form-item label="默认类">
          <el-input v-model="htbCfg.default_class" placeholder="如 default" style="width:140px" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="htbCfg.enabled" />
        </el-form-item>
      </el-form>

      <!-- 类表格 -->
      <div style="margin-bottom:8px; display:flex; justify-content:space-between; align-items:center">
        <span style="font-weight:600; font-size:13px">叶子类</span>
        <el-button size="small" @click="addHtbClass">+ 添加类</el-button>
      </div>
      <el-table :data="htbCfg.classes" border size="small" style="width:100%">
        <el-table-column label="名称" width="100">
          <template #default="{ row }"><el-input v-model="row.name" size="small" /></template>
        </el-table-column>
        <el-table-column label="保证 (Mbit)" width="110">
          <template #default="{ row }"><el-input-number v-model="row.rate_mbit" :min="0" :precision="1" size="small" controls-position="right" style="width:90px" /></template>
        </el-table-column>
        <el-table-column label="峰值 (Mbit)" width="110">
          <template #default="{ row }"><el-input-number v-model="row.ceil_mbit" :min="0" :precision="1" size="small" controls-position="right" style="width:90px" /></template>
        </el-table-column>
        <el-table-column label="优先级" width="70">
          <template #default="{ row }"><el-input-number v-model="row.priority" :min="1" :max="7" size="small" controls-position="right" style="width:55px" /></template>
        </el-table-column>
        <el-table-column label="匹配端口" min-width="120">
          <template #default="{ row }"><el-input v-model="row.match_ports_str" size="small" placeholder="80,443" @change="syncHtbPorts(row)" /></template>
        </el-table-column>
        <el-table-column label="匹配目的 IP" min-width="140">
          <template #default="{ row }"><el-input v-model="row.match_dst_ips_str" size="small" placeholder="10.0.0.0/8" @change="parseHtbDst(row)" /></template>
        </el-table-column>
        <el-table-column label="fwmark" width="80">
          <template #default="{ row }"><el-input-number v-model="row.match_mark" :min="0" :max="65535" size="small" controls-position="right" style="width:70px" /></template>
        </el-table-column>
        <el-table-column width="60">
          <template #default="{ $index }">
            <el-button type="danger" size="small" link @click="htbCfg.classes.splice($index, 1)">删</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 操作按钮 -->
      <div style="margin-top:12px; display:flex; gap:8px; flex-wrap:wrap">
        <el-button type="primary" size="small" :loading="htbSaving" @click="saveHtbConfig">保存配置</el-button>
        <el-button size="small" :loading="htbDryRunning" @click="htbDryRun">Dry-run 预览</el-button>
        <el-button type="danger" size="small" :loading="htbApplying" @click="htbConfirmApply">确认应用</el-button>
        <el-button type="warning" size="small" :loading="htbCleaning" @click="htbCleanup">清理接口</el-button>
        <el-button size="small" :loading="htbShowing" @click="showHtbTc">显示 tc</el-button>
        <el-button size="small" :loading="htbReporting" @click="showHtbReport">报告</el-button>
      </div>

      <!-- 结果/信息 -->
      <el-alert
        v-if="htbMsg"
        :title="htbMsg"
        :type="htbMsgOk ? 'success' : 'error'"
        :closable="false"
        show-icon
        style="margin-top:10px"
      />
      <pre v-if="htbScript" class="cmd-output" style="margin-top:10px; max-height:300px; overflow:auto">{{ htbScript }}</pre>
      <el-empty v-if="!htbScript && !htbMsg" description="保存配置或 Dry-run 后此处显示渲染脚本" :image-size="50" />
    </el-card>
    <!-- HTB 限速规则（只读） -->
    <el-card shadow="never" style="margin-top:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>HTB 限速规则（/api/v1/htb/limit-rules）</span>
          <div style="display:flex; gap:8px; align-items:center">
            <el-input v-model="limitRulesIface" placeholder="接口" style="width:120px" />
            <el-button size="small" :loading="limitRulesLoading" @click="loadLimitRules">刷新</el-button>
          </div>
        </div>
      </template>
      <el-alert v-if="limitRulesError" :title="limitRulesError" type="error" :closable="false" style="margin-bottom:10px" />
      <el-descriptions v-if="limitRulesData.iface" :column="2" border style="margin-bottom:10px">
        <el-descriptions-item label="接口">{{ limitRulesData.iface }}</el-descriptions-item>
        <el-descriptions-item label="根带宽 (Mbit)">{{ limitRulesData.root_rate_mbit }}</el-descriptions-item>
        <el-descriptions-item label="默认类">{{ limitRulesData.default_class || '-' }}</el-descriptions-item>
        <el-descriptions-item label="启用">{{ limitRulesData.enabled ? '是' : '否' }}</el-descriptions-item>
      </el-descriptions>
      <el-table v-if="limitRulesData.classes && limitRulesData.classes.length" :data="limitRulesData.classes" border size="small" style="width:100%">
        <el-table-column prop="name" label="名称" width="120" />
        <el-table-column prop="rate_mbit" label="保证 (Mbit)" width="110" />
        <el-table-column prop="ceil_mbit" label="峰值 (Mbit)" width="110" />
        <el-table-column prop="priority" label="优先级" width="80" />
        <el-table-column prop="match_mark" label="fwmark" width="90" />
        <el-table-column label="匹配端口" min-width="120">
          <template #default="{ row }">{{ (row.match_ports || []).join(',') || '-' }}</template>
        </el-table-column>
        <el-table-column label="匹配目的 IP" min-width="140">
          <template #default="{ row }">{{ (row.match_dst_ips || []).join(',') || '-' }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!limitRulesData.iface && !limitRulesError" description="请输入接口并点击刷新" :image-size="60" />
    </el-card>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

// 统一错误提示：后端 {code,message,detail}，detail 往往才是真正原因
function showError(e: any) {
  const d = e?.response?.data;
  if (d) {
    const parts: string[] = [];
    if (d.message) parts.push(d.message);
    if (d.detail) parts.push(d.detail);
    if (parts.length) { ElMessage.error(parts.join('：')); return; }
  }
  ElMessage.error(e?.message || '请求失败');
}

// Field names mirror the backend JSON (snake_case) to avoid a mapping layer.
interface LimitRow {
  interface: string;
  rate_mbps: number;
  burst_kb: number;
  direction: string;
  priority: number;
}

function emptyLimit(): LimitRow {
  return { interface: 'eth0', rate_mbps: 100, burst_kb: 512, direction: 'egress', priority: 1 };
}

const newLimit = ref<LimitRow>(emptyLimit());
const limits = ref<LimitRow[]>([]);
const tcOutput = ref<string[]>([]);
const submitting = ref(false);

// --- per-client (IP/MAC) limits ---
interface ClientLimitRow {
  selector: string;
  upload_kbps: number;
  download_kbps: number;
  comment: string;
}

function emptyClientLimit(): ClientLimitRow {
  return { selector: '', upload_kbps: 0, download_kbps: 0, comment: '' };
}

const newClient = ref<ClientLimitRow>(emptyClientLimit());
const clientLimits = ref<ClientLimitRow[]>([]);
const clientDevice = ref('eth0');
const clientNftOutput = ref('');
const clientSubmitting = ref(false);

function addClientLimit() {
  if (!newClient.value.selector) {
    ElMessage.warning('选择器必填');
    return;
  }
  clientLimits.value.push({ ...newClient.value });
  newClient.value = emptyClientLimit();
}

function removeClientLimit(row: ClientLimitRow) {
  clientLimits.value = clientLimits.value.filter((l) => l !== row);
}

async function loadClientLimits() {
  try {
    const res = await api.get('/api/v1/qos/client-limits');
    clientLimits.value = (res.data?.limits || []) as ClientLimitRow[];
    clientNftOutput.value = res.data?.nft || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submitClientLimits() {
  if (!clientLimits.value.length) {
    ElMessage.warning('请先添加限速规则');
    return;
  }
  clientSubmitting.value = true;
  try {
    const res = await api.post('/api/v1/qos/client-limits', {
      limits: clientLimits.value,
      device: clientDevice.value,
    });
    clientNftOutput.value = res.data?.nft || '';
    ElMessage.success('限速已提交');
    await loadClientLimits();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    clientSubmitting.value = false;
  }
}

const cmdText = computed(() => tcOutput.value.join('\n'));

function addLimit() {
  if (!newLimit.value.interface) {
    ElMessage.warning('接口必填');
    return;
  }
  limits.value.push({ ...newLimit.value });
  newLimit.value = emptyLimit();
}

function removeLimit(row: LimitRow) {
  limits.value = limits.value.filter((l) => l !== row);
}

async function load() {
  try {
    const res = await api.get('/api/v1/qos/limits');
    limits.value = (res.data?.limits || []) as LimitRow[];
    tcOutput.value = res.data?.tc || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submit() {
  if (!limits.value.length) {
    ElMessage.warning('请先添加规则');
    return;
  }
  submitting.value = true;
  try {
    const res = await api.post('/api/v1/qos/limits', { limits: limits.value });
    tcOutput.value = res.data?.tc || res.data?.commands || [];
    ElMessage.success('已提交');
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    submitting.value = false;
  }
}

const smartRoot = ref(100);
const smartIface = ref('eth0');
const smartLoading = ref(false);
const smartApplying = ref(false);
const smartPlan = ref<any[]>([]);

async function previewSmart() {
  smartLoading.value = true;
  try {
    const res = await api.get('/api/v1/htb/smart-plan', { params: { iface: smartIface.value, root_mbit: smartRoot.value } });
    smartPlan.value = res.data?.plan || res.data?.members || [];
  } catch (e: any) { showError(e); }
  smartLoading.value = false;
}
async function applySmart() {
  smartApplying.value = true;
  try {
    await api.post('/api/v1/htb/smart-apply', { iface: smartIface.value, root_mbit: smartRoot.value });
    ElMessage.success('已下发');
    smartPlan.value = [];
  } catch (e: any) { showError(e); }
  smartApplying.value = false;
}

// ── HTB ──
interface HtbClassRow {
  name: string;
  rate_mbit: number;
  ceil_mbit: number;
  priority: number;
  match_ports: number[];
  match_dst_ips: string[];
  match_mark: number;
  // UI 辅助字段
  match_ports_str?: string;
  match_dst_ips_str?: string;
}
interface HtbCfg {
  iface: string;
  root_rate_mbit: number;
  default_class: string;
  classes: HtbClassRow[];
  enabled: boolean;
}

const htbCfg = ref<HtbCfg>({
  iface: 'eth0', root_rate_mbit: 100, default_class: 'default',
  classes: [], enabled: true,
});
const htbConfig = ref({ applied: false });
const htbScript = ref('');
const htbMsg = ref('');
const htbMsgOk = ref(false);
const htbSaving = ref(false);
const htbDryRunning = ref(false);
const htbApplying = ref(false);
const htbCleaning = ref(false);
const htbShowing = ref(false);
const htbReporting = ref(false);

function addHtbClass() {
  htbCfg.value.classes.push({
    name: 'class' + (htbCfg.value.classes.length + 1),
    rate_mbit: 10, ceil_mbit: 50, priority: 1,
    match_ports: [], match_dst_ips: [], match_mark: 0,
    match_ports_str: '', match_dst_ips_str: '',
  });
}

function syncHtbPorts(row: any) {
  row.match_ports = (row.match_ports_str || '').split(',').map((s: string) => parseInt(s.trim(), 10)).filter((n: number) => !isNaN(n));
}
function parseHtbDst(row: any) {
  row.match_dst_ips = (row.match_dst_ips_str || '').split(',').map((s: string) => s.trim()).filter(Boolean);
}

async function loadHtbConfig() {
  try {
    const res = await api.get('/api/v1/htb/config');
    htbConfig.value.applied = res.data?.applied || false;
    if (res.data?.config && Object.keys(res.data.config).length) {
      const c = res.data.config;
      htbCfg.value = {
        iface: c.iface || 'eth0',
        root_rate_mbit: c.root_rate_mbit || 100,
        default_class: c.default_class || '',
        classes: (c.classes || []).map((cl: any) => ({
          ...cl,
          match_ports_str: (cl.match_ports || []).join(','),
          match_dst_ips_str: (cl.match_dst_ips || []).join(','),
        })),
        enabled: c.enabled !== false,
      };
    }
  } catch (e: any) { showError(e); }
}

function prepareHtbBody() {
  // 同步 match_ports_str → match_ports
  for (const c of htbCfg.value.classes) {
    c.match_ports = (c.match_ports_str || '').split(',').map((s: string) => parseInt(s.trim(), 10)).filter((n: number) => !isNaN(n));
    c.match_dst_ips = (c.match_dst_ips_str || '').split(',').map((s: string) => s.trim()).filter(Boolean);
  }
  return {
    ...htbCfg.value,
    classes: htbCfg.value.classes.map((c) => ({
      name: c.name, rate_mbit: c.rate_mbit, ceil_mbit: c.ceil_mbit,
      priority: c.priority, match_ports: c.match_ports || [],
      match_dst_ips: c.match_dst_ips || [], match_mark: c.match_mark || 0,
    })),
  };
}

async function saveHtbConfig() {
  htbSaving.value = true;
  htbMsg.value = ''; htbMsgOk.value = false;
  try {
    const body = prepareHtbBody();
    const res = await api.post('/api/v1/htb/config', body);
    htbScript.value = res.data?.script || '';
    htbMsg.value = '配置已保存（未下发）';
    htbMsgOk.value = true;
  } catch (e: any) {
    htbMsg.value = errDetail(e, '保存失败');
    htbMsgOk.value = false;
  } finally {
    htbSaving.value = false;
  }
}

async function htbDryRun() {
  htbDryRunning.value = true;
  htbMsg.value = ''; htbMsgOk.value = false;
  try {
    const body = { confirm: false, config: prepareHtbBody() };
    const res = await api.post('/api/v1/htb/apply', body);
    htbScript.value = res.data?.script || '';
    htbMsg.value = res.data?.detail || 'Dry-run 完成，未下发';
    htbMsgOk.value = true;
  } catch (e: any) {
    htbMsg.value = errDetail(e, 'Dry-run 失败');
    htbMsgOk.value = false;
  } finally {
    htbDryRunning.value = false;
  }
}

async function htbConfirmApply() {
  if (!confirm('确认应用将删除并重建该接口的全部 qdisc，是否继续？')) return;
  htbApplying.value = true;
  htbMsg.value = ''; htbMsgOk.value = false;
  try {
    const body = { confirm: true, config: prepareHtbBody() };
    const res = await api.post('/api/v1/htb/apply', body);
    htbScript.value = res.data?.script || '';
    htbMsg.value = res.data?.applied ? '已应用' : (res.data?.detail || '未应用');
    htbMsgOk.value = !!res.data?.applied;
    htbConfig.value.applied = !!res.data?.applied;
  } catch (e: any) {
    htbMsg.value = errDetail(e, '应用失败');
    htbMsgOk.value = false;
  } finally {
    htbApplying.value = false;
  }
}

async function htbCleanup() {
  if (!confirm('确认清理该接口全部 HTB qdisc？')) return;
  htbCleaning.value = true;
  htbMsg.value = ''; htbMsgOk.value = false;
  try {
    const res = await api.post('/api/v1/htb/cleanup', { confirm: true, iface: htbCfg.value.iface });
    htbMsg.value = res.data?.applied ? '已清理 ' + htbCfg.value.iface : (res.data?.detail || '未执行');
    htbMsgOk.value = !!res.data?.applied;
  } catch (e: any) {
    htbMsg.value = errDetail(e, '清理失败');
    htbMsgOk.value = false;
  } finally {
    htbCleaning.value = false;
  }
}

async function showHtbTc() {
  htbShowing.value = true;
  htbMsg.value = '';
  try {
    const res = await api.get('/api/v1/htb/show', { params: { iface: htbCfg.value.iface } });
    if (res.data?.raw) {
      htbScript.value = res.data.raw;
    } else if (res.data?.error) {
      htbMsg.value = res.data.error;
      htbMsgOk.value = false;
    } else {
      htbMsg.value = '无 qdisc 输出';
      htbMsgOk.value = true;
    }
  } catch (e: any) {
    htbMsg.value = errDetail(e, '获取 tc 失败');
    htbMsgOk.value = false;
  } finally {
    htbShowing.value = false;
  }
}

async function showHtbReport() {
  htbReporting.value = true;
  htbMsg.value = '';
  try {
    const res = await api.get('/api/v1/htb/report');
    htbScript.value = res.data?.script || '';
    htbMsg.value = res.data?.config?.iface ? '当前配置：' + res.data.config.iface + ' / ' + res.data.config.root_rate_mbit + ' Mbit' : '无保存配置';
    htbMsgOk.value = true;
  } catch (e: any) {
    htbMsg.value = errDetail(e, '获取报告失败');
    htbMsgOk.value = false;
  } finally {
    htbReporting.value = false;
  }
}

function errDetail(e: any, fallback: string): string {
  const d = e?.response?.data;
  if (!d) return fallback;
  const parts: string[] = [];
  if (d.code) parts.push(`[${d.code}]`);
  if (d.message) parts.push(d.message);
  if (d.detail) parts.push(d.detail);
  return parts.length ? parts.join(' ') : fallback;
}

const limitRulesIface = ref('eth0');
const limitRulesLoading = ref(false);
const limitRulesError = ref('');
const limitRulesData = ref<any>({});

async function loadLimitRules() {
  limitRulesLoading.value = true;
  limitRulesError.value = '';
  limitRulesData.value = {};
  try {
    const res = await api.get('/api/v1/htb/limit-rules', { params: { iface: limitRulesIface.value } });
    limitRulesData.value = res.data || {};
  } catch (e: any) {
    limitRulesError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(limitRulesError.value);
  } finally {
    limitRulesLoading.value = false;
  }
}

onMounted(() => {
  load();
  loadClientLimits();
  loadHtbConfig();
  loadLimitRules();
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
