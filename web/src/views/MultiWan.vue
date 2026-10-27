<template>
  <div>
    <el-card shadow="never">
      <template #header>多 WAN 负载均衡配置</template>
      <el-form inline @submit.prevent>
        <el-form-item label="名称">
          <el-input v-model="newLink.name" placeholder="如 WAN1" />
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="newLink.interface" placeholder="如 eth0" />
        </el-form-item>
        <el-form-item label="网关">
          <el-input v-model="newLink.gateway" placeholder="如 192.168.1.1" />
        </el-form-item>
        <el-form-item label="权重">
          <el-input-number v-model="newLink.weight" :min="1" :max="255" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addLink">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="links" border style="width: 100%">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="interface" label="接口" />
        <el-table-column prop="gateway" label="网关" />
        <el-table-column prop="weight" label="权重" width="80" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeLink(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button
        type="primary"
        style="margin-top: 16px"
        :loading="submitting"
        @click="submit"
      >
        提交
      </el-button>

      <el-divider>策略路由命令（返回）</el-divider>
      <pre v-if="tcOutput.length" class="cmd-output">{{ cmdText }}</pre>
      <el-empty v-else description="尚未提交" />

      <el-divider>WAN 健康状态</el-divider>
      <el-button type="primary" size="small" :loading="checking" style="margin-bottom:12px" @click="loadHealth">
        刷新健康状态
      </el-button>
      <el-table v-if="health.length" :data="health" border style="width:100%">
        <el-table-column prop="name" label="WAN名" width="120" />
        <el-table-column prop="interface" label="接口" width="120" />
        <el-table-column label="在线" width="100">
          <template #default="{ row }">
            <el-tag :type="row.alive ? 'success' : 'danger'">{{ row.alive ? '在线' : '离线' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="latency_ms" label="延迟(ms)" width="110" />
        <el-table-column prop="loss_pct" label="丢包(%)" width="110">
          <template #default="{ row }">{{ (row.loss_pct ?? 0).toFixed(1) }}</template>
        </el-table-column>
        <el-table-column label="检查时间">
          <template #default="{ row }">
            {{ new Date((row.checked_at || 0) * 1000).toLocaleString() }}
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-else-if="!checking" description="暂无健康数据（点击刷新检查）" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface WanLink {
  name: string;
  interface: string;
  gateway: string;
  weight: number;
}

const newLink = ref<WanLink>({ name: '', interface: '', gateway: '', weight: 1 });
const links = ref<WanLink[]>([]);
const tcOutput = ref<string[]>([]);
const submitting = ref(false);
const health = ref<any[]>([]);
const checking = ref(false);

const cmdText = computed(() => tcOutput.value.join('\n'));

function addLink() {
  if (!newLink.value.name || !newLink.value.interface) {
    ElMessage.warning('请填写名称和接口');
    return;
  }
  links.value.push({ ...newLink.value });
  newLink.value = { name: '', interface: '', gateway: '', weight: 1 };
}

function removeLink(row: WanLink) {
  links.value = links.value.filter((l) => l !== row);
}

async function load() {
  try {
    const res = await api.get('/api/v1/wan/links');
    links.value = res.data?.links || [];
    tcOutput.value = res.data?.tc_commands || res.data?.commands || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submit() {
  if (!links.value.length) {
    ElMessage.warning('请先添加至少一条 WAN');
    return;
  }
  submitting.value = true;
  try {
    const res = await api.post('/api/v1/wan/links', { links: links.value });
    tcOutput.value =
      res.data?.tc_commands || res.data?.commands || res.data?.routes || [];
    ElMessage.success('已提交');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    submitting.value = false;
  }
}

async function loadHealth() {
  checking.value = true;
  try {
    const res = await api.get('/api/v1/wan/status');
    health.value = res.data?.status || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    checking.value = false;
  }
}

onMounted(() => {
  load();
  loadHealth();
});
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