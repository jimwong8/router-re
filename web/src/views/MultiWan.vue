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