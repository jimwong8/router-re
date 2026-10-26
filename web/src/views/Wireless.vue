<template>
  <div>
    <el-card shadow="never" class="section">
      <template #header>AP 列表</template>
      <el-table :data="aps" border style="width: 100%">
        <el-table-column prop="id" label="AP ID" width="140" />
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="mac" label="MAC" />
        <el-table-column prop="channel" label="信道" width="80" />
        <el-table-column prop="online" label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.online ? 'success' : 'danger'">{{ row.online ? '在线' : '离线' }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <el-button type="primary" style="margin-top: 12px" @click="loadAPs">刷新</el-button>
    </el-card>

    <el-card shadow="never" class="section" style="margin-top: 16px;">
      <template #header>SSID 配置</template>
      <el-form inline @submit.prevent="addSSID">
        <el-form-item label="名称">
          <el-input v-model="newSSID.name" placeholder="SSID 名称" />
        </el-form-item>
        <el-form-item label="安全">
          <el-select v-model="newSSID.security" style="width: 120px;">
            <el-option label="wpa2" value="wpa2" />
            <el-option label="wpa3" value="wpa3" />
            <el-option label="open" value="open" />
          </el-select>
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="newSSID.password" placeholder="密码（open 时可空）" />
        </el-form-item>
        <el-form-item label="VLAN">
          <el-input-number v-model="newSSID.vlan" :min="1" :max="4094" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addSSID">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="ssids" border style="width: 100%; margin-top: 12px;">
        <el-table-column prop="name" label="SSID" />
        <el-table-column prop="security" label="安全" width="100" />
        <el-table-column prop="vlan" label="VLAN" width="80" />
        <el-table-column prop="enabled" label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" size="small" />
          </template>
        </el-table-column>
      </el-table>
      <el-button type="primary" style="margin-top: 12px" :loading="submitting" @click="submitSSIDs">提交</el-button>

      <el-divider v-if="hostapdText || wpaText">渲染配置</el-divider>
      <div v-if="hostapdText || wpaText" style="display:flex; gap: 12px; margin-top:8px;">
        <div v-if="hostapdText" style="flex:1;">
          <strong>hostapd.conf</strong>
          <pre class="cmd-output">{{ hostapdText }}</pre>
        </div>
        <div v-if="wpaText" style="flex:1;">
          <strong>wpa_supplicant.conf</strong>
          <pre class="cmd-output">{{ wpaText }}</pre>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const aps = ref<any[]>([]);
const ssids = ref<any[]>([]);
const submitting = ref(false);
const hostapdText = ref('');
const wpaText = ref('');

const newSSID = ref({ name: '', security: 'wpa2', password: '', vlan: 10, enabled: true });

function loadAPs() {
  api.get('/api/v1/wireless/aps').then((res) => {
    aps.value = res.data?.aps || [];
  }).catch((e: any) => ElMessage.error(e?.response?.data?.message || e.message));
}

function addSSID() {
  if (!newSSID.value.name) {
    ElMessage.warning('SSID 名称必填');
    return;
  }
  ssids.value.push({ ...newSSID.value });
  newSSID.value = { name: '', security: 'wpa2', password: '', vlan: 10, enabled: true };
}

async function submitSSIDs() {
  submitting.value = true;
  try {
    const res = await api.post('/api/v1/wireless/ssids', { ssids: ssids.value.map(s => ({ name: s.name, security: s.security, password: s.password, vlan: s.vlan, enabled: s.enabled })) });
    hostapdText.value = res.data?.hostapd || res.data?.hostapd_conf || '';
    wpaText.value = res.data?.wpa_supplicant || res.data?.wpa_conf || '';
    ElMessage.success('已提交');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    submitting.value = false;
  }
}

onMounted(loadAPs);
</script>

<style scoped>
.section { margin-bottom: 16px; }
.cmd-output {
  background: #1e1e1e; color: #d4d4d4; padding: 12px;
  border-radius: 4px; white-space: pre-wrap; word-break: break-all; font-size: 13px;
}
</style>