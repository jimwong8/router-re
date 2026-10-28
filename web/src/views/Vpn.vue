<template>
  <div>
    <el-card shadow="never">
      <template #header>WireGuard 配置</template>

      <el-button type="primary" @click="generateKeypair">生成密钥对</el-button>
      <el-alert v-if="publicKey" type="info" style="margin-top: 12px">
        公钥: <code>{{ publicKey }}</code>
      </el-alert>
      <el-alert title="私钥已生成" type="warning" show-icon style="margin-top: 8px">
        <div>私钥仅在本次响应中显示一次——请立即保存到安全位置。</div>
        <pre style="margin-top: 8px; background:#1e1e1e; color:#d4d4d4; padding:8px; border-radius:4px;">{{ privateKey || '(已隐藏，请重新生成并保存)' }}</pre>
      </el-alert>

      <el-divider>配置</el-divider>
      <el-form label-width="100px" @submit.prevent="submitConfig">
        <el-form-item label="接口">
          <el-input v-model="cfg.interface" placeholder="如 wg0" />
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="cfg.address" placeholder="如 10.200.200.2/32" />
        </el-form-item>
        <el-form-item label="监听端口">
          <el-input-number v-model="cfg.listenPort" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="Peer 列表">
          <el-table :data="cfg.peers" border size="small" style="width: 100%">
            <el-table-column prop="public_key" label="公钥" />
            <el-table-column prop="allowed_ips" label="允许 IP" />
            <el-table-column prop="endpoint" label="端点" width="140" />
          </el-table>
        </el-form-item>
        <el-form-item>
          <el-button @click="addPeer" size="small">添加 Peer</el-button>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="submitConfig">提交</el-button>
        </el-form-item>
      </el-form>

      <el-divider v-if="wgConfigText">WireGuard 配置文本</el-divider>
      <pre v-if="wgConfigText" class="cmd-output">{{ wgConfigText }}</pre>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface Peer {
  public_key: string;
  allowed_ips: string;
  endpoint: string;
}

interface KeypairResp {
  private_key: string;
  public_key: string;
}

const publicKey = ref('');
const privateKey = ref('');
const wgConfigText = ref('');
const loading = ref(false);

const cfg = ref({
  interface: 'wg0',
  address: '10.200.200.2/32',
  listenPort: 51820,
  peers: [
    { public_key: '', allowed_ips: '0.0.0.0/0', endpoint: '' },
  ] as Peer[],
});

function addPeer() {
  cfg.value.peers.push({ public_key: '', allowed_ips: '0.0.0.0/0', endpoint: '' });
}

async function generateKeypair() {
  try {
    const res = await api.post('/api/v1/vpn/wireguard/keypair', {});
    const kp: KeypairResp = res.data;
    publicKey.value = kp.public_key || '';
    privateKey.value = kp.private_key || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submitConfig() {
  loading.value = true;
  try {
    const res = await api.post('/api/v1/vpn/wireguard/config', {
      interface: cfg.value.interface,
      address: cfg.value.address,
      listen_port: cfg.value.listenPort,
      peers: cfg.value.peers,
    });
    wgConfigText.value = res.data?.config || res.data?.text || res.data?.wg_config || JSON.stringify(res.data);
    ElMessage.success('已提交');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
pre {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
}
</style>