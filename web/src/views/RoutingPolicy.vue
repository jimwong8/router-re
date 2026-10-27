<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>策略路由</span>
          <el-button type="primary" size="small" :loading="loading" @click="loadAll">刷新</el-button>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon :closable="true" @close="error=''" style="margin-bottom:12px" />

      <!-- 区块一：路由表 -->
      <el-card shadow="hover" style="margin-bottom:16px">
        <template #header>
          <div style="display:flex; justify-content:space-between; align-items:center">
            <span>路由表</span>
            <el-button type="primary" size="small" @click="openTableDialog">新增表</el-button>
          </div>
        </template>
        <el-table :data="tables" border style="width:100%">
          <el-table-column prop="id" label="表ID" width="90" />
          <el-table-column prop="name" label="表名" />
          <el-table-column label="路由数" width="100">
            <template #default="{ row }">{{ (row.routes || []).length }}</template>
          </el-table-column>
        </el-table>
      </el-card>

      <!-- 区块二：策略规则 -->
      <el-card shadow="hover" style="margin-bottom:16px">
        <template #header>
          <div style="display:flex; justify-content:space-between; align-items:center">
            <span>策略规则</span>
            <el-button type="primary" size="small" @click="openRuleDialog">新增规则</el-button>
          </div>
        </template>
        <el-table :data="rules" border style="width:100%">
          <el-table-column prop="priority" label="优先级" width="90" />
          <el-table-column prop="from" label="源(from)" />
          <el-table-column prop="to" label="目的(to)" />
          <el-table-column prop="fwmark" label="fwmark" width="90" />
          <el-table-column prop="table" label="目标表" width="90" />
          <el-table-column prop="iif" label="入接口(iif)" />
        </el-table>
      </el-card>

      <!-- 区块三：生成的命令 -->
      <el-card shadow="hover">
        <template #header><span>生成的命令</span></template>
        <pre style="background:#f5f7fa; padding:12px; border-radius:4px; overflow:auto; max-height:400px; margin:0">{{ renderedText }}</pre>
      </el-card>
    </el-card>

    <!-- 新增表弹窗 -->
    <el-dialog v-model="tableDialog" title="新增路由表" width="720px">
      <el-form label-width="90px">
        <el-form-item label="表ID">
          <el-input-number v-model="newTable.id" :min="1" :max="252" />
          <div style="color:#909399; font-size:12px; margin-top:4px">范围 1-252；253(default)/254(main)/255(local) 是内核保留表，会被后端拒绝</div>
        </el-form-item>
        <el-form-item label="表名">
          <el-input v-model="newTable.name" placeholder="如 wan1table" />
        </el-form-item>
        <el-form-item label="路由">
          <div v-for="(r, idx) in newTable.routes" :key="idx" style="display:flex; gap:8px; margin-bottom:8px; align-items:center">
            <el-input v-model="r.dst" placeholder="dst (如 10.0.0.0/8 或 default)" style="width:200px" />
            <el-input v-model="r.via" placeholder="via (网关IP)" style="width:160px" />
            <el-input v-model="r.dev" placeholder="dev (接口)" style="width:120px" />
            <el-input-number v-model="r.metric" :min="0" placeholder="metric" style="width:100px" />
            <el-button type="danger" size="small" @click="removeRoute(idx)">删</el-button>
          </div>
          <el-button size="small" @click="addRoute">+ 添加路由</el-button>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="tableDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveTable">保存</el-button>
      </template>
    </el-dialog>

    <!-- 新增规则弹窗 -->
    <el-dialog v-model="ruleDialog" title="新增策略规则" width="600px">
      <el-form label-width="90px">
        <el-form-item label="优先级">
          <el-input-number v-model="newRule.priority" :min="1" />
        </el-form-item>
        <el-form-item label="源(from)">
          <el-input v-model="newRule.from" placeholder="如 10.0.0.0/8" />
        </el-form-item>
        <el-form-item label="目的(to)">
          <el-input v-model="newRule.to" placeholder="如 8.8.8.0/24" />
        </el-form-item>
        <el-form-item label="fwmark">
          <el-input-number v-model="newRule.fwmark" :min="0" />
        </el-form-item>
        <el-form-item label="目标表">
          <el-input-number v-model="newRule.table" :min="1" :max="252" />
        </el-form-item>
        <el-form-item label="入接口(iif)">
          <el-input v-model="newRule.iif" placeholder="如 eth0" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const loading = ref(false);
const saving = ref(false);
const error = ref('');

const tables = ref<any[]>([]);
const rules = ref<any[]>([]);
const renderedTables = ref<Record<string, string[]>>({});
const renderedRules = ref<string[]>([]);

const tableDialog = ref(false);
const ruleDialog = ref(false);

const newTable = ref<any>({ id: 100, name: '', routes: [{ dst: '', via: '', dev: '', metric: 0 }] });
const newRule = ref<any>({ priority: 10, from: '', to: '', fwmark: 0, table: 100, iif: '' });

const renderedText = computed(() => {
  const lines: string[] = [];
  // tables rendered: {id: [commands]}
  for (const t of tables.value) {
    const cmds = renderedTables.value[String(t.id)] || renderedTables.value[t.id] || [];
    for (const c of cmds) lines.push(c);
  }
  for (const c of renderedRules.value) lines.push(c);
  return lines.join('\n');
});

function showError(e: any) {
  const d = e?.response?.data || {};
  const msg = d.message || d.detail || e?.message || '请求失败';
  const detail = d.detail;
  const full = detail && detail !== msg ? `${msg}：${detail}` : msg;
  error.value = full;
  ElMessage.error(full);
}

async function loadTables() {
  try {
    const res = await api.get('/api/v1/routing/tables');
    tables.value = res.data?.tables || [];
    renderedTables.value = res.data?.rendered || {};
  } catch (e: any) {
    showError(e);
  }
}

async function loadRules() {
  try {
    const res = await api.get('/api/v1/routing/rules');
    rules.value = res.data?.rules || [];
    renderedRules.value = res.data?.rendered || [];
  } catch (e: any) {
    showError(e);
  }
}

async function loadAll() {
  loading.value = true;
  error.value = '';
  await Promise.all([loadTables(), loadRules()]);
  loading.value = false;
}

function openTableDialog() {
  newTable.value = { id: 100, name: '', routes: [{ dst: '', via: '', dev: '', metric: 0 }] };
  tableDialog.value = true;
}

function addRoute() {
  newTable.value.routes.push({ dst: '', via: '', dev: '', metric: 0 });
}

function removeRoute(idx: number) {
  newTable.value.routes.splice(idx, 1);
}

async function saveTable() {
  saving.value = true;
  error.value = '';
  try {
    const payload = { tables: [...tables.value, {
      id: newTable.value.id,
      name: newTable.value.name,
      routes: newTable.value.routes,
    }] };
    const res = await api.post('/api/v1/routing/tables', payload);
    if (res.data) {
      ElMessage.success('保存成功');
      tableDialog.value = false;
      await loadTables();
    }
  } catch (e: any) {
    showError(e);
  } finally {
    saving.value = false;
  }
}

function openRuleDialog() {
  newRule.value = { priority: 10, from: '', to: '', fwmark: 0, table: 100, iif: '' };
  ruleDialog.value = true;
}

async function saveRule() {
  saving.value = true;
  error.value = '';
  try {
    const payload = { rules: [...rules.value, { ...newRule.value }] };
    const res = await api.post('/api/v1/routing/rules', payload);
    if (res.data) {
      ElMessage.success('保存成功');
      ruleDialog.value = false;
      await loadRules();
    }
  } catch (e: any) {
    showError(e);
  } finally {
    saving.value = false;
  }
}

onMounted(loadAll);
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; white-space: pre-wrap; word-break: break-all; }
</style>
