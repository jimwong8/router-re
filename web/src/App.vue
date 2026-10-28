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
          <router-link to="/ips" style="color:#eee; text-decoration:none;">入侵防御</router-link>
          <router-link to="/users" style="color:#eee; text-decoration:none;">用户管理</router-link>
          <router-link to="/radius" style="color:#eee; text-decoration:none;">RADIUS</router-link>
          <router-link to="/pppoe" style="color:#eee; text-decoration:none;">PPPoE服务</router-link>
          <router-link to="/l2" style="color:#eee; text-decoration:none;">VLAN/桥接</router-link>
          <router-link to="/dpi" style="color:#eee; text-decoration:none;">应用识别</router-link>
          <router-link to="/dpi-flow" style="color:#eee; text-decoration:none;">DPI 流表</router-link>
          <router-link to="/logs" style="color:#eee; text-decoration:none;">日志中心</router-link>
          <router-link to="/portal" style="color:#eee; text-decoration:none;">上网认证</router-link>
          <router-link to="/stats" style="color:#eee; text-decoration:none;">系统监控</router-link>
          <router-link to="/traffic" style="color:#eee; text-decoration:none;">流量统计</router-link>
          <router-link to="/network" style="color:#eee; text-decoration:none;">端口映射/策略路由</router-link>
          <router-link to="/security" style="color:#eee; text-decoration:none;">安全管控</router-link>
          <router-link to="/dhcp" style="color:#eee; text-decoration:none;">DHCP 服务</router-link>
          <router-link to="/nac" style="color:#eee; text-decoration:none;">接入控制</router-link>
          <router-link to="/lb" style="color:#eee; text-decoration:none;">负载均衡</router-link>
          <router-link to="/firmware" style="color:#eee; text-decoration:none;">固件升级</router-link>
          <router-link to="/routing-policy" style="color:#eee; text-decoration:none;">策略路由</router-link>
          <router-link to="/config-sync" style="color:#eee; text-decoration:none;">配置备份</router-link>
          <router-link to="/diagnostics" style="color:#eee; text-decoration:none;">系统诊断</router-link>
          <router-link to="/alerts" style="color:#eee; text-decoration:none;">告警规则</router-link>
          <router-link to="/network-advanced" style="color:#eee; text-decoration:none;">网络高级</router-link>
          <router-link to="/cert" style="color:#eee; text-decoration:none;">证书管理</router-link>
          <router-link to="/online-users" style="color:#eee; text-decoration:none;">在线用户</router-link>
          <router-link to="/terminals" style="color:#eee; text-decoration:none;">终端列表</router-link>
          <router-link to="/snmp" style="color:#eee; text-decoration:none;">SNMP</router-link>
          <router-link to="/services" style="color:#eee; text-decoration:none;">高级服务</router-link>
          <router-link to="/ipv6" style="color:#eee; text-decoration:none;">IPv6</router-link>
          <router-link to="/diag" style="color:#eee; text-decoration:none;">网络诊断</router-link>
          <router-link to="/linkcheck" style="color:#eee; text-decoration:none;">链路检测</router-link>
          <router-link to="/remote" style="color:#eee; text-decoration:none;">远程与系统</router-link>
          <router-link to="/accounts" style="color:#eee; text-decoration:none;">认证计费</router-link>
          <router-link to="/dns" style="color:#eee; text-decoration:none;">DNS 服务</router-link>
          <router-link to="/upnp" style="color:#eee; text-decoration:none;">UPnP/端口自动映射</router-link>
          <router-link to="/l7" style="color:#eee; text-decoration:none;">应用层过滤</router-link>
          <router-link to="/threat" style="color:#eee; text-decoration:none;">威胁情报</router-link>
          <router-link to="/quota" style="color:#eee; text-decoration:none;">限速配额</router-link>
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
