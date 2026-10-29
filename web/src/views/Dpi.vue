<template>
  <div>
    <el-card shadow="never">
      <template #header>应用识别（DPI）— 标记下发到数据面</template>

      <el-alert
        v-if="applyError"
        :title="'下发失败：' + applyError"
        type="error"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      />

      <el-form inline @submit.prevent>
        <el-form-item label="应用名">
          <el-input v-model="newRule.app" placeholder="如 netflix" style="width: 150px" />
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="newRule.protocol" style="width: 100px">
            <el-option label="tcp" value="tcp" />
            <el-option label="udp" value="udp" />
          </el-select>
        </el-form-item>
        <el-form-item label="端口">
          <el-input
            v-model="portsText"
            placeholder="6881,6882（可空）"
            style="width: 160px"
          />
        </el-form-item>
        <el-form-item label="Mark">
          <el-input-number v-model="newRule.mark" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="动作">
          <el-select v-model="newRule.action" style="width: 110px">
            <el-option label="mark" value="mark" />
            <el-option label="drop" value="drop" />
            <el-option label="limit" value="limit" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="newRule.action === 'limit'" label="限速(Mbps)">
          <el-input-number v-model="newRule.rate_mbps" :min="1" :max="1000" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addRule">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="rules" border style="width: 100%; margin-top: 12px">
        <el-table-column prop="app" label="应用" width="150" />
        <el-table-column prop="protocol" label="协议" width="90" />
        <el-table-column label="端口" width="160">
          <template #default="{ row }">
            {{ (row.dports || []).join(',') || '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="mark" label="Mark" width="100" />
        <el-table-column prop="action" label="动作" width="100">
          <template #default="{ row }">
            <el-tag :type="row.action === 'drop' ? 'danger' : row.action === 'limit' ? 'warning' : 'info'">
              {{ row.action }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeRule(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button
        type="primary"
        style="margin-top: 16px"
        :loading="submitting"
        @click="submit"
      >
        下发到数据面
      </el-button>

      <el-divider>nft 规则（返回）</el-divider>
      <pre v-if="nftText" class="cmd-output">{{ nftText }}</pre>
      <el-empty v-else description="尚未下发" :image-size="60" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface MarkRule {
  app: string;
  protocol: string;
  dports: number[];
  mark: number;
  action: string;
  rate_mbps?: number;
}

function emptyRule(): MarkRule {
  return { app: '', protocol: 'tcp', dports: [], mark: 200, action: 'mark', rate_mbps: 10 };
}

const newRule = ref<MarkRule>(emptyRule());
const portsText = ref('');
const rules = ref<MarkRule[]>([]);
const nftText = ref('');
const applyError = ref('');
const submitting = ref(false);

function parsePorts(): number[] {
  return portsText.value
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0 && n < 65536);
}

function addRule() {
  if (!newRule.value.app) {
    ElMessage.warning('应用名必填');
    return;
  }
  rules.value.push({ ...newRule.value, dports: parsePorts() });
  newRule.value = emptyRule();
  portsText.value = '';
}

function removeRule(row: MarkRule) {
  rules.value = rules.value.filter((r) => r !== row);
}

async function load() {
  try {
    const res = await api.get('/api/v1/dpi/marks');
    rules.value = res.data?.rules || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submit() {
  if (!rules.value.length) {
    ElMessage.warning('请先添加规则');
    return;
  }
  submitting.value = true;
  try {
    const res = await api.post('/api/v1/dpi/marks', { rules: rules.value });
    nftText.value = res.data?.nft || '';
    if (res.data?.applied) {
      applyError.value = '';
      ElMessage.success('已下发到数据面');
    } else {
      applyError.value = res.data?.error || '下发未生效（可能缺少 root/nftables 权限）';
      ElMessage.warning('规则已保存但未下发成功');
    }
    await load();
  } catch (e: any) {
    const msg = e?.response?.data?.message || e.message;
    ElMessage.error(msg);
    applyError.value = msg;
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
