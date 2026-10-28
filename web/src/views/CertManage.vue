<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>证书管理</span>
          <el-button size="small" :loading="loading" @click="load">刷新</el-button>
        </div>
      </template>

      <el-skeleton :rows="4" animated v-if="loading" />

      <!-- 不存在：提示 + 生成按钮 -->
      <template v-else-if="!exists">
        <el-alert
          type="warning"
          :closable="false"
          show-icon
          :title="detailText || '未找到配置文件证书'"
          description="需要为管理界面配置 TLS 自签证书才能启用 HTTPS。"
          style="margin-bottom:12px"
        />
        <el-button type="primary" :loading="generating" @click="openGen">生成自签证书</el-button>
      </template>

      <!-- 存在：证书信息 + 失效告警 -->
      <template v-else>
        <el-alert
          v-if="!valid"
          type="error"
          :closable="false"
          show-icon
          :title="warningText || '证书无效'"
          style="margin-bottom:12px"
        />

        <el-descriptions v-if="info" :column="2" border size="small">
          <el-descriptions-item label="Subject">{{ info.subject }}</el-descriptions-item>
          <el-descriptions-item label="Issuer">{{ info.issuer }}</el-descriptions-item>
          <el-descriptions-item label="生效时间">{{ fmtTime(info.not_before) }}</el-descriptions-item>
          <el-descriptions-item label="到期时间">{{ fmtTime(info.not_after) }}</el-descriptions-item>
          <el-descriptions-item label="剩余天数">
            <el-tag :type="daysTag" size="small">{{ daysLeft }} 天</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="签名算法">{{ info.sig_alg }}</el-descriptions-item>
          <el-descriptions-item label="DNS 名称">{{ dnsNames }}</el-descriptions-item>
          <el-descriptions-item label="自签">
            {{ info.self_signed ? '是' : '否' }}
          </el-descriptions-item>
          <el-descriptions-item label="密钥位数">{{ info.key_bits }}</el-descriptions-item>
        </el-descriptions>
      </template>
    </el-card>

    <!-- 生成自签证书弹窗 -->
    <el-dialog v-model="dialog" title="生成自签证书" width="560px">
      <el-form label-width="110px" size="small">
        <el-form-item label="主机名列表">
          <el-input v-model="hostnamesText" type="textarea" :rows="3" placeholder="逗号分隔，如：router.local, 192.168.1.1" />
          <div style="color:#909399; font-size:12px; margin-top:4px">默认填当前主机名，可逗号分隔多个</div>
        </el-form-item>
      </el-form>

      <el-alert
        v-if="genResult"
        :closable="false"
        show-icon
        type="success"
        title="自签证书已生成"
        description="证书路径见下；私钥仅保存在服务器，页面不会显示或下载私钥。"
        style="margin-bottom:10px"
      />
      <el-descriptions v-if="genResult" :column="1" border size="small" style="margin-bottom:6px">
        <el-descriptions-item label="证书路径">{{ genResult.cert_path }}</el-descriptions-item>
        <el-descriptions-item label="密钥路径">{{ genResult.key_path }}</el-descriptions-item>
        <el-descriptions-item label="证书大小">{{ genResult.cert_bytes }} 字节</el-descriptions-item>
        <el-descriptions-item label="密钥大小">{{ genResult.key_bytes }} 字节</el-descriptions-item>
      </el-descriptions>
      <el-alert v-if="genDetail" :closable="false" :title="genDetail" type="info" style="margin-bottom:6px" />

      <template #footer>
        <el-button size="small" @click="dialog = false">关闭</el-button>
        <el-button size="small" type="primary" :loading="generating" @click="generate">生成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface CertInfo {
  subject: string; issuer: string; not_before: number; not_after: number;
  dns_names: string[]; is_ca: boolean; self_signed: boolean;
  key_bits: number; sig_alg: string;
}

const loading = ref(false);
const generating = ref(false);
const exists = ref(false);
const valid = ref(true);
const info = ref<CertInfo | null>(null);
const detailText = ref('');
const warningText = ref('');
const daysLeft = ref(0);

const dialog = ref(false);
const hostnamesText = ref('');
const genResult = ref<{ cert_path: string; key_path: string; cert_bytes: number; key_bytes: number } | null>(null);
const genDetail = ref('');

// 后端错误格式 {message, detail}；detail 往往才是真正原因，一起显示。
function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

function fmtTime(ts: number | undefined): string {
  if (!ts) return '—';
  return new Date(ts * 1000).toLocaleString();
}

const dnsNames = computed(() => (info.value?.dns_names ?? []).join(', ') || '—');

// 剩余天数：<=30 橙，<0 红，否则正常
const daysTag = computed<'success' | 'warning' | 'danger'>(() => {
  if (daysLeft.value < 0) return 'danger';
  if (daysLeft.value <= 30) return 'warning';
  return 'success';
});

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/cert');
    const d = res.data || {};
    exists.value = !!d.exists;
    if (!d.exists) {
      detailText.value = d.detail || '';
      valid.value = true;
      info.value = null;
    } else {
      info.value = d.info || null;
      daysLeft.value = d.days_left ?? 0;
      valid.value = !!d.valid;
      warningText.value = d.warning || '';
    }
  } catch (e: any) {
    showError(e, '加载证书信息失败');
  } finally {
    loading.value = false;
  }
}

function openGen() {
  const host = (typeof location !== 'undefined' && location.hostname)
    ? location.hostname.split(':')[0]
    : '';
  hostnamesText.value = host ? `${host}, localhost` : 'localhost';
  genResult.value = null;
  genDetail.value = '';
  dialog.value = true;
}

async function generate() {
  const hostnames = hostnamesText.value
    .split(',')
    .map(s => s.trim())
    .filter(Boolean);
  if (hostnames.length === 0) {
    ElMessage.warning('请填写至少一个主机名');
    return;
  }
  generating.value = true;
  genResult.value = null;
  try {
    const res = await api.post('/api/v1/cert/self-signed', { hostnames });
    const d = res.data || {};
    genResult.value = {
      cert_path: d.cert_path || '',
      key_path: d.key_path || '',
      cert_bytes: d.cert_bytes ?? 0,
      key_bytes: d.key_bytes ?? 0,
    };
    genDetail.value = d.detail || '';
    if (!d.cert_path) genDetail.value = genDetail.value || '已生成（未返回路径）';
    // 注意：后端绝不返回私钥内容，页面也不显示/下载私钥。
    ElMessage.success('自签证书已生成');
    dialog.value = false;
    await load();
  } catch (e: any) {
    showError(e, '生成自签证书失败');
  } finally {
    generating.value = false;
  }
}

onMounted(load);
</script>