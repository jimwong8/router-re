<template>
  <div style="display:flex; flex-direction:column; gap:16px;">

    <!-- 策略操作区 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>策略管理</span>
          <div style="display:flex; gap:8px">
            <el-button size="small" type="primary" :loading="applying" @click="applyNft">应用下发</el-button>
            <el-button size="small" type="warning" plain @click="nftRefresh">查看内核 nft</el-button>
          </div>
        </div>
      </template>

      <!-- 添加策略表单 -->
      <el-form label-width="90px" label-position="left" style="max-width:720px">
        <el-form-item label="维度">
          <el-select v-model="form.target_type" style="width:160px" @change="onTypeChange">
            <el-option label="IP" value="ip" />
            <el-option label="网段 CIDR" value="cidr" />
            <el-option label="MAC" value="mac" />
            <el-option label="应用 app" value="app" />
          </el-select>
        </el-form-item>
        <el-form-item :label="targetLabel">
          <el-input v-model="form.target" :placeholder="targetPlaceholder" style="width:280px" />
          <el-tag v-if="form.target_type==='cidr'" type="warning" size="small" style="margin-left:8px">不接受 /0</el-tag>
        </el-form-item>
        <el-form-item v-if="form.target_type==='app'" label="应用名">
          <el-input v-model="form.app" placeholder="如 wechat / tiktok（仅字母数字与 -_）" style="width:280px" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="策略名称" style="width:280px" />
        </el-form-item>
        <el-form-item label="速率 Mbit/s">
          <el-input-number v-model="form.rate_mbit" :min="0" :max="100000" style="width:180px" />
          <span style="margin-left:8px;color:#909399;font-size:12px">0=仅限连接数</span>
        </el-form-item>
        <el-form-item label="突发 KB">
          <el-input-number v-model="form.burst_kb" :min="0" :max="65535" style="width:180px" />
        </el-form-item>
        <el-form-item label="最大连接">
          <el-input-number v-model="form.max_conns" :min="0" :max="100000" style="width:180px" />
        </el-form-item>
        <el-form-item label="每秒新建">
          <el-input-number v-model="form.max_conns_per_sec" :min="0" :max="10000" style="width:180px" />
        </el-form-item>
      </el-form>
      <div style="display:flex; gap:12px">
        <el-checkbox v-model="form.enabled">启用</el-checkbox>
        <el-button type="primary" size="small" :loading="saving" @click="addPolicy">添加策略</el-button>
      </div>
    </el-card>

    <!-- 策略测试器 -->
    <el-card shadow="never">
      <template #header><span>策略测试器（调试：看哪条策略会命中）</span></template>
      <div style="display:flex; gap:12px; flex-wrap:wrap; align-items:flex-end">
        <el-input v-model="test.src_ip" placeholder="源 IP" style="width:150px" />
        <el-input v-model="test.src_mac" placeholder="源 MAC" style="width:180px" />
        <el-input v-model="test.dst_ip" placeholder="目的 IP" style="width:150px" />
        <el-input v-model="test.dst_port" placeholder="端口" style="width:90px" />
        <el-input v-model="test.app" placeholder="应用" style="width:130px" />
        <el-button size="small" :loading="testing" @click="doTest">测试命中</el-button>
      </div>
      <div v-if="testResult" style="margin-top:10px;padding:10px;background:#f5f7fa;border-radius:6px">
        <el-tag :type="testResult.matched ? 'danger' : 'info'">
          {{ testResult.matched ? `命中：${testResult.policy_id} (${testResult.hit.name})` : '未命中任何策略' }}
        </el-tag>
      </div>
    </el-card>

    <!-- 规则报告 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>规则报告</span>
          <el-button size="small" @click="load">刷新</el-button>
        </div>
      </template>
      <pre class="report-pre">{{ report || '暂无策略，报告为空' }}</pre>
    </el-card>

    <!-- 配额报表（/api/v1/quota/report） -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>配额报表</span>
          <el-button size="small" :loading="reportLoading" @click="loadQuotaReport">刷新</el-button>
        </div>
      </template>
      <el-alert v-if="reportErr" type="error" :closable="false" show-icon :title="reportErr" style="margin-bottom:12px" />
      <div style="margin-bottom:8px; font-size:13px; color:#606266">策略数量：{{ quotaReportCount }}</div>
      <pre class="report-pre">{{ quotaReportLines.length ? quotaReportLines.join('\n') : '暂无报表数据' }}</pre>
    </el-card>

    <!-- 内核 nft 表内容 -->
    <el-card v-if="nftContent" shadow="never">
      <template #header><span>内核 router_quota 表内容</span></template>
      <pre class="report-pre">{{ nftContent }}</pre>
    </el-card>

    <!-- 策略列表 -->
    <el-card shadow="never">
      <template #header><span>策略列表</span></template>
      <el-table :data="policies" size="small" stripe v-loading="loading">
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column label="维度" width="90">
          <template #default="{ row }">{{ row.target_type }}</template>
        </el-table-column>
        <el-table-column prop="target" label="目标" min-width="130" show-overflow-tooltip />
        <el-table-column prop="app" label="应用" width="110" />
        <el-table-column label="速率" width="100">
          <template #default="{ row }">{{ row.rate_mbit || '—' }} </template>
        </el-table-column>
        <el-table-column label="最大连接" width="90">
          <template #default="{ row }">{{ row.max_conns || '—' }}</template>
        </el-table-column>
        <el-table-column label="每秒新建" width="90">
          <template #default="{ row }">{{ row.max_conns_per_sec || '—' }}</template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain @click="removePolicy(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

interface Policy {
  id: string; name: string; target: string; target_type: string;
  app: string; rate_mbit: number; burst_kb: number;
  max_conns: number; max_conns_per_sec: number; enabled: boolean;
}

const policies = ref<Policy[]>([]);
const loading = ref(false);
const report = ref('');
const nftContent = ref('');
const saving = ref(false);
const applying = ref(false);
const testing = ref(false);

const emptyForm = () => ({
  id: '', name: '', target: '', target_type: 'ip', app: '',
  rate_mbit: 10, burst_kb: 0, max_conns: 100, max_conns_per_sec: 0, enabled: true,
});
const form = ref(emptyForm());

const test = ref({ src_ip: '', src_mac: '', dst_ip: '', dst_port: '', app: '' });
const testResult = ref<any>(null);

const targetLabel = computed(() => {
  switch (form.value.target_type) {
    case 'ip': return '目标 IP';
    case 'cidr': return '目标网段';
    case 'mac': return '目标 MAC';
    case 'app': return '目标';
    default: return '目标';
  }
});
const targetPlaceholder = computed(() => {
  switch (form.value.target_type) {
    case 'ip': return '如 203.0.113.5';
    case 'cidr': return '如 10.0.0.0/8（不接受 /0）';
    case 'mac': return '如 aa:bb:cc:dd:ee:ff';
    case 'app': return '（app 维度目标可留空）';
    default: return '';
  }
});

function onTypeChange() {
  form.value.app = '';
  if (form.value.target_type !== 'app') form.value.target = '';
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/quota/policies');
    policies.value = res.data?.policies || [];
    report.value = res.data?.report || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}

async function save(list: Policy[]) {
  // 清空所有策略是破坏性操作，后端要求显式 confirm；删除最后一条时如果不带
  // confirm，后端会 422，界面表现为"点了删除但列表没变"。
  const body: any = { policies: list };
  if (list.length === 0) body.confirm = true;
  const res = await api.post('/api/v1/quota/policies', body);
  policies.value = res.data?.policies || [];
  report.value = res.data?.report || '';
}

async function addPolicy() {
  const f = form.value;
  if (!f.name.trim()) { ElMessage.warning('请输入名称'); return; }
  if (!f.target.trim() && f.target_type !== 'app') { ElMessage.warning('请输入目标'); return; }
  if (f.target_type !== 'app' && f.rate_mbit === 0 && f.max_conns === 0 && f.max_conns_per_sec === 0) {
    ElMessage.warning('空策略无法生效，请至少设定速率或连接数之一');
    return;
  }
  if (f.target_type === 'app' && !f.app.trim()) { ElMessage.warning('请填写应用名'); return; }
  saving.value = true;
  try {
    const p: Policy = {
      id: f.id || `q-${Date.now()}`,
      name: f.name.trim(),
      target: f.target.trim(),
      target_type: f.target_type,
      app: f.target_type === 'app' ? f.app.trim() : '',
      rate_mbit: f.rate_mbit, burst_kb: f.burst_kb,
      max_conns: f.max_conns, max_conns_per_sec: f.max_conns_per_sec,
      enabled: f.enabled,
    };
    await save([...policies.value, p]);
    ElMessage.success('策略已添加');
    form.value = emptyForm();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

async function removePolicy(row: Policy) {
  try {
    await ElMessageBox.confirm(`删除策略「${row.name}」？`, '确认', { type: 'warning' });
    await save(policies.value.filter((p) => p.id !== row.id));
    ElMessage.success('已删除');
  } catch (e: any) {
    if (e?.message) ElMessage.error(e.message);
  }
}

async function doTest() {
  testing.value = true;
  testResult.value = null;
  try {
    const res = await api.post('/api/v1/quota/test', test.value);
    testResult.value = res.data;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    testing.value = false;
  }
}

async function applyNft() {
  applying.value = true;
  try {
    await ElMessageBox.confirm('将重建内核 router_quota 表（破坏性），确认下发？', '确认下发', { type: 'warning' });
    const res = await api.post('/api/v1/quota/apply', { confirm: true });
    ElMessage.success(`已下发 ${res.data?.count} 条策略`);
    nftContent.value = res.data?.nft || '';
  } catch (e: any) {
    if (e?.message) ElMessage.error(e.message);
  } finally {
    applying.value = false;
  }
}

async function nftRefresh() {
  try {
    const res = await api.get('/api/v1/quota/nft');
    nftContent.value = res.data?.exists ? res.data.nft : (res.data?.note || '（空）');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

const reportLoading = ref(false);
const reportErr = ref('');
const quotaReportCount = ref(0);
const quotaReportLines = ref<string[]>([]);

async function loadQuotaReport() {
  reportLoading.value = true;
  reportErr.value = '';
  try {
    const res = await api.get('/api/v1/quota/report');
    quotaReportCount.value = res.data?.count ?? 0;
    quotaReportLines.value = res.data?.report || [];
  } catch (e: any) {
    reportErr.value = e?.response?.data?.detail || e?.response?.data?.message || e?.message || '加载配额报表失败';
    ElMessage.error(reportErr.value);
  } finally {
    reportLoading.value = false;
  }
}

onMounted(() => { load(); loadQuotaReport(); });
</script>

<style scoped>
.report-pre {
  background: #0d1117; color: #c9d1d9; padding: 12px; border-radius: 6px;
  font-size: 12px; overflow: auto; white-space: pre-wrap; line-height: 1.6;
}
</style>