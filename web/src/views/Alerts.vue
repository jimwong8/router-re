<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>告警规则</span>
          <div>
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
            <el-button type="primary" size="small" @click="openNew">新增规则</el-button>
          </div>
        </div>
      </template>

      <el-table :data="rules" v-loading="loading" size="small" style="width:100%" empty-text="还没有告警规则">
        <el-table-column prop="name" label="规则名" min-width="140" />
        <el-table-column prop="pattern" label="匹配内容" min-width="180" show-overflow-tooltip />
        <el-table-column label="正则" width="70">
          <template #default="{ row }">{{ row.is_regex ? '是' : '否' }}</template>
        </el-table-column>
        <el-table-column label="限定服务" min-width="140">
          <template #default="{ row }">
            {{ (row.units && row.units.length) ? row.units.join(', ') : '全部' }}
          </template>
        </el-table-column>
        <el-table-column label="级别" width="90">
          <template #default="{ row }">
            <el-tag :type="severityType(row.severity)" size="small">{{ row.severity }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="cooldown" label="冷却(秒)" width="90" />
        <el-table-column label="启用" width="70">
          <template #default="{ row }"><el-switch v-model="row.enabled" size="small" /></template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ $index }">
            <el-button link type="danger" size="small" @click="removeRule($index)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div style="margin-top:12px; color:#909399; font-size:12px">
        共 {{ rules.length }} 条规则。同一规则在冷却时间内重复命中只会记一次，避免告警风暴。
      </div>
      <div style="margin-top:12px; text-align:right">
        <el-button type="primary" :loading="saving" @click="saveAll('已保存')">保存</el-button>
      </div>
    </el-card>

    <el-card shadow="never" style="margin-bottom:16px">
      <template #header><span>测试匹配</span></template>
      <el-form :inline="true" size="small">
        <el-form-item label="服务名">
          <el-input v-model="testUnit" placeholder="如 sshd.service" style="width:180px" />
        </el-form-item>
        <el-form-item label="日志内容">
          <el-input v-model="testMessage" placeholder="如 Failed password for root" style="width:320px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="testing" @click="runTest">测试</el-button>
        </el-form-item>
      </el-form>

      <div v-if="tested">
        <el-alert v-if="matched.length === 0" type="info" :closable="false" show-icon
          title="没有任何规则命中这段日志" />
        <div v-else>
          <div style="margin-bottom:8px; font-size:13px">命中 {{ matched.length }} 条规则：</div>
          <el-tag v-for="m in matched" :key="m.id" :type="severityType(m.severity)"
            style="margin-right:8px">{{ m.name }}（{{ m.severity }}）</el-tag>
        </div>
      </div>
    </el-card>

    <el-dialog v-model="dialog" title="新增告警规则" width="560px">
      <el-form :model="form" label-width="100px" size="small">
        <el-form-item label="规则名">
          <el-input v-model="form.name" placeholder="如 SSH 暴力破解" />
        </el-form-item>
        <el-form-item label="匹配内容">
          <el-input v-model="form.pattern" placeholder="关键字，或正则表达式" />
        </el-form-item>
        <el-form-item label="按正则匹配">
          <el-switch v-model="form.is_regex" />
          <span style="margin-left:8px; color:#909399; font-size:12px">
            开启后 pattern 按正则解析，写错会在保存时报错并给出编译信息
          </span>
        </el-form-item>
        <el-form-item label="限定服务">
          <el-input v-model="form.unitsText" placeholder="逗号分隔，留空表示全部服务" />
        </el-form-item>
        <el-form-item label="级别">
          <el-select v-model="form.severity" style="width:100%">
            <el-option label="info（提示）" value="info" />
            <el-option label="warning（警告）" value="warning" />
            <el-option label="critical（严重）" value="critical" />
          </el-select>
        </el-form-item>
        <el-form-item label="冷却时间">
          <el-input-number v-model="form.cooldown" :min="0" :max="86400" />
          <span style="margin-left:8px; color:#909399; font-size:12px">秒</span>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button size="small" @click="dialog = false">取消</el-button>
        <el-button type="primary" size="small" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface Rule {
  id: string; name: string; pattern: string; is_regex: boolean;
  units: string[]; severity: string; cooldown: number; enabled: boolean;
}
interface Matched { id: string; name: string; severity: string }

const loading = ref(false);
const saving = ref(false);
const testing = ref(false);
const tested = ref(false);
const rules = ref<Rule[]>([]);
const matched = ref<Matched[]>([]);
const dialog = ref(false);
const testUnit = ref('sshd.service');
const testMessage = ref('Failed password for root');

const form = ref({
  id: '', name: '', pattern: '', is_regex: false,
  unitsText: '', severity: 'warning', cooldown: 300, enabled: true,
});

function severityType(s: string) {
  if (s === 'critical') return 'danger';
  if (s === 'warning') return 'warning';
  return 'info';
}

// 后端错误格式是 {code, message, detail}；detail 往往才是真正的原因
// （比如正则编译失败会带出编译器的原文），必须一起显示，不能只弹 message。
function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/alerts/rules');
    rules.value = res.data?.rules || [];
  } catch (e: any) {
    showError(e, '加载告警规则失败');
  } finally {
    loading.value = false;
  }
}

function openNew() {
  form.value = {
    id: 'r' + Date.now(), name: '', pattern: '', is_regex: false,
    unitsText: '', severity: 'warning', cooldown: 300, enabled: true,
  };
  dialog.value = true;
}

function removeRule(idx: number) {
  rules.value.splice(idx, 1);
  saveAll('已删除（记得保存生效）');
}

async function save() {
  if (!form.value.name.trim() || !form.value.pattern.trim()) {
    ElMessage.warning('规则名和匹配内容都不能为空');
    return;
  }
  const units = form.value.unitsText
    .split(',').map(s => s.trim()).filter(Boolean);
  rules.value.push({
    id: form.value.id,
    name: form.value.name.trim(),
    pattern: form.value.pattern,
    is_regex: form.value.is_regex,
    units,
    severity: form.value.severity,
    cooldown: form.value.cooldown,
    enabled: form.value.enabled,
  });
  dialog.value = false;
  await saveAll('已保存');
}

async function saveAll(okMsg: string) {
  saving.value = true;
  try {
    // 后端要的是 {rules:[...]}，不是裸数组
    await api.post('/api/v1/alerts/rules', { rules: rules.value });
    ElMessage.success(okMsg);
    await load();
  } catch (e: any) {
    showError(e, '保存失败');
    await load();
  } finally {
    saving.value = false;
  }
}

async function runTest() {
  if (!testUnit.value.trim() || !testMessage.value.trim()) {
    ElMessage.warning('服务名和日志内容都要填');
    return;
  }
  testing.value = true;
  try {
    const res = await api.post('/api/v1/alerts/test', {
      unit: testUnit.value.trim(),
      message: testMessage.value.trim(),
    });
    matched.value = res.data?.matched || [];
    tested.value = true;
  } catch (e: any) {
    showError(e, '测试失败');
  } finally {
    testing.value = false;
  }
}

onMounted(load);
</script>
