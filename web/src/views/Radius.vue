<template>
  <div>
    <el-card shadow="never">
      <template #header>RADIUS 认证后端</template>

      <el-form label-width="100px" style="max-width: 600px;">
        <el-form-item label="服务器">
          <el-input v-model="cfg.server" placeholder="10.0.0.1" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="cfg.port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="密钥">
          <el-input
            v-model="cfg.secret"
            type="password"
            show-password
            :placeholder="secretSet ? '已设置，留空则不修改' : '未设置'"
          />
        </el-form-item>
        <el-form-item label="超时(秒)">
          <el-input-number v-model="cfg.timeout_sec" :min="1" :max="30" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存</el-button>
          <el-button @click="load">刷新</el-button>
        </el-form-item>
      </el-form>

      <el-divider />

      <el-card shadow="never" style="margin-top: 12px;">
        <template #header>测试认证</template>
        <el-form inline>
          <el-form-item label="用户名">
            <el-input v-model="testUser" />
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="testPass" type="password" show-password />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="testing" @click="runTest">测试</el-button>
          </el-form-item>
        </el-form>
        <el-alert v-if="testResult" :title="testResult.accept ? '认证通过' : '认证拒绝'" :type="testResult.accept ? 'success' : 'error'" show-icon :closable="false" style="margin-top: 8px;">
          <div>消息：{{ testResult.message || '-' }}</div>
          <div>属性：{{ JSON.stringify(testResult.attributes || {}) }}</div>
        </el-alert>
      </el-card>

      <el-divider />

      <el-collapse>
        <el-collapse-item title="radiusclient.conf" name="conf">
          <pre style="white-space: pre-wrap;">{{ confText }}</pre>
        </el-collapse-item>
        <el-collapse-item title="servers 文件" name="servers">
          <pre style="white-space: pre-wrap;">{{ serversText }}</pre>
        </el-collapse-item>
      </el-collapse>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const cfg = ref<any>({ server: '', port: 1812, secret: '', timeout_sec: 5 });
const confText = ref('');
const serversText = ref('');
const secretSet = ref(false);
const saving = ref(false);
const testing = ref(false);
const testUser = ref('');
const testPass = ref('');
const testResult = ref<any>(null);

async function load() {
  try {
    const res = await api.get('/api/v1/radius/config');
    cfg.value = res.data?.config || cfg.value;
    secretSet.value = !!res.data?.config?.secret_set;
    // 密钥只写不读：后端不回明文，输入框留空，避免把掩码值再提交回去
    cfg.value.secret = '';
    confText.value = res.data?.conf || '';
    serversText.value = res.data?.servers || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || '加载失败');
  }
}

async function save() {
  saving.value = true;
  try {
    const res = await api.post('/api/v1/radius/config', cfg.value);
    cfg.value = res.data?.config || cfg.value;
    secretSet.value = !!res.data?.config?.secret_set;
    cfg.value.secret = '';
    confText.value = res.data?.conf || '';
    serversText.value = res.data?.servers || '';
    ElMessage.success('保存成功');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || '保存失败');
  } finally {
    saving.value = false;
  }
}

async function runTest() {
  testing.value = true;
  testResult.value = null;
  try {
    const res = await api.post('/api/v1/radius/test', { username: testUser.value, password: testPass.value });
    testResult.value = res.data;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || '测试失败');
    testResult.value = e?.response?.data || null;
  } finally {
    testing.value = false;
  }
}

onMounted(load);
</script>