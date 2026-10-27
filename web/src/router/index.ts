import { createRouter, createWebHistory } from 'vue-router';
import Login from '../views/Login.vue';
import Dashboard from '../views/Dashboard.vue';
import MultiWan from '../views/MultiWan.vue';
import Vpn from '../views/Vpn.vue';
import Qos from '../views/Qos.vue';
import Cloud from '../views/Cloud.vue';
import Wireless from '../views/Wireless.vue';
import Clients from '../views/Clients.vue';
import Interface from '../views/Interface.vue';
import StaticRoute from '../views/StaticRoute.vue';
import Audit from '../views/Audit.vue';
import Radius from '../views/Radius.vue';
import Pppoe from '../views/Pppoe.vue';
import L2 from '../views/L2.vue';
import Dpi from '../views/Dpi.vue';
import DpiFlow from '../views/DpiFlow.vue';
import LogCenter from '../views/LogCenter.vue';
import Portal from '../views/Portal.vue';
import Stats from '../views/Stats.vue';
import Traffic from '../views/Traffic.vue';
import Network from '../views/Network.vue';
import Security from '../views/Security.vue';
import Services from '../views/Services.vue';
import Ipv6 from '../views/Ipv6.vue';
import Accounts from '../views/Accounts.vue';
import Users from '../views/Users.vue';
import Dns from '../views/Dns.vue';
import Dhcp from '../views/Dhcp.vue';
import Diag from '../views/Diag.vue';
import Nac from '../views/Nac.vue';
import Lb from '../views/Lb.vue';
import Remote from '../views/Remote.vue';
import Snmp from '../views/Snmp.vue';
import Upnp from '../views/Upnp.vue';
import L7 from '../views/L7.vue';
import Threat from '../views/Threat.vue';
import Ips from '../views/Ips.vue';
import Firmware from '../views/Firmware.vue';
import RoutingPolicy from '../views/RoutingPolicy.vue';
import ConfigSync from '../views/ConfigSync.vue';
import Diagnostics from '../views/Diagnostics.vue';
import Alerts from '../views/Alerts.vue';
import NetworkAdvanced from '../views/NetworkAdvanced.vue';
import CertManage from '../views/CertManage.vue';
import Quota from '../views/Quota.vue';

const routes = [
  { path: '/', component: Dashboard, meta: { requiresAuth: true } },
  { path: '/multi-wan', component: MultiWan, meta: { requiresAuth: true } },
  { path: '/vpn', component: Vpn, meta: { requiresAuth: true } },
  { path: '/qos', component: Qos, meta: { requiresAuth: true } },
  { path: '/cloud', component: Cloud, meta: { requiresAuth: true } },
  { path: '/wireless', component: Wireless, meta: { requiresAuth: true } },
  { path: '/clients', component: Clients, meta: { requiresAuth: true } },
  { path: '/interfaces', component: Interface, meta: { requiresAuth: true } },
  { path: '/static-route', component: StaticRoute, meta: { requiresAuth: true } },
  { path: '/audit', component: Audit, meta: { requiresAuth: true } },
  { path: '/radius', component: Radius, meta: { requiresAuth: true } },
  { path: '/pppoe', component: Pppoe, meta: { requiresAuth: true } },
  { path: '/l2', component: L2, meta: { requiresAuth: true } },
  { path: '/dpi', component: Dpi, meta: { requiresAuth: true } },
  { path: '/dpi-flow', component: DpiFlow, meta: { requiresAuth: true } },
  { path: '/logs', component: LogCenter, meta: { requiresAuth: true } },
  { path: '/portal', component: Portal, meta: { requiresAuth: true } },
  { path: '/services', component: Services, meta: { requiresAuth: true } },
  { path: '/ipv6', component: Ipv6, meta: { requiresAuth: true } },
  { path: '/stats', component: Stats, meta: { requiresAuth: true } },
  { path: '/traffic', component: Traffic, meta: { requiresAuth: true } },
  { path: '/network', component: Network, meta: { requiresAuth: true } },
  { path: '/security', component: Security, meta: { requiresAuth: true } },
  { path: '/accounts', component: Accounts, meta: { requiresAuth: true } },
  { path: '/users', component: Users, meta: { requiresAuth: true } },
  { path: '/dns', component: Dns, meta: { requiresAuth: true } },
  { path: "/dhcp", component: Dhcp, meta: { requiresAuth: true } },
  { path: "/nac", component: Nac, meta: { requiresAuth: true } },
  { path: "/lb", component: Lb, meta: { requiresAuth: true } },
  { path: '/diag', component: Diag, meta: { requiresAuth: true } },
  { path: '/remote', component: Remote, meta: { requiresAuth: true } },
  { path: '/snmp', component: Snmp, meta: { requiresAuth: true } },
  { path: '/upnp', component: Upnp, meta: { requiresAuth: true } },
  { path: '/l7', component: L7, meta: { requiresAuth: true } },
  { path: '/threat', component: Threat, meta: { requiresAuth: true } },
  { path: '/ips', component: Ips, meta: { requiresAuth: true } },
  { path: '/firmware', component: Firmware, meta: { requiresAuth: true } },
  { path: '/routing-policy', component: RoutingPolicy, meta: { requiresAuth: true } },
  { path: '/config-sync', component: ConfigSync, meta: { requiresAuth: true } },
  { path: '/diagnostics', component: Diagnostics, meta: { requiresAuth: true } },
  { path: '/alerts', component: Alerts, meta: { requiresAuth: true } },
  { path: '/network-advanced', component: NetworkAdvanced, meta: { requiresAuth: true } },
  { path: '/cert', component: CertManage, meta: { requiresAuth: true } },
  { path: '/quota', component: Quota, meta: { requiresAuth: true } },
  { path: '/login', component: Login },
  { path: '/:pathMatch(.*)*', redirect: '/login' },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('router_token');
  if (to.meta.requiresAuth && !token) {
    next('/login');
  } else if (to.path === '/login' && token) {
    next('/');
  } else {
    next();
  }
});

export default router;
