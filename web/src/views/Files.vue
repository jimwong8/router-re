<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>文件管理</span>
          <div class="tools">
            <el-button size="small" @click="router.push('/storage')">磁盘管理</el-button>
            <el-button size="small" @click="create('mkdir')">新建文件夹</el-button>
            <el-button size="small" @click="create('newfile')">新建文件</el-button>
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
          </div>
        </div>
      </template>

      <div class="pathbar">
        <span class="root">{{ root }}</span>
        <template v-for="(seg, i) in segments" :key="i">
          <span class="sep">/</span>
          <a class="seg" @click="jump(i)">{{ seg }}</a>
        </template>
        <span v-if="!segments.length" class="sep">/</span>
      </div>

      <el-alert v-if="forbidden" type="error" :closable="false" show-icon
        :title="forbidden" style="margin-bottom:12px" />

      <el-table :data="entries" size="small" v-loading="loading" empty-text="这个目录是空的"
        @row-dblclick="onDblClick">
        <el-table-column label="名称" min-width="280">
          <template #default="{ row }">
            <span class="name" :class="{ dir: row.is_dir }" @click="onDblClick(row)">
              {{ row.is_dir ? '📁' : '📄' }} {{ row.name }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="90">
          <template #default="{ row }">{{ row.is_dir ? '目录' : '文件' }}</template>
        </el-table-column>
        <el-table-column label="大小" width="110">
          <template #default="{ row }">{{ row.is_dir ? '-' : fmtSize(row.size) }}</template>
        </el-table-column>
        <el-table-column label="权限" width="120">
          <template #default="{ row }"><span class="mono">{{ row.mode }}</span></template>
        </el-table-column>
        <el-table-column label="修改时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.mod_time) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="rename(row)">重命名</el-button>
            <el-button link type="primary" size="small" @click="move(row)">移动</el-button>
            <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

type E = { name: string; path: string; is_dir: boolean; size: number; mode: string; mod_time: number };

const router = useRouter();
const cwd = ref('/');
const root = ref('/etc/disk_user');
const entries = ref<E[]>([]);
const loading = ref(false);
const forbidden = ref('');

const segments = computed(() => cwd.value.split('/').filter(Boolean));

function fmtSize(b: number) {
  if (!b) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB'];
  let i = 0, v = b;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(1)} ${u[i]}`;
}
function fmtTime(ts: number) {
  if (!ts) return '-';
  const d = new Date(ts * 1000);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

async function load() {
  loading.value = true;
  forbidden.value = '';
  try {
    const res = await api.get('/api/v1/system/files', { params: { path: cwd.value } });
    entries.value = res.data?.entries || [];
    root.value = res.data?.root || root.value;
  } catch (e: any) {
    entries.value = [];
    forbidden.value = e?.response?.data?.message || e?.message || '读取失败';
  } finally {
    loading.value = false;
  }
}

function onDblClick(row: E) {
  if (!row.is_dir) return;
  cwd.value = row.path;
  load();
}

function jump(i: number) {
  cwd.value = '/' + segments.value.slice(0, i + 1).join('/');
  load();
}

async function op(body: any, okMsg: string) {
  try {
    await api.post('/api/v1/system/files', body);
    ElMessage.success(okMsg);
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '操作失败');
  }
}

async function create(kind: 'mkdir' | 'newfile') {
  try {
    const { value } = await ElMessageBox.prompt(kind === 'mkdir' ? '新文件夹名称' : '新文件名称', '新建', {
      inputPattern: /^[^/\\]+$/, inputErrorMessage: '名字不能包含路径分隔符',
    });
    await op({ op: kind, path: cwd.value, name: value }, kind === 'mkdir' ? '文件夹已创建' : '文件已创建');
  } catch { /* 取消 */ }
}

async function rename(row: E) {
  try {
    const { value } = await ElMessageBox.prompt('新名称', `重命名 ${row.name}`, {
      inputValue: row.name, inputPattern: /^[^/\\]+$/, inputErrorMessage: '名字不能包含路径分隔符',
    });
    await op({ op: 'rename', path: cwd.value, name: row.name, new_name: value }, '已重命名');
  } catch { /* 取消 */ }
}

async function move(row: E) {
  try {
    const { value } = await ElMessageBox.prompt('目标目录（绝对路径，如 /backup）', `移动 ${row.name}`, {
      inputValue: cwd.value,
    });
    await op({ op: 'move', path: cwd.value, name: row.name, dst_dir: value }, '已移动');
  } catch { /* 取消 */ }
}

async function remove(row: E) {
  try {
    await ElMessageBox.confirm(
      row.is_dir ? `删除目录「${row.name}」及其全部内容？此操作不可恢复。` : `删除文件「${row.name}」？`,
      '确认删除', { type: 'warning' });
  } catch { return; }
  await op({ op: 'remove', path: cwd.value, name: row.name }, '已删除');
}

onMounted(load);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.pathbar { display: flex; align-items: center; gap: 4px; font-size: 13px; margin-bottom: 12px; flex-wrap: wrap;
  background: #0f1117; padding: 8px 10px; border-radius: 6px; }
.root { color: #8b91a5; font-family: ui-monospace, Menlo, Consolas, monospace; }
.sep { color: #4b5563; }
.seg { color: #4f8cff; cursor: pointer; font-family: ui-monospace, Menlo, Consolas, monospace; }
.seg:hover { text-decoration: underline; }
.name { cursor: pointer; }
.name.dir { color: #6ee7b7; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; }
</style>
