<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>高级设置 · 内核网络参数</span>
          <div class="tools">
            <el-button size="small" :loading="saving" @click="save(false)">保存</el-button>
            <el-button type="primary" size="small" :loading="applying" @click="save(true)">保存并应用</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="这些参数决定路由器能同时撑多少连接、半开连接占多少内存。改错的表现是连不上或内存被连接跟踪表吃光，每一项都有硬区间，超范围会被拒绝。" />

      <el-alert v-if="missing.length" type="warning" :closable="false" show-icon style="margin-bottom:12px"
        :title="'当前内核不支持这些参数（模块未加载）：' + missing.join(', ')" />

      <el-form label-width="180px" style="max-width:860px">
        <el-form-item label="BBR 拥塞控制">
          <el-switch v-model="cfg.bbr" :active-value="1" :inactive-value="0" />
          <span class="hint">开启时同时把 qdisc 设为 fq（只设 bbr 不设 fq，加速效果会打折）</span>
        </el-form-item>

        <el-divider content-position="left">连接跟踪超时（秒）</el-divider>

        <el-form-item v-for="s in specs" :key="s.field" :label="s.label">
          <el-input-number v-model="cfg[s.field]" :min="s.min" :max="s.max" controls-position="right" style="width:150px" />
          <el-tag size="small" type="info" effect="plain" style="margin-left:8px">区间 {{ s.min }}–{{ s.max }}</el-tag>
          <span class="cur">
            当前生效：
            <b :class="{ changed: current && current[s.field] !== cfg[s.field] }">{{ current ? current[s.field] : '—' }}</b>
            <span v-if="current && current[s.field] !== cfg[s.field]" class="tag-unsaved">未应用</span>
          </span>
          <div class="desc">{{ s.desc }}</div>
        </el-form-item>
      </el-form>

      <el-card v-if="conf" shadow="never" style="margin-top:16px">
        <template #header><span>生成的配置（{{ path }}）</span></template>
        <pre class="code">{{ conf }}</pre>
      </el-card>
    </el-card>

    <!-- 系统标识 / Identity -->
    <el-card shadow="never" style="margin-top:16px">
      <template #header><span>系统标识（Identity）</span></template>
      <el-alert v-if="idErr" type="error" :closable="false" show-icon :title="idErr" style="margin-bottom:12px" />
      <el-form label-width="140px" style="max-width:620px">
        <el-form-item label="主机名 hostname">
          <el-input v-model="identity.hostname" placeholder="router-re" />
        </el-form-item>
        <el-form-item label="域名 domain">
          <el-input v-model="identity.domain" placeholder="lan" />
        </el-form-item>
        <el-form-item label="位置 location">
          <el-input v-model="identity.location" placeholder="机房/位置（可空）" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="idSaving" @click="saveIdentity">保存并应用</el-button>
          <el-button @click="loadIdentity">刷新</el-button>
        </el-form-item>
      </el-form>
      <div v-if="idResult" style="margin-top:8px; font-size:13px; color:#606266">{{ idResult }}</div>
    </el-card>

    <!-- mDNS 中继 -->
    <el-card shadow="never" style="margin-top:16px">
      <template #header><span>mDNS 中继</span></template>
      <el-alert v-if="mdnsErr" type="error" :closable="false" show-icon :title="mdnsErr" style="margin-bottom:12px" />
      <el-form label-width="140px" style="max-width:620px">
        <el-form-item label="启用">
          <el-switch v-model="mdns.config.enabled" />
        </el-form-item>
        <el-form-item label="入接口 in_ifaces">
          <el-input v-model="mdnsIn" placeholder="br-lan,eth0" />
          <span class="hint">逗号分隔</span>
        </el-form-item>
        <el-form-item label="出接口 out_ifaces">
          <el-input v-model="mdnsOut" placeholder="br-wan" />
          <span class="hint">逗号分隔；入/出不能完全相同（组播环路）</span>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="mdnsSaving" @click="saveMDNS">保存</el-button>
          <el-button @click="loadMDNS">刷新</el-button>
        </el-form-item>
      </el-form>
      <el-card v-if="mdns.avahi" shadow="never" style="margin-top:12px">
        <template #header><span>生成的 avahi-daemon.conf 片段</span></template>
        <pre class="code">{{ mdns.avahi }}</pre>
      </el-card>
    </el-card>

    <!-- Web 管理端口 -->
    <el-card shadow="never" style="margin-top:16px">
      <template #header><span>Web 管理端口（WebAdmin）</span></template>
      <el-alert v-if="waErr" type="error" :closable="false" show-icon :title="waErr" style="margin-bottom:12px" />
      <el-form label-width="160px" style="max-width:620px">
        <el-form-item label="HTTP 端口 http_port">
          <el-input-number v-model="wa.config.http_port" :min="0" :max="65535" />
          <span class="hint">0=关闭</span>
        </el-form-item>
        <el-form-item label="HTTPS 端口 https_port">
          <el-input-number v-model="wa.config.https_port" :min="0" :max="65535" />
          <span class="hint">0=关闭；两者不能同时为 0</span>
        </el-form-item>
        <el-form-item label="绑定地址 bind_addr">
          <el-input v-model="wa.config.bind_addr" placeholder="0.0.0.0" />
        </el-form-item>
        <el-form-item label="HTTP 301 到 HTTPS">
          <el-switch v-model="wa.config.redirect_http" />
        </el-form-item>
        <el-form-item label="空闲超时 idle_timeout_sec">
          <el-input-number v-model="wa.config.idle_timeout_sec" :min="0" :max="86400" />
          <span class="hint">0=默认；有效 60-86400</span>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="waSaving" @click="saveWebAdmin">保存</el-button>
          <el-button @click="loadWebAdmin">刷新</el-button>
        </el-form-item>
      </el-form>
      <el-alert type="info" :closable="false" show-icon style="margin-top:8px"
        title="配置已保存；实际生效需重新加载 nginx（本接口不会重启服务，避免把自己锁在外面）。" />
      <el-card v-if="wa.nginx" shadow="never" style="margin-top:12px">
        <template #header><span>生成的 nginx 配置</span></template>
        <pre class="code">{{ wa.nginx }}</pre>
      </el-card>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type Spec = { Field: string; Key: string; Min: number; Max: number; Def: number; Unit: string; Desc: string };

const cfg = ref<Record<string, number>>({ bbr: 1 });
const current = ref<Record<string, number> | null>(null);
const specs = ref<{ field: string; min: number; max: number; label: string; desc: string }[]>([]);
const missing = ref<string[]>([]);
const path = ref('');
const conf = ref('');
const saving = ref(false);
const applying = ref(false);

// 后端字段名 → 中文标签（与 iKuai 的参数名对齐）
const labels: Record<string, string> = {
  syn_send_timeout: 'SYN 发送超时',
  syn_recv_timeout: 'SYN 接收超时',
  established_timeout: '已建立连接超时',
  fin_wait_timeout: 'FIN_WAIT 超时',
  close_wait_timeout: 'CLOSE_WAIT 超时',
  last_ack_timeout: 'LAST_ACK 超时',
  time_wait_timeout: 'TIME_WAIT 超时',
  close_timeout: 'CLOSE 超时',
  udp_timeout: 'UDP 超时',
  udp_stream_timeout: 'UDP 流超时',
  icmp_timeout: 'ICMP 超时',
};

async function load() {
  try {
    const res = await api.get('/api/v1/system/kernel-params');
    cfg.value = { ...(res.data?.config || {}) };
    current.value = res.data?.current || null;
    missing.value = res.data?.missing || [];
    path.value = res.data?.path || '';
    const raw = res.data?.specs || [];
    // Go 的字段名是大写开头的 JSON（Field/Min/Max），统一成小写方便 v-model
    specs.value = raw.map((s: any) => ({
      field: s.Field ?? s.field,
      min: s.Min ?? s.min,
      max: s.Max ?? s.max,
      label: labels[s.Field ?? s.field] || (s.Field ?? s.field),
      desc: s.Desc ?? s.desc ?? '',
    }));
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  }
}

async function save(apply: boolean) {
  if (apply) applying.value = true;
  else saving.value = true;
  try {
    const res = await api.post('/api/v1/system/kernel-params', { ...cfg.value, apply });
    conf.value = res.data?.conf || '';
    if (apply) {
      current.value = res.data?.current || current.value;
      missing.value = res.data?.missing || [];
      ElMessage.success('已写入并生效');
    } else {
      ElMessage.success('已保存（未应用）');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败');
  } finally {
    saving.value = false;
    applying.value = false;
  }
}

const identity = ref<any>({ hostname: '', domain: '', location: '' });
const idErr = ref('');
const idSaving = ref(false);
const idResult = ref('');

async function loadIdentity() {
  idErr.value = '';
  try {
    const res = await api.get('/api/v1/identity');
    identity.value = { ...identity.value, ...(res.data?.identity || {}) };
  } catch (e: any) {
    idErr.value = e?.response?.data?.message || e?.response?.data?.detail || e?.message || '加载 identity 失败';
    ElMessage.error(idErr.value);
  }
}

async function saveIdentity() {
  idSaving.value = true;
  idErr.value = '';
  try {
    const res = await api.post('/api/v1/identity', identity.value);
    if (res.data?.identity) identity.value = { ...identity.value, ...res.data.identity };
    idResult.value = res.data?.detail || (res.data?.applied ? '已应用' : '未应用');
    if (res.data?.applied) ElMessage.success('系统标识已应用');
    else ElMessage.warning(idResult.value);
  } catch (e: any) {
    idErr.value = e?.response?.data?.message || e?.response?.data?.detail || e?.message || '保存 identity 失败';
    ElMessage.error(idErr.value);
  } finally {
    idSaving.value = false;
  }
}

const mdns = ref<any>({ config: { in_ifaces: [], out_ifaces: [], enabled: false }, avahi: '' });
const mdnsIn = ref('');
const mdnsOut = ref('');
const mdnsErr = ref('');
const mdnsSaving = ref(false);

function splitIfaces(s: string): string[] {
  return s.split(',').map(x => x.trim()).filter(Boolean);
}

async function loadMDNS() {
  mdnsErr.value = '';
  try {
    const res = await api.get('/api/v1/mdns');
    if (res.data?.config) mdns.value.config = { ...mdns.value.config, ...res.data.config };
    mdns.value.avahi = res.data?.avahi || '';
    mdnsIn.value = (mdns.value.config.in_ifaces || []).join(',');
    mdnsOut.value = (mdns.value.config.out_ifaces || []).join(',');
  } catch (e: any) {
    mdnsErr.value = e?.response?.data?.message || e?.response?.data?.detail || e?.message || '加载 mDNS 失败';
    ElMessage.error(mdnsErr.value);
  }
}

async function saveMDNS() {
  mdnsSaving.value = true;
  mdnsErr.value = '';
  try {
    const cfg = { ...mdns.value.config, in_ifaces: splitIfaces(mdnsIn.value), out_ifaces: splitIfaces(mdnsOut.value) };
    const res = await api.post('/api/v1/mdns', cfg);
    if (res.data?.config) mdns.value.config = { ...mdns.value.config, ...res.data.config };
    mdns.value.avahi = res.data?.avahi || mdns.value.avahi;
    ElMessage.success('mDNS 配置已保存');
  } catch (e: any) {
    mdnsErr.value = e?.response?.data?.message || e?.response?.data?.detail || e?.message || '保存 mDNS 失败';
    ElMessage.error(mdnsErr.value);
  } finally {
    mdnsSaving.value = false;
  }
}

const wa = ref<any>({ config: { http_port: 0, https_port: 443, bind_addr: '0.0.0.0', redirect_http: true, idle_timeout_sec: 0 }, nginx: '' });
const waErr = ref('');
const waSaving = ref(false);

async function loadWebAdmin() {
  waErr.value = '';
  try {
    const res = await api.get('/api/v1/webadmin');
    if (res.data?.config) wa.value.config = { ...wa.value.config, ...res.data.config };
    wa.value.nginx = res.data?.nginx || '';
  } catch (e: any) {
    waErr.value = e?.response?.data?.message || e?.response?.data?.detail || e?.message || '加载 WebAdmin 失败';
    ElMessage.error(waErr.value);
  }
}

async function saveWebAdmin() {
  waSaving.value = true;
  waErr.value = '';
  try {
    const res = await api.post('/api/v1/webadmin', wa.value.config);
    if (res.data?.config) wa.value.config = { ...wa.value.config, ...res.data.config };
    wa.value.nginx = res.data?.nginx || wa.value.nginx;
    ElMessage.success(res.data?.detail || 'WebAdmin 配置已保存');
  } catch (e: any) {
    waErr.value = e?.response?.data?.message || e?.response?.data?.detail || e?.message || '保存 WebAdmin 失败';
    ElMessage.error(waErr.value);
  } finally {
    waSaving.value = false;
  }
}

onMounted(() => { load(); loadIdentity(); loadMDNS(); loadWebAdmin(); });
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.hint { color: #8b91a5; font-size: 12px; margin-left: 10px; }
.cur { color: #8b91a5; font-size: 12px; margin-left: 14px; }
.cur b { color: #d5dae6; }
.cur b.changed { color: #f0a020; }
.tag-unsaved { color: #f0a020; margin-left: 4px; }
.desc { color: #6b7280; font-size: 12px; margin-top: 2px; }
.code { background: #0f1117; color: #d5dae6; padding: 12px; border-radius: 6px; font-size: 12px; overflow: auto; max-height: 360px; }
</style>
