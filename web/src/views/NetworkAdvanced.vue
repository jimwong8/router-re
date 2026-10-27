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

const activeTab = ref('bond');

onMounted(() => {});
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; background:#1e1e1e; color:#d4d4d4; padding:8px; border-radius:4px; overflow-x:auto; white-space:pre-wrap; }
</style>
