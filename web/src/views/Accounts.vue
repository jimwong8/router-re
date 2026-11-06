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

      <el-tab-pane label="流量计费/账单" name="accounting">
        <el-card shadow="never" style="margin-bottom:16px">
          <template #header>计费状态</template>
          <el-alert v-if="acctError" type="error" :closable="false" show-icon :title="acctError" style="margin-bottom:10px" />
          <el-descriptions :column="2" border>
            <el-descriptions-item label="连接计费">
              <el-tag :type="acctStatus.acct_enabled ? 'success' : 'danger'">
                {{ acctStatus.acct_enabled ? '已启用' : '未启用' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="数据可用">
              <el-tag :type="acctStatus.available ? 'success' : 'warning'">
                {{ acctStatus.available ? '可用' : '不可用' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="连接总数">{{ acctStatus.conntrack_total }}</el-descriptions-item>
            <el-descriptions-item label="带字节字段">{{ acctStatus.entries_with_bytes }}</el-descriptions-item>
          </el-descriptions>
          <div v-if="acctStatus.reason" style="margin-top:8px; color:#f56c6c; font-size:13px">{{ acctStatus.reason }}</div>
        </el-card>

        <el-card shadow="never" style="margin-bottom:16px">
          <template #header>用量报表</template>
          <el-table :data="acctReport" border style="width:100%">
            <el-table-column prop="username" label="账号" width="150" />
            <el-table-column prop="used_mb" label="已用(MB)" width="110" />
            <el-table-column prop="quota_mb" label="配额(MB)" width="110">
              <template #default="{ row }">{{ row.quota_mb > 0 ? row.quota_mb : '不限' }}</template>
            </el-table-column>
            <el-table-column prop="percent" label="使用率" width="100">
              <template #default="{ row }">{{ row.percent }}%</template>
            </el-table-column>
            <el-table-column label="超量" width="90">
              <template #default="{ row }">
                <el-tag :type="row.exceeded ? 'danger' : 'success'" size="small">
                  {{ row.exceeded ? '超额' : '正常' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="last_seen" label="最近活跃" width="170">
              <template #default="{ row }">
                {{ row.last_seen ? new Date(row.last_seen * 1000).toLocaleString() : '-' }}
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!acctReport.length" description="暂无用量数据" :image-size="60" />
        </el-card>

        <el-card shadow="never" style="margin-bottom:16px">
          <template #header>账单管理</template>
          <div style="display:flex; gap:8px; margin-bottom:12px; align-items:center">
            <el-button type="primary" :loading="generatingBills" @click="generateBills">生成账单</el-button>
            <el-input-number v-model="periodDays" :min="1" :max="365" :step="1" style="width:120px" />
            <span style="color:#666; font-size:13px">周期天数</span>
          </div>
          <el-alert v-if="billGenError" type="error" :closable="false" show-icon :title="billGenError" style="margin-bottom:10px" />
          <el-alert v-if="skippedBills.length" type="warning" :closable="false" show-icon style="margin-bottom:10px">
            <div v-for="(s, i) in skippedBills" :key="i">{{ s }}</div>
          </el-alert>
          <el-table :data="bills" border style="width:100%">
            <el-table-column prop="id" label="账单ID" width="200" />
            <el-table-column prop="username" label="账号" width="120" />
            <el-table-column prop="plan_id" label="套餐" width="100" />
            <el-table-column prop="amount" label="金额(元)" width="100">
              <template #default="{ row }">{{ row.amount.toFixed(2) }}</template>
            </el-table-column>
            <el-table-column prop="used_mb" label="已用(MB)" width="100" />
            <el-table-column prop="quota_mb" label="配额(MB)" width="100">
              <template #default="{ row }">{{ row.quota_mb > 0 ? row.quota_mb : '不限' }}</template>
            </el-table-column>
            <el-table-column prop="status" label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="row.status === 'paid' ? 'success' : row.status === 'overdue' ? 'danger' : row.status === 'waived' ? 'info' : 'warning'" size="small">
                  {{ row.status }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="创建时间" width="170">
              <template #default="{ row }">
                {{ row.created_at ? new Date(row.created_at * 1000).toLocaleString() : '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="period_start" label="周期开始" width="170">
              <template #default="{ row }">
                {{ row.period_start ? new Date(row.period_start * 1000).toLocaleString() : '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="period_end" label="周期结束" width="170">
              <template #default="{ row }">
                {{ row.period_end ? new Date(row.period_end * 1000).toLocaleString() : '-' }}
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!bills.length" description="暂无账单" :image-size="60" />
        </el-card>
      </el-tab-pane>

      <!-- 登录方式 -->
      <el-tab-pane label="登录方式" name="login">
        <!-- SMS 卡片 -->
        <el-card shadow="never" style="margin-bottom:16px">
          <template #header>
            <div style="display:flex; justify-content:space-between; align-items:center">
              <span>短信验证码登录（SMS）</span>
              <el-tag :type="smsConfigured ? 'success' : 'info'" size="small">
                {{ smsConfigured ? '已配置' : '未配置' }}
              </el-tag>
            </div>
          </template>
          <el-alert v-if="smsError" type="error" :closable="false" show-icon :title="smsError" style="margin-bottom:10px" />
          <el-form label-width="110px" label-position="left" size="small">
            <el-form-item label="网关 URL">
              <el-input v-model="smsConfig.url" placeholder="https://sms.example.com/send" />
            </el-form-item>
            <el-form-item label="请求方法">
              <el-select v-model="smsConfig.method" style="width:120px">
                <el-option label="POST" value="POST" />
                <el-option label="GET" value="GET" />
              </el-select>
            </el-form-item>
            <el-form-item label="正文模板">
              <el-input v-model="smsConfig.body_template" type="textarea" :rows="3"
                placeholder="您的验证码是 {{code}}，5分钟内有效" />
            </el-form-item>
            <el-form-item label="HTTP Headers">
              <el-table :data="smsHeaders" border size="small" style="width:100%; margin-top:6px">
                <el-table-column prop="key" label="Key" width="160" />
                <el-table-column prop="value" label="Value" />
                <el-table-column label="操作" width="80">
                  <template #default="{ row }">
                    <el-button size="small" type="danger" @click="removeHeader(row)">删</el-button>
                  </template>
                </el-table-column>
              </el-table>
              <div style="display:flex; gap:6px; margin-top:6px">
                <el-input v-model="newHeaderKey" placeholder="key" style="width:120px" />
                <el-input v-model="newHeaderValue" placeholder="value" style="width:160px" />
                <el-button size="small" @click="addHeader">+</el-button>
              </div>
            </el-form-item>
          </el-form>
          <div style="display:flex; gap:8px; margin-top:12px">
            <el-button type="primary" :loading="smsSaving" @click="saveSmsConfig">保存配置</el-button>
            <el-button :loading="smsTesting" @click="testSms">测试发送</el-button>
          </div>
          <el-card shadow="never" style="margin-top:12px">
            <template #header>验证码校验测试</template>
            <el-form inline style="margin-bottom:8px">
              <el-form-item label="手机号">
                <el-input v-model="smsTestPhone" placeholder="13800138000" style="width:160px" />
              </el-form-item>
              <el-form-item label="验证码">
                <el-input v-model="smsTestCode" placeholder="6位码" style="width:120px" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" :loading="smsVerifying" @click="verifySms">校验验证码</el-button>
              </el-form-item>
            </el-form>
          </el-card>
        </el-card>

        <!-- 微信卡片 -->
        <el-card shadow="never">
          <template #header>
            <div style="display:flex; justify-content:space-between; align-items:center">
              <span>微信登录</span>
              <el-tag :type="wechatConfigured ? 'success' : 'info'" size="small">
                {{ wechatConfigured ? '已配置' : '未配置' }}
              </el-tag>
            </div>
          </template>
          <el-alert v-if="wechatError" type="error" :closable="false" show-icon :title="wechatError" style="margin-bottom:10px" />
          <el-form label-width="110px" label-position="left" size="small">
            <el-form-item label="AppID">
              <el-input v-model="wechatConfig.appid" placeholder="wx..." />
            </el-form-item>
            <el-form-item label="AppSecret">
              <el-input v-model="wechatConfig.appsecret" type="password" show-password
                :placeholder="wechatSecretSet ? '已设置（掩码，留空不修改）' : 'AppSecret'"
                @input="onWechatSecretInput" />
              <span class="hint" style="color:#909399">GET 返回掩码；仅在手动改动时提交，未改动时后端保留旧值</span>
            </el-form-item>
          </el-form>
          <div style="display:flex; gap:8px; margin-top:12px">
            <el-button type="primary" :loading="wechatSaving" @click="saveWechatConfig">保存配置</el-button>
            <el-button :loading="wechatLogining" @click="wechatLogin">测试登录</el-button>
          </div>
          <el-card shadow="never" style="margin-top:12px">
            <template #header>Code 换 OpenID 测试</template>
            <el-form inline style="margin-bottom:8px">
              <el-form-item label="微信 code">
                <el-input v-model="wechatTestCode" placeholder="前端页面换取的 code" style="width:260px" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" :loading="wechatVerifying" @click="verifyWechat">换取 openid</el-button>
              </el-form-item>
            </el-form>
          </el-card>
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

interface HeaderKV { key: string; value: string }

interface SmsConfig {
  url: string;
  method: string;
  body_template: string;
  headers: Record<string, string>;
}

interface WechatConfig {
  appid: string;
  appsecret: string;
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

// ---- SMS ----
const smsConfig = ref<SmsConfig>({ url: '', method: 'POST', body_template: '', headers: {} });
const smsHeaders = ref<HeaderKV[]>([]);
const newHeaderKey = ref('');
const newHeaderValue = ref('');
const smsConfigured = ref(false);
const smsError = ref('');
const smsSaving = ref(false);
const smsTesting = ref(false);
const smsVerifying = ref(false);
const smsTestPhone = ref('');
const smsTestCode = ref('');

// ---- WeChat ----
const wechatConfig = ref<WechatConfig>({ appid: '', appsecret: '' });
const wechatConfigured = ref(false);
const wechatSecretSet = ref(false);
const wechatError = ref('');
const wechatSaving = ref(false);
const wechatLogining = ref(false);
const wechatVerifying = ref(false);
const wechatTestCode = ref('');

function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

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

async function loadSmsConfig() {
  smsError.value = '';
  try {
    const res = await api.get('/api/v1/auth/sms/config');
    const d = res.data || {};
    smsConfigured.value = !!d.configured;
    if (d.configured) {
      smsConfig.value = {
        url: d.url || '',
        method: d.method || 'POST',
        body_template: d.body_template || '',
        headers: d.headers || {},
      };
      smsHeaders.value = Object.entries(d.headers || {}).map(([k, v]) => ({ key: k, value: String(v) }));
    }
  } catch (e: any) {
    showError(e, '加载 SMS 配置失败');
  }
}

function headersToMap() {
  const m: Record<string, string> = {};
  for (const h of smsHeaders.value) {
    if (h.key) m[h.key] = h.value;
  }
  return m;
}

async function saveSmsConfig() {
  smsSaving.value = true;
  smsError.value = '';
  try {
    const body = {
      url: smsConfig.value.url,
      method: smsConfig.value.method,
      body_template: smsConfig.value.body_template,
      headers: headersToMap(),
    };
    await api.post('/api/v1/auth/sms/config', body);
    smsConfigured.value = true;
    ElMessage.success('SMS 配置已保存');
    await loadSmsConfig();
  } catch (e: any) {
    showError(e, '保存 SMS 配置失败');
  } finally {
    smsSaving.value = false;
  }
}

async function addHeader() {
  if (!newHeaderKey.value) {
    ElMessage.warning('Header key 必填');
    return;
  }
  smsHeaders.value.push({ key: newHeaderKey.value, value: newHeaderValue.value });
  newHeaderKey.value = '';
  newHeaderValue.value = '';
}

function removeHeader(row: HeaderKV) {
  const i = smsHeaders.value.indexOf(row);
  if (i >= 0) smsHeaders.value.splice(i, 1);
}

async function testSms() {
  if (!smsTestPhone.value) {
    ElMessage.warning('手机号必填');
    return;
  }
  smsTesting.value = true;
  smsError.value = '';
  try {
    await api.post('/api/v1/auth/sms/send', { phone: smsTestPhone.value });
    ElMessage.success('验证码已发送');
  } catch (e: any) {
    showError(e, '发送验证码失败');
  } finally {
    smsTesting.value = false;
  }
}

async function verifySms() {
  if (!smsTestPhone.value || !smsTestCode.value) {
    ElMessage.warning('手机号和验证码必填');
    return;
  }
  smsVerifying.value = true;
  smsError.value = '';
  try {
    await api.post('/api/v1/auth/sms/verify', { phone: smsTestPhone.value, code: smsTestCode.value });
    ElMessage.success('验证码校验通过');
  } catch (e: any) {
    showError(e, '验证码校验失败');
  } finally {
    smsVerifying.value = false;
  }
}

async function loadWechatConfig() {
  wechatError.value = '';
  try {
    const res = await api.get('/api/v1/auth/wechat/config');
    const d = res.data || {};
    wechatConfigured.value = !!d.secret_set;
    wechatSecretSet.value = !!d.secret_set;
    wechatConfig.value = {
      appid: d.appid || '',
      appsecret: '',
    };
  } catch (e: any) {
    showError(e, '加载微信配置失败');
  }
}

function onWechatSecretInput() {
  // 只要用户输入，就视为已改动，提交时会带上 appsecret
}

async function saveWechatConfig() {
  wechatSaving.value = true;
  wechatError.value = '';
  try {
    // appsecret 为空时，后端会保留旧值（掩码原样提交也会被忽略）
    await api.post('/api/v1/auth/wechat/config', {
      appid: wechatConfig.value.appid,
      appsecret: wechatConfig.value.appsecret,
    });
    wechatConfigured.value = true;
    wechatSecretSet.value = true;
    ElMessage.success('微信配置已保存');
    await loadWechatConfig();
  } catch (e: any) {
    showError(e, '保存微信配置失败');
  } finally {
    wechatSaving.value = false;
  }
}

async function wechatLogin() {
  if (!wechatTestCode.value) {
    ElMessage.warning('code 必填');
    return;
  }
  wechatLogining.value = true;
  wechatVerifying.value = true;
  wechatError.value = '';
  try {
    const res = await api.post('/api/v1/auth/wechat/login', { code: wechatTestCode.value });
    const d = res.data || {};
    ElMessage.success(`登录成功，openid=${d.openid || '-'}`);
  } catch (e: any) {
    showError(e, '微信登录失败');
  } finally {
    wechatLogining.value = false;
    wechatVerifying.value = false;
  }
}

async function verifyWechat() {
  return wechatLogin();
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

const periodDays = ref(30);
const acctStatus = reactive({
  acct_enabled: false,
  conntrack_total: 0,
  entries_with_bytes: 0,
  available: false,
  reason: '',
});
const acctReport = ref<any[]>([]);
const acctError = ref('');
const bills = ref<any[]>([]);
const generatingBills = ref(false);
const billGenError = ref('');
const skippedBills = ref<string[]>([]);

async function loadAccountingStatus() {
  acctError.value = '';
  try {
    const res = await api.get('/api/v1/accounting/status');
    const d = res.data || {};
    acctStatus.acct_enabled = !!d.acct_enabled;
    acctStatus.conntrack_total = d.conntrack_total || 0;
    acctStatus.entries_with_bytes = d.entries_with_bytes || 0;
    acctStatus.available = !!d.available;
    acctStatus.reason = d.reason || '';
  } catch (e: any) {
    showError(e, '加载计费状态失败');
    acctError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
  }
}

async function loadAccountingReport() {
  try {
    const res = await api.get('/api/v1/accounting/report');
    acctReport.value = res.data?.report || [];
  } catch (e: any) {
    showError(e, '加载用量报表失败');
  }
}

async function loadBills() {
  try {
    const res = await api.get('/api/v1/billing/bills');
    bills.value = res.data?.bills || [];
  } catch (e: any) {
    showError(e, '加载账单列表失败');
  }
}

async function generateBills() {
  try {
    await ElMessageBox.confirm(`确认生成周期为 ${periodDays.value} 天的账单？`, '生成账单', { type: 'warning' });
  } catch {
    return;
  }
  generatingBills.value = true;
  billGenError.value = '';
  skippedBills.value = [];
  try {
    const res = await api.post('/api/v1/billing/bills/generate', { period_days: periodDays.value });
    const d = res.data || {};
    bills.value = d.generated || [];
    skippedBills.value = d.skipped || [];
    ElMessage.success(`已生成 ${d.count || 0} 条账单`);
    await loadBills();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(msg);
    billGenError.value = msg;
  } finally {
    generatingBills.value = false;
  }
}

async function loadAccountingAll() {
  await Promise.all([loadAccountingStatus(), loadAccountingReport(), loadBills()]);
}

onMounted(() => {
  loadAll();
  loadSmsConfig();
  loadWechatConfig();
  loadAccountingAll();
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
