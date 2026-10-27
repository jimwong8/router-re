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
import LogCenter from '../views/LogCenter.vue';
import Portal from '../views/Portal.vue';
import Stats from '../views/Stats.vue';
import Network from '../views/Network.vue';
import Security from '../views/Security.vue';
import Services from '../views/Services.vue';
import Accounts from '../views/Accounts.vue';
import Users from '../views/Users.vue';

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
  { path: '/logs', component: LogCenter, meta: { requiresAuth: true } },
  { path: '/portal', component: Portal, meta: { requiresAuth: true } },
  { path: '/services', component: Services, meta: { requiresAuth: true } },
  { path: '/stats', component: Stats, meta: { requiresAuth: true } },
  { path: '/network', component: Network, meta: { requiresAuth: true } },
  { path: '/security', component: Security, meta: { requiresAuth: true } },
  { path: '/accounts', component: Accounts, meta: { requiresAuth: true } },
  { path: '/users', component: Users, meta: { requiresAuth: true } },
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
