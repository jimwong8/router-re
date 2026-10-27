<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>SNMP 服务端</span>
          <div>
            <el-button type="primary" link :loading="querying" @click="querySystem">查询系统信息</el-button>
            <el-button type="primary" size="small" :loading="saving" @click="apply">保存下发</el-button>
          </div>
        </div>
      </template>

      <el-form :model="form" label-width="110px">
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="端口"><el-input-number v-model="form.port" :min="1" :max="65535" /></el-form-item>
        <el-form-item label="位置"><el-input v-model="form.location" placeholder="如 Fangchenggang" /></el-form-item>
        <el-form-item label="联系人"><el-input v-model="form.contact" placeholder="如 admin@example.com" /></el-form-item>
      </el-form>

      <div style="margin-top:12px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
          <span>Community（v2c）</span>
          <el-button type="primary" link size="small" @click="addCommunity">添加</el-button>
        </div>
        <table style="width:100%; border-collapse:collapse">
          <thead><tr><th style="text-align:left;padding:4px">名称</th><th style="text-align:left;padding:4px">权限</th><th style="text-align:left;padding:4px">操作</th></tr></thead>
          <tbody>
            <tr v-for="(c, i) in form.communities" :key="i">
              <td><el-input v-model="c.name" size="small" placeholder="如 public" /></td>
              <td><el-select v-model="c.access" size="small" style="width:100%">
                <el-option label="ro" value="ro" /><el-option label="rw" value="rw" />
              </el-select></td>
              <td><el-button type="danger" link size="small" @click="form.communities.splice(i,1)">删除</el-button></td>
            </tr>
          </tbody>
        </table>
      </div>

      <div style="margin-top:16px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
          <span>SNMPv3 用户（<span style="color:#f56c6c">密码显示已打码</span>）</span>
          <el-button type="primary" link size="small" @click="addV3">添加</el-button>
        </div>
        <table style="width:100%; border-collapse:collapse">
          <thead><tr>
            <th style="text-align:left;padding:4px">名称</th><th style="text-align:left;padding:4px">认证协议</th>
            <th style="text-align:left;padding:4px">认证密码</th><th style="text-align:left;padding:4px">加密协议</th>
            <th style="text-align:left;padding:4px">加密密码</th><th style="text-align:left;padding:4px">操作</th>
          </tr></thead>
          <tbody>
            <tr v-for="(u, i) in form.v3_users" :key="i">
              <td><el-input v-model="u.name" size="small" /></td>
              <td><el-select v-model="u.auth_proto" size="small" style="width:100%">
                <el-option label="md5" value="md5" /><el-option label="sha" value="sha" /><el-option label="sha256" value="sha256" />
              </el-select></td>
              <td><el-input v-model="u.auth_pass" type="password" size="small" placeholder="≥8 位" /></td>
              <td><el-select v-model="u.priv_proto" size="small" style="width:100%">
                <el-option label="des" value="des" /><el-option label="aes" value="aes" />
              </el-select></td>
              <td><el-input v-model="u.priv_pass" type="password" size="small" placeholder="≥8 位" /></td>
              <td><el-button type="danger" link size="small" @click="form.v3_users.splice(i,1)">删除</el-button></td>
            </tr>
          </tbody>
        </table>
      </div>

      <div style="margin-top:16px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
          <span>Trap 目标（host:port 或 IP）</span>
          <el-button type="primary" link size="small" @click="addTrap">添加</el-button>
        </div>
        <div v-for="(t, i) in form.trap_targets" :key="i" style="display:flex; align-items:center; margin-bottom:6px">
          <el-input v-model="form.trap_targets[i]" size="small" placeholder="如 192.168.1.100:162" />
          <el-button type="danger" link size="small" @click="form.trap_targets.splice(i,1)">删除</el-button>
        </div>
      </div>
    </el-card>

    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>运行状态</span>
          <el-button type="primary" link size="small" @click="loadStatus">刷新</el-button>
        </div>
      </template>
      <el-descriptions :column="4" style="width:100%">
        <el-descriptions-item label="running">{{ status.running ?? '—' }}</el-descriptions-item>
        <el-descriptions-item label="port">{{ status.port ?? '—' }}</el-descriptions-item>
        <el-descriptions-item label="version">{{ status.version ?? '—' }}</el-descriptions-item>
        <el-descriptions-item label="config_exists">{{ status.config_exists ?? '—' }}</el-descriptions-item>
        <el-descriptions-item label="detail" v-if="status.detail">{{ status.detail }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>生成的 snmpd.conf</span>
          <span style="color:#f56c6c; font-size:12px">密码已打码</span>
        </div>
      </template>
      <pre style="background:#1e1e1e; color:#d4d4d4; padding:12px; border-radius:4px; min-height:80px; white-space:pre-wrap; word-break:break-all">{{ displaySnmpd }}</pre>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface Community { name: string; access: string }
interface V3User { name: string; auth_proto: string; auth_pass: string; priv_proto: string; priv_pass: string }
interface SnmpCfg {
  enabled: boolean;
  port: number;
  location: string;
  contact: string;
  communities: Community[];
  v3_users: V3User[];
  trap_targets: string[];
}

const form = ref<SnmpCfg>({
  enabled: false, port: 161, location: '', contact: '',
  communities: [], v3_users: [], trap_targets: [],
});
const status = ref<Record<string, string>>({});
const snmpdText = ref('');
const saving = ref(false);
const querying = ref(false);

const displaySnmpd = computed(() => {
  // 展示时把明文密码打码成 ***
  return snmpdText.value.replace(/(createUser \S+ (?:md5|sha|sha256) )\S+( (?:des|aes) )\S+/g, '$1***$2***');
});

function buildPayload(): SnmpCfg { return { ...form.value }; }

async function loadConfig() {
  try {
    const res = await api.get('/api/v1/snmp/config');
    const cfg = res.data?.config;
    if (cfg) {
      form.value = {
        enabled: !!cfg.enabled,
        port: cfg.port || 161,
        location: cfg.location || '',
        contact: cfg.contact || '',
        communities: cfg.communities || [],
        v3_users: cfg.v3_users || [],
        trap_targets: cfg.trap_targets || [],
      };
      snmpdText.value = res.data?.snmpd || '';
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function apply() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/snmp/config', buildPayload());
    const d = res.data || {};
    snmpdText.value = d.snmpd || '';
    if (d.applied) {
      ElMessage.success('下发成功');
    } else {
      ElMessage.error(d.detail || '下发失败');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

async function loadStatus() {
  try {
    const res = await api.get('/api/v1/snmp/status');
    status.value = res.data || {};
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function querySystem() {
  querying.value = true;
  try {
    const res = await api.post('/api/v1/snmp/query');
    ElMessage.info(res.data?.sysDescr || '无结果');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    querying.value = false;
  }
}

function addCommunity() { form.value.communities.push({ name: '', access: 'ro' }); }
function addV3() { form.value.v3_users.push({ name: '', auth_proto: 'sha', auth_pass: '', priv_proto: 'aes', priv_pass: '' }); }
function addTrap() { form.value.trap_targets.push(''); }

onMounted(() => { loadConfig(); loadStatus(); });
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; }
</style>
