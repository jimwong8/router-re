<template>
  <div>
    <el-card shadow="never">
      <template #header>云管理 — 设备列表</template>
      <el-table :data="devices" border style="width: 100%">
        <el-table-column prop="id" label="ID" width="160" />
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="model" label="型号" />
        <el-table-column prop="version" label="固件" />
        <el-table-column prop="last_seen" label="最后心跳" />
        <el-table-column prop="online" label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.online ? 'success' : 'danger'">{{ row.online ? '在线' : '离线' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="config_version" label="配置版本" width="100" />
      </el-table>
      <el-button type="primary" style="margin-top: 12px" @click="load">刷新</el-button>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const devices = ref<any[]>([]);
const loadError = ref('');

async function load() {
  try {
    const res = await api.get('/api/v1/cloud/devices');
    devices.value = res.data?.devices || [];
    loadError.value = '';
  } catch (e: any) {
    loadError.value = e?.response?.data?.message || e.message || '加载设备失败';
    ElMessage.error(loadError.value);
  }
}

onMounted(load);
</script>
