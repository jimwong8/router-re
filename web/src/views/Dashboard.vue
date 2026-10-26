<template>
  <div class="dashboard">
    <header class="dashboard-header">
      <h1>系统概览</h1>
    </header>

    <el-alert
      v-if="loadError"
      :title="loadError"
      type="error"
      show-icon
      :closable="false"
      style="margin-bottom: 12px"
    />

    <el-row :gutter="16" class="stat-row">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">CPU</div>
          <div class="stat-value">{{ cpuModel }}</div>
          <div class="stat-sub">{{ cpuCores }} 核</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">内存</div>
          <div class="stat-value">{{ memUsed }} MB</div>
          <div class="stat-sub">已用 / 共 {{ memTotal }} MB · 可用 {{ memAvail }} MB</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">接口</div>
          <div class="stat-value">{{ interfaces.length }}</div>
          <div class="stat-sub">活跃接口</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">系统</div>
          <div class="stat-value">{{ osVersion }}</div>
          <div class="stat-sub">{{ hostname }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="placeholder-card">
      <template #header>接口列表</template>
      <el-table :data="interfaces" border style="width: 100%">
        <el-table-column prop="name" label="接口" width="140" />
        <el-table-column prop="ipaddr" label="IP 地址" width="180" />
        <el-table-column prop="rx_bytes" label="接收字节" />
        <el-table-column prop="tx_bytes" label="发送字节" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import api from '../api';

const hostname = ref('-');
const cpuModel = ref('-');
const cpuCores = ref(0);
const memUsed = ref(0);
const memAvail = ref(0);
const memTotal = ref(0);
const osVersion = ref('-');
const interfaces = ref<Array<{ name: string; ipaddr: string; rx_bytes: number; tx_bytes: number }>>([]);
const loadError = ref('');

async function load() {
  try {
    const [info, ifs] = await Promise.all([
      api.get('/api/v1/system/info'),
      api.get('/api/v1/interfaces'),
    ]);
    const d = info.data;
    hostname.value = d.hostname || '-';
    cpuModel.value = d.cpu_model || '-';
    cpuCores.value = d.cpu_cores || 0;
    memAvail.value = Math.round((d.mem_avail_kb || 0) / 1024);
    memTotal.value = Math.round((d.mem_total_kb || 0) / 1024);
    memUsed.value = Math.round(((d.mem_total_kb || 0) - (d.mem_avail_kb || 0)) / 1024);
    osVersion.value = d.os_version || '-';
    interfaces.value = ifs.data.interfaces || [];
    loadError.value = '';
  } catch (e: any) {
    // 401 is handled globally by the shared api interceptor (redirects to /login).
    loadError.value = e?.response?.data?.message || e.message || '加载失败';
  }
}

onMounted(load);
</script>

<style scoped>
.dashboard {
  padding: 24px;
  background: #f5f7fa;
  min-height: 100vh;
}
.dashboard-header h1 {
  font-size: 24px;
  margin-bottom: 20px;
}
.stat-row {
  margin-bottom: 20px;
}
.stat-card {
  text-align: center;
}
.stat-title {
  color: #909399;
  font-size: 14px;
}
.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: #303133;
  margin: 8px 0;
}
.stat-sub {
  color: #909399;
  font-size: 12px;
}
.placeholder-card {
  margin-top: 4px;
}
</style>
