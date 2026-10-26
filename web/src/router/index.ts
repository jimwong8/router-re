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
