<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>DHCP 服务端</span>
          <el-button type="primary" size="small" :loading="saving" @click="apply">保存下发</el-button>
        </div>
      </template>

      <el-form :model="form" label-width="110px">
        <el-form-item label="接口"><el-input v-model="form.interface" placeholder="如 lan" /></el-form-item>
        <el-form-item label="地址池"><el-input v-model="form.range_start" placeholder="起始" style="width:38%" />
          <el-input v-model="form.range_end" placeholder="结束" style="width:38%; margin-left:8px" /></el-form-item>
        <el-form-item label="子网掩码"><el-input v-model="form.netmask" placeholder="如 255.255.255.0" /></el-form-item>
        <el-form-item label="租期"><el-input v-model="form.lease_time" placeholder="如 12h / 30m" />
          <span class="el-form-item__error" v-if="false"></span>
          <span style="color:#909399; font-size:12px">格式: 数字+单位(s/m/h/d)</span>
        </el-form-item>
        <el-form-item label="网关"><el-input v-model="form.gateway" /></el-form-item>
        <el-form-item label="DNS（逗号分隔）"><el-input v-model="form.dns" placeholder="如 223.5.5.5,114.114.114.114" /></el-form-item>
        <el-form-item label="域名"><el-input v-model="form.domain" placeholder="如 router.local" /></el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>

      <div style="margin-top:12px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
          <span>静态租约</span>
          <el-button type="primary" link size="small" @click="addLease">添加</el-button>
        </div>
        <table style="width:100%; border-collapse:collapse">
          <thead>
            <tr>
              <th style="text-align:left;padding:4px">MAC</th>
              <th style="text-align:left;padding:4px">IP</th>
              <th style="text-align:left;padding:4px">主机名</th>
              <th style="text-align:left;padding:4px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(l, i) in form.static_leases" :key="i">
              <td><el-input v-model="l.mac" size="small" placeholder="aa:bb:cc:dd:ee:ff" /></td>
              <td><el-input v-model="l.ip" size="small" placeholder="192.168.1.50" /></td>
              <td><el-input v-model="l.hostname" size="small" /></td>
              <td><el-button type="danger" link size="small" @click="form.static_leases.splice(i,1)">删除</el-button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </el-card>


    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>DHCP 配置（/api/v1/dhcp/config）</span>
          <el-button type="primary" size="small" :loading="configSaving" @click="saveDhcpConfig">保存下发</el-button>
        </div>
      </template>
      <el-form :model="configForm" label-width="110px">
        <el-form-item label="接口"><el-input v-model="configForm.interface" placeholder="如 lan" /></el-form-item>
        <el-form-item label="地址池"><el-input v-model="configForm.range_start" placeholder="起始" style="width:38%" />
          <el-input v-model="configForm.range_end" placeholder="结束" style="width:38%; margin-left:8px" /></el-form-item>
        <el-form-item label="子网掩码"><el-input v-model="configForm.netmask" placeholder="如 255.255.255.0" /></el-form-item>
        <el-form-item label="租期"><el-input v-model="configForm.lease_time" placeholder="如 12h / 30m" />
          <span style="color:#909399; font-size:12px; margin-left:8px">格式: 数字+单位(s/m/h/d)</span></el-form-item>
        <el-form-item label="网关"><el-input v-model="configForm.gateway" /></el-form-item>
        <el-form-item label="DNS（逗号分隔）"><el-input v-model="configForm.dns" placeholder="如 223.5.5.5,114.114.114.114" /></el-form-item>
        <el-form-item label="域名"><el-input v-model="configForm.domain" placeholder="如 router.local" /></el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="configForm.enabled" />
        </el-form-item>
      </el-form>
      <div v-if="configDnsmasq" style="margin-top:12px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
          <span>生成的 dnsmasq 配置</span>
          <el-button type="primary" link size="small" @click="copyConfigText(configDnsmasq)">复制</el-button>
        </div>
        <pre style="background:#1e1e1e; color:#d4d4d4; padding:12px; border-radius:4px; min-height:80px; white-space:pre-wrap; word-break:break-all">{{ configDnsmasq }}</pre>
      </div>
      <el-alert v-if="configError" :title="configError" type="error" :closable="false" style="margin:12px 0" />
    </el-card>

    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>DHCPv6 服务端</span>
          <el-button type="primary" size="small" :loading="dhcpV6Saving" @click="saveDhcpV6">保存下发</el-button>
        </div>
      </template>
      <el-form :model="dhcpV6Form" label-width="110px">
        <el-form-item label="v6 起始"><el-input v-model="dhcpV6Form.v6_range_start" placeholder="2001:db8::10" /></el-form-item>
        <el-form-item label="v6 结束"><el-input v-model="dhcpV6Form.v6_range_end" placeholder="2001:db8::ff" /></el-form-item>
        <el-form-item label="前缀长度"><el-input v-model.number="dhcpV6Form.v6_prefix_len" placeholder="64" /></el-form-item>
        <el-form-item label="启用 RA"><el-switch v-model="dhcpV6Form.enable_ra" /></el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>运行状态</span>
          <el-button type="primary" link size="small" @click="loadStatus">刷新</el-button>
        </div>
      </template>
      <el-descriptions :data="status" border style="width:100%">
        <el-descriptions-item label="running">{{ status.running ?? '—' }}</el-descriptions-item>
        <el-descriptions-item label="config_exists">{{ status.config_exists ?? '—' }}</el-descriptions-item>
        <el-descriptions-item label="detail">{{ status.detail ?? '—' }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>生成的 dnsmasq 配置</span>
          <el-button type="primary" link size="small" :loading="copying" @click="copyConfig">复制</el-button>
        </div>
      </template>
      <pre style="background:#1e1e1e; color:#d4d4d4; padding:12px; border-radius:4px; min-height:80px; white-space:pre-wrap; word-break:break-all">{{ dnsmasqText }}</pre>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface StaticLease { mac: string; ip: string; hostname: string }

interface ServerCfg {
  interface: string;
  range_start: string;
  range_end: string;
  netmask: string;
  lease_time: string;
  gateway: string;
  dns: string;
  domain: string;
  static_leases: StaticLease[];
  enabled: boolean;
}

const form = ref<ServerCfg>({
  interface: 'lan',
  range_start: '',
  range_end: '',
  netmask: '255.255.255.0',
  lease_time: '12h',
  gateway: '',
  dns: '',
  domain: '',
  static_leases: [],
  enabled: false,
});
const status = ref<Record<string, string>>({});
const dhcpV6Saving = ref(false);
const dhcpV6Form = ref({ v6_range_start: "", v6_range_end: "", v6_prefix_len: 64, enable_ra: false });
const dnsmasqText = ref('');
const saving = ref(false);
const copying = ref(false);
const configSaving = ref(false);
const configDnsmasq = ref('');
const configError = ref('');
const configForm = ref<ServerCfg>({
  interface: 'lan',
  range_start: '',
  range_end: '',
  netmask: '255.255.255.0',
  lease_time: '12h',
  gateway: '',
  dns: '',
  domain: '',
  static_leases: [],
  enabled: false,
});

function toDnsArray(dns: string): string[] {
  return dns ? dns.split(',').map((s) => s.trim()).filter(Boolean) : [];
}

function buildPayload(): Record<string, any> {
  const c = form.value;
  return {
    interface: c.interface,
    range_start: c.range_start,
    range_end: c.range_end,
    netmask: c.netmask,
    lease_time: c.lease_time,
    gateway: c.gateway,
    dns: toDnsArray(c.dns),
    domain: c.domain,
    static_leases: c.static_leases,
    enabled: c.enabled,
  };
}

async function loadDhcpConfig() {
  configError.value = '';
  try {
    const res = await api.get('/api/v1/dhcp/config');
    const cfg = res.data?.config;
    if (cfg) {
      configForm.value = {
        interface: cfg.interface || '',
        range_start: cfg.range_start || '',
        range_end: cfg.range_end || '',
        netmask: cfg.netmask || '',
        lease_time: cfg.lease_time || '',
        gateway: cfg.gateway || '',
        dns: (cfg.dns || []).join(','),
        domain: cfg.domain || '',
        static_leases: cfg.static_leases || [],
        enabled: !!cfg.enabled,
      };
      configDnsmasq.value = res.data?.dnsmasq || '';
    }
  } catch (e: any) {
    configError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(configError.value);
  }
}

async function saveDhcpConfig() {
  configSaving.value = true;
  configError.value = '';
  try {
    const c = configForm.value;
    const payload = {
      interface: c.interface,
      range_start: c.range_start,
      range_end: c.range_end,
      netmask: c.netmask,
      lease_time: c.lease_time,
      gateway: c.gateway,
      dns: toDnsArray(c.dns),
      domain: c.domain,
      static_leases: c.static_leases,
      enabled: c.enabled,
    };
    const res = await api.post('/api/v1/dhcp/config', payload);
    const d = res.data || {};
    configDnsmasq.value = d.dnsmasq || '';
    if (d.applied) {
      ElMessage.success('下发成功');
    } else {
      ElMessage.error(d.detail || '下发失败');
    }
  } catch (e: any) {
    configError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(configError.value);
  } finally {
    configSaving.value = false;
  }
}

function copyConfigText(text: string) {
  navigator.clipboard.writeText(text || '').then(() => {
    ElMessage.success('已复制到剪贴板');
  }).catch(() => {
    ElMessage.error('复制失败');
  });
}
async function loadConfig() {
  try {
    const res = await api.get('/api/v1/dhcp/server');
    const cfg = res.data?.config;
    if (cfg) {
      form.value = {
        interface: cfg.interface || '',
        range_start: cfg.range_start || '',
        range_end: cfg.range_end || '',
        netmask: cfg.netmask || '',
        lease_time: cfg.lease_time || '',
        gateway: cfg.gateway || '',
        dns: (cfg.dns || []).join(','),
        domain: cfg.domain || '',
        static_leases: cfg.static_leases || [],
        enabled: !!cfg.enabled,
      };
      dnsmasqText.value = res.data?.dnsmasq || '';
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function apply() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/dhcp/server', buildPayload());
    const d = res.data || {};
    dnsmasqText.value = d.dnsmasq || '';
    if (d.applied) {
      ElMessage.success('下发成功');
    } else {
      dnsmasqText.value = d.dnsmasq || dnsmasqText.value;
      ElMessage.error(d.detail || '下发失败');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

async function loadDhcpV6() {
  try {
    const res = await api.get('/api/v1/dhcp/v6');
    const cfg = res.data?.config || res.data || {};
    dhcpV6Form.value = {
      v6_range_start: cfg.v6_range_start || '',
      v6_range_end: cfg.v6_range_end || '',
      v6_prefix_len: cfg.v6_prefix_len || 64,
      enable_ra: !!cfg.enable_ra,
    };
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function saveDhcpV6() {
  dhcpV6Saving.value = true;
  try {
    await api.post('/api/v1/dhcp/v6', dhcpV6Form.value);
    ElMessage.success('DHCPv6 下发成功');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    dhcpV6Saving.value = false;
  }
}

async function loadStatus() {
  try {
    const res = await api.get('/api/v1/dhcp/status');
    status.value = res.data || {};
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

function addLease() {
  form.value.static_leases.push({ mac: '', ip: '', hostname: '' });
}

async function copyConfig() {
  copying.value = true;
  try {
    await navigator.clipboard.writeText(dnsmasqText.value || '');
    ElMessage.success('已复制到剪贴板');
  } catch {
    ElMessage.error('复制失败');
  } finally {
    copying.value = false;
  }
}

onMounted(() => { loadConfig(); loadDhcpV6(); loadStatus(); loadDhcpConfig(); });
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; }
</style>
