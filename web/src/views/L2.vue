<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>VLAN 子接口</template>
      <el-alert v-if="vlanError" :title="vlanError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form inline @submit.prevent="addVlan">
        <el-form-item label="父接口">
          <el-input v-model="newVlan.parent" placeholder="eth0" style="width: 120px;" />
        </el-form-item>
        <el-form-item label="VID">
          <el-input-number v-model="newVlan.vid" :min="1" :max="4094" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="newVlan.name" placeholder="eth0.100" style="width: 140px;" />
        </el-form-item>
        <el-form-item label="IP/掩码">
          <el-input v-model="newVlan.address" placeholder="10.0.0.1/24" style="width: 160px;" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="vlanSaving" @click="addVlan">添加</el-button>
          <el-button @click="loadVlans">刷新</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="vlans" border style="width: 100%; margin-top: 8px;">
        <el-table-column prop="parent" label="父接口" width="120" />
        <el-table-column prop="vid" label="VID" width="80" />
        <el-table-column prop="name" label="名称" width="140" />
        <el-table-column prop="address" label="IP/掩码" />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="delVlan(row.name)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!vlans.length" description="暂无 VLAN" />
    </el-card>

    <el-card shadow="never">
      <template #header>桥接（Bridge）</template>
      <el-alert v-if="bridgeError" :title="bridgeError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form inline @submit.prevent="addBridge">
        <el-form-item label="名称">
          <el-input v-model="newBridge.name" placeholder="br0" style="width: 120px;" />
        </el-form-item>
        <el-form-item label="成员">
          <el-select v-model="newBridge.members" multiple filterable style="width: 240px;">
            <el-option v-for="iface in ifaces" :key="iface" :label="iface" :value="iface" />
          </el-select>
        </el-form-item>
        <el-form-item label="STP">
          <el-switch v-model="newBridge.stp" />
        </el-form-item>
        <el-form-item label="IP/掩码">
          <el-input v-model="newBridge.address" placeholder="192.168.1.1/24" style="width: 160px;" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="bridgeSaving" @click="addBridge">添加</el-button>
          <el-button @click="loadBridges">刷新</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="bridges" border style="width: 100%; margin-top: 8px;">
        <el-table-column prop="name" label="名称" width="120" />
        <el-table-column label="成员">
          <template #default="{ row }">{{ (row.members || []).join(', ') }}</template>
        </el-table-column>
        <el-table-column label="STP" width="80">
          <template #default="{ row }">{{ row.stp ? '开' : '关' }}</template>
        </el-table-column>
        <el-table-column prop="address" label="IP/掩码" />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="delBridge(row.name)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!bridges.length" description="暂无桥接" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const vlans = ref<any[]>([]);
const bridges = ref<any[]>([]);
const ifaces = ref<string[]>([]);
const vlanError = ref('');
const bridgeError = ref('');
const vlanSaving = ref(false);
const bridgeSaving = ref(false);

const newVlan = ref<any>({ parent: '', vid: 100, name: '', address: '' });
const newBridge = ref<any>({ name: '', members: [], stp: false, address: '' });

async function loadIfaces() {
  try {
    const res = await api.get('/api/v1/interfaces');
    const data = res.data;
    const arr = Array.isArray(data) ? data : (data?.interfaces || []);
    ifaces.value = arr.map((x: any) => x.name).filter(Boolean);
  } catch (e: any) {
    ifaces.value = [];
  }
}

async function loadVlans() {
  vlanError.value = '';
  try {
    const res = await api.get('/api/v1/vlan');
    vlans.value = res.data?.vlans || [];
  } catch (e: any) {
    vlanError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载 VLAN 失败';
    ElMessage.error(vlanError.value);
  }
}

async function addVlan() {
  vlanSaving.value = true;
  vlanError.value = '';
  try {
    const res = await api.post('/api/v1/vlan', {
      parent: newVlan.value.parent,
      vid: newVlan.value.vid,
      name: newVlan.value.name,
      address: newVlan.value.address,
    });
    vlans.value = res.data?.vlans || vlans.value;
    ElMessage.success('VLAN 已应用');
  } catch (e: any) {
    vlanError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '应用 VLAN 失败';
    ElMessage.error(vlanError.value);
  } finally {
    vlanSaving.value = false;
  }
}

async function delVlan(name: string) {
  try {
    await api.delete(`/api/v1/vlan/${encodeURIComponent(name)}`);
    ElMessage.success('已删除');
    await loadVlans();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '删除失败';
    vlanError.value = msg;
    ElMessage.error(msg);
  }
}

async function loadBridges() {
  bridgeError.value = '';
  try {
    const res = await api.get('/api/v1/bridge');
    bridges.value = res.data?.bridges || [];
  } catch (e: any) {
    bridgeError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载桥接失败';
    ElMessage.error(bridgeError.value);
  }
}

async function addBridge() {
  bridgeSaving.value = true;
  bridgeError.value = '';
  try {
    const res = await api.post('/api/v1/bridge', {
      name: newBridge.value.name,
      members: newBridge.value.members,
      stp: newBridge.value.stp,
      address: newBridge.value.address,
    });
    bridges.value = res.data?.bridges || bridges.value;
    ElMessage.success('桥接已应用');
  } catch (e: any) {
    bridgeError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '应用桥接失败';
    ElMessage.error(bridgeError.value);
  } finally {
    bridgeSaving.value = false;
  }
}

async function delBridge(name: string) {
  try {
    await api.delete(`/api/v1/bridge/${encodeURIComponent(name)}`);
    ElMessage.success('已删除');
    await loadBridges();
  } catch (e: any) {
    const msg = e?.response?.data?.detail || e?.response?.data?.message || e.message || '删除失败';
    bridgeError.value = msg;
    ElMessage.error(msg);
  }
}

onMounted(() => {
  loadIfaces();
  loadVlans();
  loadBridges();
});
</script>