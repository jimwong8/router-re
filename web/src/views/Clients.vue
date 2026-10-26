<template>
  <div>
    <el-card shadow="never">
      <template #header>终端列表 — 网络邻居 + DHCP 租约</template>
      <el-table :data="clients" border style="width: 100%">
        <el-table-column prop="ip" label="IP 地址" width="140" />
        <el-table-column prop="mac" label="MAC 地址" width="180" />
        <el-table-column prop="hostname" label="主机名">
          <template #default="{ row }">
            <span>{{ row.hostname || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="state" label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="row.state === 'REACHABLE' ? 'success' : 'info'">{{ row.state }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <el-button type="primary" style="margin-top: 12px" @click="load">刷新</el-button>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const clients = ref<any[]>([]);
const loadError = ref('');

async function load() {
  try {
    const res = await api.get('/api/v1/clients');
    clients.value = res.data?.clients || [];
    loadError.value = '';
  } catch (e: any) {
    loadError.value = e?.response?.data?.message || e.message || '加载终端失败';
    ElMessage.error(loadError.value);
  }
}

onMounted(load);
</script>