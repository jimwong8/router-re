<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>应用商店</span>
          <div class="tools">
            <el-select v-model="cat" size="small" style="width:130px">
              <el-option label="全部分类" value="" />
              <el-option v-for="c in cats" :key="c" :label="c" :value="c" />
            </el-select>
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px" :title="note" />
      <el-alert v-if="conflictText" type="warning" :closable="false" show-icon style="margin-bottom:12px" :title="conflictText" />

      <div class="grid">
        <div v-for="p in shown" :key="p.id" class="card" :class="{ missing: !p.installed }">
          <div class="card-hdr">
            <span class="name">{{ p.name }}</span>
            <el-tag size="small" :type="tagType(p)">{{ p.state_text }}</el-tag>
          </div>
          <div class="cat-tag">{{ p.category }}</div>
          <div class="desc">{{ p.desc }}</div>
          <div class="meta">
            <span class="mono">{{ p.binary }}</span>
            <span v-if="p.version" class="ver" :title="p.version">{{ p.version }}</span>
          </div>
          <div class="meta" v-if="p.ports && p.ports.length">
            端口：<span class="mono">{{ p.ports.join(', ') }}</span>
          </div>
          <div class="meta" v-if="p.pid">PID <span class="mono">{{ p.pid }}</span></div>

          <div class="actions">
            <template v-if="p.installed">
              <el-button v-if="!p.running" size="small" type="primary" :loading="busy === p.id"
                @click="act(p, 'start')">启动</el-button>
              <el-button v-else size="small" type="warning" :loading="busy === p.id"
                @click="act(p, 'stop')">停止</el-button>
              <el-button size="small" @click="toggleAuto(p)">
                {{ p.enabled ? '取消自启' : '开机自启' }}
              </el-button>
              <el-button size="small" link @click="showLog(p)">日志</el-button>
            </template>
            <template v-else>
              <span class="hint">未安装，需先把 {{ p.binary }} 放到 PATH</span>
              <a v-if="p.homepage" class="link" :href="p.homepage" target="_blank" rel="noreferrer">项目地址</a>
            </template>
          </div>
        </div>
      </div>
      <el-empty v-if="!loading && !shown.length" description="没有匹配的插件" />
    </el-card>

    <el-dialog v-model="logDlg" :title="'日志：' + logId" width="720px">
      <pre class="code">{{ logLines.length ? logLines.join('\n') : '（暂无日志）' }}</pre>
      <template #footer><el-button @click="logDlg = false">关闭</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type P = {
  id: string; name: string; category: string; desc: string; homepage: string;
  binary: string; installed: boolean; version?: string; enabled: boolean; running: boolean;
  pid?: number; ports?: number[]; state_text: string; config_path?: string;
};

const list = ref<P[]>([]);
const notes = ref('');
const note = ref('');
const conflicts = ref<Record<string, string[]>>({});
const loading = ref(false);
const busy = ref('');
const cat = ref('');

const logDlg = ref(false);
const logId = ref('');
const logLines = ref<string[]>([]);

const cats = computed(() => Array.from(new Set(list.value.map((p) => p.category))));
const shown = computed(() => (cat.value ? list.value.filter((p) => p.category === cat.value) : list.value));
const conflictText = computed(() => {
  const ks = Object.keys(conflicts.value);
  if (!ks.length) return '';
  return ks.map((k) => `端口 ${k} 被 ${conflicts.value[k].join(' / ')} 同时占用`).join('；');
});

function tagType(p: P) {
  if (!p.installed) return 'info';
  if (p.running) return 'success';
  if (p.enabled) return 'warning';
  return 'info';
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/plugins');
    list.value = res.data?.plugins || [];
    conflicts.value = res.data?.conflicts || {};
    note.value = res.data?.note || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  } finally {
    loading.value = false;
  }
}

async function act(p: P, action: string) {
  busy.value = p.id;
  try {
    const res = await api.post(`/api/v1/plugins/${p.id}`, { action });
    if (res.data?.note) ElMessage.success(res.data.note);
    else ElMessage.success(action === 'start' ? '已启动' : '已停止');
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '操作失败');
  } finally {
    busy.value = '';
  }
}

async function toggleAuto(p: P) {
  await act(p, p.enabled ? 'disable' : 'enable');
}

async function showLog(p: P) {
  logId.value = p.name;
  logDlg.value = true;
  logLines.value = [];
  try {
    const res = await api.get(`/api/v1/plugins/${p.id}/log`);
    logLines.value = res.data?.lines || [];
    if (!logLines.value.length && res.data?.note) logLines.value = ['（' + res.data.note + '）'];
  } catch (e: any) {
    logLines.value = ['读取日志失败：' + (e?.response?.data?.message || e?.message)];
  }
}

onMounted(load);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 12px; }
.card { border: 1px solid #2a2f3a; border-radius: 8px; padding: 12px; display: flex; flex-direction: column; gap: 5px; }
.card.missing { opacity: 0.78; border-style: dashed; }
.card-hdr { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.name { font-weight: 600; font-size: 14px; }
.cat-tag { font-size: 11px; color: #6b7280; }
.desc { font-size: 12px; color: #9aa3b2; line-height: 1.45; min-height: 34px; }
.meta { font-size: 11px; color: #8b91a5; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; }
.ver { margin-left: 8px; color: #6b7280; }
.actions { display: flex; align-items: center; gap: 6px; margin-top: 6px; flex-wrap: wrap; }
.hint { font-size: 11px; color: #8b91a5; }
.link { font-size: 11px; color: #4f8cff; margin-left: 6px; }
.code { background: #0f1117; color: #d5dae6; padding: 12px; border-radius: 6px; font-size: 12px; overflow: auto; max-height: 420px; margin: 0; white-space: pre-wrap; }
</style>
