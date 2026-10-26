<template>
  <el-container style="min-height: 100vh">
    <el-header style="padding:0; background:#1a1a2e; color:#fff;">
      <div style="display:flex; align-items:center; height:60px; padding:0 24px; gap:24px;">
        <strong style="font-size:18px;">router-re</strong>
        <nav style="display:flex; gap:16px;">
          <router-link to="/" style="color:#eee; text-decoration:none;">仪表盘</router-link>
          <router-link to="/multi-wan" style="color:#eee; text-decoration:none;">多WAN</router-link>
          <router-link to="/vpn" style="color:#eee; text-decoration:none;">VPN</router-link>
          <router-link to="/qos" style="color:#eee; text-decoration:none;">流量控制</router-link>
          <router-link to="/cloud" style="color:#eee; text-decoration:none;">云管理</router-link>
          <router-link to="/wireless" style="color:#eee; text-decoration:none;">无线AC</router-link>
        </nav>
      </div>
    </el-header>
    <el-main style="padding:0; background:#f5f7fa;">
      <router-view />
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
</script>
