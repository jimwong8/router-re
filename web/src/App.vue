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
          <router-link to="/wireless" style="color:#eee; text-decoration:none;">无线设置</router-link>
          <router-link to="/clients" style="color:#eee; text-decoration:none;">终端</router-link>
          <router-link to="/interfaces" style="color:#eee; text-decoration:none;">接口配置</router-link>
          <router-link to="/static-route" style="color:#eee; text-decoration:none;">静态路由</router-link>
          <router-link to="/audit" style="color:#eee; text-decoration:none;">审计日志</router-link>
          <router-link to="/users" style="color:#eee; text-decoration:none;">用户管理</router-link>
          <router-link to="/radius" style="color:#eee; text-decoration:none;">RADIUS</router-link>
          <router-link to="/pppoe" style="color:#eee; text-decoration:none;">PPPoE服务</router-link>
          <router-link to="/l2" style="color:#eee; text-decoration:none;">VLAN/桥接</router-link>
          <router-link to="/dpi" style="color:#eee; text-decoration:none;">应用识别</router-link>
          <router-link to="/logs" style="color:#eee; text-decoration:none;">日志中心</router-link>
          <router-link to="/portal" style="color:#eee; text-decoration:none;">上网认证</router-link>
          <router-link to="/stats" style="color:#eee; text-decoration:none;">系统监控</router-link>
          <router-link to="/network" style="color:#eee; text-decoration:none;">端口映射/策略路由</router-link>
          <router-link to="/security" style="color:#eee; text-decoration:none;">安全管控</router-link>
          <router-link to="/services" style="color:#eee; text-decoration:none;">高级服务</router-link>
          <router-link to="/accounts" style="color:#eee; text-decoration:none;">认证计费</router-link>
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
