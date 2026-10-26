<template>
  <div class="login-page">
    <el-card class="login-card" shadow="always">
      <template #header>
        <div class="login-header">router-re 管理系统</div>
      </template>
      <el-form :model="form" label-width="0" @submit.prevent="handleLogin">
        <el-form-item>
          <el-input v-model="form.username" placeholder="用户名" autocomplete="username" />
        </el-form-item>
        <el-form-item>
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            autocomplete="current-password"
            show-password
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" style="width: 100%" @click="handleLogin">
            登录
          </el-button>
        </el-form-item>
        <div v-if="errorMsg" class="login-error">{{ errorMsg }}</div>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import axios from 'axios';
import { ElMessage } from 'element-plus';

const router = useRouter();
const form = ref({ username: 'admin', password: '' });
const loading = ref(false);
const errorMsg = ref('');

async function handleLogin() {
  if (!form.value.username || !form.value.password) {
    errorMsg.value = '请输入用户名和密码';
    return;
  }
  loading.value = true;
  errorMsg.value = '';
  try {
    const res = await axios.post('/api/v1/auth/login', {
      username: form.value.username,
      password: form.value.password,
    });
    const token: string = res.data.token;
    localStorage.setItem('router_token', token);
    ElMessage.success('登录成功');
    router.push('/');
  } catch (e: any) {
    errorMsg.value = e?.response?.data?.message || '登录失败，请检查用户名密码';
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.login-page {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background: linear-gradient(135deg, #1e3c72 0%, #2a5298 100%);
}
.login-card {
  width: 380px;
}
.login-header {
  text-align: center;
  font-size: 18px;
  font-weight: 600;
}
.login-error {
  color: #f56c6c;
  font-size: 13px;
  text-align: center;
  margin-top: 4px;
}
</style>
