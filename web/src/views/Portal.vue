<template>
  <div>
    <!-- 设置卡片 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>上网认证设置</template>
      <el-form inline>
        <el-form-item label="启用">
          <el-switch v-model="cfg.enabled" />
        </el-form-item>
        <el-form-item label="门户端口">
          <el-input-number v-model="cfg.portal_port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="默认时长(分钟)">
          <el-input-number v-model="cfg.default_minutes" :min="1" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveConfig">保存设置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 授权表单 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>授权客户端</template>
      <el-form inline>
        <el-form-item label="MAC">
          <el-input v-model="authForm.mac" placeholder="aa:bb:cc:dd:ee:ff" style="width:180px" />
        </el-form-item>
        <el-form-item label="IP">
          <el-input v-model="authForm.ip" placeholder="192.168.1.10" style="width:150px" />
        </el-form-item>
        <el-form-item label="时长(分钟)">
          <el-input-number v-model="authForm.minutes" :min="1" :default-value="cfg.default_minutes" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="authing" @click="authorize">授权</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Portal 状态 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>Portal 状态</span>
          <el-button size="small" :loading="statusLoading" @click="loadPortalStatus">刷新</el-button>
        </div>
      </template>
      <el-alert v-if="statusErr" type="error" :closable="false" show-icon :title="statusErr" style="margin-bottom:12px" />
      <el-descriptions :column="3" border size="small" style="margin-bottom:12px">
        <el-descriptions-item label="在线">{{ portalStatus.online ? '是' : '否' }}</el-descriptions-item>
        <el-descriptions-item label="在线数量">{{ portalStatus.online_count ?? 0 }}</el-descriptions-item>
        <el-descriptions-item label="已认证客户端">{{ (portalStatus.clients || []).length }}</el-descriptions-item>
      </el-descriptions>
      <el-table :data="portalStatus.clients || []" border size="small" style="width:100%" empty-text="暂无在线客户端">
        <el-table-column prop="mac" label="MAC" width="170" />
        <el-table-column prop="ip" label="IP" width="150" />
        <el-table-column label="已授权" width="90">
          <template #default="{ row }">
            <el-tag :type="row.authorized ? 'success' : 'info'">{{ row.authorized ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="授权时间">
          <template #default="{ row }">
            {{ row.authorized_at ? new Date(row.authorized_at * 1000).toLocaleString() : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="过期时间">
          <template #default="{ row }">
            {{ row.expires_at ? new Date(row.expires_at * 1000).toLocaleString() : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160">
          <template #default="{ row }">
            <el-button type="warning" size="small" :loading="logoutBusy === row.mac" @click="logoutClient(row)">登出</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="logoutResult" style="margin-top:8px; font-size:13px; color:#606266">
        上次登出：revoked={{ logoutResult.revoked }}，online={{ logoutResult.online }}，online_count={{ logoutResult.online_count }}
      </div>
    </el-card>

    <!-- 客户端表格 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>在线客户端（{{ clients.length }}）</span>
          <el-button size="small" :loading="loading" @click="loadClients">刷新</el-button>
        </div>
      </template>
      <el-table :data="clients" border style="width:100%">
        <el-table-column prop="mac" label="MAC" width="170" />
        <el-table-column prop="ip" label="IP" width="150" />
        <el-table-column label="已授权" width="90">
          <template #default="{ row }">
            <el-tag :type="row.authorized ? 'success' : 'info'">{{ row.authorized ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="授权时间">
          <template #default="{ row }">
            {{ row.authorized_at ? new Date(row.authorized_at * 1000).toLocaleString() : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="过期时间">
          <template #default="{ row }">
            {{ row.expires_at ? new Date(row.expires_at * 1000).toLocaleString() : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="revoke(row)">撤销</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-divider>nft 输出</el-divider>
      <pre v-if="nftText" class="nft-out">{{ nftText }}</pre>
      <el-empty v-else description="授权后将在此显示下发的 nft 规则" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

interface PortalClient {
  mac: string;
  ip: string;
  authorized: boolean;
  authorized_at: number;
  expires_at: number;
}

const clients = ref<PortalClient[]>([]);
const loading = ref(false);
const authing = ref(false);
const saving = ref(false);
const nftText = ref('');
const cfg = reactive({ enabled: false, portal_port: 8080, default_minutes: 60 });
const authForm = reactive({ mac: '', ip: '', minutes: 60 });

async function loadClients() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/portal/clients');
    clients.value = res.data?.clients || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}

async function loadConfig() {
  try {
    const res = await api.get('/api/v1/portal/config');
    Object.assign(cfg, res.data || {});
    authForm.minutes = cfg.default_minutes;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function saveConfig() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/portal/config', { ...cfg });
    Object.assign(cfg, res.data || {});
    ElMessage.success('设置已保存');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

async function authorize() {
  if (!authForm.mac || !authForm.ip) {
    ElMessage.warning('MAC 和 IP 必填');
    return;
  }
  authing.value = true;
  try {
    const res = await api.post('/api/v1/portal/authorize', {
      mac: authForm.mac,
      ip: authForm.ip,
      minutes: authForm.minutes,
    });
    nftText.value = res.data?.nft || '';
    if (res.data?.applied) {
      ElMessage.success('已授权并下发规则');
    } else {
      ElMessage.warning('已授权，但 nft 下发失败: ' + (res.data?.error || ''));
    }
    await loadClients();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    authing.value = false;
  }
}

async function revoke(row: PortalClient) {
  try {
    await ElMessageBox.confirm(`确认撤销客户端 ${row.mac}?`, '撤销授权', { type: 'warning' });
  } catch {
    return;
  }
  try {
    const res = await api.post('/api/v1/portal/revoke', { mac: row.mac });
    nftText.value = res.data?.nft || '';
    ElMessage.success(res.data?.revoked ? '已撤销' : '未找到该客户端');
    await loadClients();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

const portalStatus = ref<any>({ online: false, online_count: 0, clients: [] });
const statusLoading = ref(false);
const statusErr = ref('');
const logoutBusy = ref('');
const logoutResult = ref<any>(null);

async function loadPortalStatus() {
  statusLoading.value = true;
  statusErr.value = '';
  try {
    const res = await api.get('/api/v1/portal/status');
    portalStatus.value = res.data || { online: false, online_count: 0, clients: [] };
  } catch (e: any) {
    statusErr.value = e?.response?.data?.message || e?.response?.data?.detail || e?.message || '加载 Portal 状态失败';
    ElMessage.error(statusErr.value);
  } finally {
    statusLoading.value = false;
  }
}

async function logoutClient(row: PortalClient) {
  try {
    await ElMessageBox.confirm(`确认登出客户端 ${row.mac}（${row.ip}）？`, '登出确认', { type: 'warning' });
  } catch {
    return;
  }
  logoutBusy.value = row.mac;
  try {
    const res = await api.post('/api/v1/portal/logout', { mac: row.mac });
    logoutResult.value = res.data || null;
    ElMessage.success(`已登出 ${row.mac}（revoked=${res.data?.revoked ?? 0}）`);
    await loadPortalStatus();
    await loadClients();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.response?.data?.detail || e?.message || '登出失败');
  } finally {
    logoutBusy.value = '';
  }
}

onMounted(() => {
  loadConfig();
  loadClients();
  loadPortalStatus();
});
</script>

<style scoped>
.nft-out {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
}
</style>