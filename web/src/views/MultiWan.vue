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

    <el-card shadow="never" style="margin-top: 16px;">
      <template #header>Multi-WAN 策略</template>
      <el-form inline style="margin-bottom: 12px;">
        <el-form-item label="探测类型">
          <el-select v-model="newProbe.type" style="width: 120px;">
            <el-option label="ICMP" value="icmp" />
            <el-option label="HTTP" value="http" />
            <el-option label="DNS" value="dns" />
          </el-select>
        </el-form-item>
        <el-form-item label="目标">
          <el-input v-model="newProbe.target" placeholder="如 114.114.114.114" style="width: 160px;" />
        </el-form-item>
        <el-form-item label="间隔(s)">
          <el-input-number v-model="newProbe.interval_sec" :min="1" :max="300" style="width: 120px;" />
        </el-form-item>
        <el-form-item label="超时(s)">
          <el-input-number v-model="newProbe.timeout_sec" :min="1" :max="30" style="width: 120px;" />
        </el-form-item>
        <el-form-item label="失败阈值">
          <el-input-number v-model="newProbe.fail_threshold" :min="1" :max="10" style="width: 120px;" />
        </el-form-item>
        <el-form-item label="恢复阈值">
          <el-input-number v-model="newProbe.recover_threshold" :min="1" :max="10" style="width: 120px;" />
        </el-form-item>
      </el-form>
      <el-form inline style="margin-bottom: 12px;">
        <el-form-item label="成员">
          <el-input v-model="newMember.name" placeholder="名称" style="width: 110px;" />
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="newMember.iface" placeholder="如 eth0" style="width: 110px;" />
        </el-form-item>
        <el-form-item label="网关">
          <el-input v-model="newMember.gateway" placeholder="如 1.2.3.4" style="width: 140px;" />
        </el-form-item>
        <el-form-item label="权重">
          <el-input-number v-model="newMember.weight" :min="1" :max="100" style="width: 100px;" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="newMember.enabled" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="addMember">添加成员</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="multiwanError" :title="multiwanError" type="error" show-icon :closable="false" style="margin-bottom: 12px;" />
      <el-table :data="multiwanMembers" border style="width: 100%;" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="60" />
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="iface" label="接口" width="100" />
        <el-table-column prop="gateway" label="网关" />
        <el-table-column prop="weight" label="权重" width="80" />
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="markDirty" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row, $index }">
            <el-button type="danger" size="small" @click="removeMember($index)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-button type="primary" :loading="multiwanSaving" @click="saveMultiWan">保存策略</el-button>
      <el-button @click="loadMultiWan">加载</el-button>
      <el-button style="margin-left: 8px;" @click="confirmClear">清空全部（需确认）</el-button>

      <el-divider style="margin: 16px 0;">成员探测</el-divider>
      <el-form inline style="margin-bottom: 12px;">
        <el-form-item label="成员">
          <el-select v-model="probeMember" :options="multiwanMembers.map(m => ({ value: m.name, label: m.name }))" style="width: 160px;" />
        </el-form-item>
        <el-form-item label="结果">
          <el-radio-group v-model="probeSuccess">
            <el-radio label="成功" :value="true" />
            <el-radio label="失败" :value="false" />
          </el-radio-group>
        </el-form-item>
        <el-form-item label="错误信息">
          <el-input v-model="probeError" placeholder="可选" style="width: 200px;" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="multiwanProbing" @click="sendProbe">上报探测结果</el-button>
        </el-form-item>
      </el-form>
      <el-alert v-if="probeResult" :title="probeResult" type="info" show-icon :closable="false" style="margin-bottom: 12px;" />

      <el-divider style="margin: 16px 0;">线路状态</el-divider>
      <el-table :data="multiwanStatesList" border style="width: 100%;">
        <el-table-column prop="name" label="名称" />
        <el-table-column label="在线" width="100">
          <template #default="{ row }">
            <el-tag :type="row.up ? 'success' : 'danger'">{{ row.up ? '在线' : '离线' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="fail_count" label="失败计数" width="100" />
        <el-table-column prop="recover_count" label="恢复计数" width="100" />
        <el-table-column prop="last_error" label="最后错误" />
        <el-table-column label="最后探测时间" width="160">
          <template #default="{ row }">{{ formatTime(row.last_probe_at) }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!multiwanStatesList.length" description="暂无线路状态" />
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

// ---- Multi-WAN 策略（/api/v1/multiwan, /api/v1/multiwan/probe） ----
const multiwanMembers = ref<any[]>([]);
const multiwanProbe = ref<any>({
  type: 'icmp',
  target: '114.114.114.114',
  interval_sec: 10,
  timeout_sec: 3,
  fail_threshold: 3,
  recover_threshold: 2,
});
const multiwanStates = ref<any>({});
const multiwanSaving = ref(false);
const multiwanError = ref('');
const multiwanProbing = ref(false);
const probeResult = ref('');
const newMember = ref<any>({ name: '', iface: '', gateway: '', weight: 1, enabled: true });
const newProbe = ref<any>({
  type: 'icmp',
  target: '114.114.114.114',
  interval_sec: 10,
  timeout_sec: 3,
  fail_threshold: 3,
  recover_threshold: 2,
});
const probeMember = ref('');
const probeSuccess = ref(true);
const probeError = ref('');

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

async function loadMultiWan() {
  multiwanError.value = '';
  try {
    const res = await api.get('/api/v1/multiwan');
    multiwanMembers.value = res.data?.members || [];
    multiwanProbe.value = res.data?.probe || multiwanProbe.value;
    multiwanStates.value = res.data?.states || {};
  } catch (e: any) {
    multiwanError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '加载 Multi-WAN 策略失败';
    ElMessage.error(multiwanError.value);
  }
}

async function saveMultiWan() {
  multiwanSaving.value = true;
  multiwanError.value = '';
  try {
    const confirm = multiwanMembers.value.length === 0;
    const res = await api.post('/api/v1/multiwan', {
      members: multiwanMembers.value,
      probe: multiwanProbe.value,
      confirm,
    });
    multiwanMembers.value = res.data?.members || multiwanMembers.value;
    multiwanProbe.value = res.data?.probe || multiwanProbe.value;
    ElMessage.success(res.data?.count === 0 ? '已清空全部 Multi-WAN 成员' : 'Multi-WAN 策略已保存');
  } catch (e: any) {
    multiwanError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '保存失败';
    ElMessage.error(multiwanError.value);
  } finally {
    multiwanSaving.value = false;
  }
}

function addMember() {
  const m = newMember.value;
  if (!m.name || !m.iface || !m.gateway) {
    ElMessage.warning('请填写名称、接口和网关');
    return;
  }
  if (multiwanMembers.value.some((x: any) => x.name === m.name)) {
    ElMessage.warning('成员名称已存在');
    return;
  }
  if (m.iface === 'lo') {
    ElMessage.warning('拒绝使用 loopback 接口作为 WAN 成员');
    return;
  }
  multiwanMembers.value.push({ ...m });
  newMember.value = { name: '', iface: '', gateway: '', weight: 1, enabled: true };
}

function removeMember(index: number) {
  multiwanMembers.value.splice(index, 1);
}

function markDirty() {}

function handleSelectionChange(val: any[]) {}

function confirmClear() {
  if (!window.confirm('清空全部 Multi-WAN 成员后选路将退化为无线路可用，确认继续？')) return;
  multiwanMembers.value = [];
  saveMultiWan();
}

async function sendProbe() {
  if (!probeMember.value) {
    ElMessage.warning('请选择成员');
    return;
  }
  multiwanProbing.value = true;
  probeResult.value = '';
  try {
    const res = await api.post('/api/v1/multiwan/probe', {
      member: probeMember.value,
      success: probeSuccess.value,
      error: probeError.value,
    });
    const st = res.data?.state;
    const usable = res.data?.usable || [];
    const usableCount = res.data?.usable_count ?? 0;
    const warning = res.data?.warning;
    probeResult.value = warning
      ? `警告：${warning}`
      : `成员 ${st?.name ?? probeMember.value} 状态已更新（在线=${st?.up}, 失败=${st?.fail_count}, 恢复=${st?.recover_count}），可用线路 ${usableCount} 条`;
    if (warning) ElMessage.warning(probeResult.value);
    else ElMessage.success(probeResult.value);
    // 刷新状态
    await loadMultiWan();
  } catch (e: any) {
    probeResult.value = e?.response?.data?.detail || e?.response?.data?.message || e.message || '探测上报失败';
    ElMessage.error(probeResult.value);
  } finally {
    multiwanProbing.value = false;
  }
}

function formatTime(ts: number): string {
  if (!ts) return '—';
  return new Date(ts * 1000).toLocaleString();
}

function multiwanStatesList(): any[] {
  const states = multiwanStates.value || {};
  return Object.values(states);
}

onMounted(() => {
  load();
  loadHealth();
  loadMultiWan();
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