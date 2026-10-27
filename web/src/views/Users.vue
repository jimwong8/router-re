<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>用户管理</span>
          <el-button type="primary" size="small" :loading="saving" @click="openCreate = true">新建用户</el-button>
        </div>
      </template>

      <el-table :data="users" style="width:100%; margin-bottom:12px" row-key="username"
        :empty-text="errorMsg ? errorMsg : '暂无用户'">
        <el-table-column prop="username" label="用户名" />
        <el-table-column prop="role" label="角色">
          <template #default="s">
            <el-tag :type="roleTag(s.row.role)">
              {{ s.row.role }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="enabled" label="启用" width="70">
          <template #default="s">
            <el-switch :active-value="true" :inactive-value="false" v-model="s.row.enabled" disabled />
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间">
          <template #default="s">
            <span>{{ fmtTime(s.row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="last_login" label="最后登录">
          <template #default="s">
            <span>{{ fmtTime(s.row.last_login) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="s">
            <el-button
              type="primary"
              link
              size="small"
              :disabled="s.row.username === currentUser"
              @click="openChangePassword(s.row.username)"
            >改密</el-button>
            <el-button
              type="danger"
              link
              size="small"
              :disabled="s.row.username === currentUser"
              title="不能删除当前登录用户"
              @click="remove(s.row.username)"
            >删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新建用户对话框 -->
    <el-dialog title="新建用户" :visible="openCreate" @close="openCreate = false">
      <el-form :model="form">
        <el-form-item label="用户名" label-width="80px">
          <el-input v-model="form.username" placeholder="3-32 位，a-z A-Z 0-9 _ . -" />
        </el-form-item>
        <el-form-item label="密码" label-width="80px">
          <el-input v-model="form.password" type="password" />
        </el-form-item>
        <el-form-item label="角色" label-width="80px">
          <el-select v-model="form.role" style="width:100%">
            <el-option label="admin" value="admin" />
            <el-option label="operator" value="operator" />
            <el-option label="viewer" value="viewer" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="openCreate = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="create">创建</el-button>
      </template>
    </el-dialog>

    <!-- 改密对话框 -->
    <el-dialog title="修改密码" :visible="pwDialogVisible" @close="pwDialogVisible = false">
      <el-form :model="pwForm">
        <el-form-item label="新密码" label-width="80px">
          <el-input v-model="pwForm.password" type="password" placeholder="8-64 位，含字母与数字" />
        </el-form-item>
        <el-form-item label="确认" label-width="80px">
          <el-input v-model="pwForm.confirm" type="password" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pwDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingPw" @click="changePwConfirm">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

interface User {
  username: string;
  role: string;
  enabled: boolean;
  created_at: number;
  last_login: number;
}

const users = ref<User[]>([]);
const errorMsg = ref('');
const saving = ref(false);
const savingPw = ref(false);
const openCreate = ref(false);
const pwDialogVisible = ref(false);
const changeTarget = ref('');
const currentUser = localStorage.getItem('router_user') || '';
const form = ref({ username: '', password: '', role: 'viewer' });
const pwForm = ref({ password: '', confirm: '' });

const roleTag = (role: string) => {
  if (role === 'admin') return 'danger';
  if (role === 'operator') return 'success';
  return 'info';
};

function fmtTime(ts: number | undefined): string {
  if (!ts) return '-';
  return new Date(ts * 1000).toLocaleString();
}

async function loadUsers() {
  errorMsg.value = '';
  try {
    const res = await api.get('/api/v1/users');
    users.value = res.data?.users || [];
  } catch (e: any) {
    errorMsg.value = e?.response?.data?.message || e.message;
    ElMessage.error(errorMsg.value);
  }
}

async function create() {
  if (!form.value.username || !form.value.password) {
    ElMessage.error('请输入用户名与密码');
    return;
  }
  saving.value = true;
  try {
    const res = await api.post('/api/v1/users', form.value);
    ElMessage.success(res.data?.user?.username ? '创建成功' : '创建完成');
    openCreate.value = false;
    form.value = { username: '', password: '', role: 'viewer' };
    await loadUsers();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

function openChangePassword(name: string) {
  changeTarget.value = name;
  pwForm.value = { password: '', confirm: '' };
  pwDialogVisible.value = true;
}

async function changePwConfirm() {
  if (pwForm.value.password !== pwForm.value.confirm) {
    ElMessage.error('两次密码不一致');
    return;
  }
  if (!pwForm.value.password) {
    ElMessage.error('请输入新密码');
    return;
  }
  savingPw.value = true;
  try {
    await api.post(`/api/v1/users/${encodeURIComponent(changeTarget.value)}/password`, pwForm.value);
    ElMessage.success('密码修改成功');
    pwDialogVisible.value = false;
    pwForm.value = { password: '', confirm: '' };
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    savingPw.value = false;
  }
}

async function remove(name: string) {
  try {
    await ElMessageBox.confirm(`确定删除用户 ${name}？`, '删除', { type: 'warning' });
  } catch {
    return;
  }
  try {
    await api.delete(`/api/v1/users/${encodeURIComponent(name)}`);
    ElMessage.success('删除成功');
    await loadUsers();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

onMounted(loadUsers);
</script>

<style scoped>
</style>