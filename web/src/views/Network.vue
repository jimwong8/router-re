<template>
  <div>
    <el-tabs v-model="activeTab">
      <el-tab-pane label="端口映射" name="portmap">
        <el-card shadow="never">
          <el-alert v-if="pmError" :title="pmError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
          <el-form inline @submit.prevent="addPm">
            <el-form-item label="名称">
              <el-input v-model="newPm.name" placeholder="web" style="width: 110px;" />
            </el-form-item>
            <el-form-item label="协议">
              <el-select v-model="newPm.proto" style="width: 90px;">
                <el-option label="tcp" value="tcp" />
                <el-option label="udp" value="udp" />
              </el-select>
            </el-form-item>
            <el-form-item label="外端口">
              <el-input-number v-model="newPm.ext_port" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item label="内网IP">
              <el-input v-model="newPm.int_ip" placeholder="192.168.1.100" style="width: 150px;" />
            </el-form-item>
            <el-form-item label="内端口">
              <el-input-number v-model="newPm.int_port" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="addPm">添加</el-button>
              <el-button @click="loadPm">刷新</el-button>
            </el-form-item>
          </el-form>

          <el-table :data="pmRules" border style="width: 100%; margin-top: 8px;">
            <el-table-column prop="name" label="名称" width="120" />
            <el-table-column prop="proto" label="协议" width="80" />
            <el-table-column prop="ext_port" label="外端口" width="100" />
            <el-table-column prop="int_ip" label="内网IP" width="150" />
            <el-table-column prop="int_port" label="内端口" width="100" />
            <el-table-column label="启用" width="90">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button type="danger" size="small" @click="delPm(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!pmRules.length" description="暂无端口映射规则" />

          <el-button type="primary" style="margin-top: 16px;" :loading="pmSaving" @click="savePm">
            保存并下发
          </el-button>

          <el-divider>nft 回读（下发后实际生效内容）</el-divider>
          <pre v-if="pmNft" class="cmd-output">{{ pmNft }}</pre>
          <el-empty v-else description="尚未下发" />
        </el-card>
      </el-tab-pane>
      <el-tab-pane label="策略路由" name="policyroute">
        <el-card shadow="never">
          <el-alert v-if="prError" :title="prError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
          <el-form inline @submit.prevent="addPr">
            <el-form-item label="名称">
              <el-input v-model="newPr.name" placeholder="p1" style="width: 100px;" />
            </el-form-item>
            <el-form-item label="源">
              <el-input v-model="newPr.src_cidr" placeholder="192.168.1.0/24（可空）" style="width: 190px;" />
            </el-form-item>
            <el-form-item label="目的">
              <el-input v-model="newPr.dst_cidr" placeholder="10.0.0.0/8（可空）" style="width: 170px;" />
            </el-form-item>
            <el-form-item label="表号">
              <el-input-number v-model="newPr.table" :min="1" :max="252" />
            </el-form-item>
            <el-form-item label="优先级">
              <el-input-number v-model="newPr.priority" :min="1" :max="32765" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="addPr">添加</el-button>
              <el-button @click="loadPr">刷新</el-button>
            </el-form-item>
          </el-form>

          <el-table :data="prRules" border style="width: 100%; margin-top: 8px;">
            <el-table-column prop="name" label="名称" width="100" />
            <el-table-column prop="src_cidr" label="源" width="150" />
            <el-table-column prop="dst_cidr" label="目的" width="150" />
            <el-table-column prop="table" label="表号" width="80" />
            <el-table-column prop="priority" label="优先级" width="90" />
            <el-table-column label="启用" width="90">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button type="danger" size="small" @click="delPr(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!prRules.length" description="暂无策略路由规则" />

          <el-button type="primary" style="margin-top: 16px;" :loading="prSaving" @click="savePr">
            保存并下发
          </el-button>

          <el-divider>ip rule 实际列表（下发后回读）</el-divider>
          <el-table :data="prLive" border size="small" style="width: 100%;">
            <el-table-column prop="priority" label="优先级" width="90" />
            <el-table-column prop="from" label="源" width="160" />
            <el-table-column prop="to" label="目的" width="160" />
            <el-table-column prop="lookup" label="表" width="90" />
            <el-table-column prop="action" label="动作" width="100" />
          </el-table>
          <el-empty v-if="!prLive.length" description="尚未下发" />
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface PmRule {
  id: number;
  name: string;
  proto: string;
  ext_port: number;
  int_ip: string;
  int_port: number;
  enabled: boolean;
}

interface PrRule {
  id: number;
  name: string;
  src_cidr: string;
  dst_cidr: string;
  table: number;
  priority: number;
  enabled: boolean;
}

interface PrLiveRow {
  priority: string;
  from: string;
  to: string;
  lookup: string;
  action: string;
}

const activeTab = ref('portmap');

function emptyPm(): PmRule {
  return { id: 0, name: '', proto: 'tcp', ext_port: 8080, int_ip: '', int_port: 80, enabled: true };
}
function emptyPr(): PrRule {
  return { id: 0, name: '', src_cidr: '', dst_cidr: '', table: 100, priority: 100, enabled: true };
}

const pmRules = ref<PmRule[]>([]);
const prRules = ref<PrRule[]>([]);
const prLive = ref<PrLiveRow[]>([]);
const pmNft = ref('');
const pmError = ref('');
const prError = ref('');
const pmSaving = ref(false);
const prSaving = ref(false);
const newPm = ref<PmRule>(emptyPm());
const newPr = ref<PrRule>(emptyPr());

function nextPmId(): number {
  return pmRules.value.reduce((m, r) => Math.max(m, r.id), 0) + 1;
}
function nextPrId(): number {
  return prRules.value.reduce((m, r) => Math.max(m, r.id), 0) + 1;
}

function addPm() {
  if (!newPm.value.name) { ElMessage.warning('名称必填'); return; }
  if (!newPm.value.int_ip) { ElMessage.warning('内网IP必填'); return; }
  pmRules.value.push({ ...newPm.value, id: nextPmId() });
  newPm.value = emptyPm();
}

function delPm(row: PmRule) {
  pmRules.value = pmRules.value.filter(r => r.id !== row.id);
}

function addPr() {
  if (!newPr.value.name) { ElMessage.warning('名称必填'); return; }
  prRules.value.push({ ...newPr.value, id: nextPrId() });
  newPr.value = emptyPr();
}

function delPr(row: PrRule) {
  prRules.value = prRules.value.filter(r => r.id !== row.id);
}

async function loadPm() {
  pmError.value = '';
  try {
    const res = await api.get('/api/v1/portmap/rules');
    pmRules.value = (res.data?.rules || []) as PmRule[];
  } catch (e: any) {
    pmError.value = e?.response?.data?.message || e.message;
  }
}

async function loadPr() {
  prError.value = '';
  try {
    const res = await api.get('/api/v1/policyroute/rules');
    prRules.value = (res.data?.rules || []) as PrRule[];
  } catch (e: any) {
    prError.value = e?.response?.data?.message || e.message;
  }
}

async function savePm() {
  pmError.value = '';
  if (!pmRules.value.length) { ElMessage.warning('请先添加端口映射规则'); return; }
  pmSaving.value = true;
  try {
    const res = await api.post('/api/v1/portmap/rules', { rules: pmRules.value });
    pmRules.value = (res.data?.rules || []) as PmRule[];
    pmNft.value = res.data?.nft || '';
    ElMessage.success('端口映射已下发');
  } catch (e: any) {
    pmError.value = e?.response?.data?.message || e.message;
  } finally {
    pmSaving.value = false;
  }
}

async function savePr() {
  prError.value = '';
  if (!prRules.value.length) { ElMessage.warning('请先添加策略路由规则'); return; }
  prSaving.value = true;
  try {
    const res = await api.post('/api/v1/policyroute/rules', { rules: prRules.value });
    prRules.value = (res.data?.rules || []) as PrRule[];
    prLive.value = (res.data?.live || []) as PrLiveRow[];
    ElMessage.success('策略路由已下发');
  } catch (e: any) {
    prError.value = e?.response?.data?.message || e.message;
  } finally {
    prSaving.value = false;
  }
}

onMounted(() => {
  loadPm();
  loadPr();
});
</script>
