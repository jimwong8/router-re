<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>固件升级</span>
        </div>
      </template>

      <!-- 镜像检查 -->
      <el-card shadow="never" style="margin-bottom:16px">
        <template #header><span>镜像检查</span></template>
        <el-form :inline="true" label-width="90px">
          <el-form-item label="镜像路径">
            <el-input v-model="path" placeholder="/tmp/openwrt-sysupgrade.bin" style="width:320px" />
          </el-form-item>
          <el-form-item label="板型">
            <el-input v-model="localBoard" placeholder="x86/64" style="width:160px" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="checking" @click="checkImage">检查</el-button>
          </el-form-item>
        </el-form>

        <el-alert v-if="checkError" :title="checkError" type="error" :closable="false" style="margin-bottom:12px" />

        <div v-if="image">
          <el-descriptions title="镜像信息" :column="2" border style="margin-bottom:12px">
            <el-descriptions-item label="路径">{{ image.path }}</el-descriptions-item>
            <el-descriptions-item label="大小">{{ image.size }} bytes</el-descriptions-item>
            <el-descriptions-item label="SHA256">{{ image.sha256 }}</el-descriptions-item>
            <el-descriptions-item label="版本">{{ image.version || '-' }}</el-descriptions-item>
            <el-descriptions-item label="板型">{{ image.board }}</el-descriptions-item>
            <el-descriptions-item label="魔数">{{ image.magic }}</el-descriptions-item>
          </el-descriptions>
        </div>

        <div v-if="steps.length">
          <div style="font-weight:bold; margin:8px 0 4px">升级步骤：</div>
          <ol style="margin-left:20px">
            <li v-for="(s, i) in steps" :key="i">{{ s }}</li>
          </ol>
        </div>

        <div v-if="risks.length" style="margin-top:12px">
          <el-alert v-for="(r, i) in risks" :key="i" :title="r" type="warning" :closable="false" style="margin-bottom:8px" />
        </div>
      </el-card>

      <!-- 升级 -->
      <el-card shadow="never">
        <template #header><span>升级</span></template>
        <el-form label-width="120px">
          <el-form-item label="保留配置">
            <el-checkbox v-model="keepConfig">保留配置</el-checkbox>
          </el-form-item>
          <el-form-item label="确认">
            <el-checkbox v-model="confirmed">我确认升级会重启设备并中断网络</el-checkbox>
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="confirming" @click="verifyImage">二次验证</el-button>
            <el-button type="danger" :disabled="!confirmed" :loading="applying" @click="applyUpgrade" style="margin-left:8px">升级</el-button>
          </el-form-item>
        </el-form>

        <el-alert v-if="confirmError" :title="confirmError" type="error" :closable="false" style="margin:12px 0" />

        <div v-if="confirmResult">
          <el-descriptions title="二次验证结果" :column="2" border style="margin-bottom:12px">
            <el-descriptions-item label="verified">{{ confirmResult.verified }}</el-descriptions-item>
            <el-descriptions-item label="board_checked">{{ confirmResult.board_checked }}</el-descriptions-item>
            <el-descriptions-item label="SHA256">{{ confirmResult.image?.sha256 }}</el-descriptions-item>
            <el-descriptions-item label="板型">{{ confirmResult.image?.board }}</el-descriptions-item>
            <el-descriptions-item label="魔数">{{ confirmResult.image?.magic }}</el-descriptions-item>
            <el-descriptions-item label="大小">{{ confirmResult.image?.size }}</el-descriptions-item>
          </el-descriptions>
        </div>

        <el-alert v-if="applyError" :title="applyError" type="error" :closable="false" style="margin:12px 0" />

        <div v-if="applyResult">
          <el-alert title="设备即将重启，当前连接会断开" type="error" :closable="false" style="margin:12px 0" />
          <div style="font-weight:bold; margin:8px 0 4px">返回详情：</div>
          <pre>{{ applyResult.detail }}</pre>
          <div v-if="applyResult.note" style="margin-top:8px; color:#909399">{{ applyResult.note }}</div>
        </div>
      </el-card>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const path = ref('/tmp/openwrt-sysupgrade.bin');
const localBoard = ref('x86/64');
const checking = ref(false);
const checkError = ref('');
const image = ref<any>(null);
const steps = ref<string[]>([]);
const risks = ref<string[]>([]);

const keepConfig = ref(true);
const confirmed = ref(false);
const applying = ref(false);
const applyError = ref('');
const applyResult = ref<any>(null);

const confirming = ref(false);
const confirmError = ref('');
const confirmResult = ref<any>(null);

async function verifyImage() {
  confirming.value = true;
  confirmError.value = '';
  confirmResult.value = null;
  try {
    const res = await api.post('/api/v1/firmware/confirm', {
      path: path.value,
      sha256: image.value?.sha256 || '',
      board: localBoard.value,
    });
    const d = res.data || {};
    confirmResult.value = d;
    if (d.verified !== true) {
      confirmError.value = '二次验证未通过';
      ElMessage.error('二次验证未通过');
    } else {
      ElMessage.success('二次验证通过');
    }
  } catch (e: any) {
    confirmError.value = e?.response?.data?.message || e?.response?.data?.detail || e.message;
    ElMessage.error(confirmError.value);
  } finally {
    confirming.value = false;
  }
}

async function checkImage() {
  checking.value = true;
  checkError.value = '';
  image.value = null;
  steps.value = [];
  risks.value = [];
  try {
    const res = await api.post('/api/v1/firmware/plan', {
      path: path.value,
      local_board: localBoard.value,
      has_backup: false,
    });
    const d = res.data || {};
    image.value = d.image ?? null;
    steps.value = d.steps ?? [];
    risks.value = d.risks ?? [];
  } catch (e: any) {
    checkError.value = e?.response?.data?.message || e?.response?.data?.detail || e.message;
    ElMessage.error(checkError.value);
  } finally {
    checking.value = false;
  }
}

async function applyUpgrade() {
  applying.value = true;
  applyError.value = '';
  applyResult.value = null;
  try {
    const cres = await api.post('/api/v1/firmware/confirm', {
      path: path.value,
      sha256: image.value?.sha256 || '',
      board: localBoard.value,
    });
    const cd = cres.data || {};
    if (cd.verified !== true) {
      applyError.value = '二次验证未通过';
      ElMessage.error('二次验证未通过');
      return;
    }
    const res = await api.post('/api/v1/firmware/apply', {
      confirm: true,
      path: path.value,
      local_board: localBoard.value,
      keep_config: keepConfig.value,
    });
    const d = res.data || {};
    applyResult.value = d;
    ElMessage.success('升级已提交');
  } catch (e: any) {
    applyError.value = e?.response?.data?.message || e?.response?.data?.detail || e.message;
    ElMessage.error(applyError.value);
  } finally {
    applying.value = false;
  }
}

onMounted(() => {});
</script>

<style scoped>
pre { font-family: ui-monospace, SFMono-Regular, "Courier New", monospace; background:#1e1e1e; color:#d4d4d4; padding:8px; border-radius:4px; overflow-x:auto; }
</style>
