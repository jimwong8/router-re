<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>在线用户</span>
          <el-button size="small" :loading="loading" @click="load">刷新</el-button>
        </div>
      </template>

      <el-alert v-if="byteCountingNote" :title="byteCountingNote" type="warning" show-icon :closable="false" style="margin-bottom:12px" />

      <el-alert v-if="error" :title="error" type="error" show-icon :closable="true" @close="error=''" style="margin-bottom:12px" />

      <el-row :gutter="12" style="margin-bottom:16px">
        <el-col :span="6">
          <el-card shadow="hover" class="stat"><div class="n">{{ count }}</div><div class="t">账号总数</div></el-card>
        </el-col>
        <el-col :span="6">
          <el-card shadow="hover" class="stat"><div class="n">{{ conntrackEntries }}</div><div class="t">conntrack 条目</div></el-card>
        </el-col>
        <el-col :span="6">
          <el-card shadow="hover" class="stat"><div class="n">{{ onlineCount }}</div><div class="t">在线会话数</div></el-card>
        </el-col>
        <el-col :span="6">
          <el-card shadow="hover" class="stat">
            <div class="n" :style="{ color: byteCounting ? '#67c23a' : '#e6a23c' }">{{ byteCounting ? '已启用' : '未启用' }}</div>
            <div class="t">字节统计</div>
          </el-card>
        </el-col>
      </el-row>

      <el-table :data="sessions" v-loading="loading" size="small" style="width:100%" empty-text="暂无在线会话">
        <el-table-column prop="username" label="用户名" min-width="120" />
        <el-table-column label="在线" width="80">
          <template #default="{ row }">
            <el-tag :type="row.online ? 'success' : 'info'" size="small">{{ row.online ? '在线' : '离线' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" min-width="130" />
        <el-table-column prop="iface" label="接口" width="90" />
        <el-table-column label="登录时间" width="160">
          <template #default="{ row }">
            {{ row.login_at ? formatTime(row.login_at) : '未知' }}
          </template>
        </el-table-column>
        <el-table-column prop="rx_bytes" label="接收(rx)" width="120" />
        <el-table-column prop="tx_bytes" label="发送(tx)" width="120" />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button link type="danger" size="small" :disabled="!row.ip"
              @click="kick(row)">{{ row.ip ? '断开' : '无IP' }}</el-button>
            <div v-if="!row.ip" style="color:#909399; font-size:11px">该会话没有 IP，无法定位</div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

const loading = ref(false);
const error = ref('');
const count = ref(0);
const conntrackEntries = ref(0);
const byteCounting = ref(false);
const byteCountingNote = ref('');
const sessions = ref<any[]>([]);

const onlineCount = computed(() => sessions.value.filter((s: any) => s.online).length);

function formatTime(ts: number) {
  if (!ts) return '未知';
  const d = new Date(ts * 1000);
  if (isNaN(d.getTime())) return '未知';
  return d.toLocaleString();
}

function showError(e: any) {
  const d = e?.response?.data || {};
  const msg = d.message || d.detail || e?.message || '请求失败';
  const detail = d.detail;
  const full = detail && detail !== msg ? `${msg}：${detail}` : msg;
  error.value = full;
  ElMessage.error(full);
}

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const res = await api.get('/api/v1/online');
    const d = res.data || {};
    sessions.value = d.sessions || [];
    count.value = d.count ?? sessions.value.length;
    conntrackEntries.value = d.conntrack_entries ?? 0;
    byteCounting.value = !!d.byte_counting;
    byteCountingNote.value = d.byte_counting ? '' : (d.byte_counting_note || '');
  } catch (e: any) {
    showError(e);
  } finally {
    loading.value = false;
  }
}

async function kick(row: any) {
  if (!row.ip) {
    ElMessage.warning('该会话没有 IP，无法定位');
    return;
  }
  try {
    await ElMessageBox.confirm(`确定断开 ${row.username} (${row.ip})？`, '断开', { type: 'warning' });
  } catch {
    return;
  }
  try {
    const res = await api.post('/api/v1/online/kick', { username: row.username, ip: row.ip });
    const d = res.data || {};
    ElMessage.success(d.detail || '已断开');
    await load();
  } catch (e: any) {
    const d = e?.response?.data || {};
    const msg = d.message || d.detail || e?.message || '断开失败';
    const detail = d.detail;
    const full = detail && detail !== msg ? `${msg}：${detail}` : msg;
    error.value = full;
    ElMessage.error(full);
  }
}

onMounted(load);
</script>

<style scoped>
.stat { text-align: center; }
.stat .n { font-size: 26px; font-weight: 600; }
.stat .t { color: #909399; font-size: 12px; margin-top: 4px; }
</style>
