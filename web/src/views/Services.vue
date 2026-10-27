<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>DDNS 动态域名</template>
      <el-alert v-if="ddnsError" :title="ddnsError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />

      <el-form label-width="120px" style="max-width: 560px;">
        <el-form-item label="服务商">
          <el-select v-model="cfg.provider" style="width: 100%;">
            <el-option label="Cloudflare" value="cloudflare" />
            <el-option label="阿里云 (AliDNS)" value="alidns" />
          </el-select>
        </el-form-item>
        <el-form-item label="域名">
          <el-input v-model="cfg.domain" placeholder="example.com" />
        </el-form-item>
        <el-form-item label="子域名">
          <el-input v-model="cfg.subdomain" placeholder="home" />
        </el-form-item>
        <el-form-item label="Token/密钥">
          <el-input v-model="cfg.token" type="password" show-password placeholder="不显示完整密钥" />
        </el-form-item>
        <el-form-item label="Zone ID" v-if="cfg.provider === 'cloudflare'">
          <el-input v-model="cfg.zone_id" placeholder="zone id" />
        </el-form-item>
        <el-form-item label="Record ID">
          <el-input v-model="cfg.record_id" placeholder="record id" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="cfg.enabled" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存</el-button>
          <el-button :loading="detecting" @click="detectIP">探测公网 IP</el-button>
          <el-button type="success" :loading="updating" @click="updateNow">立即更新</el-button>
        </el-form-item>
      </el-form>

      <el-descriptions v-if="publicIP" :column="1" border style="margin-top: 8px;">
        <el-descriptions-item label="当前公网 IP">{{ publicIP }}</el-descriptions-item>
      </el-descriptions>
      <el-alert v-if="updateResult" :title="updateResult.ok ? '更新成功' : '更新失败'" :type="updateResult.ok ? 'success' : 'error'" show-icon :closable="false" style="margin-top: 8px;">
        <div>IP：{{ updateResult.ip }}　服务商：{{ updateResult.provider }}</div>
        <div>详情：{{ updateResult.detail }}</div>
      </el-alert>
    </el-card>

    <el-card shadow="never">
      <template #header>NTP 时间同步</template>
      <el-alert v-if="ntpError" :title="ntpError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form label-width="120px" style="max-width: 560px;">
        <el-form-item label="服务器列表">
          <el-select v-model="ntp.servers" multiple filterable allow-create default-first-option style="width: 100%;" placeholder="time.google.com">
          </el-select>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="ntp.enabled" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="ntpSaving" @click="saveNTP">保存并应用</el-button>
          <el-button :loading="ntpLoading" @click="loadNTPStatus">状态</el-button>
        </el-form-item>
      </el-form>
      <el-descriptions v-if="ntpStatus && Object.keys(ntpStatus).length" :column="1" border style="margin-top: 8px;">
        <el-descriptions-item label="服务器">{{ ntpStatus.servername || '-' }}</el-descriptions-item>
        <el-descriptions-item label="时区">{{ ntpStatus.timezone || '-' }}</el-descriptions-item>
        <el-descriptions-item label="NTP 活动">{{ ntpStatus.ntp === 'yes' ? '是' : ntpStatus.ntp || '-' }}</el-descriptions-item>
        <el-descriptions-item label="已同步">{{ ntpStatus.synchronized === 'yes' ? '是' : ntpStatus.synchronized || '-' }}</el-descriptions-item>
      </el-descriptions>
      <el-alert v-if="ntpText" title="渲染的 timesyncd.conf" type="info" :closable="false" style="margin-top: 8px;">
        <pre style="white-space: pre-wrap;">{{ ntpText }}</pre>
      </el-alert>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const cfg = ref<any>({ provider: 'cloudflare', domain: '', subdomain: '', token: '', zone_id: '', record_id: '', enabled: true });
const saving = ref(false);
const detecting = ref(false);
const updating = ref(false);
const publicIP = ref('');
const updateResult = ref<any>(null);
const ddnsError = ref('');

const ntp = ref<any>({ servers: [], enabled: true });
const ntpSaving = ref(false);
const ntpLoading = ref(false);
const ntpStatus = ref<any>(null);
const ntpText = ref('');
const ntpError = ref('');

async function loadDDNS() {
  try {
    const res = await api.get('/api/v1/ddns/config');
    cfg.value = res.data?.config || cfg.value;
  } catch (e: any) {
    ddnsError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载 DDNS 配置失败';
  }
}

async function save() {
  saving.value = true;
  try {
    await api.post('/api/v1/ddns/config', cfg.value);
    ElMessage.success('已保存');
    await loadDDNS();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    ddnsError.value = msg;
    ElMessage.error(msg);
  } finally {
    saving.value = false;
  }
}

async function detectIP() {
  detecting.value = true;
  try {
    const res = await api.get('/api/v1/ddns/ip');
    publicIP.value = res.data?.ip || '';
    ElMessage.success('公网 IP：' + publicIP.value);
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '探测失败';
    ddnsError.value = msg;
    ElMessage.error(msg);
  } finally {
    detecting.value = false;
  }
}

async function updateNow() {
  updating.value = true;
  updateResult.value = null;
  try {
    const res = await api.post('/api/v1/ddns/update', {});
    updateResult.value = res.data;
    publicIP.value = res.data?.ip || publicIP.value;
    if (res.data?.ok) ElMessage.success('DDNS 更新成功');
    else ElMessage.error('DDNS 更新失败：' + (res.data?.detail || ''));
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '更新失败';
    ddnsError.value = msg;
    ElMessage.error(msg);
  } finally {
    updating.value = false;
  }
}

async function loadNTP() {
  try {
    const res = await api.get('/api/v1/ntp/config');
    ntp.value = res.data?.config || ntp.value;
  } catch (e: any) {
    ntpError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载 NTP 配置失败';
  }
}

async function saveNTP() {
  ntpSaving.value = true;
  ntpText.value = '';
  try {
    const res = await api.post('/api/v1/ntp/config', ntp.value);
    ntp.value = res.data?.config || ntp.value;
    ntpText.value = res.data?.text || '';
    if (res.data?.applied) ElMessage.success('NTP 已应用');
    else ElMessage.warning('NTP 已保存但未应用：' + (res.data?.reason || '未启用'));
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    ntpError.value = msg;
    ElMessage.error(msg);
  } finally {
    ntpSaving.value = false;
  }
}

async function loadNTPStatus() {
  ntpLoading.value = true;
  try {
    const res = await api.get('/api/v1/ntp/status');
    ntpStatus.value = res.data;
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '状态获取失败';
    ntpError.value = msg;
    ElMessage.error(msg);
  } finally {
    ntpLoading.value = false;
  }
}

onMounted(() => {
  loadDDNS();
  loadNTP();
});
</script>