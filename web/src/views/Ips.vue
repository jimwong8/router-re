<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>入侵防御（IPS）</span>
          <el-button type="primary" size="small" :loading="loading" @click="loadRules">刷新规则</el-button>
        </div>
      </template>

      <!-- 统计卡片 -->
      <el-row :gutter="16" style="margin-bottom:12px">
        <el-col :span="6"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold">{{ stats.total ?? 0 }}</div>
          <div style="color:#909399; font-size:12px">总规则</div>
        </div></el-card></el-col>
        <el-col :span="6"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold; color:#409EFC">{{ stats.enabled ?? 0 }}</div>
          <div style="color:#909399; font-size:12px">已启用</div>
        </div></el-card></el-col>
        <el-col :span="6"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold; color:#f76711">{{ stats.alerted ?? 0 }}</div>
          <div style="color:#909399; font-size:12px">告警（alert）</div>
        </div></el-card></el-col>
        <el-col :span="6"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold; color:#e63000">{{ stats.dropped ?? 0 }}</div>
          <div style="color:#909399; font-size:12px">阻断（drop）</div>
        </div></el-card></el-col>
      </el-row>

      <!-- 两种添加入口 -->
      <el-tabs v-model="addTab" type="card" style="margin-bottom:12px">
        <el-tab-pane name="single" label="单条表单" />
        <el-tab-pane name="text" label="Suricata 批量导入" />
      </el-tabs>

      <el-form v-if="addTab === 'single'" :model="newRule" label-width="90px">
        <el-row :gutter="16">
          <el-col :span="6"><el-form-item label="动作"><el-select v-model="newRule.action" style="width:100%">
            <el-option label="alert" value="alert" />
            <el-option label="drop" value="drop" />
            <el-option label="pass" value="pass" />
          </el-select></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="协议"><el-select v-model="newRule.proto" style="width:100%">
            <el-option label="tcp" value="tcp" />
            <el-option label="udp" value="udp" />
            <el-option label="icmp" value="icmp" />
            <el-option label="any" value="any" />
          </el-select></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="目的端口"><el-input v-model="newRule.dst_port" placeholder="如 80 或 any" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="描述"><el-input v-model="newRule.msg" /></el-form-item></el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="8"><el-form-item label="载荷子串"><el-input v-model="newRule.content" placeholder="区分大小写英文" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="严重级别"><el-input-number v-model="newRule.severity" :min="1" :max="4" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item label="启用"><el-switch v-model="newRule.enabled" /></el-form-item></el-col>
        </el-row>
        <el-form-item><el-button type="primary" :loading="saving" @click="addSingle">添加</el-button></el-form-item>
      </el-form>

      <el-form v-else :model="{ text: suricataText }" label-width="90px">
        <el-form-item label="规则文本">
          <el-input type="textarea" v-model="suricataText" :rows="6" placeholder="粘贴 Suricata 风格规则，每行一条 ..." />
        </el-form-item>
        <el-form-item><el-button type="primary" :loading="saving" @click="importText">批量导入</el-button></el-form-item>
      </el-form>

      <!-- 导入结果 -->
      <el-alert v-if="importResult" :title="importResult" type="info" closable @close="importResult = ''" style="margin-top:12px" />

      <!-- 规则表格 -->
      <el-table :data="rules" style="width:100%; margin-top:12px" row-key="sid">
        <el-table-column prop="sid" label="SID" width="100" />
        <el-table-column prop="msg" label="描述" />
        <el-table-column prop="proto" label="协议" width="90" />
        <el-table-column prop="src_port" label="源端口" width="100" />
        <el-table-column prop="dst_port" label="目的端口" width="110" />
        <el-table-column prop="action" label="动作" width="90" />
        <el-table-column prop="severity" label="严重级别" width="100" />
        <el-table-column prop="enabled" label="启用" width="80">
          <template #default="s"><el-switch v-model="s.row.enabled" size="small" /></template>
        </el-table-column>
        <el-table-column label="操作" width="110">
          <template #default="s">
            <el-button type="danger" link size="small" @click="delRule(s.row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 规则测试器 -->
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header><span>规则测试器</span></template>
      <el-form :inline="true" :model="testForm" label-width="80px">
        <el-form-item label="规则 SID"><el-input-number v-model="testForm.rule_sid" :min="0" /></el-form-item>
        <el-form-item label="协议"><el-select v-model="testForm.proto" style="width:120px">
          <el-option label="tcp" value="tcp" /><el-option label="udp" value="udp" /><el-option label="icmp" value="icmp" /><el-option label="any" value="any" />
        </el-select></el-form-item>
        <el-form-item label="目的端口"><el-input-number v-model="testForm.dst_port" :min="0" /></el-form-item>
        <el-form-item label="载荷"><el-input v-model="testForm.payload" style="width:320px" /></el-form-item>
        <el-form-item><el-button type="primary" :loading="testing" @click="runTest">测试</el-button></el-form-item>
      </el-form>
      <el-alert v-if="testResult" :title="testResult" type="info" closable @close="testResult = ''" style="margin-top:12px" />
    </el-card>

    <!-- 封禁列表 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>封禁列表（blackhole）</span>
          <el-button type="primary" link size="small" @click="addBlock = true">添加 IP</el-button>
        </div>
      </template>
      <el-table :data="blocklist" style="width:100%" row-key="ip">
        <el-table-column prop="ip" label="IP 地址" />
        <el-table-column label="操作" width="110">
          <template #default="s">
            <el-button type="danger" link size="small" @click="delBlock(s.row.ip)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-dialog v-model="addBlock" title="添加封禁 IP">
        <el-input v-model="newBlockIp" placeholder="如 1.2.3.4" />
        <template #footer><el-button @click="addBlock = false">取消</el-button><el-button type="primary" @click="addBlockIp">确定</el-button></template>
      </el-dialog>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface Rule {
  sid: number; msg: string; proto: string; src_port: string; dst_port: string;
  action: string; severity: number; enabled: boolean;
}

const loading = ref(false);
const saving = ref(false);
const testing = ref(false);
const rules = ref<Rule[]>([]);
const stats = ref<Record<string, number>>({});
const blocklist = ref<string[]>([]);
const importResult = ref('');
const testResult = ref('');
const addTab = ref('single');
const addBlock = ref(false);
const newBlockIp = ref('');

const newRule = ref({
  action: 'alert', proto: 'tcp', dst_port: '80', msg: '', content: '',
  severity: 2, enabled: true,
});
const suricataText = ref('');
const testForm = ref({ rule_sid: 0, proto: 'tcp', dst_port: 80, payload: '' });

async function loadRules() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/ips/rules');
    const arr = res.data?.rules ?? [];
    rules.value = arr.map((r: any) => ({
      sid: r.sid, msg: r.msg, proto: r.proto, src_port: r.src_port, dst_port: r.dst_port,
      action: r.action, severity: r.severity, enabled: r.enabled,
    }));
    const st = res.data?.stats;
    if (st) stats.value = st;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    loading.value = false;
  }
}

async function loadStats() {
  try {
    const res = await api.get('/api/v1/ips/stats');
    const st = res.data?.stats;
    if (st) stats.value = st;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function loadBlocklist() {
  try {
    const res = await api.get('/api/v1/ips/blocklist');
    blocklist.value = res.data?.ips ?? [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function addSingle() {
  saving.value = true;
  try {
    const payload = { rules: [{
      sid: 0, msg: newRule.value.msg, proto: newRule.value.proto,
      src_port: 'any', dst_port: newRule.value.dst_port,
      payload_ascii: newRule.value.content, action: newRule.value.action,
      severity: newRule.value.severity, enabled: newRule.value.enabled,
    }] };
    const res = await api.post('/api/v1/ips/rules', payload);
    if (res.data?.applied) {
      ElMessage.success('添加成功');
      await loadRules();
    } else {
      ElMessage.error(res.data?.detail || '添加失败');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

async function importText() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/ips/rules', { text: suricataText.value });
    const d = res.data || {};
    const ok = d.count ?? 0;
    const fail = d.failures ? (d.failures.match(/,/g) || []).length + 1 : 0;
    if (d.applied) {
      importResult.value = `成功 ${ok} 条，失败 ${fail} 条${d.failures ? '：' + d.failures : ''}`;
      await loadRules();
      suricataText.value = '';
    } else {
      ElMessage.error(d.detail || d.failures || '导入失败');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

async function delRule(row: Rule) {
  try {
    // 简单删除：从规则列表中移除 SID 并回写。
    const idx = rules.value.findIndex(r => r.sid === row.sid);
    if (idx < 0) return;
    rules.value.splice(idx, 1);
    const text = rules.value.map(r =>
      `${r.action ?? 'alert'} ${r.proto ?? 'tcp'} any any -> any ${r.dst_port ?? 'any'} (msg:"${r.msg}"; content:"${r.action}"; sid:${r.sid};)`).join('\n');
    await api.post('/api/v1/ips/rules', { text });
    ElMessage.success('删除成功');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function runTest() {
  testing.value = true;
  try {
    const res = await api.post('/api/v1/ips/test', {
      rule_sid: testForm.value.rule_sid,
      proto: testForm.value.proto,
      dst_port: testForm.value.dst_port,
      payload: testForm.value.payload,
    });
    const d = res.data || {};
    testResult.value = d.matched
      ? `命中（rule_sid=${testForm.value.rule_sid}） — ${d.hits?.length ?? 0} 条命中`
      : `未命中 — ${d.hits?.length ?? 0} 条命中`;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    testing.value = false;
  }
}

async function addBlockIp() {
  if (!newBlockIp.value) return;
  try {
    const res = await api.post('/api/v1/ips/blocklist', { ips: [...blocklist.value, newBlockIp.value] });
    if (res.data?.applied) {
      await loadBlocklist();
      addBlock.value = false;
      newBlockIp.value = '';
      ElMessage.success('封禁成功');
    } else {
      ElMessage.error(res.data?.detail || '封禁失败');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function delBlock(ip: string) {
  try {
    const next = blocklist.value.filter(x => x !== ip);
    const res = await api.post('/api/v1/ips/blocklist', { ips: next });
    if (res.data?.applied) {
      await loadBlocklist();
      ElMessage.success('解封成功');
    } else {
      ElMessage.error(res.data?.detail || '解封失败');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

onMounted(() => { loadRules(); loadStats(); loadBlocklist(); });
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; }
</style>
