<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>
        <div style="display:flex; align-items:center; justify-content:space-between;">
          <span>DNS 代理与本地解析</span>
          <el-tag :type="status.running ? 'success' : 'info'" size="small">
            dnsmasq {{ status.running ? '运行中' : '未运行' }}
          </el-tag>
        </div>
      </template>
      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-alert v-if="detail" :title="detail" :type="applied ? 'success' : 'warning'" show-icon :closable="false" style="margin-bottom: 12px;">
        <div v-if="!applied">配置已保存但未生效（通常为缺少权限或 dnsmasq 未安装），请检查下方详情。</div>
      </el-alert>

      <el-form label-width="120px" style="max-width: 640px;">
        <el-form-item label="启用">
          <el-switch v-model="cfg.enabled" />
        </el-form-item>
        <el-form-item label="缓存大小">
          <el-input-number v-model="cfg.cache_size" :min="0" :max="1000000" :step="50" />
          <span style="margin-left:8px; color:#909399;">条记录，0 表示不限</span>
        </el-form-item>
        <el-form-item label="domain-needed">
          <el-switch v-model="cfg.domain_needed" />
          <span style="margin-left:8px; color:#909399;">不转发不含点的单标签名</span>
        </el-form-item>
        <el-form-item label="bogus-priv">
          <el-switch v-model="cfg.bogus_priv" />
          <span style="margin-left:8px; color:#909399;">对私有反向解析返回空（防泄漏）</span>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存并下发</el-button>
          <el-button :loading="loading" @click="loadAll">刷新</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>
        <div style="display:flex; align-items:center; justify-content:space-between;">
          <span>上游 DNS 服务器</span>
          <el-button size="small" @click="addUpstream">添加上游</el-button>
        </div>
      </template>
      <el-table :data="cfg.upstreams" border style="width: 100%;">
        <el-table-column prop="address" label="地址" min-width="220">
          <template #default="{ row }">
            <el-input v-model="row.address" placeholder="223.5.5.5 或 2400:3200::1" />
          </template>
        </el-table-column>
        <el-table-column prop="port" label="端口" width="140">
          <template #default="{ row }">
            <el-input-number v-model="row.port" :min="0" :max="65535" style="width:100%;" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ $index }">
            <el-button type="danger" size="small" @click="cfg.upstreams.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!cfg.upstreams.length" description="暂无上游，启用时至少需要一个" />
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>
        <div style="display:flex; align-items:center; justify-content:space-between;">
          <span>本地解析（address=/域名/IP）</span>
          <el-button size="small" @click="addHost">添加映射</el-button>
        </div>
      </template>
      <el-alert type="info" show-icon :closable="false" style="margin-bottom: 12px;"
        title="address=/name/ip 会把该域名及其所有子域名的查询固定返回此 IP（等同本地解析，作用于所有客户端）；下方 hosts 文本仅作对照展示，写入 /etc/hosts 只对本机生效。" />
      <el-table :data="cfg.hosts" border style="width: 100%;">
        <el-table-column prop="ip" label="IP" min-width="200">
          <template #default="{ row }">
            <el-input v-model="row.ip" placeholder="192.168.1.10" />
          </template>
        </el-table-column>
        <el-table-column label="域名（空格分隔多个）" min-width="260">
          <template #default="{ row }">
            <el-input v-model="row.namesText" placeholder="nas.lan files.lan" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ $index }">
            <el-button type="danger" size="small" @click="cfg.hosts.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!cfg.hosts.length" description="暂无本地映射" />
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>生成的 dnsmasq 配置</template>
      <el-input v-model="dnsmasqText" type="textarea" :rows="12" readonly
        style="font-family: monospace;" placeholder="保存后显示实际写入的配置" />
    </el-card>

    <el-card shadow="never">
      <template #header>测试解析</template>
      <el-form inline @submit.prevent="doResolve">
        <el-form-item label="域名">
          <el-input v-model="resolveName" placeholder="example.com" style="width: 260px;" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="resolving" @click="doResolve">解析</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="resolveError" :title="resolveError" type="error" show-icon :closable="false" style="margin-top: 8px;" />
      <div v-if="resolveResult" style="margin-top: 8px;">
        <span>{{ resolveResult.name }} → </span>
        <el-tag v-for="(ip, i) in resolveResult.ips" :key="i" style="margin-right:6px;">{{ ip }}</el-tag>
        <span v-if="!resolveResult.ips.length" style="color:#909399;">无记录</span>
      </div>
      <el-descriptions v-if="status && status.upstreams" :column="2" border size="small" style="margin-top: 12px;">
        <el-descriptions-item label="配置文件">{{ status.conf_path }}</el-descriptions-item>
        <el-descriptions-item label="配置文件存在">{{ status.conf_exists ? '是' : '否' }}</el-descriptions-item>
        <el-descriptions-item label="已配置上游数">{{ (status.upstreams || []).length }}</el-descriptions-item>
        <el-descriptions-item label="缓存大小">{{ status.cache_size }}</el-descriptions-item>
      </el-descriptions>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const cfg = ref<any>({
  upstreams: [{ address: '', port: 53 }],
  hosts: [{ ip: '', names: [], namesText: '' }],
  cache_size: 150,
  domain_needed: false,
  bogus_priv: false,
  enabled: true,
});
const dnsmasqText = ref('');
const error = ref('');
const detail = ref('');
const applied = ref(false);
const saving = ref(false);
const loading = ref(false);
const status = ref<any>({ running: false, status: '', conf_exists: false, conf_path: '' });
const resolveName = ref('');
const resolveResult = ref<any>(null);
const resolveError = ref('');
const resolving = ref(false);

// namesText is the editable form of names[]; sync before every save so the
// payload always carries the parsed list.
function syncNames() {
  cfg.value.hosts = cfg.value.hosts.map((h: any) => {
    const names = (h.namesText || '').split(/[\s,]+/).filter(Boolean);
    return { ip: h.ip, names, namesText: h.namesText };
  });
}

async function loadConfig() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/dns/config');
    const c = res.data?.config || {};
    cfg.value = {
      upstreams: c.upstreams || [],
      hosts: (c.hosts || []).map((h: any) => ({
        ip: h.ip,
        names: h.names || [],
        namesText: (h.names || []).join(' '),
      })),
      cache_size: c.cache_size ?? 150,
      domain_needed: !!c.domain_needed,
      bogus_priv: !!c.bogus_priv,
      enabled: !!c.enabled,
    };
    dnsmasqText.value = res.data?.dnsmasq || '';
    error.value = '';
  } catch (e: any) {
    error.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载配置失败';
  } finally {
    loading.value = false;
  }
}

async function loadStatus() {
  try {
    const res = await api.get('/api/v1/dns/status');
    status.value = res.data || {};
  } catch (e: any) {
    error.value = e?.response?.data?.detail || e.message || '读取状态失败';
  }
}

async function loadAll() {
  await Promise.all([loadConfig(), loadStatus()]);
}

async function save() {
  syncNames();
  saving.value = true;
  detail.value = '';
  applied.value = false;
  try {
    const res = await api.post('/api/v1/dns/config', cfg.value);
    applied.value = !!res.data?.applied;
    detail.value = res.data?.detail || (applied.value ? 'dnsmasq 已重启' : '');
    dnsmasqText.value = res.data?.dnsmasq || dnsmasqText.value;
    if (applied.value) ElMessage.success('已保存并下发');
    else ElMessage.warning('已保存，但未生效：' + detail.value);
    await loadStatus();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    error.value = msg;
    ElMessage.error(msg);
  } finally {
    saving.value = false;
  }
}

async function doResolve() {
  resolveResult.value = null;
  resolveError.value = '';
  if (!resolveName.value.trim()) {
    resolveError.value = '请输入域名';
    return;
  }
  resolving.value = true;
  try {
    const res = await api.post('/api/v1/dns/resolve', { name: resolveName.value.trim() });
    resolveResult.value = res.data;
  } catch (e: any) {
    resolveError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '解析失败';
  } finally {
    resolving.value = false;
  }
}

function addUpstream() {
  cfg.value.upstreams.push({ address: '', port: 53 });
}

function addHost() {
  cfg.value.hosts.push({ ip: '', names: [], namesText: '' });
}

onMounted(loadAll);
</script>

<style scoped>
</style>
