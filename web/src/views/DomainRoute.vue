<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>分流策略 · 域名分流</span>
          <div class="tools">
            <el-button size="small" @click="openNew">新增规则</el-button>
            <el-button type="primary" size="small" :loading="saving" @click="save(false)">保存</el-button>
            <el-button type="success" size="small" :loading="applying" @click="save(true)">保存并应用</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="域名命中后按 nft mark + ip rule 走指定路由表；选「拦截」则解析到的地址直接 drop。改完必须先保存再应用。" />

      <el-table :data="rules" size="small" empty-text="还没有域名分流规则">
        <el-table-column prop="domain" label="域名" min-width="220" />
        <el-table-column label="动作" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="row.action === 'block' ? 'danger' : 'primary'" effect="dark">
              {{ row.action === 'block' ? '拦截' : '走路由' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="路由表" width="90">
          <template #default="{ row }">{{ row.action === 'route' ? row.table : '-' }}</template>
        </el-table-column>
        <el-table-column label="fwmark" width="90">
          <template #default="{ row }">{{ row.fwmark || '自动' }}</template>
        </el-table-column>
        <el-table-column prop="priority" label="优先级" width="90" />
        <el-table-column label="启用" width="70">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" size="small" />
          </template>
        </el-table-column>
        <el-table-column prop="comment" label="备注" min-width="140" show-overflow-tooltip />
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row, $index }">
            <el-button link type="primary" size="small" @click="openEdit(row, $index)">编辑</el-button>
            <el-button link type="danger" size="small" @click="rules.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card v-if="hasRender" shadow="never" style="margin-top:16px">
      <template #header>
        <el-radio-group v-model="tab" size="small">
          <el-radio-button value="dnsmasq">dnsmasq ipset</el-radio-button>
          <el-radio-button value="nft">nft 规则</el-radio-button>
          <el-radio-button value="iprule">ip rule</el-radio-button>
        </el-radio-group>
      </template>
      <pre class="code">{{ tab === 'dnsmasq' ? render.dnsmasq : tab === 'nft' ? render.nft : render.iprule }}</pre>
      <div v-if="render.error" class="err">应用失败：{{ render.error }}</div>
    </el-card>

    <el-drawer v-model="drawer" :title="editing === null ? '新增域名分流规则' : '编辑规则'" size="480px">
      <el-form label-width="90px">
        <el-form-item label="域名" required>
          <el-input v-model="form.domain" placeholder="example.com 或 *.example.com" />
        </el-form-item>
        <el-form-item label="动作">
          <el-radio-group v-model="form.action">
            <el-radio-button value="route">走指定路由</el-radio-button>
            <el-radio-button value="block">拦截</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.action === 'route'" label="路由表">
          <el-input-number v-model="form.table" :min="1" :max="252" controls-position="right" />
          <span class="hint">1-252，需是已存在的路由表</span>
        </el-form-item>
        <el-form-item label="fwmark">
          <el-input-number v-model="form.fwmark" :min="0" :max="65535" controls-position="right" />
          <span class="hint">0 = 自动分配（100 起，跳过已占用）</span>
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="form.priority" :min="0" :max="10000" controls-position="right" />
          <span class="hint">越小越优先，0 视作 100</span>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.comment" placeholder="可选" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="drawer = false">取消</el-button>
        <el-button type="primary" @click="confirmDrawer">确定</el-button>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type Rule = {
  id: string; domain: string; action: string; table: number;
  fwmark: number; priority: number; enabled: boolean; comment?: string;
};

const rules = ref<Rule[]>([]);
const drawer = ref(false);
const editing = ref<number | null>(null);
const saving = ref(false);
const applying = ref(false);
const tab = ref('dnsmasq');
const render = ref<{ dnsmasq: string; nft: string; iprule: string; error?: string }>({
  dnsmasq: '', nft: '', iprule: '',
});
const hasRender = computed(() => !!(render.value.dnsmasq || render.value.nft || render.value.iprule));

const form = ref<Rule>({ id: '', domain: '', action: 'route', table: 100, fwmark: 0, priority: 100, enabled: true, comment: '' });

function openNew() {
  editing.value = null;
  form.value = { id: '', domain: '', action: 'route', table: 100, fwmark: 0, priority: 100, enabled: true, comment: '' };
  drawer.value = true;
}

function openEdit(row: Rule, idx: number) {
  editing.value = idx;
  form.value = { ...row };
  drawer.value = true;
}

function confirmDrawer() {
  if (!form.value.domain.trim()) {
    ElMessage.warning('域名必填');
    return;
  }
  const r = { ...form.value, id: form.value.id || `dom_${Date.now()}` };
  if (editing.value === null) rules.value.push(r);
  else rules.value[editing.value] = r;
  drawer.value = false;
}

async function load() {
  try {
    const res = await api.get('/api/v1/policy/domain-rules');
    rules.value = res.data?.rules || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  }
}

async function save(apply: boolean) {
  if (apply) applying.value = true;
  else saving.value = true;
  try {
    const res = await api.post('/api/v1/policy/domain-rules', { rules: rules.value, apply });
    rules.value = res.data?.rules || rules.value;
    render.value = {
      dnsmasq: res.data?.dnsmasq || '',
      nft: res.data?.nft || '',
      iprule: (res.data?.ip_rules || []).join('\n'),
      error: res.data?.error,
    };
    if (apply) {
      if (res.data?.applied) ElMessage.success('已下发 nft 规则并重载 dnsmasq');
      else ElMessage.error('未完全生效：' + (res.data?.error || '见下方输出'));
    } else {
      ElMessage.success('已保存，未下发');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败');
  } finally {
    saving.value = false;
    applying.value = false;
  }
}

onMounted(load);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.tools { display: flex; gap: 8px; }
.hint { color: #8b91a5; font-size: 12px; margin-left: 8px; }
.code { background: #0f1117; color: #d5dae6; padding: 12px; border-radius: 6px; font-size: 12px; overflow: auto; max-height: 380px; }
.err { color: #f56c6c; font-size: 12px; margin-top: 8px; }
</style>
