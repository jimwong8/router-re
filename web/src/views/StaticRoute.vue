<template>
  <div>
    <el-card shadow="never">
      <template #header>静态路由</template>
      <el-form inline @submit.prevent>
        <el-form-item label="目标网段">
          <el-input v-model="newRoute.dest" placeholder="10.0.0.0/8" style="width:150px" />
        </el-form-item>
        <el-form-item label="网关">
          <el-input v-model="newRoute.gateway" placeholder="192.168.1.1" style="width:140px" />
        </el-form-item>
        <el-form-item label="设备">
          <el-input v-model="newRoute.device" placeholder="eth0" style="width:120px" />
        </el-form-item>
        <el-form-item label="度量">
          <el-input-number v-model="newRoute.metric" :min="0" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addRoute">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="routes" border style="width:100%; margin-top:12px">
        <el-table-column prop="dest" label="目标网段" width="150" />
        <el-table-column prop="gateway" label="网关" width="140" />
        <el-table-column prop="device" label="设备" width="120" />
        <el-table-column prop="metric" label="度量" width="100" />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeRoute(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button type="primary" style="margin-top:16px" :loading="submitting" @click="submit">
        提交并应用
      </el-button>

      <el-divider>ip 命令（返回）</el-divider>
      <pre v-if="commands.length" class="cmd-output">{{ commands.join('\n') }}</pre>
      <el-empty v-else description="尚未提交" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface StaticRouteRow {
  dest: string;
  gateway: string;
  device: string;
  metric: number;
}

function emptyRoute(): StaticRouteRow {
  return { dest: '', gateway: '', device: '', metric: 0 };
}

const newRoute = ref<StaticRouteRow>(emptyRoute());
const routes = ref<StaticRouteRow[]>([]);
const commands = ref<string[]>([]);
const submitting = ref(false);

function addRoute() {
  if (!newRoute.value.dest) {
    ElMessage.warning('目标网段必填');
    return;
  }
  routes.value.push({ ...newRoute.value });
  newRoute.value = emptyRoute();
}

function removeRoute(row: StaticRouteRow) {
  routes.value = routes.value.filter(r => r !== row);
}

async function load() {
  try {
    const res = await api.get('/api/v1/route/static');
    routes.value = (res.data?.routes || []) as StaticRouteRow[];
    commands.value = res.data?.commands || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submit() {
  if (!routes.value.length) {
    ElMessage.warning('请先添加路由');
    return;
  }
  submitting.value = true;
  try {
    const res = await api.post('/api/v1/route/static', { routes: routes.value });
    commands.value = res.data?.commands || [];
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

<style scoped>
.cmd-output {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
}
</style>