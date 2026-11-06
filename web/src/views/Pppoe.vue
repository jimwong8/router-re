<template>
  <div>
    <el-card shadow="never">
      <template #header>PPPoE 服务配置</template>
      <el-form label-width="120px" @submit.prevent>
        <el-form-item label="接口">
          <el-input v-model="cfg.interface" placeholder="如 eth0" style="width: 240px" />
        </el-form-item>
        <el-form-item label="本地 IP">
          <el-input v-model="cfg.local_ip" placeholder="10.0.0.1" style="width: 200px" />
        </el-form-item>
        <el-form-item label="IP 池起始">
          <el-input v-model="cfg.pool_start" placeholder="10.0.0.100" style="width: 200px" />
        </el-form-item>
        <el-form-item label="IP 池结束">
          <el-input v-model="cfg.pool_end" placeholder="10.0.0.200" style="width: 200px" />
        </el-form-item>
        <el-form-item label="DNS">
          <el-input v-model="dnsStr" placeholder="8.8.8.8, 223.5.5.5" style="width: 280px" />
        </el-form-item>
        <el-form-item label="服务名">
          <el-input v-model="cfg.service_name" placeholder="mypppoe" style="width: 200px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="submitConfig">提交配置</el-button>
        </el-form-item>
      </el-form>
      <el-divider>渲染配置</el-divider>
      <pre v-if="optionsText" class="cmd-output">{{ optionsText }}</pre>
      <pre v-if="secretsText" class="cmd-output">{{ secretsText }}</pre>
      <el-alert v-if="applyDetail" :title="applyDetail" :type="applyApplied ? 'success' : 'warning'" show-icon />
    </el-card>

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>认证用户 (chap-secrets)</template>
      <el-table :data="users" border style="width: 100%">
        <el-table-column prop="name" label="用户名" />
        <el-table-column prop="password" label="密码" />
        <el-table-column prop="ip" label="分配 IP" />
      </el-table>
      <el-form inline @submit.prevent style="margin-top: 12px">
        <el-form-item label="用户名">
          <el-input v-model="newUser.name" placeholder="alice" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="newUser.password" placeholder="secret" />
        </el-form-item>
        <el-form-item label="IP">
          <el-input v-model="newUser.ip" placeholder="10.0.0.101" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addUser">添加用户</el-button>
        </el-form-item>
      </el-form>
      <el-button type="primary" style="margin-top: 12px" @click="submitUsers">提交用户</el-button>
    </el-card>

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>活跃会话</template>
      <el-table :data="sessions" border style="width: 100%">
        <el-table-column prop="pid" label="PID" width="80" />
        <el-table-column prop="user" label="用户" />
        <el-table-column prop="ip" label="IP" />
      </el-table>
      <el-button type="primary" style="margin-top: 12px" @click="loadSessions">刷新</el-button>
    </el-card>

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>服务状态</template>
      <div>
        <el-tag :type="status === 'running' ? 'success' : 'danger'">{{ status }}</el-tag>
        <span style="margin-left: 12px; color: #666">{{ detail }}</span>
      </div>
      <el-button type="primary" style="margin-top: 12px" @click="loadStatus">刷新状态</el-button>
    </el-card>

    <!-- 多拨配置 -->
    <el-card shadow="never" style="margin-top: 16px;">
      <template #header>
        <div style="display:flex; align-items:center; gap:12px; justify-content:space-between; flex-wrap:wrap">
          <span>多拨配置</span>
          <div style="display:flex; align-items:center; gap:8px">
            <el-button size="small" type="primary" :loading="multiDialSaving" @click="saveMultiDial">保存</el-button>
            <el-button size="small" @click="loadMultiDial">加载</el-button>
          </div>
        </div>
      </template>

      <el-table :data="multiDialSessions" border style="width: 100%;">
        <el-table-column prop="name" label="会话名" width="140" />
        <el-table-column prop="iface" label="接口" width="100" />
        <el-table-column prop="username" label="用户名" width="140" />
        <el-table-column prop="password" label="密码" width="140">
          <template #default="{ row }">
            <el-input v-model="row.password" show-password style="width:120px" />
          </template>
        </el-table-column>
        <el-table-column prop="service" label="服务名" width="120" />
        <el-table-column prop="ac" label="AC 名称" width="120" />
        <el-table-column prop="metric" label="权重(metric)" width="100" />
        <el-table-column prop="mru" label="MRU" width="80" />
        <el-table-column prop="lcp_echo" label="LCP Echo" width="90" />
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ $index }">
            <el-button type="danger" size="small" @click="removeSession($index)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-form inline @submit.prevent="addSession" style="margin-top: 12px;">
        <el-form-item label="会话名">
          <el-input v-model="newSession.name" placeholder="如 wan1" style="width: 120px;" />
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="newSession.iface" placeholder="如 eth0" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="newSession.username" placeholder="用户名" style="width: 120px;" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="newSession.password" placeholder="密码" show-password style="width: 120px;" />
        </el-form-item>
        <el-form-item label="服务名">
          <el-input v-model="newSession.service" placeholder="可选" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="AC">
          <el-input v-model="newSession.ac" placeholder="可选" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="权重">
          <el-input-number v-model="newSession.metric" :min="0" :max="1000" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="MRU">
          <el-input-number v-model="newSession.mru" :min="0" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="LCP Echo">
          <el-input-number v-model="newSession.lcp_echo" :min="0" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="newSession.enabled" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary">添加会话</el-button>
        </el-form-item>
      </el-form>

      <el-alert v-if="multiDialError" :title="multiDialError" type="error" show-icon :closable="false" style="margin-top: 12px;" />
    </el-card>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const cfg = ref({ interface: '', local_ip: '', pool_start: '', pool_end: '', service_name: '' });
const users = ref<any[]>([]);
const newUser = ref({ name: '', password: '', ip: '' });
const sessions = ref<any[]>([]);
const status = ref('stopped');
const detail = ref('not checked');
const optionsText = ref('');
const secretsText = ref('');
const applyApplied = ref(false);
const applyDetail = ref('');

// ---- 多拨配置（/api/v1/pppoe/multidial） ----
const multiDialSessions = ref<any[]>([]);
const multiDialSaving = ref(false);
const multiDialError = ref('');
const originalPasswords = ref<Record<string, string>>({});
const newSession = ref<any>({
  name: '', iface: '', username: '', password: '', service: '', ac: '',
  metric: 0, mru: 0, lcp_echo: 0, enabled: true,
});

const dnsStr = computed<string>({
  get: () => (cfg.value as any).dns ? (cfg.value as any).dns.join(', ') : '',
  set: (v: string) => {
    (cfg.value as any).dns = v.split(',').map((s) => s.trim()).filter(Boolean);
  },
});

async function loadConfig() {
  try {
    const res = await api.get('/api/v1/pppoe/config');
    if (res.data?.config) {
      cfg.value = { ...cfg.value, ...res.data.config };
    }
    users.value = res.data?.users || [];
    optionsText.value = res.data?.options_text || '';
    secretsText.value = res.data?.secrets_text || '';
    applyApplied.value = !!res.data?.applied;
    applyDetail.value = res.data?.detail || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submitConfig() {
  try {
    const res = await api.post('/api/v1/pppoe/config', {
      config: cfg.value,
      users: users.value,
      apply_real: true,
    });
    applyApplied.value = !!res.data?.applied;
    applyDetail.value = res.data?.detail || '';
    optionsText.value = res.data?.options_text || '';
    secretsText.value = res.data?.secrets_text || '';
    ElMessage.success('配置已提交');
    await loadConfig();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

function addUser() {
  if (!newUser.value.name) {
    ElMessage.warning('用户名必填');
    return;
  }
  users.value.push({ ...newUser.value });
  newUser.value = { name: '', password: '', ip: '' };
}

async function submitUsers() {
  try {
    const res = await api.post('/api/v1/pppoe/users', { users: users.value });
    users.value = res.data?.users || users.value;
    ElMessage.success('用户已提交');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function loadSessions() {
  try {
    const res = await api.get('/api/v1/pppoe/sessions');
    sessions.value = res.data?.sessions || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function loadStatus() {
  try {
    const res = await api.get('/api/v1/pppoe/status');
    status.value = res.data?.running ? 'running' : 'stopped';
    detail.value = res.data?.detail || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function loadMultiDial() {
  multiDialError.value = '';
  try {
    const res = await api.get('/api/v1/pppoe/multidial');
    const sessions = res.data?.sessions || [];
    multiDialSessions.value = sessions.map((s: any) => ({
      name: s.name || '',
      iface: s.iface || '',
      username: s.username || '',
      password: s.password === '***' ? '' : s.password || '',
      service: s.service || '',
      ac: s.ac || '',
      metric: s.metric ?? 0,
      mru: s.mru ?? 0,
      lcp_echo: s.lcp_echo ?? 0,
      enabled: s.enabled !== false,
    }));
    // 初始化原始密码映射：GET 返回的 password 已 redact 为 "***"
    // 这里保留本地已有值，避免把掩码值当作真密码提交
    sessions.forEach((s: any) => {
      if (s.password === '***' && !originalPasswords.value[s.name]) {
        // 后端未返回真实密码，保留本地旧值（如有）
      }
    });
  } catch (e: any) {
    multiDialError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载多拨配置失败';
    ElMessage.error(multiDialError.value);
  }
}

async function saveMultiDial() {
  multiDialSaving.value = true;
  multiDialError.value = '';
  try {
    const sessions = multiDialSessions.value.map((s) => ({
      name: s.name,
      iface: s.iface,
      username: s.username,
      password: s.password || originalPasswords.value[s.name] || '',
      service: s.service,
      ac: s.ac,
      metric: s.metric,
      mru: s.mru,
      lcp_echo: s.lcp_echo,
      enabled: s.enabled,
    }));
    if (sessions.length === 0) {
      multiDialError.value = '至少需要一条会话配置';
      ElMessage.warning(multiDialError.value);
      return;
    }
    const res = await api.post('/api/v1/pppoe/multidial', { sessions });
    ElMessage.success(res.data?.detail || `已保存 ${res.data?.count || sessions.length} 条会话`);
    await loadMultiDial();
  } catch (e: any) {
    multiDialError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存多拨配置失败';
    ElMessage.error(multiDialError.value);
  } finally {
    multiDialSaving.value = false;
  }
}

function addSession() {
  const s = newSession.value;
  if (!s.name || !s.iface || !s.username) {
    ElMessage.warning('会话名、接口、用户名均必填');
    return;
  }
  if (multiDialSessions.value.some((x: any) => x.name === s.name)) {
    ElMessage.warning('会话名已存在');
    return;
  }
  if (s.iface === 'lo') {
    ElMessage.warning('接口不能为 lo');
    return;
  }
  multiDialSessions.value.push({ ...s });
  if (s.password) originalPasswords.value[s.name] = s.password;
  newSession.value = { name: '', iface: '', username: '', password: '', service: '', ac: '', metric: 0, mru: 0, lcp_echo: 0, enabled: true };
}

function removeSession(index: number) {
  const name = multiDialSessions.value[index]?.name;
  if (name) delete originalPasswords.value[name];
  multiDialSessions.value.splice(index, 1);
}

onMounted(() => {
  loadConfig();
  loadSessions();
  loadStatus();
  loadMultiDial();
});
</script>

<style scoped>
.cmd-output {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
}
</style>