<template>
  <div>
    <el-card shadow="never">
      <template #header>接口配置</template>
      <el-form inline @submit.prevent>
        <el-form-item label="接口名">
          <el-input v-model="newCfg.name" placeholder="如 eth0" style="width:120px" />
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="newCfg.address" placeholder="192.168.1.10" style="width:140px" />
        </el-form-item>
        <el-form-item label="掩码">
          <el-input v-model="newCfg.netmask" placeholder="255.255.255.0" style="width:140px" />
        </el-form-item>
        <el-form-item label="网关">
          <el-input v-model="newCfg.gateway" placeholder="192.168.1.1" style="width:140px" />
        </el-form-item>
        <el-form-item label="DNS">
          <el-input v-model="dnsText" placeholder="223.5.5.5,8.8.8.8" style="width:160px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addCfg">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="configs" border style="width:100%; margin-top:12px">
        <el-table-column prop="name" label="接口" width="100" />
        <el-table-column prop="address" label="地址" width="140" />
        <el-table-column prop="netmask" label="掩码" width="140" />
        <el-table-column prop="gateway" label="网关" width="140" />
        <el-table-column label="DNS">
          <template #default="{ row }">
            {{ (row.dns || []).join(', ') }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeCfg(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button type="primary" style="margin-top:16px" :loading="submitting" @click="submit">
        提交并应用
      </el-button>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface IfaceCfg {
  name: string;
  address: string;
  netmask: string;
  gateway: string;
  dns: string[];
}

function emptyCfg(): IfaceCfg {
  return { name: 'eth0', address: '', netmask: '255.255.255.0', gateway: '', dns: [] };
}

const newCfg = ref<IfaceCfg>(emptyCfg());
const dnsText = ref('');
const configs = ref<IfaceCfg[]>([]);
const submitting = ref(false);

function addCfg() {
  if (!newCfg.value.name) {
    ElMessage.warning('接口名必填');
    return;
  }
  const cfg = { ...newCfg.value };
  cfg.dns = dnsText.value ? dnsText.value.split(',').map(s => s.trim()).filter(Boolean) : [];
  configs.value.push(cfg);
  newCfg.value = emptyCfg();
  dnsText.value = '';
}

function removeCfg(row: IfaceCfg) {
  configs.value = configs.value.filter(c => c !== row);
}

async function load() {
  try {
    const res = await api.get('/api/v1/interfaces/config');
    configs.value = (res.data?.configs || []) as IfaceCfg[];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submit() {
  submitting.value = true;
  try {
    await api.post('/api/v1/interfaces/config', { configs: configs.value });
    ElMessage.success('已提交并应用');
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    submitting.value = false;
  }
}

onMounted(load);
</script>
