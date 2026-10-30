<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>对象管理 · 分组</span>
          <div class="tools">
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
            <el-button size="small" type="primary" @click="openNew">新建分组</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="分组是独立对象，可以在 ACL、分流、限速里按名字引用同一个分组；改一处，所有引用它的规则一起生效。" />

      <el-tabs v-model="tab">
        <el-tab-pane v-for="k in kinds" :key="k.kind" :label="k.label" :name="k.kind" />
      </el-tabs>

      <el-table :data="byKind" size="small" v-loading="loading" empty-text="还没有分组">
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column label="条目" width="80">
          <template #default="{ row }">{{ row.items.length }}</template>
        </el-table-column>
        <el-table-column label="内容" min-width="280">
          <template #default="{ row }">
            <span class="mono">{{ preview(row.items) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="说明" min-width="140" show-overflow-tooltip />
        <el-table-column label="ID" width="200">
          <template #default="{ row }"><span class="mono id">{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="210" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="row.kind !== 'terminal'" link size="small" @click="previewNft(row)">预览 nft</el-button>
            <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg" :title="editing ? '编辑分组' : '新建分组'" width="600px">
      <el-form label-width="90px">
        <el-form-item label="类型">
          <el-radio-group v-model="form.kind" :disabled="editing">
            <el-radio-button v-for="k in kinds" :key="k.kind" :label="k.kind">{{ k.label }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" style="width:260px" :placeholder="namePlaceholder" />
        </el-form-item>
        <el-form-item label="条目">
          <el-input v-model="form.itemsText" type="textarea" :rows="8" style="width:100%"
            :placeholder="itemPlaceholder" />
          <div class="hint">一行一个，空行忽略。<span v-if="form.kind === 'ip'">支持单个 IP 和 CIDR（192.168.1.0/24）。</span></div>
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="form.description" style="width:100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="nftDlg" title="渲染为 nft 集合" width="640px">
      <pre class="code">{{ nftText }}</pre>
      <div class="hint">这段文本可以直接放进 nftables 配置。集合名用的是分组 ID，保证不同分组不会撞名。</div>
      <template #footer>
        <el-button @click="nftDlg = false">关闭</el-button>
        <el-button type="primary" @click="copyNft">复制</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

type Group = { id: string; name: string; kind: string; items: string[]; description: string };

const kinds = [
  { kind: 'ip', label: 'IP 分组', placeholder: '192.168.10.0/24\n10.0.0.5' },
  { kind: 'mac', label: 'MAC 分组', placeholder: 'aa:bb:cc:dd:ee:ff' },
  { kind: 'terminal', label: '终端分组', placeholder: '办公电脑-01\n笔记本-李工' },
];

const groups = ref<Group[]>([]);
const loading = ref(false);
const saving = ref(false);
const tab = ref('ip');

const dlg = ref(false);
const editing = ref<Group | null>(null);
const form = reactive({ kind: 'ip', name: '', itemsText: '', description: '' });

const nftDlg = ref(false);
const nftText = ref('');

const byKind = computed(() => groups.value.filter((g) => g.kind === tab.value));
const namePlaceholder = computed(() => (form.kind === 'ip' ? '例如：办公网段' : form.kind === 'mac' ? '例如：打印机' : '例如：服务器组'));
const itemPlaceholder = computed(() => kinds.find((k) => k.kind === form.kind)?.placeholder || '');

function preview(items: string[]) {
  if (!items.length) return '（空）';
  const head = items.slice(0, 3).join('，');
  return items.length > 3 ? `${head} … 等 ${items.length} 项` : head;
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/objects/groups');
    groups.value = res.data?.groups || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  } finally {
    loading.value = false;
  }
}

function openNew() {
  editing.value = null;
  form.kind = tab.value;
  form.name = '';
  form.itemsText = '';
  form.description = '';
  dlg.value = true;
}

function openEdit(row: Group) {
  editing.value = row;
  form.kind = row.kind;
  form.name = row.name;
  form.itemsText = row.items.join('\n');
  form.description = row.description || '';
  dlg.value = true;
}

async function save() {
  const items = form.itemsText.split('\n').map((s) => s.trim()).filter(Boolean);
  if (!form.name.trim()) { ElMessage.warning('分组名不能为空'); return; }
  if (!items.length) { ElMessage.warning('至少填一个条目'); return; }

  saving.value = true;
  try {
    // 接口是"整列表保存"语义：在现有列表上替换/追加这一条
    const list = groups.value.filter((g) => !(editing.value && g.id === editing.value.id));
    list.push({
      id: editing.value?.id || '',
      name: form.name.trim(),
      kind: form.kind,
      items,
      description: form.description,
    } as Group);
    const res = await api.post('/api/v1/objects/groups', { groups: list });
    groups.value = res.data?.groups || [];
    ElMessage.success('已保存');
    dlg.value = false;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败');
  } finally {
    saving.value = false;
  }
}

async function previewNft(row: Group) {
  try {
    const res = await api.get('/api/v1/objects/groups/nft', { params: { id: row.id } });
    nftText.value = res.data?.nft || '';
    nftDlg.value = true;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '渲染失败');
  }
}

async function copyNft() {
  try {
    await navigator.clipboard.writeText(nftText.value);
    ElMessage.success('已复制');
  } catch {
    ElMessage.warning('浏览器不允许自动复制，请手动选中');
  }
}

async function remove(row: Group) {
  try {
    await ElMessageBox.confirm(
      `删除分组「${row.name}」？引用它的规则会失去这个成员列表。`, '确认删除', { type: 'warning' });
  } catch { return; }
  try {
    const list = groups.value.filter((g) => g.id !== row.id);
    const res = await api.post('/api/v1/objects/groups', { groups: list });
    groups.value = res.data?.groups || [];
    ElMessage.success('已删除');
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '删除失败');
  }
}

onMounted(load);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; }
.id { color: #6b7280; }
.hint { color: #8b91a5; font-size: 12px; margin-top: 4px; }
.code { margin: 0; padding: 10px 12px; background: #12151c; border: 1px solid #2a2f3a; border-radius: 6px;
  color: #d7dae0; font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px;
  white-space: pre-wrap; word-break: break-all; max-height: 320px; overflow: auto; }
</style>
