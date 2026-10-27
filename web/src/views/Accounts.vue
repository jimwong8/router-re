<template>
  <div>
    <el-tabs v-model="tab">
      <el-tab-pane label="账号" name="accounts">
        <!-- 统计卡片 -->
        <el-row :gutter="12" style="margin-bottom:16px">
          <el-col :span="6">
            <el-card shadow="hover"><div class="stat-num">{{ summary.total_accounts }}</div><div class="stat-label">账号总数</div></el-card>
          </el-col>
          <el-col :span="6">
            <el-card shadow="hover"><div class="stat-num" style="color:#3fb950">{{ summary.online }}</div><div class="stat-label">在线</div></el-card>
          </el-col>
          <el-col :span="6">
            <el-card shadow="hover"><div class="stat-num" style="color:#f85149">{{ summary.expired }}</div><div class="stat-label">已过期</div></el-card>
          </el-col>
          <el-col :span="6">
            <el-card shadow="hover"><div class="stat-num" style="color:#d29922">{{ summary.over_quota }}</div><div class="stat-label">超额</div></el-card>
          </el-col>
        </el-row>

        <!-- 新建表单 -->
        <el-card shadow="never" style="margin-bottom:16px">
          <template #header>新建账号</template>
          <el-form inline>
            <el-form-item label="用户名"><el-input v-model="form.username" placeholder="3-32位" style="width:150px" /></el-form-item>
            <el-form-item label="密码"><el-input v-model="form.password" placeholder="6-64位" style="width:150px" /></el-form-item>
            <el-form-item label="类型">
              <el-select v-model="form.type" style="width:120px">
                <el-option label="PPPoE" value="pppoe" />
                <el-option label="Portal" value="portal" />
              </el-select>
            </el-form-item>
            <el-form-item label="套餐"><el-input v-model="form.profile" placeholder="默认" style="width:120px" /></el-form-item>
            <el-form-item label="配额MB"><el-input-number v-model="form.quota_mb" :min="0" /></el-form-item>
            <el-form-item label="有效天数"><el-input-number v-model="form.expire_days" :min="0" /></el-form-item>
            <el-form-item><el-button type="primary" :loading="saving" @click="createAccount">创建</el-button></el-form-item>
          </el-form>
        </el-card>

        <!-- 批量导入 -->
        <el-card shadow="never" style="margin-bottom:16px">
          <template #header>批量导入（CSV，每行一条）</template>
          <div style="margin-bottom:8px; color:#666; font-size:13px">
            格式：<code>username,password,type,profile,quota_mb,expire_days</code>
            （type 为 pppoe 或 portal；quota_mb 为 0 表示不限；expire_days 为 0 表示永不过期；以 # 开头为注释）
          </div>
          <el-input v-model="importText" type="textarea" :rows="5" placeholder="user1,pass123,pppoe,basic,1024,30" />
          <div style="margin-top:8px">
            <el-button type="primary" :loading="importing" @click="doImport">导入</el-button>
            <span v-if="importResult" style="margin-left:12px; color:#3fb950">
              成功 {{ importResult.created }} 条<template v-if="importResult.errors && importResult.errors.length">，失败 {{ importResult.errors.length }} 条</template>
            </span>
          </div>
          <el-alert v-if="importErrors.length" type="error" :closable="false" style="margin-top:8px">
            <div v-for="(e, i) in importErrors" :key="i">{{ e }}</div>
          </el-alert>
        </el-card>

        <!-- 账号表格 -->
        <el-card shadow="never">
          <template #header>
            <div style="display:flex; justify-content:space-between; align-items:center">
              <span>账号列表（{{ accounts.length }}）</span>
              <el-button size="small" :loading="loading" @click="loadAll">刷新</el-button>
            </div>
          </template>
          <el-table :data="accounts" border style="width:100%">
            <el-table-column prop="username" label="用户名" width="150" />
            <el-table-column prop="type" label="类型" width="90" />
            <el-table-column prop="profile" label="套餐" width="110">
              <template #default="{ row }">{{ row.profile || '默认' }}</template>
            </el-table-column>
            <el-table-column label="已用配额" width="130">
              <template #default="{ row }">
                <span v-if="row.quota_mb > 0">{{ row.used_mb }} / {{ row.quota_mb }} MB</span>
                <span v-else>{{ row.used_mb }} MB（不限）</span>
              </template>
            </el-table-column>
            <el-table-column label="到期" width="170">
              <template #default="{ row }">
                {{ row.expire_at ? new Date(row.expire_at * 1000).toLocaleString() : '永不过期' }}
              </template>
            </el-table-column>
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <el-tag :type="statusType(statusOf(row))">{{ statusText(statusOf(row)) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="在线" width="80">
              <template #default="{ row }">
                <el-tag :type="row.online ? 'success' : 'info'" size="small">{{ row.online ? '在线' : '离线' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="250">
              <template #default="{ row }">
                <el-button size="small" @click="resetUsage(row)">重置用量</el-button>
                <el-button size="small" @click="bindMac(row)">绑定MAC</el-button>
                <el-button type="danger" size="small" @click="removeAccount(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="套餐计费" name="billing">
        <el-card shadow="never" style="margin-bottom:16px">
          <template #header>新增套餐</template>
          <el-form inline>
            <el-form-item label="名称"><el-input v-model="planForm.name" placeholder="basic" style="width:130px" /></el-form-item>
            <el-form-item label="价格(元)"><el-input-number v-model="planForm.price_yuan" :min="0" :precision="2" /></el-form-item>
            <el-form-item label="天数"><el-input-number v-model="planForm.days" :min="0" /></el-form-item>
            <el-form-item label="配额MB"><el-input-number v-model="planForm.quota_mb" :min="0" /></el-form-item>
            <el-form-item><el-button type="primary" :loading="planSaving" @click="savePlans">保存套餐</el-button></el-form-item>
          </el-form>
        </el-card>

        <el-card shadow="never" style="margin-bottom:16px">
          <template #header>套餐列表（{{ plans.length }}）</template>
          <el-table :data="plans" border style="width:100%">
            <el-table-column prop="name" label="名称" />
            <el-table-column prop="price_yuan" label="价格(元)" width="120" />
            <el-table-column prop="days" label="天数" width="100" />
            <el-table-column prop="quota_mb" label="配额MB" width="110" />
          </el-table>
        </el-card>

        <el-card shadow="never">
          <template #header>收入汇总</template>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="账号总数">{{ summary.total_accounts }}</el-descriptions-item>
            <el-descriptions-item label="在线">{{ summary.online }}</el-descriptions-item>
            <el-descriptions-item label="已过期">{{ summary.expired }}</el-descriptions-item>
            <el-descriptions-item label="超额">{{ summary.over_quota }}</el-descriptions-item>
            <el-descriptions-item label="预计收入">{{ summary.revenue.toFixed(2) }} 元</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

interface Account {
  username: string;
  type: string;
  profile: string;
  quota_mb: number;
  used_mb: number;
  expire_at: number;
  enabled: boolean;
  created_at: number;
  online: boolean;
  bound_mac: string;
}

interface Plan {
  name: string;
  price_yuan: number;
  days: number;
  quota_mb: number;
}

const tab = ref('accounts');
const accounts = ref<Account[]>([]);
const plans = ref<Plan[]>([]);
const loading = ref(false);
const saving = ref(false);
const importing = ref(false);
const planSaving = ref(false);
const importText = ref('');
const importResult = ref<{ created: number; errors: string[] } | null>(null);
const importErrors = ref<string[]>([]);

const summary = reactive({
  total_accounts: 0,
  online: 0,
  expired: 0,
  over_quota: 0,
  revenue: 0,
});

const form = reactive({
  username: '',
  password: '',
  type: 'pppoe',
  profile: '',
  quota_mb: 0,
  expire_days: 0,
});

const planForm = reactive<Plan>({ name: '', price_yuan: 0, days: 30, quota_mb: 0 });

function statusOf(row: Account): string {
  const now = Math.floor(Date.now() / 1000);
  if (!row.enabled) return 'disabled';
  if (row.expire_at > 0 && now > row.expire_at) return 'expired';
  if (row.quota_mb > 0 && row.used_mb >= row.quota_mb) return 'over_quota';
  if (row.online) return 'online';
  return 'ok';
}

function statusText(s: string): string {
  const map: Record<string, string> = {
    disabled: '已禁用',
    expired: '已过期',
    over_quota: '超额',
    online: '在线',
    ok: '正常',
  };
  return map[s] || s;
}

function statusType(s: string): string {
  const map: Record<string, string> = {
    disabled: 'info',
    expired: 'danger',
    over_quota: 'warning',
    online: 'success',
    ok: 'success',
  };
  return map[s] || 'info';
}

async function loadAccounts() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/accounts');
    accounts.value = res.data?.accounts || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}

async function loadPlans() {
  try {
    const res = await api.get('/api/v1/billing/plans');
    plans.value = res.data?.plans || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function loadSummary() {
  try {
    const res = await api.get('/api/v1/billing/summary');
    Object.assign(summary, res.data || {});
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function loadAll() {
  await Promise.all([loadAccounts(), loadPlans(), loadSummary()]);
}

async function createAccount() {
  if (!form.username || !form.password) {
    ElMessage.warning('用户名和密码必填');
    return;
  }
  saving.value = true;
  try {
    const payload: any = {
      username: form.username,
      password: form.password,
      type: form.type,
      profile: form.profile,
      quota_mb: form.quota_mb,
      enabled: true,
    };
    if (form.expire_days > 0) {
      payload.expire_at = Math.floor(Date.now() / 1000) + form.expire_days * 86400;
    }
    await api.post('/api/v1/accounts', payload);
    ElMessage.success('账号已创建');
    form.username = '';
    form.password = '';
    form.profile = '';
    form.quota_mb = 0;
    form.expire_days = 0;
    await loadAll();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

async function doImport() {
  const lines = importText.value.split(/\r?\n/);
  if (!importText.value.trim()) {
    ElMessage.warning('请输入 CSV 行');
    return;
  }
  importing.value = true;
  importErrors.value = [];
  importResult.value = null;
  try {
    const res = await api.post('/api/v1/accounts/import', { lines });
    importResult.value = { created: res.data?.created || 0, errors: res.data?.errors || [] };
    importErrors.value = importResult.value.errors;
    if (importResult.value.created > 0) {
      ElMessage.success(`导入成功 ${importResult.value.created} 条`);
    } else {
      ElMessage.warning('没有账号被导入');
    }
    await loadAll();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    importing.value = false;
  }
}

async function resetUsage(row: Account) {
  try {
    await api.post(`/api/v1/accounts/${encodeURIComponent(row.username)}/reset-usage`);
    ElMessage.success('用量已重置');
    await loadAll();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function bindMac(row: Account) {
  try {
    const { value } = await ElMessageBox.prompt('输入要绑定的 MAC 地址', '绑定 MAC', {
      inputPattern: /^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$/,
      inputErrorMessage: 'MAC 格式不正确',
      confirmButtonText: '绑定',
      cancelButtonText: '取消',
    });
    await api.post(`/api/v1/accounts/${encodeURIComponent(row.username)}/bind-mac`, { mac: value });
    ElMessage.success('MAC 已绑定');
    await loadAll();
  } catch (e: any) {
    if (e === 'cancel' || e?.action === 'cancel') return;
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function removeAccount(row: Account) {
  try {
    await ElMessageBox.confirm(`确认删除账号 ${row.username}?`, '删除账号', { type: 'warning' });
  } catch {
    return;
  }
  try {
    await api.delete(`/api/v1/accounts/${encodeURIComponent(row.username)}`);
    ElMessage.success('账号已删除');
    await loadAll();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function savePlans() {
  if (!planForm.name) {
    ElMessage.warning('套餐名称必填');
    return;
  }
  planSaving.value = true;
  try {
    const next = [...plans.value, { ...planForm }];
    await api.post('/api/v1/billing/plans', { plans: next });
    ElMessage.success('套餐已保存');
    planForm.name = '';
    planForm.price_yuan = 0;
    planForm.days = 30;
    planForm.quota_mb = 0;
    await loadPlans();
    await loadSummary();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    planSaving.value = false;
  }
}

onMounted(() => {
  loadAll();
});
</script>

<style scoped>
.stat-num {
  font-size: 26px;
  font-weight: 600;
  line-height: 1.2;
}
.stat-label {
  color: #666;
  font-size: 13px;
  margin-top: 4px;
}
</style>
