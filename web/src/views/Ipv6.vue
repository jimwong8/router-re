<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>IPv6 地址</template>
      <el-alert v-if="addrError" :title="addrError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form inline @submit.prevent="addAddr">
        <el-form-item label="接口">
          <el-input v-model="newAddr.iface" placeholder="ens18" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="IPv6 地址">
          <el-input v-model="newAddr.addr" placeholder="2001:db8::1" style="width: 200px;" />
        </el-form-item>
        <el-form-item label="前缀">
          <el-input-number v-model="newAddr.prefix_len" :min="1" :max="128" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="addrSaving" @click="addAddr">添加</el-button>
          <el-button @click="loadAddrs">刷新</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="addrs" border style="width: 100%; margin-top: 8px;">
        <el-table-column prop="iface" label="接口" width="120" />
        <el-table-column prop="addr" label="地址" />
        <el-table-column prop="prefix_len" label="前缀" width="80" />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="delAddr(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!addrs.length" description="暂无全局 IPv6 地址" />
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>Router Advertisement (radvd)</template>
      <el-alert v-if="raError" :title="raError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form label-width="140px" style="max-width: 560px;">
        <el-form-item label="启用">
          <el-switch v-model="ra.enabled" />
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="ra.iface" style="width: 180px;" />
        </el-form-item>
        <el-form-item label="Managed 标志">
          <el-switch v-model="ra.managed_flag" />
        </el-form-item>
        <el-form-item label="Other 标志">
          <el-switch v-model="ra.other_flag" />
        </el-form-item>
        <el-form-item label="最大间隔(秒)">
          <el-input-number v-model="ra.max_interval" :min="4" :max="1800" />
        </el-form-item>
        <el-form-item label="前缀">
          <el-input v-model="ra.prefix" placeholder="2001:db8:1::" style="width: 200px;" />
        </el-form-item>
        <el-form-item label="前缀长度">
          <el-input-number v-model="ra.prefix_len" :min="1" :max="128" />
        </el-form-item>
        <el-form-item label="有效生命周期(秒)">
          <el-input-number v-model="ra.valid_lifetime" :min="1" />
        </el-form-item>
        <el-form-item label="首选生命周期(秒)">
          <el-input-number v-model="ra.preferred_lifetime" :min="1" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="raSaving" @click="saveRA">保存下发</el-button>
          <el-button @click="loadRA">加载</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="raDetail" :title="raDetail" :type="raApplied ? 'success' : 'warning'" show-icon :closable="false" style="margin-top: 8px;" />
      <el-collapse v-if="radvdText" style="margin-top: 8px;">
        <el-collapse-item title="radvd.conf" name="radvd">
          <pre style="white-space: pre-wrap;">{{ radvdText }}</pre>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>DHCPv6 服务端</template>
      <el-form label-width="140px" style="max-width: 560px;">
        <el-form-item label="启用 RA">
          <el-switch v-model="dhcpV6.enable_ra" />
        </el-form-item>
        <el-form-item label="v6_range_start">
          <el-input v-model="dhcpV6.v6_range_start" placeholder="2001:db8::100" style="width: 220px;" />
        </el-form-item>
        <el-form-item label="v6_range_end">
          <el-input v-model="dhcpV6.v6_range_end" placeholder="2001:db8::200" style="width: 220px;" />
        </el-form-item>
        <el-form-item label="v6_prefix_len">
          <el-input-number v-model="dhcpV6.v6_prefix_len" :min="1" :max="128" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="dhcpV6Saving" @click="saveDhcpV6">保存</el-button>
          <el-button @click="loadDhcpV6">加载</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>ND 参数（邻居发现 sysctl）</template>
      <el-alert v-if="ndError" :title="ndError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form label-width="160px" style="max-width: 620px;">
        <el-form-item label="接口">
          <el-input v-model="nd.config.iface" placeholder="ens18" style="width: 180px;" @change="loadND" />
        </el-form-item>
        <el-form-item label="ra_interval_min(秒)">
          <el-input-number v-model="nd.config.ra_interval_min" :min="0" :max="1350" />
          <span class="hint">0 = 不修改该项</span>
        </el-form-item>
        <el-form-item label="ra_interval_max(秒)">
          <el-input-number v-model="nd.config.ra_interval_max" :min="0" :max="1800" />
          <span class="hint">0 = 不修改该项</span>
        </el-form-item>
        <el-form-item label="hop_limit">
          <el-input-number v-model="nd.config.hop_limit" :min="0" :max="255" />
        </el-form-item>
        <el-form-item label="mtu">
          <el-input-number v-model="nd.config.mtu" :min="0" :max="65535" :step="100" />
          <span class="hint">0 = 不修改；有效范围 1280-65535</span>
        </el-form-item>
        <el-form-item label="reachable_time(ms)">
          <el-input-number v-model="nd.config.reachable_time_ms" :min="0" :max="3600000" :step="1000" />
        </el-form-item>
        <el-form-item label="retrans_timer(ms)">
          <el-input-number v-model="nd.config.retrans_timer_ms" :min="0" :max="3600000" :step="1000" />
        </el-form-item>
        <el-form-item label="dad_transmits">
          <el-input-number v-model="nd.config.dad_transmits" :min="0" :max="10" />
        </el-form-item>
        <el-form-item label="accept_ra">
          <el-switch v-model="nd.config.accept_ra" />
        </el-form-item>
        <el-form-item label="forwarding">
          <el-switch v-model="nd.config.forwarding" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="ndSaving" @click="saveND">保存下发</el-button>
          <el-button @click="loadND">刷新</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="ndResult" :title="ndResult" :type="ndApplied ? 'success' : 'warning'" show-icon :closable="false" style="margin-top: 8px;" />
      <el-collapse v-if="Object.keys(nd.live).length" style="margin-top: 8px;">
        <el-collapse-item title="当前内核实时值 (live)" name="ndlive">
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item v-for="(v, k) in nd.live" :key="k" :label="String(k)">{{ v }}</el-descriptions-item>
          </el-descriptions>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>前缀委派 PD (dhcp6c)</template>
      <el-alert v-if="pdError" :title="pdError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form label-width="160px" style="max-width: 620px;">
        <el-form-item label="启用">
          <el-switch v-model="pd.config.enabled" />
        </el-form-item>
        <el-form-item label="上游接口">
          <el-input v-model="pd.config.upstream_iface" placeholder="ppp0" style="width: 180px;" />
        </el-form-item>
        <el-form-item label="下游接口">
          <el-input v-model="pd.config.downstream_iface" placeholder="ens18" style="width: 180px;" />
        </el-form-item>
        <el-form-item label="prefix_len">
          <el-input-number v-model="pd.config.prefix_len" :min="32" :max="64" />
        </el-form-item>
        <el-form-item label="subnet_id_bits">
          <el-input-number v-model="pd.config.subnet_id_bits" :min="0" :max="16" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="pdSaving" @click="savePD">保存下发</el-button>
          <el-button @click="loadPD">加载</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="pdResult" :title="pdResult" :type="pdApplied ? 'success' : 'warning'" show-icon :closable="false" style="margin-top: 8px;" />
      <el-collapse v-if="pd.dhcp6c" style="margin-top: 8px;">
        <el-collapse-item title="dhcp6c.conf 预览" name="pdconf">
          <pre style="white-space: pre-wrap;">{{ pd.dhcp6c }}</pre>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-card shadow="never">
      <template #header>IPv6 状态</template>
      <el-descriptions :column="1" border>
        <el-descriptions-item label="IPv6 启用">{{ status.ipv6_enabled || '-' }}</el-descriptions-item>
        <el-descriptions-item label="地址数">{{ status.addr_count || '-' }}</el-descriptions-item>
        <el-descriptions-item label="radvd 运行">{{ status.radvd_running || '-' }}</el-descriptions-item>
        <el-descriptions-item label="转发">{{ status.forwarding || '-' }}</el-descriptions-item>
      </el-descriptions>
      <el-button size="small" style="margin-top: 8px;" @click="loadStatus">刷新状态</el-button>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const addrs = ref<any[]>([]);
const newAddr = ref<any>({ iface: '', addr: '', prefix_len: 64 });
const addrError = ref('');
const addrSaving = ref(false);

const ra = ref<any>({ iface: '', enabled: true, managed_flag: false, other_flag: false, max_interval: 600, prefix: '', prefix_len: 64, valid_lifetime: 86400, preferred_lifetime: 14400 });
const raError = ref('');
const raSaving = ref(false);
const radvdText = ref('');
const raApplied = ref(false);
const raDetail = ref('');

const status = ref<any>({});
const dhcpV6 = ref<any>({ v6_range_start: '', v6_range_end: '', v6_prefix_len: 64, enable_ra: false });
const dhcpV6Saving = ref(false);

// ND：GET /api/v1/ipv6/nd?iface=... -> {config, live}；POST raw NDConfig -> {applied, config, detail}
const nd = ref<any>({ config: { iface: 'ens18', ra_interval_min: 0, ra_interval_max: 0, hop_limit: 0, mtu: 0, reachable_time_ms: 0, retrans_timer_ms: 0, dad_transmits: 1, accept_ra: false, forwarding: true }, live: {} });
const ndError = ref('');
const ndSaving = ref(false);
const ndApplied = ref(false);
const ndResult = ref('');

function errMsg(e: any, fallback: string): string {
  return e?.response?.data?.detail || e?.response?.data?.message || e.message || fallback;
}

async function loadND() {
  ndError.value = '';
  try {
    const res = await api.get('/api/v1/ipv6/nd', { params: { iface: nd.value.config.iface || 'ens18' } });
    if (res.data?.config) nd.value.config = { ...nd.value.config, ...res.data.config };
    nd.value.live = res.data?.live || {};
    if (nd.value.live?.error) ndError.value = String(nd.value.live.error);
  } catch (e: any) {
    ndError.value = errMsg(e, '加载 ND 配置失败');
    ElMessage.error(ndError.value);
  }
}

async function saveND() {
  ndSaving.value = true;
  ndError.value = '';
  try {
    const res = await api.post('/api/v1/ipv6/nd', nd.value.config);
    ndApplied.value = !!res.data?.applied;
    ndResult.value = res.data?.detail || (res.data?.applied ? 'ND 参数已下发并通过回读确认' : '未下发');
    if (res.data?.config) nd.value.config = { ...nd.value.config, ...res.data.config };
    if (res.data?.applied) ElMessage.success('ND 参数已下发');
    else ElMessage.warning(ndResult.value);
    await loadND();
  } catch (e: any) {
    ndError.value = errMsg(e, '保存 ND 配置失败');
    ElMessage.error(ndError.value);
  } finally {
    ndSaving.value = false;
  }
}

// PD：GET /api/v1/ipv6/pd -> {config, dhcp6c}；POST raw PDConfig -> {applied, config, dhcp6c, detail}
const pd = ref<any>({ config: { upstream_iface: '', downstream_iface: 'ens18', prefix_len: 60, subnet_id_bits: 4, enabled: false }, dhcp6c: '' });
const pdError = ref('');
const pdSaving = ref(false);
const pdApplied = ref(false);
const pdResult = ref('');

async function loadPD() {
  pdError.value = '';
  try {
    const res = await api.get('/api/v1/ipv6/pd');
    if (res.data?.config) pd.value.config = { ...pd.value.config, ...res.data.config };
    pd.value.dhcp6c = res.data?.dhcp6c || '';
  } catch (e: any) {
    pdError.value = errMsg(e, '加载 PD 配置失败');
    ElMessage.error(pdError.value);
  }
}

async function savePD() {
  pdSaving.value = true;
  pdError.value = '';
  try {
    const res = await api.post('/api/v1/ipv6/pd', pd.value.config);
    pdApplied.value = !!res.data?.applied;
    pdResult.value = res.data?.detail || (res.data?.applied ? 'PD 配置已写入 dhcp6c.conf 并重启 dhcp6c' : '未下发');
    if (res.data?.config) pd.value.config = { ...pd.value.config, ...res.data.config };
    pd.value.dhcp6c = res.data?.dhcp6c || pd.value.dhcp6c;
    if (res.data?.applied) ElMessage.success('PD 配置已下发');
    else ElMessage.warning(pdResult.value);
  } catch (e: any) {
    pdError.value = errMsg(e, '保存 PD 配置失败');
    ElMessage.error(pdError.value);
  } finally {
    pdSaving.value = false;
  }
}

async function loadAddrs() {
  addrError.value = '';
  try {
    const res = await api.get('/api/v1/ipv6/addresses');
    addrs.value = res.data?.addresses || [];
  } catch (e: any) {
    addrError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载地址失败';
    ElMessage.error(addrError.value);
  }
}

async function addAddr() {
  addrSaving.value = true;
  addrError.value = '';
  try {
    const res = await api.post('/api/v1/ipv6/addresses', { addresses: [newAddr.value] });
    addrs.value = res.data?.addresses || addrs.value;
    ElMessage.success('地址已添加');
    newAddr.value = { iface: '', addr: '', prefix_len: 64 };
  } catch (e: any) {
    addrError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '添加失败';
    ElMessage.error(addrError.value);
  } finally {
    addrSaving.value = false;
  }
}

async function delAddr(row: any) {
  try {
    await api.delete('/api/v1/ipv6/addresses', { data: { iface: row.iface, addr: row.addr, prefix_len: row.prefix_len } });
    ElMessage.success('已删除');
    await loadAddrs();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '删除失败';
    addrError.value = msg;
    ElMessage.error(msg);
  }
}

async function loadRA() {
  raError.value = '';
  try {
    const res = await api.get('/api/v1/ipv6/ra');
    ra.value = res.data?.config || ra.value;
    radvdText.value = res.data?.radvd || '';
  } catch (e: any) {
    raError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载 RA 配置失败';
  }
}

async function saveRA() {
  raSaving.value = true;
  raError.value = '';
  try {
    const res = await api.post('/api/v1/ipv6/ra', ra.value);
    ra.value = res.data?.config || ra.value;
    radvdText.value = res.data?.radvd || '';
    raApplied.value = !!res.data?.applied;
    raDetail.value = res.data?.detail || (res.data?.applied ? '已下发' : '未下发');
    if (res.data?.applied) ElMessage.success('RA 已下发');
    else ElMessage.warning(raDetail.value);
  } catch (e: any) {
    raError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    ElMessage.error(raError.value);
  } finally {
    raSaving.value = false;
  }
}

async function loadDhcpV6() {
  try {
    const res = await api.get('/api/v1/dhcp/v6');
    dhcpV6.value = res.data?.config || dhcpV6.value;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载 DHCPv6 失败');
  }
}

async function saveDhcpV6() {
  dhcpV6Saving.value = true;
  try {
    const res = await api.post('/api/v1/dhcp/v6', dhcpV6.value);
    dhcpV6.value = res.data?.config || dhcpV6.value;
    ElMessage.success('DHCPv6 已保存');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存 DHCPv6 失败');
  } finally {
    dhcpV6Saving.value = false;
  }
}

async function loadStatus() {
  try {
    const res = await api.get('/api/v1/ipv6/status');
    status.value = res.data;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '状态获取失败');
  }
}

onMounted(() => {
  loadAddrs();
  loadRA();
  loadDhcpV6();
  loadND();
  loadPD();
  loadStatus();
});
</script>

<style scoped>
.hint {
  margin-left: 8px;
  color: #909399;
  font-size: 12px;
}
</style>