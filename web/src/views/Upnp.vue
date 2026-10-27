<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>
        <div style="display:flex; align-items:center; justify-content:space-between;">
          <span>UPnP / IGD 服务</span>
          <el-tag :type="status.running ? 'success' : 'info'" size="small">
            miniupnpd {{ status.running ? '运行中' : '未运行' }}
          </el-tag>
        </div>
      </template>
      <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-alert v-if="detail" :title="detail" :type="applied ? 'success' : 'warning'" show-icon :closable="false" style="margin-bottom: 12px;">
        <div v-if="!applied">配置已保存但未生效（通常为缺少权限或 miniupnpd 未安装）。</div>
      </el-alert>

      <el-form label-width="140px" style="max-width: 640px;">
        <el-form-item label="启用">
          <el-switch v-model="cfg.enabled" />
        </el-form-item>
        <el-form-item label="外网接口">
          <el-input v-model="cfg.external_iface" placeholder="eth0 / pppoe-wan" />
        </el-form-item>
        <el-form-item label="内网接口">
          <el-input v-model="cfg.internal_iface" placeholder="br-lan" />
        </el-form-item>
        <el-form-item label="允许网段">
          <el-input v-model="cfg.allow_cidr" placeholder="192.168.0.0/16" />
          <div style="color:#f56c6c; font-size:12px; line-height:1.4; margin-top:4px;">禁止填写 0.0.0.0/0 —— 那会把你内网的 UPnP 映射开放给整个互联网。</div>
        </el-form-item>
        <el-form-item label="最大映射数">
          <el-input-number v-model="cfg.max_mappings" :min="1" :max="1000" />
        </el-form-item>
        <el-form-item label="租约时长(秒)">
          <el-input-number v-model="cfg.lease_seconds" :min="60" :max="604800" :step="60" />
          <span style="margin-left:8px; color:#909399;">60 ~ 604800</span>
        </el-form-item>
        <el-form-item label="NAT-PMP">
          <el-switch v-model="cfg.natpmp" />
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
          <span>当前映射</span>
          <div>
            <el-button size="small" :loading="loadingMappings" @click="loadMappings">刷新映射</el-button>
            <el-button size="small" type="primary" @click="showAdd = !showAdd">手动添加映射</el-button>
          </div>
        </div>
      </template>
      <el-alert v-if="!status.installed" type="info" show-icon :closable="false" style="margin-bottom: 12px;"
        title="未检测到 upnpc 客户端，无法读取活动映射（后台需安装 miniupnpc）。" />

      <el-form v-if="showAdd" inline label-width="0" style="margin-bottom: 12px;">
        <el-form-item>
          <el-select v-model="newMapping.protocol" style="width: 100px;">
            <el-option label="tcp" value="tcp" /><el-option label="udp" value="udp" />
          </el-select>
        </el-form-item>
        <el-form-item><el-input v-model.number="newMapping.ext_port" placeholder="外端口" style="width: 110px;" /></el-form-item>
        <el-form-item><el-input v-model="newMapping.int_ip" placeholder="内网IP" style="width: 150px;" /></el-form-item>
        <el-form-item><el-input v-model.number="newMapping.int_port" placeholder="内端口" style="width: 110px;" /></el-form-item>
        <el-form-item><el-input v-model="newMapping.description" placeholder="描述(≤64)" style="width: 180px;" /></el-form-item>
        <el-form-item><el-input-number v-model="newMapping.lease_seconds" :min="60" :max="cfg.lease_seconds || 604800" placeholder="租约" style="width: 140px;" /></el-form-item>
        <el-form-item><el-button type="success" :loading="adding" @click="addMapping">添加</el-button></el-form-item>
      </el-form>

      <el-table :data="mappings" border style="width: 100%;">
        <el-table-column prop="protocol" label="协议" width="80" />
        <el-table-column prop="ext_port" label="外端口" width="100" />
        <el-table-column prop="int_ip" label="内网IP" min-width="140" />
        <el-table-column prop="int_port" label="内端口" width="90" />
        <el-table-column prop="description" label="描述" min-width="150">
          <template #default="{ row }">{{ row.description || '—' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="delMapping(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!mappings.length" description="暂无映射" />
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>生成的 nftables（router_upnp）</template>
      <el-input v-model="nftText" type="textarea" :rows="10" readonly style="font-family: monospace;" placeholder="保存/添加映射后显示实际 nft 回读" />
    </el-card>

    <el-card shadow="never">
      <template #header>miniupnpd 配置</template>
      <el-input v-model="miniupnpdText" type="textarea" :rows="10" readonly style="font-family: monospace;" placeholder="保存后显示实际写入的 miniupnpd.conf" />
      <el-descriptions v-if="status.installed !== undefined" :column="3" border size="small" style="margin-top: 12px;">
        <el-descriptions-item label="已安装">{{ status.installed ? '是' : '否' }}</el-descriptions-item>
        <el-descriptions-item label="活动映射数">{{ status.active_count ?? 0 }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ status.status || '—' }}</el-descriptions-item>
      </el-descriptions>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const cfg = ref<any>({
  enabled: true,
  external_iface: 'eth0',
  internal_iface: 'br-lan',
  allow_cidr: '',
  max_mappings: 100,
  lease_seconds: 3600,
  natpmp: false,
});
const mappings = ref<any[]>([]);
const nftText = ref('');
const miniupnpdText = ref('');
const error = ref('');
const detail = ref('');
const applied = ref(false);
const saving = ref(false);
const loading = ref(false);
const loadingMappings = ref(false);
const adding = ref(false);
const showAdd = ref(false);
const status = ref<any>({ installed: false, running: false, status: '', active_count: 0 });
const newMapping = ref<any>({
  protocol: 'tcp', ext_port: null, int_ip: '', int_port: null, description: '', lease_seconds: 3600,
});

async function loadConfig() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/upnp/config');
    cfg.value = res.data?.config || cfg.value;
    miniupnpdText.value = res.data?.miniupnpd || '';
    error.value = '';
  } catch (e: any) {
    error.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载配置失败';
  } finally {
    loading.value = false;
  }
}

async function loadMappings() {
  loadingMappings.value = true;
  try {
    const res = await api.get('/api/v1/upnp/mappings');
    mappings.value = res.data?.mappings || [];
    nftText.value = res.data?.nft || '';
  } catch (e: any) {
    error.value = e?.response?.data?.detail || e.message || '加载映射失败';
  } finally {
    loadingMappings.value = false;
  }
}

async function loadStatus() {
  try {
    const res = await api.get('/api/v1/upnp/status');
    status.value = res.data || {};
  } catch (e: any) {
    error.value = e?.response?.data?.detail || e.message || '读取状态失败';
  }
}

async function loadAll() {
  await Promise.all([loadConfig(), loadMappings(), loadStatus()]);
}

async function save() {
  saving.value = true;
  detail.value = '';
  applied.value = false;
  try {
    const res = await api.post('/api/v1/upnp/config', cfg.value);
    applied.value = !!res.data?.applied;
    detail.value = res.data?.detail || (applied.value ? 'miniupnpd 已重启' : '');
    miniupnpdText.value = res.data?.miniupnpd || miniupnpdText.value;
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

async function addMapping() {
  adding.value = true;
  try {
    const res = await api.post('/api/v1/upnp/mappings', newMapping.value);
    mappings.value = res.data?.mappings || [];
    nftText.value = res.data?.nft || nftText.value;
    if (res.data?.applied) ElMessage.success('映射已添加并下发');
    else ElMessage.warning('映射已保存，但 nft 未生效：' + (res.data?.detail || ''));
    showAdd.value = false;
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '添加失败';
    error.value = msg;
    ElMessage.error(msg);
  } finally {
    adding.value = false;
  }
}

async function delMapping(row: any) {
  try {
    const res = await api.delete('/api/v1/upnp/mappings', {
      data: { protocol: row.protocol, ext_port: row.ext_port },
    });
    mappings.value = res.data?.mappings || [];
    nftText.value = res.data?.nft || nftText.value;
    ElMessage.success('映射已删除');
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '删除失败';
    error.value = msg;
    ElMessage.error(msg);
  }
}

onMounted(loadAll);
</script>

<style scoped>
</style>
