<template>
  <div>
    <VpnClientsExtra />
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>VPN 客户端（WireGuard）</span>
          <el-button size="small" :loading="loading" @click="load">刷新</el-button>
        </div>
      </template>

      <el-alert v-if="!hasConfig" type="info" show-icon :closable="false" title="还没有配置"
        description="点击下方「编辑配置」添加 peer 并应用。" style="margin-bottom:12px" />

      <el-alert v-if="live === false" type="warning" show-icon :closable="false"
        :title="liveError || '隧道未启动'"
        :description="hint || ''" style="margin-bottom:12px" />

      <div v-if="live === true" style="margin-bottom:12px">
        <el-tag type="success">隧道已启动</el-tag>
        <span style="margin-left:8px; color:#909399; font-size:13px">接口：{{ iface }}，peer 数：{{ peerCount }}</span>
      </div>

      <el-table :data="peers" v-loading="loading" size="small" style="width:100%" empty-text="没有 peer">
        <el-table-column label="公钥" width="180">
          <template #default="{ row }">{{ (row.public_key || '').slice(0,16) }}…</template>
        </el-table-column>
        <el-table-column prop="endpoint" label="对端端点" min-width="160" show-overflow-tooltip />
        <el-table-column label="允许的 IP" min-width="160">
          <template #default="{ row }">{{ (row.allowed_ips || []).join(', ') || '-' }}</template>
        </el-table-column>
        <el-table-column label="最近握手" width="130">
          <template #default="{ row }">
            {{ row.latest_handshake ? formatTs(row.latest_handshake) : '从未握手' }}
          </template>
        </el-table-column>
        <el-table-column label="接收" width="110">
          <template #default="{ row }">{{ fmtBytes(row.rx_bytes) }}</template>
        </el-table-column>
        <el-table-column label="发送" width="110">
          <template #default="{ row }">{{ fmtBytes(row.tx_bytes) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.alive" type="success" size="small">在线</el-tag>
            <el-tooltip v-else :content="row.reason || '离线'" placement="top">
              <el-tag type="danger" size="small">离线</el-tag>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>编辑配置</span>
          <div>
            <el-button size="small" :loading="previewing" @click="preview">预览</el-button>
            <el-button type="primary" size="small" :loading="applying" @click="apply">应用</el-button>
          </div>
        </div>
      </template>

      <el-alert v-if="cfgError" :title="cfgError" type="error" show-icon :closable="false" style="margin-bottom:12px" />

      <el-form label-width="110px" size="small">
        <el-form-item label="接口">
          <el-input v-model="cfg.interface" placeholder="wg0" style="width:200px" />
        </el-form-item>
        <el-form-item label="监听端口">
          <el-input-number v-model="cfg.listen_port" :min="1" :max="65535" />
        </el-form-item>
      </el-form>

      <div style="margin:12px 0 6px; font-weight:600">Peers</div>
      <div v-for="(p, idx) in cfg.peers" :key="idx" style="border:1px solid #e4e7ed; border-radius:6px; padding:12px; margin-bottom:10px">
        <el-form label-width="100px" size="small">
          <el-form-item label="名称">
            <el-input v-model="p.name" placeholder="peer1" />
          </el-form-item>
          <el-form-item label="公钥">
            <el-input v-model="p.public_key" placeholder="base64 公钥" />
          </el-form-item>
          <el-form-item label="端点">
            <el-input v-model="p.endpoint" placeholder="host:port" />
          </el-form-item>
          <el-form-item label="允许的 IP">
            <el-input v-model="p.allowed_ips_text" placeholder="逗号分隔，如 10.0.0.2/32" />
          </el-form-item>
          <el-form-item label="keepalive(秒)">
            <el-input-number v-model="p.keepalive" :min="0" :max="600" />
          </el-form-item>
          <el-form-item>
            <el-button type="danger" size="small" @click="cfg.peers.splice(idx,1)">删除 peer</el-button>
          </el-form-item>
        </el-form>
      </div>
      <el-button size="small" @click="addPeer">+ 添加 peer</el-button>

      <div v-if="previewText" style="margin-top:16px">
        <div style="font-weight:600; margin-bottom:6px">预览 conf</div>
        <pre style="white-space:pre-wrap; background:#f5f7fa; padding:12px; border-radius:6px; max-height:320px; overflow:auto">{{ previewText }}</pre>
      </div>

      <div v-if="applyResult" style="margin-top:16px">
        <div style="font-weight:600; margin-bottom:6px">应用结果</div>
        <el-descriptions :column="1" border size="small">
          <el-descriptions-item label="applied">{{ applyResult.applied }}</el-descriptions-item>
          <el-descriptions-item label="config_path">{{ applyResult.config_path }}</el-descriptions-item>
        </el-descriptions>
        <div v-if="applyResult.commands_run" style="margin-top:8px">
          <div style="font-size:13px; color:#606266; margin-bottom:4px">commands_run：</div>
          <div v-for="(c, i) in applyResult.commands_run" :key="i"
            style="font-family:monospace; font-size:12px; padding:4px 8px; background:#f5f7fa; border-radius:4px; margin-bottom:4px">
            <span :style="{ color: c.ok ? '#67c23a' : '#f56c6c' }">{{ c.ok ? '✓' : '✗' }}</span>
            {{ c.cmd }}  <span style="color:#909399">{{ c.output }}</span>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';
import VpnClientsExtra from './components/VpnClientsExtra.vue';

const loading = ref(false);
const previewing = ref(false);
const applying = ref(false);

const hasConfig = ref(false);
const iface = ref('wg0');
const live = ref<boolean | null>(null);
const liveError = ref('');
const hint = ref('');
const peers = ref<any[]>([]);
const peerCount = ref(0);

const cfg = ref<any>({ interface: 'wg0', listen_port: 51820, peers: [] });
const cfgError = ref('');
const previewText = ref('');
const applyResult = ref<any>(null);

function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

function formatTs(ts: any) {
  if (!ts) return '从未握手';
  const d = new Date(Number(ts) * 1000);
  return d.toLocaleString();
}

function fmtBytes(b: any) {
  const n = Number(b || 0);
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1024 / 1024).toFixed(2) + ' MB';
}

function addPeer() {
  cfg.value.peers.push({ name: '', public_key: '', endpoint: '', allowed_ips_text: '', keepalive: 25 });
}

function buildBody(confirm: boolean) {
  return {
    interface: cfg.value.interface,
    listen_port: cfg.value.listen_port,
    peers: (cfg.value.peers || []).map((p: any) => ({
      name: p.name,
      public_key: p.public_key,
      endpoint: p.endpoint,
      allowed_ips: (p.allowed_ips_text || '').split(',').map((s: string) => s.trim()).filter(Boolean),
      keepalive: p.keepalive,
    })),
    confirm,
  };
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/vpn/client');
    const d = res.data || {};
    hasConfig.value = !!d.has_config;
    iface.value = d.interface || 'wg0';
    live.value = (d.live === undefined ? null : d.live);
    liveError.value = d.live_error || '';
    hint.value = d.hint || '';
    peers.value = d.peers || [];
    peerCount.value = d.peer_count || peers.value.length;
    if (d.config) {
      cfg.value = {
        interface: d.config.interface || 'wg0',
        listen_port: d.config.listen_port || 51820,
        peers: (d.config.peers || []).map((p: any) => ({
          name: p.name || '',
          public_key: p.public_key || '',
          endpoint: p.endpoint || '',
          allowed_ips_text: (p.allowed_ips || []).join(', '),
          keepalive: p.keepalive || 25,
        })),
      };
    }
  } catch (e: any) {
    showError(e, '加载 VPN 客户端失败');
  } finally {
    loading.value = false;
  }
}

async function preview() {
  previewing.value = true;
  cfgError.value = '';
  try {
    const res = await api.post('/api/v1/vpn/client', buildBody(false));
    const d = res.data || {};
    previewText.value = d.conf || '';
    if (d.error) cfgError.value = d.error;
  } catch (e: any) {
    showError(e, '预览失败');
    cfgError.value = e?.response?.data?.detail || e?.response?.data?.message || e?.message || '';
  } finally {
    previewing.value = false;
  }
}

async function apply() {
  applying.value = true;
  cfgError.value = '';
  try {
    const res = await api.post('/api/v1/vpn/client', buildBody(true));
    const d = res.data || {};
    applyResult.value = d;
    if (d.error) {
      cfgError.value = d.error;
      ElMessage.error(d.error);
    } else if (d.applied) {
      ElMessage.success('已应用');
      await load();
    } else {
      ElMessage.warning(d.detail || '未应用');
    }
  } catch (e: any) {
    showError(e, '应用失败');
    cfgError.value = e?.response?.data?.detail || e?.response?.data?.message || e?.message || '';
  } finally {
    applying.value = false;
  }
}

onMounted(load);
</script>
