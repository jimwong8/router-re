<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>L7 应用层过滤规则</template>
      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-alert v-if="detail" :title="detail" :type="applied ? 'success' : 'info'" show-icon :closable="false" style="margin-bottom: 12px;" />

      <el-row :gutter="12" style="margin-bottom: 12px;">
        <el-col :span="6"><el-card shadow="never" class="stat"><div class="n">{{ stats.total ?? 0 }}</div><div class="t">总规则</div></el-card></el-col>
        <el-col :span="6"><el-card shadow="never" class="stat"><div class="n">{{ stats.enabled ?? 0 }}</div><div class="t">已启用</div></el-card></el-col>
        <el-col :span="6"><el-card shadow="never" class="stat"><div class="n">{{ typeCount }}</div><div class="t">覆盖类型数</div></el-card></el-col>
        <el-col :span="6"><el-card shadow="never" class="stat"><div class="n">{{ blockCount }}</div><div class="t">拦截规则</div></el-card></el-col>
      </el-row>
      <el-row :gutter="12" style="margin-bottom: 12px;">
        <el-col :span="6" v-for="(cnt, mt) in stats.by_type || {}" :key="mt">
          <el-card shadow="never" class="stat"><div class="n">{{ cnt }}</div><div class="t">{{ typeLabel(mt) }}</div></el-card>
        </el-col>
      </el-row>

      <el-form inline label-width="0">
        <el-form-item>
          <el-select v-model="newRule.match_type" style="width: 130px;" @change="onTypeChange">
            <el-option label="SNI" value="sni" />
            <el-option label="HTTP Host" value="http_host" />
            <el-option label="User-Agent" value="user_agent" />
            <el-option label="DNS" value="dns" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-input v-model="newRule.pattern" :placeholder="patternPlaceholder" style="width: 260px;" />
        </el-form-item>
        <el-form-item>
          <el-select v-model="newRule.action" style="width: 110px;">
            <el-option label="block" value="block" />
            <el-option label="allow" value="allow" />
            <el-option label="log" value="log" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-input v-model="newRule.name" placeholder="备注(可选)" style="width: 140px;" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="addRule">添加</el-button>
        </el-form-item>
      </el-form>
      <div style="color:#909399; font-size:12px; margin-bottom: 8px;">{{ typeHint }}</div>

      <el-table :data="rules" border style="width: 100%;">
        <el-table-column prop="name" label="名称" min-width="120">
          <template #default="{ row }">{{ row.name || '—' }}</template>
        </el-table-column>
        <el-table-column prop="match_type" label="类型" width="110">
          <template #default="{ row }">{{ typeLabel(row.match_type) }}</template>
        </el-table-column>
        <el-table-column prop="pattern" label="匹配内容" min-width="220" />
        <el-table-column prop="action" label="动作" width="90" />
        <el-table-column label="启用" width="70">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" @change="toggle(row, $event)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ $index }">
            <el-button type="danger" size="small" @click="removeRule($index)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!rules.length" description="暂无规则" />
      <div style="margin-top: 12px;">
        <el-button type="success" :loading="applying" @click="apply">应用（写入 dnsmasq 并重启）</el-button>
        <el-button :loading="loading" @click="loadAll">刷新</el-button>
      </div>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>规则测试器</template>
      <el-form inline label-width="0">
        <el-form-item label="SNI"><el-input v-model="test.sni" placeholder="a.doubleclick.net" style="width: 200px;" /></el-form-item>
        <el-form-item label="Host"><el-input v-model="test.http_host" placeholder="ads.example.com" style="width: 200px;" /></el-form-item>
        <el-form-item label="UA"><el-input v-model="test.user_agent" placeholder="curl/7.8" style="width: 200px;" /></el-form-item>
        <el-form-item label="DNS"><el-input v-model="test.dns_name" placeholder="tracker.example.net" style="width: 200px;" /></el-form-item>
        <el-form-item><el-button type="primary" :loading="testing" @click="doTest">测试</el-button></el-form-item>
      </el-form>
      <div v-if="testResult">
        <el-alert :title="testResult.blocked ? '拦截：命中 ' + (testResult.by || 'block') : '放行'"
          :type="testResult.blocked ? 'error' : 'success'" show-icon :closable="false" style="margin-bottom: 8px;" />
        <div>命中 {{ testResult.hit?.length || 0 }} 条：
          <el-tag v-for="(h, i) in testResult.hit" :key="i"
            :type="h.action==='block'?'danger':(h.action==='allow'?'success':'info')"
            style="margin:2px 4px;">{{ h.match_type }}={{ h.pattern }} → {{ h.action }}</el-tag>
        </div>
      </div>
    </el-card>

    <el-card shadow="never">
      <template #header>生成的 dnsmasq 屏蔽配置</template>
      <el-input v-model="dnsmasqText" type="textarea" :rows="12" readonly
        style="font-family: monospace;" placeholder="保存/应用后显示实际要写入的 address= 规则" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

const rules = ref<any[]>([]);
const stats = ref<any>({ total: 0, enabled: 0, by_type: {}, by_action: {} });
const dnsmasqText = ref('');
const error = ref('');
const detail = ref('');
const applied = ref(false);
const saving = ref(false);
const applying = ref(false);
const loading = ref(false);
const testing = ref(false);
const testResult = ref<any>(null);

const newRule = ref<any>({ match_type: 'sni', pattern: '', action: 'block', name: '', enabled: true });
const test = ref<any>({ sni: '', http_host: '', user_agent: '', dns_name: '' });

const typeLabels: Record<string, string> = { sni: 'SNI', http_host: 'HTTP Host', user_agent: 'User-Agent', dns: 'DNS' };
function typeLabel(mt: string) { return typeLabels[mt] || mt; }

const patternPlaceholder = computed(() =>
  ['sni', 'http_host', 'dns'].includes(newRule.value.match_type)
    ? '*.example.com 或 example.com'
    : 'UA 关键字（子串匹配）');

const typeHint = computed(() => {
  if (newRule.value.match_type === 'sni') return 'SNI 支持 *.example.com 通配（匹配 a.example.com 与 b.c.example.com，但不匹配 example.com 本身）';
  if (newRule.value.match_type === 'user_agent') return 'User-Agent 为子串匹配，大小写不敏感；禁止控制字符。';
  return '支持 *.example.com 通配';
});

function onTypeChange() { newRule.value.pattern = ''; }

const typeCount = computed(() => Object.keys(stats.value.by_type || {}).length);
const blockCount = computed(() => stats.value.by_action?.block || 0);

function genId() { return 'r' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6); }

async function loadRules() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/l7/rules');
    rules.value = res.data?.rules || [];
    dnsmasqText.value = res.data?.dnsmasq || '';
  } catch (e: any) {
    error.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载规则失败';
  } finally { loading.value = false; }
}

async function loadStats() {
  try {
    const res = await api.get('/api/v1/l7/stats');
    stats.value = res.data || {};
  } catch (e: any) { /* best-effort */ }
}

async function loadAll() { await Promise.all([loadRules(), loadStats()]); }

async function addRule() {
  if (!newRule.value.pattern.trim()) { ElMessage.error('请输入匹配内容'); return; }
  saving.value = true;
  error.value = '';
  const next = [...rules.value, { ...newRule.value, id: genId() }];
  try {
    const res = await api.post('/api/v1/l7/rules', { rules: next });
    rules.value = res.data?.rules || next;
    dnsmasqText.value = res.data?.dnsmasq || dnsmasqText.value;
    applied.value = res.data?.applied ?? false;
    detail.value = res.data?.detail || '';
    ElMessage.success('规则已保存');
    newRule.value.pattern = '';
    await loadStats();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    error.value = msg; ElMessage.error(msg);
  } finally { saving.value = false; }
}

async function removeRule(index: number) {
  try { await ElMessageBox.confirm('确定删除这条规则？', '删除', { type: 'warning' }); }
  catch { return; }
  const next = rules.value.filter((_, i) => i !== index);
  try {
    const res = await api.post('/api/v1/l7/rules', { rules: next });
    rules.value = res.data?.rules || next;
    dnsmasqText.value = res.data?.dnsmasq || dnsmasqText.value;
    await loadStats();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '删除失败';
    error.value = msg; ElMessage.error(msg);
  }
}

async function toggle(row: any, val: boolean) {
  row.enabled = val;
  const next = rules.value.map(r => r.id === row.id ? { ...r, enabled: val } : r);
  try {
    const res = await api.post('/api/v1/l7/rules', { rules: next });
    rules.value = res.data?.rules || next;
    dnsmasqText.value = res.data?.dnsmasq || dnsmasqText.value;
    await loadStats();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '切换失败';
    error.value = msg; ElMessage.error(msg);
  }
}

async function apply() {
  applying.value = true;
  error.value = ''; detail.value = ''; applied.value = false;
  try {
    const res = await api.post('/api/v1/l7/apply', {});
    applied.value = !!res.data?.applied;
    detail.value = res.data?.detail || '';
    dnsmasqText.value = res.data?.dnsmasq || dnsmasqText.value;
    if (res.data?.applied) ElMessage.success('已写入 dnsmasq 并重启');
    else ElMessage.warning('应用失败：' + (res.data?.detail || ''));
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '应用失败';
    error.value = msg; ElMessage.error(msg);
  } finally { applying.value = false; }
}

async function doTest() {
  if (!test.value.sni && !test.value.http_host && !test.value.user_agent && !test.value.dns_name) {
    ElMessage.error('请至少填一个字段'); return;
  }
  testing.value = true;
  testResult.value = null;
  try {
    const res = await api.post('/api/v1/l7/test', test.value);
    testResult.value = res.data;
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '测试失败';
    error.value = msg; ElMessage.error(msg);
  } finally { testing.value = false; }
}

onMounted(loadAll);
</script>

<style scoped>
.stat { text-align: center; }
.stat .n { font-size: 26px; font-weight: 600; }
.stat .t { color: #909399; font-size: 12px; margin-top: 4px; }
</style>
