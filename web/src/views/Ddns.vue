<template>
  <div>

    <!-- DDNS 服务配置 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between">
          <span>DDNS 服务配置</span>
          <el-button size="small" :loading="configSaving" @click="saveConfig">保存</el-button>
        </div>
      </template>
      <el-alert v-if="configDetail" type="error" :closable="false" show-icon :title="configDetail" style="margin-bottom:10px" />
      <el-form label-width="100px" label-position="left" size="small">
        <el-form-item label="服务商">
          <el-select v-model="ddnsConfig.provider" style="width:180px">
            <el-option label="Cloudflare" value="cloudflare" />
            <el-option label="AliDNS" value="alidns" />
          </el-select>
        </el-form-item>
        <el-form-item label="域名">
          <el-input v-model="ddnsConfig.domain" placeholder="example.com" />
        </el-form-item>
        <el-form-item label="子域名">
          <el-input v-model="ddnsConfig.subdomain" placeholder="@" />
        </el-form-item>
        <el-form-item label="Token">
          <el-input v-model="ddnsConfig.token" type="password" show-password
            :placeholder="ddnsConfig.token ? '已设置（掩码，留空不修改）' : 'API Token'"
            @input="onConfigTokenInput" />
          <span class="hint">GET 返回掩码；仅在手动改动时提交，未改动时后端保留旧值</span>
        </el-form-item>
        <el-form-item label="Record ID">
          <el-input v-model="ddnsConfig.record_id" placeholder="可选" />
        </el-form-item>
        <el-form-item label="Zone ID">
          <el-input v-model="ddnsConfig.zone_id" placeholder="可选" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="ddnsConfig.enabled" />
        </el-form-item>
      </el-form>
    </el-card>

    <!-- DDNS 状态 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between">
          <span>DDNS 状态</span>
          <div style="display:flex; align-items:center; gap:6px">
            <el-button size="small" :loading="checking" @click="checkIP">检测公网 IP</el-button>
            <el-button type="primary" size="small" :loading="updating" @click="updateDDNS">立即更新</el-button>
            <el-button size="small" :loading="statusLoading" @click="refreshStatus">刷新状态</el-button>
          </div>
        </div>
      </template>
      <el-alert v-if="statusDetail" type="error" :closable="false" show-icon :title="statusDetail" style="margin-bottom:10px" />
      <el-form label-width="100px" label-position="left" size="small">
        <el-form-item label="服务商">{{ ddnsStatus.provider || '-' }}</el-form-item>
        <el-form-item label="公网 IP">{{ ddnsStatus.ip || '-' }}</el-form-item>
        <el-form-item label="状态">
          <el-tag :type="ddnsStatus.ok ? 'success' : 'warning'" size="small">
            {{ ddnsStatus.status || ddnsStatus.detail || '-' }}
          </el-tag>
        </el-form-item>
        <el-form-item label="详情">{{ ddnsStatus.detail || '-' }}</el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>动态域名 DDNS</span>
          <div>
            <el-button size="small" :loading="checkingDDNS" @click="checkDDNS">检查 DDNS</el-button>
            <el-button size="small" :loading="checking" @click="checkIP">重新检测</el-button>
            <el-button type="primary" size="small" @click="openNew">新增记录</el-button>
          </div>
        </div>
      </template>

      <!-- 当前公网 IP 卡片 -->
      <el-card shadow="never" style="margin-bottom:16px; background:#f5f7fa">
        <div style="display:flex; align-items:center; gap:12px; flex-wrap:wrap">
          <span style="font-weight:600">当前公网 IP：</span>
          <el-alert v-if="checkAvailable === false" type="warning" show-icon :closable="false"
            :title="checkReason || '检测失败'" style="flex:1; min-width:240px" />
          <template v-else-if="checkAvailable === true">
            <span style="font-size:18px; font-weight:600; color:#409eff">{{ currentIP }}</span>
            <el-tag size="small" type="info">来源：{{ source }}</el-tag>
            <el-tag size="small" :type="needUpdate ? 'warning' : 'success'">
              {{ needUpdate ? '需要更新' : '已是最新' }}
            </el-tag>
            <span v-if="note" style="color:#909399; font-size:12px">{{ note }}</span>
          </template>
          <span v-else style="color:#909399">检测中…</span>
        </div>
      </el-card>

      <!-- DDNS 检查结果 -->
      <el-card shadow="never" style="margin-bottom:16px; background:#f5f7fa" v-if="checkResult && checkResult.records && checkResult.records.length > 0">
        <div style="display:flex; align-items:center; gap:12px; flex-wrap:wrap">
          <span style="font-weight:600">DDNS 检查结果：</span>
          <el-tag :type="checkResult.need_update ? 'warning' : 'success'" size="small">
            {{ checkResult.need_update ? '有记录需要更新' : '所有记录IP一致' }}
          </el-tag>
          <span v-if="checkResult.current_ip" style="color:#409eff">当前公网 IP：{{ checkResult.current_ip }}</span>
          <span v-if="checkResult.note" style="color:#909399; font-size:12px; margin-left:8px">{{ checkResult.note }}</span>
        </div>
        <el-table :data="checkResult.records" size="small" style="width:100%; margin-top:8px">
          <el-table-column prop="domain" label="域名" />
          <el-table-column prop="last_ip" label="记录 IP" width="140" />
          <el-table-column label="是否需要更新" width="120">
            <template #default="{ row }">
              <el-tag :type="row.needs_update ? 'danger' : 'success'" size="small">
                {{ row.needs_update ? '需要更新' : '一致' }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-table :data="records" v-loading="loading" size="small" style="width:100%" empty-text="还没有 DDNS 记录">
        <el-table-column label="域名" min-width="180">
          <template #default="{ row }">
            {{ row.subdomain ? row.subdomain + '.' + row.domain : row.domain }}
          </template>
        </el-table-column>
        <el-table-column label="服务商" width="120">
          <template #default="{ row }">{{ providerName(row.provider) }}</template>
        </el-table-column>
        <el-table-column prop="last_ip" label="上次 IP" width="130">
          <template #default="{ row }">{{ row.last_ip || '-' }}</template>
        </el-table-column>
        <el-table-column label="更新时间" width="130">
          <template #default="{ row }">{{ row.last_ok ? formatTs(row.last_ok) : '从未' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
              {{ row.enabled ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最近错误" min-width="160">
          <template #default="{ row }">
            <el-tooltip v-if="row.last_error" :content="row.last_error" placement="top">
              <span style="color:#f56c6c; cursor:help">有错误</span>
            </el-tooltip>
            <span v-else style="color:#909399">-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ $index, row }">
            <el-button link size="small" type="primary" @click="editRow($index)">编辑</el-button>
            <el-button link size="small" type="danger" @click="removeRow($index, row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div style="margin-top:12px; text-align:right">
        <el-button type="primary" :loading="saving" @click="saveAll">保存全部</el-button>
      </div>
    </el-card>

    <el-dialog v-model="dialog" :title="editing ? '编辑 DDNS 记录' : '新增 DDNS 记录'" width="560px">
      <el-form :model="form" label-width="100px" size="small">
        <el-form-item label="服务商">
          <el-select v-model="form.provider" style="width:100%" @change="onProviderChange">
            <el-option v-for="p in providers" :key="p.key" :label="p.name" :value="p.key" />
          </el-select>
        </el-form-item>
        <el-form-item label="域名">
          <el-input v-model="form.domain" placeholder="example.com" />
        </el-form-item>
        <el-form-item label="子域名">
          <el-input v-model="form.subdomain" placeholder="留空表示 @" />
        </el-form-item>
        <el-form-item v-if="needsToken" label="Token">
          <el-input v-model="form.token" type="password" show-password
            :placeholder="editing ? '留空表示不修改' : 'API Token'" />
        </el-form-item>
        <el-form-item v-if="needsUser" label="用户名">
          <el-input v-model="form.username" placeholder="用户名" />
        </el-form-item>
        <el-form-item v-if="needsUser" label="密码">
          <el-input v-model="form.password" type="password" show-password
            :placeholder="editing ? '留空表示不修改' : '密码'" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button size="small" @click="dialog = false">取消</el-button>
        <el-button type="primary" size="small" @click="saveForm">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

interface Record {
  id: string; provider: string; domain: string; subdomain: string;
  token: string; username: string; password: string; enabled: boolean;
  last_ip: string; last_ok: number; last_error: string;
}
interface Provider { key: string; name: string; needs_token: boolean; needs_user: boolean; endpoint: string }
interface DdnsConfig {
  provider: string; domain: string; subdomain: string; token: string;
  record_id: string; zone_id: string; enabled: boolean;
}
interface DdnsStatus {
  status: string; ip: string; provider: string; detail: string; ok: boolean;
}

const loading = ref(false);
const saving = ref(false);
const checking = ref(false);
const checkingDDNS = ref(false);
const dialog = ref(false);
const editing = ref(false);
const editIndex = ref(-1);

const records = ref<Record[]>([]);
const providers = ref<Provider[]>([]);

const checkAvailable = ref<boolean | null>(null);
const currentIP = ref('');
const source = ref('');
const needUpdate = ref(false);
const note = ref('');
const checkReason = ref('');
const checkResult = ref<any>({ need_update: false, current_ip: '', records: [] as any[], note: '' });

const ddnsConfig = ref<DdnsConfig>({ provider: '', domain: '', subdomain: '', token: '', record_id: '', zone_id: '', enabled: true });
const ddnsStatus = ref<DdnsStatus>({ status: '', ip: '', provider: '', detail: '', ok: false });
const configDetail = ref('');
const statusDetail = ref('');
const configSaving = ref(false);
const statusLoading = ref(false);
const updating = ref(false);
const configTokenDirty = ref(false);

const form = ref<any>({
  id: '', provider: '', domain: '', subdomain: '',
  token: '', username: '', password: '', enabled: true,
});

const needsToken = computed(() => {
  const p = providers.value.find(x => x.key === form.value.provider);
  return !!p?.needs_token;
});
const needsUser = computed(() => {
  const p = providers.value.find(x => x.key === form.value.provider);
  return !!p?.needs_user;
});

function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

function providerName(key: string) {
  return providers.value.find(p => p.key === key)?.name || key || '-';
}

function formatTs(ts: any) {
  if (!ts) return '从未';
  return new Date(Number(ts) * 1000).toLocaleString();
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/ddns/records');
    const d = res.data || {};
    records.value = d.records || [];
    providers.value = d.providers || [];
    if (!form.value.provider && providers.value.length) {
      form.value.provider = providers.value[0].key;
    }
  } catch (e: any) {
    showError(e, '加载 DDNS 记录失败');
  } finally {
    loading.value = false;
  }
}

async function checkIP() {
  checking.value = true;
  checkAvailable.value = null;
  try {
    const res = await api.get('/api/v1/ddns/ip');
    const d = res.data || {};
    const ip = d.ip || '';
    currentIP.value = ip;
    source.value = d.source || 'ddns/ip';
    checkAvailable.value = !!ip;
    needUpdate.value = records.value.some(r => r.enabled && r.last_ip && r.last_ip !== ip);
    note.value = ip ? `当前公网 IP：${ip}` : '未获取到公网 IP';
  } catch (e: any) {
    checkAvailable.value = false;
    checkReason.value = e?.response?.data?.detail || e?.response?.data?.message || e?.message || '检测失败';
  } finally {
    checking.value = false;
  }
}

async function checkDDNS() {
  checkingDDNS.value = true;
  checkResult.value = { need_update: false, current_ip: '', records: [], note: '' };
  try {
    const res = await api.get('/api/v1/ddns/check');
    const d = res.data || {};
    if (!d.available) {
      checkResult.value = {
        need_update: false,
        current_ip: '',
        records: [],
        note: d.reason || d.detail || '检查不可用',
      };
      ElMessage.warning(checkResult.value.note);
      return;
    }
    checkResult.value = {
      need_update: !!d.need_update,
      current_ip: d.current_ip || '',
      records: d.records || [],
      note: d.note || '',
    };
    if (d.need_update) {
      ElMessage.warning('有 DDNS 记录的 IP 已变化，需要更新');
    } else {
      ElMessage.success('所有 DDNS 记录 IP 一致，无需更新');
    }
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e?.message || 'DDNS 检查失败';
    ElMessage.error(msg);
  } finally {
    checkingDDNS.value = false;
  }
}

function openNew() {
  editing.value = false;
  editIndex.value = -1;
  form.value = {
    id: 'r' + Date.now(), provider: providers.value[0]?.key || '',
    domain: '', subdomain: '', token: '', username: '', password: '', enabled: true,
  };
  dialog.value = true;
}

function editRow(idx: number) {
  editing.value = true;
  editIndex.value = idx;
  const r = records.value[idx];
  form.value = {
    id: r.id, provider: r.provider, domain: r.domain, subdomain: r.subdomain || '',
    token: r.token || '', username: r.username || '', password: r.password || '', enabled: r.enabled,
  };
  dialog.value = true;
}

function onProviderChange() {
  // keep form fields; dynamic show handled by computed
}

function saveForm() {
  if (!form.value.domain.trim()) {
    ElMessage.warning('域名不能为空');
    return;
  }
  const rec: any = {
    id: form.value.id,
    provider: form.value.provider,
    domain: form.value.domain.trim(),
    subdomain: form.value.subdomain.trim(),
    token: form.value.token,
    username: form.value.username,
    password: form.value.password,
    enabled: form.value.enabled,
  };
  if (editing.value && editIndex.value >= 0) {
    records.value[editIndex.value] = { ...records.value[editIndex.value], ...rec };
  } else {
    records.value.push(rec as any);
  }
  dialog.value = false;
  ElMessage.success('已加入列表，点击「保存全部」提交');
}

async function saveAll() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/ddns/records', { records: records.value });
    const d = res.data || {};
    records.value = d.records || records.value;
    ElMessage.success(`已保存 ${d.count ?? records.value.length} 条`);
    await checkIP();
  } catch (e: any) {
    showError(e, '保存失败');
  } finally {
    saving.value = false;
  }
}

async function removeRow(idx: number, row: Record) {
  try {
    await ElMessageBox.confirm(`确认删除记录 ${row.subdomain ? row.subdomain + '.' : ''}${row.domain}？`, '确认', {
      confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
    });
  } catch {
    return;
  }
  try {
    const res = await api.delete(`/api/v1/ddns/records/${encodeURIComponent(row.id)}`);
    const d = res.data || {};
    if (d.deleted) {
      records.value.splice(idx, 1);
      ElMessage.success('记录已删除');
    } else {
      ElMessage.warning('记录不存在，可能已被删除');
    }
    await load();
  } catch (e: any) {
    showError(e, '删除记录失败');
  }
}

function onConfigTokenInput() {
  configTokenDirty.value = true;
}

function buildConfigBody() {
  const body: any = {
    provider: ddnsConfig.value.provider,
    domain: ddnsConfig.value.domain,
    subdomain: ddnsConfig.value.subdomain,
    record_id: ddnsConfig.value.record_id,
    zone_id: ddnsConfig.value.zone_id,
    enabled: ddnsConfig.value.enabled,
  };
  // GET 返回掩码；仅在用户手动改过 token 时提交，未改动时省略，由后端保留旧值
  if (configTokenDirty.value && ddnsConfig.value.token) {
    body.token = ddnsConfig.value.token;
  }
  return body;
}

async function loadConfig() {
  try {
    const res = await api.get('/api/v1/ddns/config');
    const c = res.data?.config || {};
    ddnsConfig.value = { provider: '', domain: '', subdomain: '', token: '', record_id: '', zone_id: '', enabled: true, ...c };
    configTokenDirty.value = false;
  } catch (e: any) {
    showError(e, '加载 DDNS 配置失败');
  }
}

async function saveConfig() {
  configSaving.value = true;
  configDetail.value = '';
  try {
    const res = await api.post('/api/v1/ddns/config', buildConfigBody());
    const d = res.data || {};
    if (d.saved) ElMessage.success('DDNS 配置已保存');
    const c = d.config || {};
    ddnsConfig.value = { ...ddnsConfig.value, ...c };
    configTokenDirty.value = false;
  } catch (e: any) {
    showError(e, '保存 DDNS 配置失败');
  } finally {
    configSaving.value = false;
  }
}

async function refreshStatus() {
  statusLoading.value = true;
  statusDetail.value = '';
  try {
    const res = await api.get('/api/v1/ddns/status');
    const d = res.data || {};
    ddnsStatus.value = { status: '', ip: '', provider: '', detail: '', ok: false, ...d };
  } catch (e: any) {
    showError(e, '获取 DDNS 状态失败');
  } finally {
    statusLoading.value = false;
  }
}

async function updateDDNS() {
  updating.value = true;
  statusDetail.value = '';
  try {
    const res = await api.post('/api/v1/ddns/update');
    const d = res.data || {};
    ddnsStatus.value = { ...ddnsStatus.value, ok: !!d.ok, status: d.ok ? 'updated' : 'failed', detail: d.detail || '', ip: d.ip || '', provider: d.provider || '' };
    ElMessage.success(d.detail || 'DDNS 更新完成');
    await refreshStatus();
    await load();
  } catch (e: any) {
    showError(e, 'DDNS 立即更新失败');
  } finally {
    updating.value = false;
  }
}

onMounted(() => { load(); checkIP(); loadConfig(); refreshStatus(); });
</script>
