<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>网络高级</span>
        </div>
      </template>

      <el-tabs v-model="activeTab" type="card">
        <!-- Tab 1: 链路聚合 -->
        <el-tab-pane name="bond" label="链路聚合">
          <el-form :model="bondForm" label-width="110px" style="max-width:640px">
            <el-form-item label="名称">
              <el-input v-model="bondForm.name" placeholder="bond0" />
            </el-form-item>
            <el-form-item label="从属接口">
              <el-input v-model="bondForm.slavesText" placeholder="eth1,eth2（逗号分隔）" />
            </el-form-item>
            <el-form-item label="模式">
              <el-select v-model="bondForm.mode" style="width:100%">
                <el-option label="802.3ad" value="802.3ad" />
                <el-option label="active-backup" value="active-backup" />
                <el-option label="balance-rr" value="balance-rr" />
                <el-option label="balance-xor" value="balance-xor" />
              </el-select>
            </el-form-item>
            <el-form-item label="MIIMon(ms)">
              <el-input-number v-model="bondForm.miimon" :min="0" :max="10000" />
            </el-form-item>
            <el-form-item label="MTU">
              <el-input-number v-model="bondForm.mtu" :min="0" :max="9000" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="bondPreviewing" @click="previewBond">预览脚本</el-button>
              <el-button type="danger" :loading="bondApplying" @click="applyBond">应用</el-button>
            </el-form-item>
          </el-form>

          <el-alert v-if="bondError" :title="bondError" type="error" :closable="false" style="margin:12px 0" />
          <div v-if="bondResult">
            <div style="margin:8px 0 4px; font-weight:bold">applied: {{ bondResult.applied }}　detail: {{ bondResult.detail }}</div>
            <pre v-if="bondResult.script">{{ bondResult.script }}</pre>
          </div>
        </el-tab-pane>

        <!-- Tab 2: 接口调优 -->
        <el-tab-pane name="tune" label="接口调优">
          <el-form :inline="true" label-width="80px" style="margin-bottom:12px">
            <el-form-item label="接口">
              <el-input v-model="tuneIface" placeholder="ens18" style="width:160px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" size="small" :loading="tuneLoading" @click="loadTune">加载</el-button>
            </el-form-item>
          </el-form>

          <el-descriptions v-if="tuneLive && Object.keys(tuneLive).length" title="当前值" :column="2" border style="margin-bottom:12px">
            <el-descriptions-item v-for="(v, k) in tuneLive" :key="k" :label="k">{{ v }}</el-descriptions-item>
          </el-descriptions>

          <el-form :model="tuneForm" label-width="110px" style="max-width:480px">
            <el-form-item label="MTU">
              <el-input-number v-model="tuneForm.mtu" :min="0" :max="9000" />
            </el-form-item>
            <el-form-item label="TxQueueLen">
              <el-input-number v-model="tuneForm.txqueuelen" :min="0" :max="10000" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="tuneSaving" @click="saveTune">保存</el-button>
            </el-form-item>
          </el-form>

          <el-alert v-if="tuneError" :title="tuneError" type="error" :closable="false" style="margin:12px 0" />
          <div v-if="tuneResult">
            <div style="margin:8px 0 4px; font-weight:bold">applied: {{ tuneResult.applied }}　detail: {{ tuneResult.detail }}</div>
          </div>
        </el-tab-pane>

        <!-- Tab 3: 端口触发 + ALG -->
        <el-tab-pane name="trigger" label="端口触发 + ALG">
          <el-card shadow="never" style="margin-bottom:16px">
            <template #header><span>端口触发规则</span></template>
            <el-table :data="triggerRules" style="width:100%" row-key="name">
              <el-table-column prop="name" label="名称" width="120" />
              <el-table-column prop="trigger_port" label="触发端口" width="110" />
              <el-table-column prop="open_port" label="开放端口" width="110" />
              <el-table-column prop="protocol" label="协议" width="90" />
              <el-table-column prop="timeout_sec" label="超时(秒)" width="100" />
              <el-table-column prop="enabled" label="启用" width="80">
                <template #default="s"><el-switch v-model="s.row.enabled" size="small" /></template>
              </el-table-column>
            </el-table>
            <div style="margin-top:12px; text-align:right">
              <el-button type="primary" :loading="triggerSaving" @click="saveTrigger">保存</el-button>
            </div>
            <el-alert v-if="triggerError" :title="triggerError" type="error" :closable="false" style="margin:12px 0" />
            <pre v-if="triggerNft">{{ triggerNft }}</pre>
          </el-card>

          <el-card shadow="never">
            <template #header><span>ALG（应用层网关）</span></template>
            <el-checkbox-group v-model="algSelected">
              <el-checkbox v-for="a in algOptions" :key="a" :label="a">{{ a }}</el-checkbox>
            </el-checkbox-group>
            <div style="margin-top:12px">
              <el-button type="primary" :loading="algSaving" @click="saveAlg">保存 ALG</el-button>
            </div>
            <el-alert v-if="algError" :title="algError" type="error" :closable="false" style="margin:12px 0" />
            <pre v-if="algScript">{{ algScript }}</pre>
          </el-card>
        </el-tab-pane>

        <!-- Tab 4: BGP -->
        <el-tab-pane name="bgp" label="BGP">
          <el-form :model="bgpForm" label-width="110px" style="max-width:720px">
            <el-form-item label="本地 AS">
              <el-input-number v-model="bgpForm.local_as" :min="0" :max="4294967295" />
            </el-form-item>
            <el-form-item label="Router ID">
              <el-input v-model="bgpForm.router_id" placeholder="0.0.0.0" />
            </el-form-item>
            <el-form-item label="邻居 (Peers)">
              <el-table :data="bgpForm.peers" style="width:100%" row-key="address">
                <el-table-column prop="address" label="地址" width="140" />
                <el-table-column prop="remote_as" label="远端 AS" width="100" />
                <el-table-column prop="password" label="MD5 密码" width="140" />
                <el-table-column label="多跳" width="80">
                  <template #default="scope">
                    <el-switch v-model="scope.row.multi_hop" size="small" />
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="100">
                  <template #default="scope">
                    <el-button size="small" type="danger" @click="removePeer(scope.$index)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>
              <div style="margin:8px 0 4px">
                <el-input v-model="newPeer.address" placeholder="对端地址" style="width:140px; display:inline-block; margin-right:6px" />
                <el-input-number v-model="newPeer.remote_as" placeholder="AS" style="width:90px; display:inline-block; margin-right:6px" :min="0" :max="4294967295" />
                <el-input v-model="newPeer.password" placeholder="密码" style="width:140px; display:inline-block; margin-right:6px" />
                <el-switch v-model="newPeer.multi_hop" active-text="多跳" style="margin-right:6px" />
                <el-button type="primary" size="small" @click="addPeer">添加</el-button>
              </div>
            </el-form-item>
            <el-form-item label="网段 (Networks)">
              <el-input v-model="bgpNetworksText" type="textarea" :rows="3" placeholder="每行一个网段，如 10.0.0.0/24" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="bgpLoading" @click="fetchBgp">加载</el-button>
              <el-button type="primary" :loading="bgpSaving" @click="saveBgp">保存</el-button>
            </el-form-item>
          </el-form>
          <el-alert v-if="bgpError" :title="bgpError" type="error" :closable="false" style="margin:12px 0" />
          <div v-if="bgpResult">
            <div style="margin:8px 0 4px; font-weight:bold">detail: {{ bgpResult.detail }}</div>
            <pre v-if="bgpResult.frr">{{ bgpResult.frr }}</pre>
          </div>
        </el-tab-pane>

        <!-- Tab 5: OSPF -->
        <el-tab-pane name="ospf" label="OSPF">
          <el-form :model="ospfForm" label-width="110px" style="max-width:720px">
            <el-form-item label="Router ID">
              <el-input v-model="ospfForm.router_id" placeholder="0.0.0.0" />
            </el-form-item>
            <el-form-item label="区域 (Area)">
              <el-input v-model="ospfForm.area" placeholder="0.0.0.0" />
            </el-form-item>
            <el-form-item label="网段 (Networks)">
              <el-input v-model="ospfNetworksText" type="textarea" :rows="3" placeholder="每行一个网段，如 10.0.0.0/24" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="ospfLoading" @click="fetchOspf">加载</el-button>
              <el-button type="primary" :loading="ospfSaving" @click="saveOspf">保存</el-button>
            </el-form-item>
          </el-form>
          <el-alert v-if="ospfError" :title="ospfError" type="error" :closable="false" style="margin:12px 0" />
          <div v-if="ospfResult">
            <div style="margin:8px 0 4px; font-weight:bold">detail: {{ ospfResult.detail }}</div>
            <pre v-if="ospfResult.frr">{{ ospfResult.frr }}</pre>
          </div>
        </el-tab-pane>
        <!-- Tab 6: 端口镜像 -->
        <el-tab-pane name="mirror" label="端口镜像">
          <el-form :model="mirrorForm" label-width="110px" style="max-width:640px">
            <el-form-item label="源接口">
              <el-input v-model="mirrorForm.source_iface" placeholder="eth0" />
            </el-form-item>
            <el-form-item label="目标接口">
              <el-input v-model="mirrorForm.target_iface" placeholder="eth1" />
            </el-form-item>
            <el-form-item label="方向">
              <el-select v-model="mirrorForm.direction" style="width:100%">
                <el-option label="ingress" value="ingress" />
                <el-option label="egress" value="egress" />
                <el-option label="both" value="both" />
              </el-select>
            </el-form-item>
            <el-form-item label="启用">
              <el-switch v-model="mirrorForm.enabled" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="mirrorPreviewing" @click="previewMirror">预览</el-button>
              <el-button type="danger" :loading="mirrorApplying" @click="applyMirror">应用</el-button>
            </el-form-item>
          </el-form>
          <el-alert v-if="mirrorError" :title="mirrorError" type="error" :closable="false" style="margin:12px 0" />
          <div v-if="mirrorResult">
            <div style="margin:8px 0 4px; font-weight:bold">applied: {{ mirrorResult.applied }}　detail: {{ mirrorResult.detail }}</div>
            <pre v-if="mirrorResult.script">{{ mirrorResult.script }}</pre>
          </div>
          <div v-if="mirrorCleanup" style="margin-top:8px">
            <div style="font-weight:bold; margin-bottom:4px">清理命令:</div>
            <pre>{{ mirrorCleanup }}</pre>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

// ---------- Tab 1: bond ----------
const bondForm = ref({
  name: 'bond0',
  slavesText: '',
  mode: '802.3ad',
  miimon: 100,
  mtu: 0,
});
const bondPreviewing = ref(false);
const bondApplying = ref(false);
const bondError = ref('');
const bondResult = ref<any>(null);

function bondConfig() {
  return {
    name: bondForm.value.name,
    slaves: bondForm.value.slavesText.split(',').map(s => s.trim()).filter(Boolean),
    mode: bondForm.value.mode,
    miimon: bondForm.value.miimon,
    mtu: bondForm.value.mtu,
  };
}

async function loadBond() {
  try {
    const res = await api.get('/api/v1/bond');
    const c = res.data?.config || {};
    if (c.name) bondForm.value.name = c.name;
    if (Array.isArray(c.slaves)) bondForm.value.slavesText = c.slaves.join(',');
    if (c.mode) bondForm.value.mode = c.mode;
    if (c.miimon) bondForm.value.miimon = c.miimon;
    if (c.mtu) bondForm.value.mtu = c.mtu;
  } catch (e: any) {
    bondError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '加载 Bond 配置失败');
  }
}

async function previewBond() {
  bondPreviewing.value = true;
  bondError.value = '';
  bondResult.value = null;
  try {
    const res = await api.post('/api/v1/bond', { config: bondConfig() });
    bondResult.value = res.data || {};
  } catch (e: any) {
    bondError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '预览失败');
  } finally {
    bondPreviewing.value = false;
  }
}

async function applyBond() {
  bondApplying.value = true;
  bondError.value = '';
  bondResult.value = null;
  try {
    const res = await api.post('/api/v1/bond', { config: bondConfig(), confirm: true });
    bondResult.value = res.data || {};
  } catch (e: any) {
    bondError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '应用失败');
  } finally {
    bondApplying.value = false;
  }
}

// ---------- Tab 2: iface tune ----------
const tuneIface = ref('ens18');
const tuneLoading = ref(false);
const tuneSaving = ref(false);
const tuneError = ref('');
const tuneLive = ref<Record<string, any>>({});
const tuneResult = ref<any>(null);
const tuneForm = ref({ mtu: 0, txqueuelen: 0 });

async function loadTune() {
  tuneLoading.value = true;
  tuneError.value = '';
  tuneLive.value = {};
  try {
    const res = await api.get('/api/v1/iface/tune', { params: { iface: tuneIface.value } });
    tuneLive.value = res.data?.live || {};
    if (res.data?.mtu) tuneForm.value.mtu = res.data.mtu;
    if (res.data?.txqueuelen) tuneForm.value.txqueuelen = res.data.txqueuelen;
  } catch (e: any) {
    tuneError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '加载失败');
  } finally {
    tuneLoading.value = false;
  }
}

async function saveTune() {
  tuneSaving.value = true;
  tuneError.value = '';
  tuneResult.value = null;
  try {
    const res = await api.post('/api/v1/iface/tune', {
      iface: tuneIface.value,
      mtu: tuneForm.value.mtu,
      txqueuelen: tuneForm.value.txqueuelen,
    });
    tuneResult.value = res.data || {};
  } catch (e: any) {
    tuneError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '保存失败');
  } finally {
    tuneSaving.value = false;
  }
}

// ---------- Tab 3: port trigger + ALG ----------
interface TriggerRule {
  name: string; trigger_port: number; open_port: number;
  protocol: string; timeout_sec: number; enabled: boolean;
}
const triggerRules = ref<TriggerRule[]>([
  { name: 'ftp-trigger', trigger_port: 21, open_port: 21, protocol: 'tcp', timeout_sec: 60, enabled: false },
]);
const triggerSaving = ref(false);
const triggerError = ref('');
const triggerNft = ref('');

const algOptions = ['ftp', 'sip', 'h323', 'tftp', 'irc', 'pptp'];
const algSelected = ref<string[]>([]);
const algSaving = ref(false);
const algError = ref('');
const algScript = ref('');

async function loadTrigger() {
  try {
    const res = await api.get('/api/v1/porttrigger');
    const rules = res.data?.rules || [];
    triggerRules.value = rules.map((r: any) => ({
      name: r.name || '',
      trigger_port: r.trigger_port || 0,
      open_port: r.open_port || 0,
      protocol: r.protocol || 'tcp',
      timeout_sec: r.timeout_sec || 60,
      enabled: !!r.enabled,
    }));
    if (res.data?.nft) triggerNft.value = res.data.nft;
  } catch (e: any) {
    triggerError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '加载端口触发规则失败');
  }
}

async function loadAlg() {
  try {
    const res = await api.get('/api/v1/alg');
    const c = res.data?.config || {};
    const selected: string[] = [];
    if (c.ftp) selected.push('ftp');
    if (c.sip) selected.push('sip');
    if (c.h323) selected.push('h323');
    if (c.tftp) selected.push('tftp');
    if (c.irc) selected.push('irc');
    if (c.pptp) selected.push('pptp');
    algSelected.value = selected;
    if (res.data?.script) algScript.value = res.data.script;
  } catch (e: any) {
    algError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '加载 ALG 配置失败');
  }
}

async function saveTrigger() {
  triggerSaving.value = true;
  triggerError.value = '';
  triggerNft.value = '';
  try {
    const res = await api.post('/api/v1/porttrigger', { rules: triggerRules.value });
    const d = res.data || {};
    triggerNft.value = d.nft || '';
    if (d.detail) ElMessage.success(d.detail);
  } catch (e: any) {
    triggerError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '保存失败');
  } finally {
    triggerSaving.value = false;
  }
}

async function saveAlg() {
  algSaving.value = true;
  algError.value = '';
  algScript.value = '';
  try {
    const res = await api.post('/api/v1/alg', { algs: algSelected.value });
    const d = res.data || {};
    algScript.value = d.script || '';
  } catch (e: any) {
    algError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '保存失败');
  } finally {
    algSaving.value = false;
  }
}

// ---------- Tab 4: BGP ----------
const bgpForm = ref({
  local_as: 0,
  router_id: '',
  peers: [] as { address: string; remote_as: number; password: string; multi_hop: boolean }[],
});
const newPeer = ref({ address: '', remote_as: 0, password: '', multi_hop: false });
const bgpNetworksText = ref('');
const bgpLoading = ref(false);
const bgpSaving = ref(false);
const bgpError = ref('');
const bgpResult = ref<any>(null);

function bgpNetworks(): string[] {
  return bgpNetworksText.value.split('\n').map(s => s.trim()).filter(Boolean);
}

function addPeer() {
  const p = newPeer.value;
  if (!p.address) return;
  bgpForm.value.peers.push({ address: p.address, remote_as: p.remote_as, password: p.password, multi_hop: p.multi_hop });
  newPeer.value = { address: '', remote_as: 0, password: '', multi_hop: false };
}

function removePeer(index: number) {
  bgpForm.value.peers.splice(index, 1);
}

async function fetchBgp() {
  bgpLoading.value = true;
  bgpError.value = '';
  bgpResult.value = null;
  try {
    const res = await api.get('/api/v1/route/bgp');
    const d = res.data || {};
    const c = d.config || {};
    bgpForm.value.local_as = c.local_as ?? 0;
    bgpForm.value.router_id = c.router_id || '';
    bgpForm.value.peers = (c.peers || []).map((p: any) => ({
      address: p.address || '',
      remote_as: p.remote_as || 0,
      password: p.password || '',
      multi_hop: Boolean(p.multi_hop),
    }));
    bgpNetworksText.value = (c.networks || []).join('\n');
    if (d.frr !== undefined) bgpResult.value = { detail: d.detail || '', frr: d.frr };
  } catch (e: any) {
    bgpError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '加载 BGP 失败');
  } finally {
    bgpLoading.value = false;
  }
}

async function saveBgp() {
  bgpSaving.value = true;
  bgpError.value = '';
  bgpResult.value = null;
  try {
    const res = await api.post('/api/v1/route/bgp', {
      local_as: bgpForm.value.local_as,
      router_id: bgpForm.value.router_id,
      peers: bgpForm.value.peers,
      networks: bgpNetworks(),
    });
    const d = res.data || {};
    bgpResult.value = { detail: d.detail || '', frr: d.frr || '' };
    if (d.detail) ElMessage.success(d.detail);
  } catch (e: any) {
    bgpError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '保存 BGP 失败');
  } finally {
    bgpSaving.value = false;
  }
}

// ---------- Tab 5: OSPF ----------
const ospfForm = ref({ router_id: '', area: '' });
const ospfNetworksText = ref('');
const ospfLoading = ref(false);
const ospfSaving = ref(false);
const ospfError = ref('');
const ospfResult = ref<any>(null);

function ospfNetworks(): string[] {
  return ospfNetworksText.value.split('\n').map(s => s.trim()).filter(Boolean);
}

async function fetchOspf() {
  ospfLoading.value = true;
  ospfError.value = '';
  ospfResult.value = null;
  try {
    const res = await api.get('/api/v1/route/ospf');
    const d = res.data || {};
    const c = d.config || {};
    ospfForm.value.router_id = c.router_id || '';
    ospfForm.value.area = c.area || '';
    ospfNetworksText.value = (c.networks || []).join('\n');
    if (d.frr !== undefined) ospfResult.value = { detail: d.detail || '', frr: d.frr };
  } catch (e: any) {
    ospfError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '加载 OSPF 失败');
  } finally {
    ospfLoading.value = false;
  }
}

async function saveOspf() {
  ospfSaving.value = true;
  ospfError.value = '';
  ospfResult.value = null;
  try {
    const res = await api.post('/api/v1/route/ospf', {
      router_id: ospfForm.value.router_id,
      area: ospfForm.value.area,
      networks: ospfNetworks(),
    });
    const d = res.data || {};
    ospfResult.value = { detail: d.detail || '', frr: d.frr || '' };
    if (d.detail) ElMessage.success(d.detail);
  } catch (e: any) {
    ospfError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '保存 OSPF 失败');
  } finally {
    ospfSaving.value = false;
  }
}

const mirrorForm = ref({
  source_iface: '',
  target_iface: '',
  direction: 'both',
  enabled: false,
});
const mirrorPreviewing = ref(false);
const mirrorApplying = ref(false);
const mirrorError = ref('');
const mirrorResult = ref<any>(null);
const mirrorCleanup = ref('');

async function loadMirror() {
  try {
    const res = await api.get('/api/v1/mirror');
    const c = res.data?.config || {};
    mirrorForm.value.source_iface = c.source_iface || '';
    mirrorForm.value.target_iface = c.target_iface || '';
    mirrorForm.value.direction = c.direction || 'both';
    mirrorForm.value.enabled = !!c.enabled;
    if (res.data?.cleanup) mirrorCleanup.value = res.data.cleanup;
  } catch (e: any) {
    mirrorError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '加载镜像配置失败');
  }
}

async function previewMirror() {
  mirrorPreviewing.value = true;
  mirrorError.value = '';
  mirrorResult.value = null;
  try {
    const res = await api.post('/api/v1/mirror', { config: mirrorForm.value, confirm: false });
    mirrorResult.value = res.data || {};
    if (res.data?.cleanup) mirrorCleanup.value = res.data.cleanup;
  } catch (e: any) {
    mirrorError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '预览失败');
  } finally {
    mirrorPreviewing.value = false;
  }
}

async function applyMirror() {
  mirrorApplying.value = true;
  mirrorError.value = '';
  mirrorResult.value = null;
  try {
    const res = await api.post('/api/v1/mirror', { config: mirrorForm.value, confirm: true });
    mirrorResult.value = res.data || {};
    if (res.data?.cleanup) mirrorCleanup.value = res.data.cleanup;
    if (res.data?.detail) ElMessage.success(res.data.detail);
  } catch (e: any) {
    mirrorError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    showError(e, '应用失败');
  } finally {
    mirrorApplying.value = false;
  }
}

const activeTab = ref('bond');

onMounted(() => {
  loadBond();
  loadTrigger();
  loadAlg();
  loadMirror();
});
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; background:#1e1e1e; color:#d4d4d4; padding:8px; border-radius:4px; overflow-x:auto; white-space:pre-wrap; }
</style>
