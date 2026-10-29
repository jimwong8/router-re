<template>
  <el-container class="app">
    <el-header v-if="!isLogin" class="topbar">
      <div class="brand">router-re</div>
      <button class="icon-btn" :title="collapsed ? '展开菜单' : '收起菜单'" @click="collapsed = !collapsed">
        {{ collapsed ? '☰' : '«' }}
      </button>
      <div class="spacer" />
      <span class="user">{{ username }}</span>
      <button class="icon-btn" @click="logout">退出</button>
    </el-header>

    <el-container class="body">
      <el-aside v-if="!isLogin" :width="collapsed ? '0px' : '232px'" class="sidebar">
        <el-menu
          class="nav-menu"
          :default-active="activePath"
          :default-openeds="openedGroups"
          router
          unique-opened
        >
          <el-sub-menu v-for="g in groups" :key="g.title" :index="g.title">
            <template #title>
              <span class="group-title">{{ g.title }}</span>
              <span class="group-count">{{ g.items.length }}</span>
            </template>
            <el-menu-item v-for="it in g.items" :key="it.path" :index="it.path">
              <span class="item-label">{{ it.label }}</span>
            </el-menu-item>
          </el-sub-menu>
        </el-menu>
      </el-aside>

      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

type NavItem = { path: string; label: string };
type NavGroup = { title: string; items: NavItem[] };

// 47 个页面按功能分成 6 组。此前是 47 个链接平铺在顶栏：总宽 1754px 超过
// 1440 视口，页面出现横向滚动条，品牌 router-re 被折成两行。
const groups: NavGroup[] = [
  {
    title: '总览',
    items: [
      { path: '/', label: '仪表盘' },
      { path: '/stats', label: '系统监控' },
      { path: '/traffic', label: '流量统计' },
      { path: '/clients', label: '终端' },
      { path: '/online-users', label: '在线用户' },
      { path: '/terminals', label: '终端列表' },
    ],
  },
  {
    title: '网络',
    items: [
      { path: '/interfaces', label: '接口配置' },
      { path: '/multi-wan', label: '多WAN' },
      { path: '/static-route', label: '静态路由' },
      { path: '/routing-policy', label: '策略路由' },
      { path: '/network', label: '端口映射/策略路由' },
      { path: '/l2', label: 'VLAN/桥接' },
      { path: '/ipv6', label: 'IPv6' },
      { path: '/dhcp', label: 'DHCP 服务' },
      { path: '/network-advanced', label: '网络高级' },
      { path: '/upnp', label: 'UPnP/端口映射' },
    ],
  },
  {
    title: '安全',
    items: [
      { path: '/ips', label: '入侵防御' },
      { path: '/security', label: '安全管控' },
      { path: '/dpi', label: '应用识别' },
      { path: '/dpi-flow', label: 'DPI 流表' },
      { path: '/l7', label: '应用层过滤' },
      { path: '/threat', label: '威胁情报' },
      { path: '/audit', label: '审计日志' },
      { path: '/nac', label: '接入控制' },
      { path: '/cert', label: '证书管理' },
    ],
  },
  {
    title: '认证',
    items: [
      { path: '/users', label: '用户管理' },
      { path: '/radius', label: 'RADIUS' },
      { path: '/pppoe', label: 'PPPoE 服务' },
      { path: '/portal', label: '上网认证' },
      { path: '/accounts', label: '认证计费' },
    ],
  },
  {
    title: '流控与高可用',
    items: [
      { path: '/qos', label: '流量控制' },
      { path: '/quota', label: '限速配额' },
      { path: '/lb', label: '负载均衡' },
      { path: '/linkcheck', label: '链路检测' },
      { path: '/vpn', label: 'VPN' },
    ],
  },
  {
    title: '系统与运维',
    items: [
      { path: '/firmware', label: '固件升级' },
      { path: '/config-sync', label: '配置备份' },
      { path: '/diagnostics', label: '系统诊断' },
      { path: '/diag', label: '网络诊断' },
      { path: '/logs', label: '日志中心' },
      { path: '/alerts', label: '告警规则' },
      { path: '/snmp', label: 'SNMP' },
      { path: '/services', label: '高级服务' },
      { path: '/remote', label: '远程与系统' },
      { path: '/dns', label: 'DNS 服务' },
      { path: '/wireless', label: '无线设置' },
      { path: '/cloud', label: '云管理' },
    ],
  },
];

const route = useRoute();
const router = useRouter();
const collapsed = ref(false);

const activePath = computed(() => route.path);
const isLogin = computed(() => route.path === '/login');
const username = ref(localStorage.getItem('router_user') || 'admin');

// 当前页所在分组默认展开，否则进了页面看不到高亮在哪
const openedGroups = computed(() =>
  groups.filter((g) => g.items.some((it) => it.path === route.path)).map((g) => g.title),
);

function logout() {
  localStorage.removeItem('router_token');
  localStorage.removeItem('router_user');
  router.push('/login');
}
</script>

<style scoped>
.app {
  min-height: 100vh;
}

.topbar {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 56px;
  padding: 0 18px;
  background: #1a1a2e;
  color: #fff;
  position: sticky;
  top: 0;
  z-index: 20;
}

.brand {
  font-size: 17px;
  font-weight: 700;
  letter-spacing: 0.4px;
  white-space: nowrap;
  flex: 0 0 auto;
  margin-right: 8px;
}

.icon-btn {
  background: transparent;
  border: 1px solid rgba(255, 255, 255, 0.22);
  color: #d8d8e8;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  min-width: 34px;
  padding: 8px 12px;
  transition: background 0.15s, color 0.15s, border-color 0.15s;
}

.icon-btn:hover {
  background: rgba(255, 255, 255, 0.12);
  color: #fff;
  border-color: rgba(255, 255, 255, 0.4);
}

.spacer {
  flex: 1 1 auto;
}

.user {
  color: #b8b8cc;
  font-size: 13px;
  white-space: nowrap;
}

.body {
  align-items: stretch;
  height: calc(100vh - 56px);
}

.sidebar {
  background: #1a1a2e;
  overflow-x: hidden;
  overflow-y: auto;
  transition: width 0.2s ease;
  border-right: 1px solid #26263f;
}

/* 顶栏与侧栏固定，只有内容区滚动——否则长菜单会把整页顶下去，
   顶栏滚出视口后连退出按钮都找不到 */

.nav-menu {
  background: #1a1a2e;
  border-right: none;
  --el-menu-bg-color: #1a1a2e;
  --el-menu-text-color: #c8c8dc;
  --el-menu-hover-bg-color: #24243c;
  --el-menu-active-color: #ffffff;
}

.nav-menu :deep(.el-sub-menu__title) {
  display: flex;
  align-items: center;
  height: 40px;
  line-height: 40px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.3px;
  color: #9d9dc0;
}

.nav-menu :deep(.el-sub-menu__title:hover) {
  background: #202036;
  color: #e8e8f5;
}

/* 子项处于激活态时，父分组标题也点亮，否则左侧只剩一条竖线看不出归属 */
.nav-menu :deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  color: #ffffff;
}

.group-title {
  flex: 1;
}

.group-count {
  font-size: 11px;
  color: #6a6a8c;
  margin-right: 14px;
  min-width: 18px;
  text-align: right;
  font-weight: 400;
}

/* 子菜单左侧引导线：让子项与所属分组有视觉从属关系，而不是像孤立浮块 */
.nav-menu :deep(.el-menu--inline) {
  border-left: 1px solid rgba(255, 255, 255, 0.08);
  margin-left: 18px;
}

.nav-menu :deep(.el-menu-item) {
  height: 36px;
  line-height: 36px;
  font-size: 13px;
  padding-left: 26px !important;
}

.nav-menu :deep(.el-menu-item.is-active) {
  background: #2d2d4d;
  color: #fff;
  border-left: 3px solid #4a7dff;
  padding-left: 23px !important;
}

/* 侧栏滚动条收细，避免 47 项展开后出现粗灰条破坏深色面板 */
.sidebar::-webkit-scrollbar {
  width: 6px;
}

.sidebar::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.28);
  border-radius: 3px;
}

.sidebar::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.45);
}

.sidebar::-webkit-scrollbar-track {
  background: rgba(255, 255, 255, 0.05);
}

.item-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main {
  padding: 0;
  background: #f5f7fa;
  height: 100%;
  overflow-x: hidden;
  overflow-y: auto;
}

@media (max-width: 820px) {
  .sidebar {
    display: none;
  }
}
</style>
