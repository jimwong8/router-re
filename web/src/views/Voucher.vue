<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>卡密认证</span>
          <div>
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
            <el-button type="primary" size="small" @click="genDialog = true">批量生成</el-button>
            <el-button size="small" @click="redeemDialog = true">核销</el-button>
          </div>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon :closable="true" @close="error=''" style="margin-bottom:12px" />

      <el-row :gutter="12" style="margin-bottom:16px">
        <el-col :span="8"><el-card shadow="hover" class="stat"><div class="n">{{ unused }}</div><div class="t">未使用</div></el-card></el-col>
        <el-col :span="8"><el-card shadow="hover" class="stat"><div class="n">{{ used }}</div><div class="t">已使用</div></el-card></el-col>
        <el-col :span="8"><el-card shadow="hover" class="stat"><div class="n">{{ count }}</div><div class="t">总计</div></el-card></el-col>
      </el-row>

      <el-table :data="vouchers" v-loading="loading" size="small" style="width:100%" empty-text="暂无卡密">
        <el-table-column prop="code" label="卡号" min-width="140" />
        <el-table-column prop="password" label="卡密" min-width="140" />
        <el-table-column label="时长(分)" width="90">
          <template #default="{ row }">{{ row.minutes === 0 ? '不限' : row.minutes }}</template>
        </el-table-column>
        <el-table-column label="流量(MB)" width="100">
          <template #default="{ row }">{{ row.traffic_mb === 0 ? '不限' : row.traffic_mb }}</template>
        </el-table-column>
        <el-table-column prop="device_limit" label="设备数" width="80" />
        <el-table-column label="有效期" width="120">
          <template #default="{ row }">{{ row.expires_at ? fmtDate(row.expires_at) : '永久' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.expired" type="danger" size="small">已过期</el-tag>
            <el-tag v-else-if="row.used" type="success" size="small">已使用</el-tag>
            <el-tag v-else type="info" size="small">未使用</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="剩余(分)" width="90">
          <template #default="{ row }">{{ row.remaining_min }}</template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-button link type="danger" size="small" @click="remove(row.code)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 批量生成 -->
    <el-dialog v-model="genDialog" title="批量生成卡密" width="560px">
      <el-form :model="genForm" label-width="110px" size="small">
        <el-form-item label="数量(count)">
          <el-input-number v-model="genForm.count" :min="1" :max="500" />
        </el-form-item>
        <el-form-item label="卡号长度">
          <el-input-number v-model="genForm.code_len" :min="6" :max="32" />
        </el-form-item>
        <el-form-item label="时长(分)">
          <el-input-number v-model="genForm.minutes" :min="0" />
        </el-form-item>
        <el-form-item label="流量(MB)">
          <el-input-number v-model="genForm.traffic_mb" :min="0" />
        </el-form-item>
        <el-form-item label="设备数">
          <el-input-number v-model="genForm.device_limit" :min="1" :max="10" />
        </el-form-item>
        <el-form-item label="有效期(天)">
          <el-input-number v-model="genForm.expire_days" :min="0" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="genForm.note" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="genDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="generate">生成</el-button>
      </template>
    </el-dialog>

    <!-- 生成结果明文展示 -->
    <el-dialog v-model="resultDialog" title="生成结果（明文仅显示一次）" width="720px">
      <el-alert type="warning" :closable="false" show-icon
        title="明文卡密只显示这一次，关闭后无法再查看，请立即保存/复制" style="margin-bottom:12px" />
      <div style="margin-bottom:8px">
        <el-button size="small" type="primary" @click="copyAll">复制全部</el-button>
      </div>
      <pre style="background:#f5f7fa; padding:12px; border-radius:4px; max-height:360px; overflow:auto; margin:0">{{ resultText }}</pre>
      <template #footer>
        <el-button type="primary" @click="resultDialog = false">我已保存，关闭</el-button>
      </template>
    </el-dialog>

    <!-- 核销 -->
    <el-dialog v-model="redeemDialog" title="核销卡密" width="480px">
      <el-form :model="redeemForm" label-width="90px" size="small">
        <el-form-item label="卡号">
          <el-input v-model="redeemForm.code" />
        </el-form-item>
        <el-form-item label="卡密">
          <el-input v-model="redeemForm.password" type="password" show-password />
        </el-form-item>
      </el-form>
      <div v-if="redeemResult" style="margin-top:12px">
        <el-alert type="success" :closable="false" show-icon
          :title="`核销成功：时长 ${redeemResult.minutes} 分钟，流量 ${redeemResult.traffic_mb} MB，剩余 ${redeemResult.remaining_min} 分钟`" />
      </div>
      <template #footer>
        <el-button @click="redeemDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="redeem">核销</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

const loading = ref(false);
const saving = ref(false);
const error = ref('');

const count = ref(0);
const used = ref(0);
const unused = ref(0);
const vouchers = ref<any[]>([]);

const genDialog = ref(false);
const resultDialog = ref(false);
const resultText = ref('');
const genForm = ref<any>({ count: 10, code_len: 10, minutes: 60, traffic_mb: 0, device_limit: 1, expire_days: 0, note: '' });

const redeemDialog = ref(false);
const redeemForm = ref<any>({ code: '', password: '' });
const redeemResult = ref<any>(null);

function showError(e: any) {
  const d = e?.response?.data || {};
  const msg = d.message || d.detail || e?.message || '请求失败';
  const detail = d.detail;
  const full = detail && detail !== msg ? `${msg}：${detail}` : msg;
  error.value = full;
  ElMessage.error(full);
}

function fmtDate(ts: number) {
  if (!ts) return '';
  const d = new Date(ts * 1000);
  if (isNaN(d.getTime())) return '';
  return d.toLocaleDateString();
}

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const res = await api.get('/api/v1/voucher/list');
    const d = res.data || {};
    vouchers.value = d.vouchers || [];
    count.value = d.count ?? vouchers.value.length;
    used.value = d.used ?? 0;
    unused.value = d.unused ?? 0;
  } catch (e: any) {
    showError(e);
  } finally {
    loading.value = false;
  }
}

async function generate() {
  saving.value = true;
  error.value = '';
  try {
    const res = await api.post('/api/v1/voucher/generate', genForm.value);
    const d = res.data || {};
    const vs = d.vouchers || [];
    resultText.value = vs.map((v: any) => `${v.code}\t${v.password}`).join('\n');
    genDialog.value = false;
    resultDialog.value = true;
    ElMessage.success(`已生成 ${d.generated ?? vs.length} 张`);
    await load();
  } catch (e: any) {
    showError(e);
  } finally {
    saving.value = false;
  }
}

async function copyAll() {
  try {
    await navigator.clipboard.writeText(resultText.value);
    ElMessage.success('已复制到剪贴板');
  } catch (e) {
    ElMessage.warning('复制失败，请手动选择文本复制');
  }
}

async function redeem() {
  saving.value = true;
  error.value = '';
  redeemResult.value = null;
  try {
    const res = await api.post('/api/v1/voucher/redeem', redeemForm.value);
    redeemResult.value = res.data || {};
    ElMessage.success(res.data?.detail || '核销成功');
    await load();
  } catch (e: any) {
    const d = e?.response?.data || {};
    const msg = d.message || d.detail || e?.message || '核销失败';
    const detail = d.detail;
    const full = detail && detail !== msg ? `${msg}：${detail}` : msg;
    error.value = full;
    ElMessage.error(full);
  } finally {
    saving.value = false;
  }
}

async function remove(code: string) {
  try {
    await ElMessageBox.confirm(`确定删除卡号 ${code}？`, '删除', { type: 'warning' });
  } catch {
    return;
  }
  try {
    await api.delete('/api/v1/voucher', { params: { code } });
    ElMessage.success('已删除');
    await load();
  } catch (e: any) {
    showError(e);
  }
}

onMounted(load);
</script>

<style scoped>
.stat { text-align: center; }
.stat .n { font-size: 26px; font-weight: 600; }
.stat .t { color: #909399; font-size: 12px; margin-top: 4px; }
</style>
