<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>负载均衡模式</template>
      <el-form inline style="margin-bottom: 12px;">
        <el-form-item label="模式">
          <el-radio-group v-model="cfg.mode">
            <el-radio value="weighted">加权（按权重分流）</el-radio>
            <el-radio value="failover">主备（第一个可用）</el-radio>
            <el-radio value="per-connection">按连接（同一源恒定）</el-radio>
            <el-radio value="per-packet">逐包（一般不建议）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="lbSaving" @click="saveLB">保存下发</el-button>
          <el-button @click="loadLB">加载</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="lbError" :title="lbError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form inline @submit.prevent="addMember">
        <el-form-item label="名称">
          <el-input v-model="newMem.name" placeholder="wan1" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="newMem.iface" placeholder="eth0" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="网关">
          <el-input v-model="newMem.gateway" placeholder="1.1.1.1" style="width: 140px;" />
        </el-form-item>
        <el-form-item label="权重">
          <el-input-number v-model="newMem.weight" :min="1" :max="100" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addMember">添加</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="cfg.members" border>
        <el-table-column prop="name" label="名称" width="100" />
        <el-table-column prop="iface" label="接口" width="100" />
        <el-table-column prop="gateway" label="网关" width="140" />
        <el-table-column prop="weight" label="权重" width="80" />
        <el-table-column label="健康" width="80">
          <template #default="{ row }">
            <el-tag v-if="statusMap[row.name]?.up" type="success" size="small">UP</el-tag>
            <el-tag v-else type="danger" size="small">DOWN</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="延迟" width="90">
          <template #default="{ row }">
            {{ statusMap[row.name]?.latency_ms != null ? statusMap[row.name].latency_ms.toFixed(1) + 'ms' : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="启用" width="70">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="saveLB" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="70">
          <template #default="{ $index }">
            <el-button type="danger" size="small" @click="cfg.members.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>健康检查</template>
      <el-form label-width="140px" style="max-width: 560px;">
        <el-form-item label="目标 IP">
          <el-input v-model="cfg.health.target" placeholder="8.8.8.8" style="width: 200px;" />
        </el-form-item>
        <el-form-item label="间隔(秒)">
          <el-input-number v-model="cfg.health.interval_sec" :min="1" :max="300" />
        </el-form-item>
        <el-form-item label="超时(秒)">
          <el-input-number v-model="cfg.health.timeout_sec" :min="1" :max="60" />
        </el-form-item>
        <el-form-item label="失败阈值">
          <el-input-number v-model="cfg.health.fail_threshold" :min="1" :max="10" />
        </el-form-item>
        <el-form-item label="恢复阈值">
          <el-input-number v-model="cfg.health.rise_threshold" :min="1" :max="10" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="checkLoading" @click="runCheck">立即检查</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="cfg.health.timeout_sec >= cfg.health.interval_sec" title="超时必须小于间隔，否则检查会重叠堆积" type="warning" show-icon :closable="false" />
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>模拟选路</template>
      <el-form inline>
        <el-form-item label="Hash Key">
          <el-input v-model="hashKey" placeholder="源IP或会话ID" style="width: 220px;" @keyup.enter="preview" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="previewLoading" @click="preview">模拟选路</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="previewMember" :title="`该连接会落到：${previewMember.name}（接口 ${previewMember.iface}，网关 ${previewMember.gateway}）`" type="info" show-icon :closable="false" />
    </el-card>

    <el-card shadow="never">
      <template #header>生成的脚本</template>
      <el-button size="small" style="margin-bottom: 8px;" @click="applyLB">应用下发</el-button>
      <el-collapse>
        <el-collapse-item title="nft mark 表" name="marks">
          <pre style="white-space: pre-wrap;">{{ marksText }}</pre>
        </el-collapse-item>
        <el-collapse-item title="ip rule 序列" name="rules">
          <pre style="white-space: pre-wrap;">{{ iprulesText }}</pre>
        </el-collapse-item>
      </el-collapse>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const cfg = ref<any>({ mode: 'weighted', members: [], health: { target: '8.8.8.8', interval_sec: 30, timeout_sec: 3, fail_threshold: 3, rise_threshold: 2 } });
const lbSaving = ref(false);
const lbError = ref('');
const newMem = ref<any>({ name: '', iface: '', gateway: '', weight: 1 });
const statusMap = ref<Record<string, any>>({});
const checkLoading = ref(false);
const hashKey = ref('');
const previewMember = ref<any>(null);
const previewLoading = ref(false);
const marksText = ref('');
const iprulesText = ref('');

async function loadLB() {
  lbError.value = '';
  try {
    const res = await api.get('/api/v1/lb/config');
    cfg.value = res.data?.config || cfg.value;
    marksText.value = res.data?.marks ? JSON.stringify(res.data.marks, null, 2) : '';
    iprulesText.value = res.data?.iprules || '';
  } catch (e: any) {
    lbError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载失败';
  }
}

async function saveLB() {
  lbSaving.value = true;
  lbError.value = '';
  try {
    const res = await api.post('/api/v1/lb/config', cfg.value);
    cfg.value = res.data?.config || cfg.value;
    marksText.value = res.data?.marks ? JSON.stringify(res.data.marks, null, 2) : '';
    iprulesText.value = res.data?.iprules || '';
    if (res.data?.applied) ElMessage.success('负载均衡已下发');
    else ElMessage.warning(res.data?.detail || '未下发');
  } catch (e: any) {
    lbError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    ElMessage.error(lbError.value);
  } finally {
    lbSaving.value = false;
  }
}

function addMember() {
  if (!newMem.value.name || !newMem.value.iface || !newMem.value.gateway) {
    ElMessage.warning('请填写名称/接口/网关');
    return;
  }
  cfg.value.members.push({ ...newMem.value, enabled: true });
  newMem.value = { name: '', iface: '', gateway: '', weight: 1 };
}

async function runCheck() {
  checkLoading.value = true;
  try {
    const res = await api.post('/api/v1/lb/check', {});
    const snap = res.data?.status || [];
    statusMap.value = {};
    for (const s of snap) statusMap.value[s.name] = s;
    ElMessage.success('已执行一轮健康检查');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '检查失败');
  } finally {
    checkLoading.value = false;
  }
}

async function preview() {
  previewLoading.value = true;
  try {
    const res = await api.get('/api/v1/lb/preview', { params: { hash_key: hashKey.value } });
    previewMember.value = res.data?.member || null;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '模拟失败');
  } finally {
    previewLoading.value = false;
  }
}

async function applyLB() {
  try {
    const res = await api.post('/api/v1/lb/apply', {});
    if (res.data?.applied) ElMessage.success('mark + ip rule 已下发');
    else ElMessage.warning(res.data?.detail || '未下发');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '下发失败');
  }
}

onMounted(() => { loadLB(); });
</script>