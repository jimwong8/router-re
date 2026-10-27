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

onMounted(() => {
  loadConfig();
  loadSessions();
  loadStatus();
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