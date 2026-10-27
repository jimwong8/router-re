<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>认证门户</span>
          <el-button type="primary" size="small" :loading="saving" @click="save">保存</el-button>
        </div>
      </template>

      <div style="color:#909399; font-size:13px; margin-bottom:12px">
        这个门户是给连上 WiFi 的终端弹出的认证页。终端连上 WiFi 后浏览器会被重定向到这里完成认证。
      </div>

      <el-row :gutter="16">
        <!-- 左侧：内置模板列表 -->
        <el-col :span="6">
          <div style="font-weight:600; margin-bottom:8px">内置模板</div>
          <el-menu :default-active="selectedIndex" @select="onSelectTemplate">
            <el-menu-item v-for="(t, idx) in builtins" :key="t.id || idx" :index="String(idx)">
              <span>{{ t.name || t.title || ('模板 ' + (idx + 1)) }}</span>
            </el-menu-item>
          </el-menu>
          <div v-if="!builtins.length" style="color:#909399; font-size:12px; padding:8px">暂无内置模板</div>
        </el-col>

        <!-- 右侧：表单 + 预览 -->
        <el-col :span="18">
          <el-form :model="form" label-width="110px" size="small">
            <el-form-item label="名称">
              <el-input v-model="form.name" placeholder="模板名称" />
            </el-form-item>
            <el-form-item label="类型">
              <el-select v-model="form.kind" style="width:100%">
                <el-option label="微信 (wechat)" value="wechat" />
                <el-option label="短信 (sms)" value="sms" />
                <el-option label="密码 (password)" value="password" />
                <el-option label="凭证 (voucher)" value="voucher" />
                <el-option label="一键 (onepress)" value="onepress" />
              </el-select>
            </el-form-item>
            <el-form-item label="标题">
              <el-input v-model="form.title" placeholder="门户标题" />
            </el-form-item>
            <el-form-item label="Logo URL">
              <el-input v-model="form.logo_url" placeholder="https://..." />
            </el-form-item>
            <el-form-item label="背景色">
              <el-color-picker v-model="form.bg_color" show-alpha />
            </el-form-item>
            <el-form-item label="文字色">
              <el-color-picker v-model="form.text_color" show-alpha />
            </el-form-item>
            <el-form-item label="公告">
              <el-input v-model="form.notice" type="textarea" :rows="3" placeholder="认证页公告内容" />
            </el-form-item>
            <el-form-item label="跳转地址">
              <el-input v-model="form.redirect_url" placeholder="认证成功后跳转的 URL" />
            </el-form-item>
          </el-form>

          <div style="margin-top:12px; font-weight:600; margin-bottom:6px">实时预览</div>
          <iframe :src="previewUrl" sandbox="allow-same-origin"
            style="width:100%; height:420px; border:1px solid #e4e7ed; border-radius:6px"></iframe>
          <div style="color:#909399; font-size:12px; margin-top:4px">
            预览地址：{{ previewUrl }}
          </div>
        </el-col>
      </el-row>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

interface Template {
  id: string; name: string; kind: string; title: string; logo_url: string;
  bg_color: string; text_color: string; notice: string; redirect_url: string;
}

const loading = ref(false);
const saving = ref(false);
const builtins = ref<Template[]>([]);
const current = ref<Template | null>(null);
const selectedIndex = ref('0');

const form = ref<Template>({
  id: '', name: '', kind: 'password', title: '', logo_url: '',
  bg_color: '#ffffff', text_color: '#303133', notice: '', redirect_url: '',
});

const previewUrl = computed(() => {
  const q = new URLSearchParams();
  if (form.value.title) q.set('title', form.value.title);
  if (form.value.kind) q.set('kind', form.value.kind);
  return '/api/v1/portal/preview?' + q.toString();
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
    const res = await api.get('/api/v1/portal/templates');
    const d = res.data || {};
    builtins.value = d.builtin || [];
    current.value = d.current || null;
    if (current.value) {
      form.value = { ...form.value, ...current.value };
    } else if (builtins.value.length) {
      form.value = { ...form.value, ...builtins.value[0] };
    }
  } catch (e: any) {
    showError(e, '加载模板失败');
  } finally {
    loading.value = false;
  }
}

function onSelectTemplate(idx: string) {
  selectedIndex.value = idx;
  const i = Number(idx);
  const t = builtins.value[i];
  if (t) {
    form.value = { ...form.value, ...t };
  }
}

async function save() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/portal/template', { template: form.value });
    const d = res.data || {};
    const msg = `saved=${d.saved}` + (d.detail ? `，${d.detail}` : '');
    ElMessage.success(msg);
    await load();
  } catch (e: any) {
    showError(e, '保存失败');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>
