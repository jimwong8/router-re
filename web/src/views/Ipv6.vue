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
  loadStatus();
});
</script>