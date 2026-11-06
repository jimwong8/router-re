<template>
  <div>
    <el-card shadow="never">
      <template #header>应用识别（DPI）— 标记下发到数据面</template>

      <el-alert
        v-if="applyError"
        :title="'下发失败：' + applyError"
        type="error"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />

      <el-form inline @submit.prevent>
        <el-form-item label="应用名">
          <el-input v-model="newRule.app" placeholder="如 netflix" style="width: 150px" />
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="newRule.protocol" style="width: 100px">
            <el-option label="tcp" value="tcp" />
            <el-option label="udp" value="udp" />
          </el-select>
        </el-form-item>
        <el-form-item label="端口">
          <el-input
            v-model="portsText"
            placeholder="6881,6882（可空）"
            style="width: 160px"
          />
        </el-form-item>
        <el-form-item label="Mark">
          <el-input-number v-model="newRule.mark" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="动作">
          <el-select v-model="newRule.action" style="width: 110px">
            <el-option label="mark" value="mark" />
            <el-option label="drop" value="drop" />
            <el-option label="limit" value="limit" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="newRule.action === 'limit'" label="限速(Mbps)">
          <el-input-number v-model="newRule.rate_mbps" :min="1" :max="1000" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addRule">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="rules" border style="width: 100%; margin-top: 12px">
        <el-table-column prop="app" label="应用" width="150" />
        <el-table-column prop="protocol" label="协议" width="90" />
        <el-table-column label="端口" width="160">
          <template #default="{ row }">
            {{ (row.dports || []).join(',') || '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="mark" label="Mark" width="100" />
        <el-table-column prop="action" label="动作" width="100">
          <template #default="{ row }">
            <el-tag :type="row.action === 'drop' ? 'danger' : row.action === 'limit' ? 'warning' : 'info'">
              {{ row.action }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeRule(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button
        type="primary"
        style="margin-top: 16px"
        :loading="submitting"
        @click="submit"
      >
        下发到数据面
      </el-button>

      <el-divider>nft 规则（返回）</el-divider>
      <pre v-if="nftText" class="cmd-output">{{ nftText }}</pre>
      <el-empty v-else description="尚未下发" :image-size="60" />

      <el-divider style="margin-top: 24px">DPI 规则（持久化到后端）</el-divider>
      <el-alert
        v-if="rulesError"
        :title="'加载/保存失败：' + rulesError"
        type="error"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />

      <el-table :data="dpiRules" border style="width: 100%; margin-top: 8px">
        <el-table-column prop="app" label="应用名" width="150" />
        <el-table-column prop="protocol" label="协议" width="90" />
        <el-table-column label="端口" width="160">
          <template #default="{ row }">{{ (row.dports || []).join(',') || '-' }}</template>
        </el-table-column>
        <el-table-column prop="mark" label="Mark" width="80" />
        <el-table-column prop="action" label="动作" width="100">
          <template #default="{ row }">
            <el-tag :type="row.action === 'drop' ? 'danger' : row.action === 'limit' ? 'warning' : 'info'">{{
              row.action
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="enabled" label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" active-value="true" inactive-value="false" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row, $index }">
            <el-button type="danger" size="small" @click="removeDpiRule($index)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-form inline @submit.prevent style="margin-top: 12px">
        <el-form-item label="应用名">
          <el-input v-model="newDpiRule.app" placeholder="如 netflix" style="width: 140px" />
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="newDpiRule.protocol" style="width: 100px">
            <el-option label="tcp" value="tcp" />
            <el-option label="udp" value="udp" />
          </el-select>
        </el-form-item>
        <el-form-item label="端口">
          <el-input
            v-model="dpiPortsText"
            placeholder="6881,6882（可空）"
            style="width: 150px"
          />
        </el-form-item>
        <el-form-item label="Mark">
          <el-input-number v-model="newDpiRule.mark" :min="1" :max="65535" style="width: 100px" />
        </el-form-item>
        <el-form-item label="动作">
          <el-select v-model="newDpiRule.action" style="width: 100px">
            <el-option label="mark" value="mark" />
            <el-option label="drop" value="drop" />
            <el-option label="limit" value="limit" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="newDpiRule.action === 'limit'" label="限速(Mbps)">
          <el-input-number v-model="newDpiRule.rate_mbps" :min="1" :max="1000" style="width: 100px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addDpiRule">添加</el-button>
        </el-form-item>
      </el-form>

      <el-button
        type="primary"
        style="margin-top: 8px"
        :loading="savingDpiRules"
        @click="saveDpiRules"
      >
        保存规则
      </el-button>

      <el-divider style="margin-top: 24px">实时分类测试</el-divider>
      <el-form inline @submit.prevent style="margin-bottom: 12px">
        <el-form-item label="目标端口">
          <el-input-number v-model="classifyDport" :min="1" :max="65535" style="width: 120px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="classifying" @click="runClassify">分类</el-button>
        </el-form-item>
      </el-form>

      <el-table v-if="classifyResult" :data="[classifyResult]" border style="width: 100%">
        <el-table-column prop="dport" label="端口" width="100" />
        <el-table-column prop="protocol" label="协议" width="120" />
        <el-table-column prop="category" label="分类" width="200" />
      </el-table>
      <el-alert v-if="classifyError" :title="'分类失败：' + classifyError" type="error" show-icon :closable="false" style="margin-top:8px" />
      <el-empty v-if="!classifyResult && !classifyError" description="未执行分类" :image-size="60" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface MarkRule {
  app: string;
  protocol: string;
  dports: number[];
  mark: number;
  action: string;
  rate_mbps?: number;
  enabled?: boolean;
}

function emptyRule(): MarkRule {
  return { app: '', protocol: 'tcp', dports: [], mark: 200, action: 'mark', rate_mbps: 10 };
}

const newRule = ref<MarkRule>(emptyRule());
const portsText = ref('');
const rules = ref<MarkRule[]>([]);
const nftText = ref('');
const applyError = ref('');
const submitting = ref(false);

function parsePorts(): number[] {
  return portsText.value
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0 && n < 65536);
}

function addRule() {
  if (!newRule.value.app) {
    ElMessage.warning('应用名必填');
    return;
  }
  rules.value.push({ ...newRule.value, dports: parsePorts() });
  newRule.value = emptyRule();
  portsText.value = '';
}

function removeRule(row: MarkRule) {
  rules.value = rules.value.filter((r) => r !== row);
}

async function load() {
  try {
    const res = await api.get('/api/v1/dpi/marks');
    rules.value = res.data?.rules || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submit() {
  if (!rules.value.length) {
    ElMessage.warning('请先添加规则');
    return;
  }
  submitting.value = true;
  try {
    const res = await api.post('/api/v1/dpi/marks', { rules: rules.value });
    nftText.value = res.data?.nft || '';
    if (res.data?.applied) {
      applyError.value = '';
      ElMessage.success('已下发到数据面');
    } else {
      applyError.value = res.data?.error || '下发未生效（可能缺少 root/nftables 权限）';
      ElMessage.warning('规则已保存但未下发成功');
    }
    await load();
  } catch (e: any) {
    const msg = e?.response?.data?.message || e.message;
    ElMessage.error(msg);
    applyError.value = msg;
  } finally {
    submitting.value = false;
  }
}

// ---------- DPI 规则（/api/v1/dpi/rules） ----------
const dpiRules = reactive<MarkRule[]>([]);
const newDpiRule = reactive<MarkRule>({ app: '', protocol: 'tcp', dports: [], mark: 200, action: 'mark' });
const dpiPortsText = ref('');
const rulesError = ref('');
const savingDpiRules = ref(false);

function parseDpiPorts(): number[] {
  return dpiPortsText.value
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0 && n < 65536);
}

function addDpiRule() {
  if (!newDpiRule.app) {
    ElMessage.warning('应用名必填');
    return;
  }
  dpiRules.push({ ...newDpiRule, dports: parseDpiPorts(), enabled: true });
  newDpiRule.app = '';
  newDpiRule.protocol = 'tcp';
  newDpiRule.dports = [];
  newDpiRule.mark = 200;
  newDpiRule.action = 'mark';
  dpiPortsText.value = '';
}

function removeDpiRule(index: number) {
  dpiRules.splice(index, 1);
}

async function loadDpiRules() {
  try {
    const res = await api.get('/api/v1/dpi/rules');
    const loaded = res.data?.rules || [];
    dpiRules.splice(0, dpiRules.length);
    loaded.forEach((r: any) => dpiRules.push(r));
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(msg);
    rulesError.value = msg;
  }
}

async function saveDpiRules() {
  if (!dpiRules.length) {
    ElMessage.warning('请先添加规则');
    return;
  }
  savingDpiRules = true;
  try {
    const res = await api.post('/api/v1/dpi/rules', {
      rules: dpiRules,
      iface: '',
      root_mbit: 1000,
    });
    ElMessage.success(res.data?.error ? '保存完成但下发失败' : '规则已保存并下发');
    rulesError.value = res.data?.error || '';
    await loadDpiRules();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(msg);
    rulesError.value = msg;
  } finally {
    savingDpiRules = false;
  }
}

// ---------- 实时分类（/api/v1/dpi/classify） ----------
const classifyDport = ref(80);
const classifying = ref(false);
const classifyResult = ref<any>(null);
const classifyError = ref('');

async function runClassify() {
  classifying = true;
  classifyResult.value = null;
  classifyError.value = '';
  try {
    const res = await api.get('/api/v1/dpi/classify', {
      params: { dport: classifyDport.value },
    });
    classifyResult.value = res.data;
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(msg);
    classifyError.value = msg;
  } finally {
    classifying = false;
  }
}

onMounted(() => {
  load();
  loadDpiRules();
});
</script>

<style scoped>
.cmd-output {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
}
</style>
