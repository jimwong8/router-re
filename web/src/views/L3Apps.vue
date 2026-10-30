<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>跨三层应用识别</span>
          <div class="tools">
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
            <el-button type="primary" size="small" :loading="saving" @click="save">保存</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px" :title="note" />

      <el-form label-width="150px" style="max-width:820px">
        <el-form-item label="启用">
          <el-switch v-model="cfg.enabled" />
        </el-form-item>
        <el-form-item label="流量引取方式">
          <el-select v-model="cfg.sniff_mode" style="width:240px">
            <el-option label="回程路由（推荐）" value="route" />
            <el-option label="上游端口镜像" value="mirror" />
            <el-option label="仅 SNMP 读拓扑" value="snmp" />
          </el-select>
          <span class="hint">{{ modeHint }}</span>
        </el-form-item>
      </el-form>

      <div class="sub-hdr">
        <span>跨三层网段（最多 {{ maxSubnets }} 个）</span>
        <el-button size="small" @click="addRow">添加网段</el-button>
      </div>

      <el-table :data="cfg.subnets" size="small" empty-text="还没有登记网段">
        <el-table-column label="网段 (CIDR)" min-width="200">
          <template #default="{ row }">
            <el-input v-model="row.cidr" placeholder="192.168.10.0/24" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="网关" min-width="170">
          <template #default="{ row }">
            <el-input v-model="row.gateway" placeholder="192.168.10.1" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="160">
          <template #default="{ row }">
            <el-input v-model="row.comment" placeholder="例如：二楼办公 VLAN" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }"><el-switch v-model="row.enabled" size="small" /></template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ $index }">
            <el-button link type="danger" size="small" @click="cfg.subnets.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="locals">
        <div class="locals-title">本机直连网段（这些不需要登记，登记会被拒绝）</div>
        <span v-for="n in localNets" :key="n" class="local-tag mono">{{ n }}</span>
        <span v-if="!localNets.length" class="muted">读取不到本机网段</span>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type Sub = { cidr: string; gateway: string; comment: string; enabled: boolean };

const cfg = reactive<{ enabled: boolean; sniff_mode: string; subnets: Sub[] }>({
  enabled: false, sniff_mode: 'route', subnets: [],
});
const localNets = ref<string[]>([]);
const note = ref('');
const maxSubnets = ref(32);
const loading = ref(false);
const saving = ref(false);

const modeHint = computed(() => {
  switch (cfg.sniff_mode) {
    case 'route': return '本机需要一条到该网段的回程路由，流量才会经过本机';
    case 'mirror': return '上游交换机把该网段流量镜像到本机接口，只收不发';
    case 'snmp': return '只读取拓扑信息，不参与应用识别与限速';
    default: return '';
  }
});

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/l3-apps');
    const c = res.data?.config || {};
    cfg.enabled = !!c.enabled;
    cfg.sniff_mode = c.sniff_mode || 'route';
    cfg.subnets = (c.subnets || []).map((s: Sub) => ({ ...s }));
    localNets.value = res.data?.local_nets || [];
    note.value = res.data?.note || '';
    maxSubnets.value = res.data?.max_subnets || 32;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  } finally {
    loading.value = false;
  }
}

function addRow() {
  cfg.subnets.push({ cidr: '', gateway: '', comment: '', enabled: true });
}

async function save() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/l3-apps', { ...cfg });
    if (res.data?.saved) ElMessage.success('已保存');
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.hint { color: #8b91a5; font-size: 12px; margin-left: 10px; }
.sub-hdr { display: flex; justify-content: space-between; align-items: center; margin: 6px 0 10px; font-size: 13px; }
.locals { margin-top: 16px; }
.locals-title { font-size: 12px; color: #8b91a5; margin-bottom: 6px; }
.local-tag { display: inline-block; margin: 0 6px 6px 0; padding: 2px 8px; border: 1px solid #2a2f3a; border-radius: 4px; font-size: 12px; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; }
.muted { color: #6b7280; font-size: 12px; }
</style>
