<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>链路检测</span>
          <div>
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
            <el-button type="primary" size="small" @click="openNew">新增目标</el-button>
          </div>
        </div>
      </template>

      <!-- 统计卡片 -->
      <el-row :gutter="16" style="margin-bottom:12px">
        <el-col :span="12"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold; color:#67c23a">{{ upCount }}</div>
          <div style="color:#909399; font-size:12px">在线</div>
        </div></el-card></el-col>
        <el-col :span="12"><el-card shadow="hover"><div style="text-align:center">
          <div style="font-size:24px; font-weight:bold; color:#f56c6c">{{ downCount }}</div>
          <div style="color:#909399; font-size:12px">离线</div>
        </div></el-card></el-col>
      </el-row>

      <!-- 目标表格 -->
      <el-table :data="targets" v-loading="loading" size="small" style="width:100%" empty-text="还没有链路检测目标">
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="iface" label="接口" width="90" />
        <el-table-column prop="kind" label="类型" width="80" />
        <el-table-column label="目标" min-width="180">
          <template #default="{ row }">
            {{ row.kind === 'http' ? row.url : row.host }}
          </template>
        </el-table-column>
        <el-table-column prop="interval" label="间隔(s)" width="90" />
        <el-table-column prop="fail_threshold" label="失败阈值" width="90" />
        <el-table-column prop="recover_threshold" label="恢复阈值" width="90" />
        <el-table-column label="启用" width="70">
          <template #default="{ row }"><el-switch v-model="row.enabled" size="small" /></template>
        </el-table-column>
        <el-table-column label="当前状态" width="100">
          <template #default="{ row }">
            <el-tooltip v-if="states[row.id]?.reason" :content="states[row.id].reason" placement="top">
              <el-tag v-if="states[row.id] && states[row.id].up" type="success" size="small">在线</el-tag>
              <el-tag v-else-if="states[row.id] && !states[row.id].up" type="danger" size="small">离线</el-tag>
              <el-tag v-else type="info" size="small">未探测</el-tag>
            </el-tooltip>
            <span v-else>
              <el-tag v-if="states[row.id] && states[row.id].up" type="success" size="small">在线</el-tag>
              <el-tag v-else-if="states[row.id] && !states[row.id].up" type="danger" size="small">离线</el-tag>
              <el-tag v-else type="info" size="small">未探测</el-tag>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160">
          <template #default="{ $index, row }">
            <el-button link type="primary" size="small" :loading="probing === row.id" @click="probe(row)">立即探测</el-button>
            <el-button link type="danger" size="small" @click="removeTarget($index)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div style="margin-top:12px; text-align:right">
        <el-button type="primary" :loading="saving" @click="saveAll">保存</el-button>
      </div>
    </el-card>

    <!-- 新增目标弹窗 -->
    <el-dialog v-model="dialog" title="新增链路检测目标" width="560px">
      <el-form :model="form" label-width="110px" size="small">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="接口"><el-input v-model="form.iface" placeholder="如 eth0" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.kind" style="width:100%">
            <el-option label="icmp" value="icmp" />
            <el-option label="dns" value="dns" />
            <el-option label="http" value="http" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.kind === 'icmp' || form.kind === 'dns'" label="Host">
          <el-input v-model="form.host" placeholder="8.8.8.8 或 example.com" />
        </el-form-item>
        <el-form-item v-if="form.kind === 'http'" label="URL">
          <el-input v-model="form.url" placeholder="https://example.com" />
        </el-form-item>
        <el-form-item label="间隔(s)">
          <el-input-number v-model="form.interval" :min="5" :max="300" />
        </el-form-item>
        <el-form-item label="超时(s)">
          <el-input-number v-model="form.timeout" :min="1" :max="30" />
        </el-form-item>
        <el-form-item label="失败阈值">
          <el-input-number v-model="form.fail_threshold" :min="1" :max="10" />
        </el-form-item>
        <el-form-item label="恢复阈值">
          <el-input-number v-model="form.recover_threshold" :min="1" :max="10" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button size="small" @click="dialog = false">取消</el-button>
        <el-button type="primary" size="small" @click="addTarget">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

interface Target {
  id: string; name: string; iface: string; kind: string;
  host: string; url: string; interval: number; timeout: number;
  fail_threshold: number; recover_threshold: number; enabled: boolean;
}
interface State { up: boolean; consec_fail: number; consec_ok: number; last_change: number; last_error: string; reason: string }

const loading = ref(false);
const saving = ref(false);
const probing = ref('');
const targets = ref<Target[]>([]);
const states = ref<Record<string, State>>({});
const dialog = ref(false);

const upCount = computed(() => {
  // 从 status 接口拿更准确；这里按 states 计
  return Object.values(states.value).filter(s => s.up).length;
});
const downCount = computed(() => {
  return Object.values(states.value).filter(s => !s.up).length;
});

const form = ref<Target>({
  id: '', name: '', iface: 'eth0', kind: 'icmp', host: '', url: '',
  interval: 10, timeout: 3, fail_threshold: 3, recover_threshold: 2, enabled: true,
});

function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/linkcheck/targets');
    targets.value = res.data?.targets || [];
    states.value = res.data?.states || {};
    // 同时拉 status 拿 up/down（states 已含 up 信息，够用）
    const st = await api.get('/api/v1/linkcheck/status');
    if (st.data?.states) states.value = st.data.states;
  } catch (e: any) {
    showError(e, '加载失败');
  } finally {
    loading.value = false;
  }
}

function openNew() {
  form.value = {
    id: 'lc' + Date.now(), name: '', iface: 'eth0', kind: 'icmp', host: '', url: '',
    interval: 10, timeout: 3, fail_threshold: 3, recover_threshold: 2, enabled: true,
  };
  dialog.value = true;
}

function addTarget() {
  if (!form.value.name.trim() || !form.value.iface.trim()) {
    ElMessage.warning('名称和接口不能为空');
    return;
  }
  targets.value.push({ ...form.value });
  dialog.value = false;
  ElMessage.success('已添加（记得保存生效）');
}

async function removeTarget(idx: number) {
  try {
    await ElMessageBox.confirm('确定删除该目标？', '提示', { type: 'warning' });
  } catch {
    return;
  }
  targets.value.splice(idx, 1);
  ElMessage.success('已删除（记得保存生效）');
}

async function saveAll() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/linkcheck/targets', { targets: targets.value });
    targets.value = res.data?.targets || targets.value;
    ElMessage.success('保存成功');
    await load();
  } catch (e: any) {
    showError(e, '保存失败');
  } finally {
    saving.value = false;
  }
}

async function probe(row: Target) {
  probing.value = row.id;
  try {
    const res = await api.post('/api/v1/linkcheck/probe', { id: row.id });
    const d = res.data || {};
    ElMessage[d.ok ? 'success' : 'warning'](d.reason || (d.ok ? '在线' : '离线'));
    if (d.state) states.value[row.id] = d.state;
    await load();
  } catch (e: any) {
    showError(e, '探测失败');
  } finally {
    probing.value = '';
  }
}

onMounted(load);
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; }
</style>
