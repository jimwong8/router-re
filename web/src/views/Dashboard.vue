<template>
  <div class="dashboard">
    <header class="dashboard-header">
      <h1>系统概览</h1>
    </header>

    <el-row :gutter="16" class="stat-row">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">CPU</div>
          <div class="stat-value">23%</div>
          <div class="stat-sub">负载 0.23 / 4 核</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">内存</div>
          <div class="stat-value">1.2 GB</div>
          <div class="stat-sub">已用 / 4 GB 总计</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">接口</div>
          <div class="stat-value">2</div>
          <div class="stat-sub">WAN × 1 · LAN × 1</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-title">连接数</div>
          <div class="stat-value">1,024</div>
          <div class="stat-sub">活跃会话</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="placeholder-card">
      <template #header>接口列表（占位）</template>
      <el-table :data="fakeInterfaces" border style="width: 100%">
        <el-table-column prop="name" label="接口" width="140" />
        <el-table-column prop="ip" label="IP 地址" />
        <el-table-column prop="status" label="状态" width="100" />
        <el-table-column prop="speed" label="速率" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const fakeInterfaces = ref([
  { name: 'eth0', ip: '192.168.1.1/24', status: 'UP', speed: '1000 Mbps' },
  { name: 'eth1', ip: 'dhcp', status: 'UP', speed: '1000 Mbps' },
]);
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
