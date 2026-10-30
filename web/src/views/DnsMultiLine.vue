<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>多线路 DNS</span>
          <div class="tools">
            <el-switch v-model="enabled" active-text="启用" inline-prompt />
            <el-button size="small" @click="addLine">新增线路</el-button>
            <el-button type="primary" size="small" :loading="saving" @click="save(false)">保存</el-button>
            <el-button type="success" size="small" :loading="applying" @click="save(true)">保存并应用</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="按 WAN 线路分别指定 DNS 上游。绑定源接口（@interface）才会真正从该线路发出查询，只写上游 IP 时内核仍可能走主路由。" />

      <el-table :data="lines" size="small" empty-text="还没有线路">
        <el-table-column label="线路名" width="150">
          <template #default="{ row }">
            <el-input v-model="row.line" size="small" placeholder="wan1 / telecom" />
          </template>
        </el-table-column>
        <el-table-column label="源接口" width="170">
          <template #default="{ row }">
            <el-input v-model="row.interface" size="small" placeholder="pppoe-wan1（可空）" />
          </template>
        </el-table-column>
        <el-table-column label="DNS 上游" min-width="220">
          <template #default="{ row }">
            <el-input v-model="row.serversText" size="small" placeholder="202.96.128.86, 8.8.8.8:53" />
          </template>
        </el-table-column>
        <el-table-column label="限定域名" min-width="220">
          <template #default="{ row }">
            <el-input v-model="row.domainsText" size="small" placeholder="留空=全局默认；如 corp.example.com" />
          </template>
        </el-table-column>
        <el-table-column label="启用" width="70">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="" width="70">
          <template #default="{ $index }">
            <el-button link type="danger" size="small" @click="lines.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-divider content-position="left">默认线路</el-divider>
      <el-select v-model="defaultLine" clearable placeholder="不指定（各线路按域名/全局规则生效）" size="small" style="width:320px">
        <el-option v-for="l in lines.filter((x) => x.line)" :key="l.line" :label="l.line" :value="l.line" />
      </el-select>
    </el-card>

    <el-card v-if="rendered" shadow="never" style="margin-top:16px">
      <template #header><span>生成的 dnsmasq 片段（/etc/dnsmasq.d/rr-multiline.conf）</span></template>
      <pre class="code">{{ rendered }}</pre>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type LineRow = { line: string; interface: string; serversText: string; domainsText: string; enabled: boolean };

const lines = ref<LineRow[]>([]);
const enabled = ref(false);
const defaultLine = ref('');
const rendered = ref('');
const saving = ref(false);
const applying = ref(false);

const splitList = (s: string) => s.split(/[,，\s]+/).map((x) => x.trim()).filter(Boolean);

function toPayload(): any {
  return {
    enabled: enabled.value,
    default_line: defaultLine.value,
    lines: lines.value.map((l) => ({
      line: l.line.trim(),
      interface: l.interface.trim(),
      servers: splitList(l.serversText),
      domains: splitList(l.domainsText),
      enabled: l.enabled,
    })),
  };
}

function addLine() {
  lines.value.push({ line: `wan${lines.value.length + 1}`, interface: '', serversText: '', domainsText: '', enabled: true });
}

async function load() {
  try {
    const res = await api.get('/api/v1/dns/multiline');
    const c = res.data?.config || {};
    enabled.value = !!c.enabled;
    defaultLine.value = c.default_line || '';
    lines.value = (c.lines || []).map((l: any) => ({
      line: l.line || '',
      interface: l.interface || '',
      serversText: (l.servers || []).join(', '),
      domainsText: (l.domains || []).join(', '),
      enabled: !!l.enabled,
    }));
    rendered.value = res.data?.rendered || '';
    if (res.data?.render_error) ElMessage.warning('渲染存在问题：' + res.data.render_error);
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  }
}

async function save(apply: boolean) {
  const payload = { ...toPayload(), apply };
  if (apply) applying.value = true;
  else saving.value = true;
  try {
    const res = await api.post('/api/v1/dns/multiline', payload);
    rendered.value = res.data?.rendered || '';
    if (apply) {
      if (res.data?.applied) ElMessage.success('已写入并重启 dnsmasq');
      else ElMessage.error('未生效：' + (res.data?.status || '未知原因'));
    } else {
      ElMessage.success('已保存，未下发。需要生效请点「保存并应用」。');
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
.tools { display: flex; gap: 8px; align-items: center; }
.code { background: #0f1117; color: #d5dae6; padding: 12px; border-radius: 6px; font-size: 12px; overflow: auto; max-height: 360px; }
</style>
