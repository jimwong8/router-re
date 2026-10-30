<template>
  <div>
    <el-row :gutter="12" style="margin-bottom:16px">
      <el-col :span="4" v-for="c in cards" :key="c.key">
        <el-card shadow="never" class="stat">
          <div class="stat-val">{{ stats[c.key] || 0 }}</div>
          <div class="stat-lbl">{{ c.label }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="never" class="stat">
          <div class="stat-val">{{ online.length }}</div>
          <div class="stat-lbl">当前在线</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>行为记录</span>
          <div class="tools">
            <el-radio-group v-model="kind" size="small" @change="reload">
              <el-radio-button value="">全部</el-radio-button>
              <el-radio-button value="web">上网记录</el-radio-button>
              <el-radio-button value="im">IM 记录</el-radio-button>
              <el-radio-button value="online">上下线</el-radio-button>
              <el-radio-button value="app">应用</el-radio-button>
            </el-radio-group>
            <el-input v-model="keyword" size="small" placeholder="域名 / 应用 / 终端" clearable style="width:200px" @keyup.enter="reload" />
            <el-button size="small" :loading="loading" @click="reload">查询</el-button>
            <el-button size="small" @click="openExempt">免记录设置</el-button>
          </div>
        </div>
      </template>

      <el-table :data="records" size="small" v-loading="loading" empty-text="还没有记录（需要 DPI / 认证模块上报）">
        <el-table-column label="时间" width="160">
          <template #default="{ row }">{{ fmt(row.ts) }}</template>
        </el-table-column>
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.kind === 'online' ? 'success' : row.kind === 'im' ? 'warning' : 'info'">
              {{ kindName(row.kind) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="terminal" label="终端" width="140" show-overflow-tooltip />
        <el-table-column prop="mac" label="MAC" width="150" />
        <el-table-column prop="ip" label="IP" width="130" />
        <el-table-column prop="target" label="目标" min-width="180" show-overflow-tooltip />
        <el-table-column prop="detail" label="详情" min-width="160" show-overflow-tooltip />
      </el-table>

      <div class="pager">
        <el-pagination
          layout="total, prev, pager, next, sizes"
          :total="total"
          :page-size="pageSize"
          :current-page="page"
          :page-sizes="[50, 100, 200, 500]"
          @current-change="onPage"
          @size-change="onSize"
        />
      </div>
    </el-card>

    <el-drawer v-model="exemptDrawer" title="免记录设置" size="560px">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="命中的终端 / IP / MAC 不再落库。用于隐私机型或无关设备（例如摄像头）。" />
      <el-table :data="exempts" size="small" empty-text="没有免记录规则">
        <el-table-column label="类型" width="110">
          <template #default="{ row }">{{ row.kind ? kindName(row.kind) : '全部' }}</template>
        </el-table-column>
        <el-table-column label="匹配" width="100">
          <template #default="{ row }">{{ row.match_type }}</template>
        </el-table-column>
        <el-table-column prop="value" label="值" min-width="150" />
        <el-table-column prop="comment" label="备注" min-width="120" />
        <el-table-column label="" width="70">
          <template #default="{ $index }">
            <el-button link type="danger" size="small" @click="exempts.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-divider />
      <el-form :inline="true" size="small">
        <el-form-item label="类型">
          <el-select v-model="newEx.EmptyKind" style="width:110px">
            <el-option label="全部" value="" />
            <el-option label="上网" value="web" />
            <el-option label="IM" value="im" />
            <el-option label="上下线" value="online" />
            <el-option label="应用" value="app" />
          </el-select>
        </el-form-item>
        <el-form-item label="匹配">
          <el-select v-model="newEx.match_type" style="width:100px">
            <el-option label="MAC" value="mac" />
            <el-option label="IP" value="ip" />
            <el-option label="终端名" value="terminal" />
          </el-select>
        </el-form-item>
        <el-form-item label="值">
          <el-input v-model="newEx.value" style="width:160px" placeholder="aa:bb:… / 1.2.3.4" />
        </el-form-item>
        <el-form-item>
          <el-button size="small" @click="addExempt">添加</el-button>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="exemptDrawer = false">取消</el-button>
        <el-button type="primary" @click="saveExempt">保存</el-button>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type Rec = { ts: number; kind: string; mac: string; ip: string; terminal: string; target: string; detail: string };
type Ex = { kind: string; match_type: string; value: string; comment?: string };

const records = ref<Rec[]>([]);
const stats = ref<Record<string, number>>({});
const online = ref<string[]>([]);
const kind = ref('');
const keyword = ref('');
const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(50);

const cards = [
  { key: 'web', label: '上网记录' },
  { key: 'im', label: 'IM 记录' },
  { key: 'online', label: '上下线' },
  { key: 'app', label: '应用记录' },
  { key: 'total', label: '总记录' },
];

const exemptDrawer = ref(false);
const exempts = ref<Ex[]>([]);
const newEx = ref<any>({ match_type: 'mac', value: '', comment: '' });

function kindName(k: string) {
  return k === 'web' ? '上网' : k === 'im' ? 'IM' : k === 'online' ? '上下线' : k === 'app' ? '应用' : k || '全部';
}
function fmt(ts: number) {
  if (!ts) return '-';
  const d = new Date(ts * 1000);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

async function reload() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/behavior/records', {
      params: { kind: kind.value, keyword: keyword.value, limit: pageSize.value, offset: (page.value - 1) * pageSize.value },
    });
    records.value = res.data?.records || [];
    total.value = res.data?.total || 0;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '查询失败');
  } finally {
    loading.value = false;
  }
}

function onPage(p: number) { page.value = p; reload(); }
function onSize(s: number) { pageSize.value = s; page.value = 1; reload(); }

async function loadStats() {
  try {
    const res = await api.get('/api/v1/behavior/stats');
    stats.value = res.data?.stats || {};
    online.value = res.data?.online || [];
    exempts.value = res.data?.exempts || [];
  } catch { /* 统计失败不影响列表 */ }
}

function openExempt() { exemptDrawer.value = true; }

function addExempt() {
  if (!newEx.value.value.trim()) { ElMessage.warning('值必填'); return; }
  exempts.value.push({
    kind: newEx.value.EmptyKind || '', match_type: newEx.value.match_type,
    value: newEx.value.value.trim(), comment: newEx.value.comment || '',
  });
  newEx.value.value = '';
}

async function saveExempt() {
  try {
    const res = await api.post('/api/v1/behavior/exempt', { exempts: exempts.value });
    exempts.value = res.data?.exempts || exempts.value;
    ElMessage.success('已保存');
    exemptDrawer.value = false;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败');
  }
}

onMounted(async () => {
  await Promise.all([reload(), loadStats()]);
});
</script>

<style scoped>
.stat { text-align: center; }
.stat-val { font-size: 22px; font-weight: 600; color: #4f8cff; }
.stat-lbl { font-size: 12px; color: #8b91a5; margin-top: 2px; }
.hdr { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.tools { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.pager { display: flex; justify-content: flex-end; margin-top: 12px; }
</style>
