<template>
  <div>
    <el-card shadow="never">
      <template #header>流量控制 (QoS)</template>
      <el-form inline @submit.prevent>
        <el-form-item label="接口">
          <el-input v-model="newLimit.interface" placeholder="如 eth0" />
        </el-form-item>
        <el-form-item label="速率 (Mbps)">
          <el-input-number v-model="newLimit.rate_mbps" :min="1" :max="10000" />
        </el-form-item>
        <el-form-item label="突发 (KB)">
          <el-input-number v-model="newLimit.burst_kb" :min="10" />
        </el-form-item>
        <el-form-item label="方向">
          <el-select v-model="newLimit.direction" style="width: 100px;">
            <el-option label="egress" value="egress" />
            <el-option label="ingress" value="ingress" />
          </el-select>
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="newLimit.priority" :min="0" :max="65535" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addLimit">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="limits" border style="width: 100%; margin-top: 12px;">
        <el-table-column prop="interface" label="接口" width="120" />
        <el-table-column prop="rate_mbps" label="速率 (Mbps)" />
        <el-table-column prop="burst_kb" label="突发 (KB)" width="120" />
        <el-table-column prop="direction" label="方向" width="100" />
        <el-table-column prop="priority" label="优先级" width="100" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeLimit(row)">删除</el-button>
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

      <el-divider>TC 命令（返回）</el-divider>
      <pre v-if="tcOutput.length" class="cmd-output">{{ cmdText }}</pre>
      <el-empty v-else description="尚未提交" />
    </el-card>

    <el-card shadow="never" style="margin-top: 16px">
      <template #header>按 IP/MAC 限速</template>
      <el-form inline @submit.prevent>
        <el-form-item label="选择器">
          <el-input v-model="newClient.selector" placeholder="ip:192.168.1.100 或 mac:aa:bb:cc:dd:ee:ff" style="width: 260px" />
        </el-form-item>
        <el-form-item label="上行 (kbps)">
          <el-input-number v-model="newClient.upload_kbps" :min="0" :max="1000000" />
        </el-form-item>
        <el-form-item label="下行 (kbps)">
          <el-input-number v-model="newClient.download_kbps" :min="0" :max="1000000" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="newClient.comment" placeholder="如 客厅电视" />
        </el-form-item>
        <el-form-item label="设备">
          <el-input v-model="clientDevice" placeholder="eth0" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addClientLimit">添加</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="clientLimits" border style="width: 100%; margin-top: 12px;">
        <el-table-column prop="selector" label="选择器" width="220" />
        <el-table-column prop="upload_kbps" label="上行 (kbps)" />
        <el-table-column prop="download_kbps" label="下行 (kbps)" />
        <el-table-column prop="comment" label="备注" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button type="danger" size="small" @click="removeClientLimit(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button
        type="primary"
        style="margin-top: 16px"
        :loading="clientSubmitting"
        @click="submitClientLimits"
      >
        提交限速
      </el-button>

      <el-divider>nft 规则（返回）</el-divider>
      <pre v-if="clientNftOutput.length" class="cmd-output">{{ clientNftOutput }}</pre>
      <el-empty v-else description="尚未提交" />
    </el-card>
  </div>
<el-card shadow="never" style="margin-top:16px">
      <template #header>智能流控 (Smart QoS)</template>
      <el-form inline @submit.prevent>
        <el-form-item label="总带宽 (Mbps)">
          <el-input-number v-model="smartRoot" :min="1" :max="10000" />
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="smartIface" placeholder="如 eth0" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="smartLoading" @click="previewSmart">预览</el-button>
        </el-form-item>
      </el-form>
      <el-table v-if="smartPlan.length" :data="smartPlan" border style="margin-top:12px">
        <el-table-column prop="terminal" label="终端" />
        <el-table-column prop="rate_mbps" label="分配速率 (Mbps)" />
      </el-table>
      <el-button v-if="smartPlan.length" type="success" style="margin-top:12px" :loading="smartApplying" @click="applySmart">确认下发</el-button>
    </el-card>

</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

// 统一错误提示：后端 422 会带具体校验原因（例如"MTU 必须 1000-1492"），
// 直接弹 message 比只弹 "Request failed" 有用得多
function showError(e: any) {
  ElMessage.error(e?.response?.data?.message || e?.message || '请求失败');
}

// Field names mirror the backend JSON (snake_case) to avoid a mapping layer.
interface LimitRow {
  interface: string;
  rate_mbps: number;
  burst_kb: number;
  direction: string;
  priority: number;
}

function emptyLimit(): LimitRow {
  return { interface: 'eth0', rate_mbps: 100, burst_kb: 512, direction: 'egress', priority: 1 };
}

const newLimit = ref<LimitRow>(emptyLimit());
const limits = ref<LimitRow[]>([]);
const tcOutput = ref<string[]>([]);
const submitting = ref(false);

// --- per-client (IP/MAC) limits ---
interface ClientLimitRow {
  selector: string;
  upload_kbps: number;
  download_kbps: number;
  comment: string;
}

function emptyClientLimit(): ClientLimitRow {
  return { selector: '', upload_kbps: 0, download_kbps: 0, comment: '' };
}

const newClient = ref<ClientLimitRow>(emptyClientLimit());
const clientLimits = ref<ClientLimitRow[]>([]);
const clientDevice = ref('eth0');
const clientNftOutput = ref('');
const clientSubmitting = ref(false);

function addClientLimit() {
  if (!newClient.value.selector) {
    ElMessage.warning('选择器必填');
    return;
  }
  clientLimits.value.push({ ...newClient.value });
  newClient.value = emptyClientLimit();
}

function removeClientLimit(row: ClientLimitRow) {
  clientLimits.value = clientLimits.value.filter((l) => l !== row);
}

async function loadClientLimits() {
  try {
    const res = await api.get('/api/v1/qos/client-limits');
    clientLimits.value = (res.data?.limits || []) as ClientLimitRow[];
    clientNftOutput.value = res.data?.nft || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submitClientLimits() {
  if (!clientLimits.value.length) {
    ElMessage.warning('请先添加限速规则');
    return;
  }
  clientSubmitting.value = true;
  try {
    const res = await api.post('/api/v1/qos/client-limits', {
      limits: clientLimits.value,
      device: clientDevice.value,
    });
    clientNftOutput.value = res.data?.nft || '';
    ElMessage.success('限速已提交');
    await loadClientLimits();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    clientSubmitting.value = false;
  }
}

const cmdText = computed(() => tcOutput.value.join('\n'));

function addLimit() {
  if (!newLimit.value.interface) {
    ElMessage.warning('接口必填');
    return;
  }
  limits.value.push({ ...newLimit.value });
  newLimit.value = emptyLimit();
}

function removeLimit(row: LimitRow) {
  limits.value = limits.value.filter((l) => l !== row);
}

async function load() {
  try {
    const res = await api.get('/api/v1/qos/limits');
    limits.value = (res.data?.limits || []) as LimitRow[];
    tcOutput.value = res.data?.tc || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function submit() {
  if (!limits.value.length) {
    ElMessage.warning('请先添加规则');
    return;
  }
  submitting.value = true;
  try {
    const res = await api.post('/api/v1/qos/limits', { limits: limits.value });
    tcOutput.value = res.data?.tc || res.data?.commands || [];
    ElMessage.success('已提交');
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    submitting.value = false;
  }
}

const smartRoot = ref(100);
const smartIface = ref('eth0');
const smartLoading = ref(false);
const smartApplying = ref(false);
const smartPlan = ref<any[]>([]);

async function previewSmart() {
  smartLoading.value = true;
  try {
    const res = await api.get('/api/v1/htb/smart-plan', { params: { iface: smartIface.value, root_mbit: smartRoot.value } });
    smartPlan.value = res.data?.plan || res.data?.members || [];
  } catch (e: any) { showError(e); }
  smartLoading.value = false;
}
async function applySmart() {
  smartApplying.value = true;
  try {
    await api.post('/api/v1/htb/smart-apply', { iface: smartIface.value, root_mbit: smartRoot.value });
    ElMessage.success('已下发');
    smartPlan.value = [];
  } catch (e: any) { showError(e); }
  smartApplying.value = false;
}

onMounted(() => {
  load();
  loadClientLimits();
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
