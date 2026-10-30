<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>分类日志</span>
          <div class="tools">
            <el-select v-model="since" size="small" style="width:130px" @change="load">
              <el-option label="最近 30 分钟" value="30m" />
              <el-option label="最近 2 小时" value="2h" />
              <el-option label="最近 1 天" value="1d" />
              <el-option label="最近 7 天" value="7d" />
              <el-option label="全部" value="" />
            </el-select>
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
          </div>
        </div>
      </template>

      <div class="layout">
        <div class="side">
          <div v-for="c in cats" :key="c.key" class="cat" :class="{ active: c.key === cur }"
            @click="pick(c.key)">
            <div class="cat-line">
              <span class="cat-name">{{ c.name }}</span>
              <el-tag v-if="!c.available" size="small" type="info" effect="plain">无源</el-tag>
            </div>
            <div class="cat-desc">{{ c.desc }}</div>
          </div>
        </div>

        <div class="main">
          <div class="meta">
            <span class="src">数据来源：<b class="mono">{{ source || '—' }}</b></span>
            <el-tag size="small" type="info">{{ entries.length }} 条</el-tag>
          </div>
          <el-alert v-if="note" type="info" :closable="false" show-icon :title="note" style="margin-bottom:10px" />
          <el-table :data="entries" size="small" v-loading="loading" empty-text="这一类没有日志" max-height="560">
            <el-table-column prop="ts" label="时间" width="170">
              <template #default="{ row }"><span class="mono">{{ row.ts }}</span></template>
            </el-table-column>
            <el-table-column label="级别" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="prioType(row.priority)">{{ row.priority }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="unit" label="来源单元" width="130" show-overflow-tooltip />
            <el-table-column prop="message" label="内容" min-width="420" show-overflow-tooltip />
          </el-table>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type Cat = { key: string; name: string; desc: string; source: string; available: boolean; note?: string };
type Entry = { ts: string; unit: string; priority: string; message: string };

const cats = ref<Cat[]>([]);
const cur = ref('system');
const entries = ref<Entry[]>([]);
const source = ref('');
const note = ref('');
const since = ref('1d');
const loading = ref(false);

function prioType(p: string) {
  if (p === 'err' || p === 'crit' || p === 'alert' || p === 'emerg') return 'danger';
  if (p === 'warning') return 'warning';
  return 'info';
}

async function loadCats() {
  try {
    const res = await api.get('/api/v1/logs/categories');
    cats.value = res.data?.categories || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载分类失败');
  }
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/logs/by-category', {
      params: { category: cur.value, since: since.value || undefined, limit: 300 },
    });
    entries.value = res.data?.entries || [];
    source.value = res.data?.source || '';
    note.value = res.data?.note || '';
  } catch (e: any) {
    entries.value = [];
    note.value = e?.response?.data?.message || e?.message || '查询失败';
  } finally {
    loading.value = false;
  }
}

function pick(k: string) {
  cur.value = k;
  load();
}

onMounted(async () => {
  await loadCats();
  await load();
});
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.layout { display: flex; gap: 16px; align-items: flex-start; }
.side { width: 240px; flex: none; }
.cat { padding: 9px 10px; border-radius: 6px; cursor: pointer; border: 1px solid transparent; margin-bottom: 4px; }
.cat:hover { background: #171b24; }
.cat.active { background: #1b2431; border-color: #2f5fa8; }
.cat-line { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
.cat-name { font-size: 13px; font-weight: 600; }
.cat-desc { font-size: 11px; color: #6b7280; margin-top: 3px; line-height: 1.35; }
.main { flex: 1; min-width: 0; }
.meta { display: flex; align-items: center; gap: 12px; margin-bottom: 10px; }
.src { font-size: 12px; color: #8b91a5; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; }
</style>
