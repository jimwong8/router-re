<template>
  <div>
    <el-card shadow="never">
      <template #header>流量控制 (QoS)</template>
      <el-form inline @submit.prevent>
        <el-form-item label="接口">
          <el-input v-model="newLimit.interface" placeholder="如 eth0" />
        </el-form-item>
        <el-form-item label="速率 (Mbps)">
          <el-input-number v-model="newLimit.rateMbps" :min="1" :max="10000" />
        </el-form-item>
        <el-form-item label="突发 (KB)">
          <el-input-number v-model="newLimit.burstKB" :min="10" />
        </el-form-item>
        <el-form-item label="方向">
          <el-select v-model="newLimit.direction" style="width: 100px;">
            <el-option label="egress" value="egress" />
            <el-option label="ingress" value="ingress" />
          </el-select>
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="newLimit.priority" :min="0" :max="65535" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addLimit">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="limits" border style="width: 100%; margin-top: 12px;">
        <el-table-column prop="interface" label="接口" width="120" />
        <el-table-column prop="rateMbps" label="速率 (Mbps)" />
        <el-table-column prop="burstKB" label="突发 (KB)" width="120" />
        <el-table-column prop="direction" label="方向" width="100" />
        <el-table-column prop="priority" label="优先级" width="100" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeLimit(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button
        type="primary"
        style="margin-top: 16px"
        :loading="submitting"
        @click="submit"
      >
        提交
      </el-button>

      <el-divider>TC 命令（返回）</el-divider>
      <pre v-if="tcOutput.length" class="cmd-output">{{ cmdText }}</pre>
      <el-empty v-else description="尚未提交" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface LimitRow {
  interface: string;
  rateMbps: number;
  burstKB: number;
  direction: string;
  priority: number;
}

const newLimit = ref<LimitRow>({ interface: 'eth0', rateMbps: 100, burstKB: 512, direction: 'egress', priority: 1 });
const limits = ref<LimitRow[]>([]);
const tcOutput = ref<string[]>([]);
const submitting = ref(false);

const cmdText = computed(() => tcOutput.value.join('\n'));

function addLimit() {
  if (!newLimit.value.interface) {
    ElMessage.warning('接口必填');
    return;
  }
  limits.value.push({ ...newLimit.value });
  newLimit.value = { interface: 'eth0', rateMbps: 100, burstKB: 512, direction: 'egress', priority: 1 };
}

function removeLimit(row: LimitRow) {
  limits.value = limits.value.filter((l) => l !== row);
}

async function load() {
  try {
    const res = await api.get('/api/v1/qos/limits');
    limits.value = res.data?.limits || [];
    tcOutput.value = res.data?.tc || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submit() {
  if (!limits.value.length) {
    ElMessage.warning('请先添加规则');
    return;
  }
  submitting.value = true;
  try {
    const res = await api.post('/api/v1/qos/limits', { limits: limits.value });
    tcOutput.value = res.data?.tc || res.data?.commands || [];
    ElMessage.success('已提交');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    submitting.value = false;
  }
}

onMounted(load);
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