<template>
  <div>
    <el-card shadow="never">
      <template #header>审计日志 — 所有配置变更与登录事件</template>

      <el-form inline @submit.prevent>
        <el-form-item label="动作过滤">
          <el-input
            v-model="actionFilter"
            placeholder="如 auth.login / firewall"
            style="width: 240px"
            clearable
          />
        </el-form-item>
        <el-form-item label="条数">
          <el-input-number v-model="limit" :min="10" :max="1000" :step="50" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="load">查询</el-button>
        </el-form-item>
      </el-form>

      <el-alert
        v-if="loadError"
        :title="loadError"
        type="error"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />

      <el-table :data="events" border style="width: 100%">
        <el-table-column label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.ts) }}</template>
        </el-table-column>
        <el-table-column prop="actor" label="操作者" width="120" />
        <el-table-column prop="action" label="动作" width="220" />
        <el-table-column prop="target" label="目标" />
        <el-table-column label="结果" width="90">
          <template #default="{ row }">
            <el-tag :type="row.result === 'ok' ? 'success' : 'danger'">{{ row.result }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="remote_ip" label="来源 IP" width="150" />
        <el-table-column prop="detail" label="详情" show-overflow-tooltip />
      </el-table>

      <el-empty v-if="!loading && !events.length" description="暂无审计记录" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const events = ref<any[]>([]);
const actionFilter = ref('');
const limit = ref(100);
const loading = ref(false);
const loadError = ref('');

function formatTime(ts: number): string {
  if (!ts) return '-';
  const d = new Date(ts * 1000);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

async function load() {
  loading.value = true;
  try {
    const params: Record<string, string | number> = { limit: limit.value };
    if (actionFilter.value) params.action = actionFilter.value;
    const res = await api.get('/api/v1/audit/events', { params });
    events.value = res.data?.events || [];
    loadError.value = '';
  } catch (e: any) {
    loadError.value = e?.response?.data?.message || e.message || '加载审计日志失败';
    ElMessage.error(loadError.value);
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>
