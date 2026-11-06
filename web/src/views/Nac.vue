<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>MAC 访问控制</template>
      <el-form inline style="margin-bottom: 12px;">
        <el-form-item label="模式">
          <el-radio-group v-model="cfg.mode">
            <el-radio value="off">关闭</el-radio>
            <el-radio value="whitelist">白名单</el-radio>
            <el-radio value="blacklist">黑名单</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="nacSaving" @click="saveNAC">保存下发</el-button>
          <el-button @click="loadNAC">加载</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="nacError" :title="nacError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form inline @submit.prevent="addDevice">
        <el-form-item label="MAC">
          <el-input v-model="newDev.mac" placeholder="aa:bb:cc:dd:ee:ff" style="width: 180px;" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="newDev.name" placeholder="可选" style="width: 140px;" />
        </el-form-item>
        <el-form-item label="IP">
          <el-input v-model="newDev.ip" placeholder="可选" style="width: 140px;" />
        </el-form-item>
        <el-form-item label="策略">
          <el-select v-model="newDev.policy" style="width: 120px;">
            <el-option label="allow" value="allow" />
            <el-option label="deny" value="deny" />
            <el-option label="quarantine" value="quarantine" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addDevice">添加</el-button>
          <el-button @click="importFromNeighbors">从邻居表导入</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="cfg.devices" border>
        <el-table-column prop="mac" label="MAC" width="180" />
        <el-table-column prop="name" label="名称" width="140" />
        <el-table-column prop="ip" label="IP" width="140" />
        <el-table-column prop="policy" label="策略" width="120" />
        <el-table-column prop="note" label="备注" />
        <el-table-column label="操作" width="80">
          <template #default="{ $index }">
            <el-button type="danger" size="small" @click="cfg.devices.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-collapse v-if="nftText" style="margin-top: 8px;">
        <el-collapse-item title="router_nac nft 规则" name="nac">
          <pre style="white-space: pre-wrap;">{{ nftText }}</pre>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>MAC 过滤</template>
      <el-form inline style="margin-bottom: 12px;">
        <el-form-item label="启用">
          <el-switch v-model="macFilterEnabled" />
        </el-form-item>
        <el-form-item label="默认动作">
          <el-select v-model="macFilterMode" style="width: 140px;">
            <el-option label="黑名单（列表内拒绝，其余放行）" value="blacklist" />
            <el-option label="白名单（列表内放行，其余拒绝）" value="whitelist" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="macFilterSaving" @click="saveMacFilter">保存下发</el-button>
          <el-button @click="loadMacFilter">加载</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="macFilterError" :title="macFilterError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-form inline @submit.prevent="addMacRule">
        <el-form-item label="MAC">
          <el-input v-model="newMac" placeholder="aa:bb:cc:dd:ee:ff" style="width: 200px;" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addMacRule">添加</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="macFilterRules" border>
        <el-table-column prop="mac" label="MAC 地址" />
        <el-table-column label="操作" width="80">
          <template #default="{ $index }">
            <el-button type="danger" size="small" @click="macFilterRules.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-collapse v-if="macFilterNft" style="margin-top: 8px;">
        <el-collapse-item title="nft 规则" name="macfilter">
          <pre style="white-space: pre-wrap;">{{ macFilterNft }}</pre>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-card shadow="never" style="margin-bottom: 16px;">
      <template #header>连接数限制</template>
      <el-form inline style="margin-bottom: 12px;">
        <el-form-item label="名称">
          <el-input v-model="newLimit.name" placeholder="规则名" style="width: 140px;" />
        </el-form-item>
        <el-form-item label="源 IP/CIDR">
          <el-input v-model="newLimit.src_ip" placeholder="1.2.3.4 或 192.168.0.0/24" style="width: 200px;" />
        </el-form-item>
        <el-form-item label="最大连接">
          <el-input-number v-model="newLimit.max_conns" :min="1" :max="10000" />
        </el-form-item>
        <el-form-item label="每秒速率">
          <el-input-number v-model="newLimit.max_per_sec" :min="0" :max="10000" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addLimit">添加</el-button>
          <el-button type="success" :loading="clApplyLoading" @click="applyConnlimits">下发</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="clError" :title="clError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-table :data="limits" border>
        <el-table-column prop="name" label="名称" width="140" />
        <el-table-column prop="src_ip" label="源 IP" width="200" />
        <el-table-column prop="max_conns" label="最大连接" width="100" />
        <el-table-column prop="max_per_sec" label="每秒速率" width="100" />
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="saveConnlimits" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ $index }">
            <el-button type="danger" size="small" @click="limits.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-collapse v-if="clNftText" style="margin-top: 8px;">
        <el-collapse-item title="router_connlimit nft 规则" name="cl">
          <pre style="white-space: pre-wrap;">{{ clNftText }}</pre>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-card shadow="never">
      <template #header>当前连接排行（Top 20）</template>
      <el-button size="small" style="margin-bottom: 8px;" :loading="topLoading" @click="loadTop">刷新</el-button>
      <el-table :data="top" border>
        <el-table-column prop="ip" label="源 IP" />
        <el-table-column prop="count" label="连接数" width="100" />
      </el-table>
      <el-empty v-if="!top.length" description="暂无连接数据" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const cfg = ref<any>({ mode: 'off', devices: [] });
const nacSaving = ref(false);
const nacError = ref('');
const nftText = ref('');
const newDev = ref<any>({ mac: '', name: '', ip: '', policy: 'allow' });

const macFilterEnabled = ref(false);
const macFilterMode = ref('blacklist');
const macFilterSaving = ref(false);
const macFilterError = ref('');
const macFilterNft = ref('');
const macFilterRules = ref<any[]>([]);
const newMac = ref('');

const limits = ref<any[]>([]);
const newLimit = ref<any>({ name: '', src_ip: '', max_conns: 100, max_per_sec: 0, enabled: true });
const clError = ref('');
const clApplyLoading = ref(false);
const clNftText = ref('');

const top = ref<any[]>([]);
const topLoading = ref(false);
const neighbors = ref<any[]>([]);

async function loadNAC() {
  nacError.value = '';
  try {
    const res = await api.get('/api/v1/nac/config');
    cfg.value = res.data?.config || cfg.value;
    nftText.value = res.data?.nft || '';
  } catch (e: any) {
    nacError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载失败';
  }
}

async function saveNAC() {
  nacSaving.value = true;
  nacError.value = '';
  try {
    const res = await api.post('/api/v1/nac/config', cfg.value);
    cfg.value = res.data?.config || cfg.value;
    nftText.value = res.data?.nft || '';
    if (res.data?.applied) ElMessage.success('MAC 访问控制已下发');
    else ElMessage.warning(res.data?.detail || '未下发');
  } catch (e: any) {
    nacError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    ElMessage.error(nacError.value);
  } finally {
    nacSaving.value = false;
  }
}

function addDevice() {
  if (!newDev.value.mac) { ElMessage.warning('请输入 MAC'); return; }
  cfg.value.devices.push({ ...newDev.value });
  newDev.value = { mac: '', name: '', ip: '', policy: 'allow' };
}

async function importFromNeighbors() {
  try {
    const res = await api.get('/api/v1/nac/neighbors');
    neighbors.value = res.data?.neighbors || [];
    let added = 0;
    for (const n of neighbors.value) {
      if (!n.mac) continue;
      if (cfg.value.devices.some((d: any) => d.mac === n.mac)) continue;
      cfg.value.devices.push({ mac: n.mac, name: n.name || '', ip: n.ip || '', policy: 'allow', note: '' });
      added++;
    }
    ElMessage.success('导入 ' + added + ' 个设备');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '导入失败');
  }
}

async function loadConnlimits() {
  try {
    const res = await api.get('/api/v1/nac/connlimits');
    limits.value = res.data?.limits || [];
    clNftText.value = res.data?.nft || '';
  } catch (e: any) {
    clError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载失败';
  }
}

async function saveConnlimits() {
  try {
    const res = await api.post('/api/v1/nac/connlimits', limits.value);
    limits.value = res.data?.limits || limits.value;
    clNftText.value = res.data?.nft || '';
  } catch (e: any) {
    clError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    ElMessage.error(clError.value);
  }
}

async function applyConnlimits() {
  clApplyLoading.value = true;
  try {
    const res = await api.post('/api/v1/nac/connlimits/apply', {});
    if (res.data?.applied) ElMessage.success('连接数限制已下发');
    else ElMessage.warning(res.data?.detail || '未下发');
  } catch (e: any) {
    clError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '下发失败';
    ElMessage.error(clError.value);
  } finally {
    clApplyLoading.value = false;
  }
}

function addLimit() {
  if (!newLimit.value.name || !newLimit.value.src_ip) { ElMessage.warning('请填写名称和源 IP'); return; }
  limits.value.push({ ...newLimit.value });
  newLimit.value = { name: '', src_ip: '', max_conns: 100, max_per_sec: 0, enabled: true };
}

async function loadTop() {
  topLoading.value = true;
  try {
    const res = await api.get('/api/v1/nac/top-connections');
    top.value = res.data?.top || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e.message || '连接排行获取失败');
  } finally {
    topLoading.value = false;
  }
}

async function loadMacFilter() {
  macFilterError.value = '';
  try {
    const res = await api.get('/api/v1/acl/mac-filter');
    const cfg = res.data?.config || {};
    macFilterEnabled.value = cfg.mode !== 'off';
    macFilterMode.value = cfg.mode === 'off' ? 'blacklist' : cfg.mode;
    macFilterRules.value = (cfg.macs || []).map((mac: string) => ({ mac }));
    macFilterNft.value = res.data?.nft || '';
  } catch (e: any) {
    macFilterError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载失败';
  }
}

async function saveMacFilter() {
  macFilterSaving.value = true;
  macFilterError.value = '';
  try {
    const mode = macFilterEnabled.value ? macFilterMode.value : 'off';
    const res = await api.post('/api/v1/acl/mac-filter', {
      mode,
      macs: macFilterRules.value.map((r: any) => r.mac),
    });
    macFilterNft.value = res.data?.nft || '';
    if (res.data?.applied) ElMessage.success('MAC 过滤已下发');
    else ElMessage.warning(res.data?.detail || '未下发');
  } catch (e: any) {
    macFilterError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    ElMessage.error(macFilterError.value);
  } finally {
    macFilterSaving.value = false;
  }
}

function addMacRule() {
  const mac = newMac.value.trim();
  if (!mac) { ElMessage.warning('请输入 MAC'); return; }
  if (macFilterRules.value.some((r: any) => r.mac === mac)) { ElMessage.warning('MAC 已存在'); return; }
  macFilterRules.value.push({ mac });
  newMac.value = '';
}

onMounted(() => { loadNAC(); loadConnlimits(); loadTop(); loadMacFilter(); });
</script>