<template>
  <div>
    <el-card shadow="never" style="margin-bottom:16px">
      <template #header>
        <div class="hdr">
          <span>应用特征库</span>
          <div class="tools">
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
            <el-button size="small" type="primary" @click="dlg = true">导入 / 升级</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon :title="note" style="margin-bottom:14px" />

      <div class="metrics">
        <div class="m"><span class="k">版本</span><span class="v mono">{{ st.version || '-' }}</span></div>
        <div class="m"><span class="k">应用</span><span class="v">{{ st.apps || 0 }}</span></div>
        <div class="m"><span class="k">域名特征</span><span class="v">{{ (st.domains || 0).toLocaleString() }}</span></div>
        <div class="m"><span class="k">加载时间</span><span class="v">{{ fmtTime(st.updated_at) }}</span></div>
        <div class="m"><span class="k">来源</span><span class="v mono small">{{ st.source || '-' }}</span></div>
      </div>

      <el-alert v-if="st.empty_reason" type="warning" :closable="false" show-icon :title="st.empty_reason"
        style="margin-top:10px" />
    </el-card>

    <el-card shadow="never" style="margin-bottom:16px">
      <template #header><span>分类分布</span></template>
      <div class="cats">
        <div v-for="c in categories" :key="c.category" class="cat"
          :class="{ on: activeCat === c.category }" @click="pickCat(c.category)">
          <span class="cat-name">{{ c.category }}</span>
          <span class="cat-n">{{ c.count }}</span>
        </div>
      </div>
      <el-table v-if="activeCat" :data="apps" size="small" style="margin-top:12px"
        :empty-text="'「' + activeCat + '」下没有应用'">
        <el-table-column prop="name" label="应用" min-width="180" />
        <el-table-column prop="id" label="标识" min-width="160">
          <template #default="{ row }"><span class="mono small">{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column prop="domains" label="域名数" width="100" />
        <el-table-column label="操作" width="110">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="testApp(row)">试查</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never">
      <template #header><span>识别测试</span></template>
      <div class="probe">
        <el-input v-model="probeHost" placeholder="输入域名，例如 www.bilibili.com"
          style="max-width:360px" @keyup.enter="probe" />
        <el-button type="primary" :loading="probing" @click="probe">查询</el-button>
        <span v-if="probeResult" class="probe-out">
          <template v-if="probeResult.matched">
            <b>{{ probeResult.app?.name }}</b>
            <el-tag size="small" effect="plain" style="margin-left:6px">{{ probeResult.app?.category }}</el-tag>
            <span class="mono small" style="margin-left:8px">id={{ probeResult.app?.id }}</span>
          </template>
          <template v-else>
            未识别<span class="small muted" style="margin-left:6px">（规范化后：{{ probeResult.normalized || '-' }}）</span>
          </template>
        </span>
      </div>
      <div class="hint">
        匹配语义：qq.com 同时匹配子域 www.qq.com；full:xxx 只匹配自身。
        IP 字面量与保留域（lan/localhost/*.test）不参与域名匹配。
      </div>
    </el-card>


    <el-card shadow="never" style="margin-top:16px">
      <template #header>
        <div class="hdr">
          <span>抓包分析</span>
          <div class="tools">
            <input ref="pcapInput" type="file" accept=".pcap,application/vnd.tcpdump.pcap" style="display:none"
              @change="onPcap" />
            <el-button size="small" :loading="analyzing" @click="analyzePcap">选择 pcap 并分析</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="上传一段抓包，按流回放给出识别结果。TLS 看 ClientHello 的 SNI，QUIC(HTTP/3) 解密 Initial 包取 SNI 并跨包重组；单包往往只能得到 Unknown，同一连接的后续包才会定性。" />

      <div v-if="pcapSum" class="pcap-sum">
        <span>包 <b>{{ pcapSum.packets }}</b></span>
        <span>流 <b>{{ pcapSum.flow_count }}</b></span>
        <span>nDPI 识别 <b>{{ pcapSum.identified }}</b></span>
        <span>特征库命中 <b>{{ pcapSum.app_matched }}</b></span>
        <span v-if="pcapSum.quic_streams">QUIC 重组 <b>{{ pcapSum.quic_streams.solved }}/{{ pcapSum.quic_streams.active }}</b></span>
      </div>

      <el-table v-if="pcapFlows.length" :data="pcapFlows" size="small" max-height="420" style="margin-top:10px">
        <el-table-column label="目标" min-width="200">
          <template #default="{ row }"><span class="mono small">{{ row.dst_ip }}:{{ row.dst_port }}</span></template>
        </el-table-column>
        <el-table-column label="nDPI 协议" width="150">
          <template #default="{ row }">
            <el-tag size="small" :type="row.protocol === 'Unknown' ? 'info' : 'success'" effect="plain">
              {{ row.protocol }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="应用" width="140">
          <template #default="{ row }">{{ row.app || '-' }}</template>
        </el-table-column>
        <el-table-column label="分类" width="100">
          <template #default="{ row }">{{ row.app_category || '-' }}</template>
        </el-table-column>
        <el-table-column label="域名" min-width="220">
          <template #default="{ row }"><span class="mono small">{{ row.host || '-' }}</span></template>
        </el-table-column>
        <el-table-column label="TLS 指纹 (JA4)" width="232">
          <template #default="{ row }">
            <span class="mono ja4" :title="row.ja4 || ''">{{ row.ja4 || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="客户端" width="150">
          <template #default="{ row }">
            <span v-if="row.client">{{ row.client }}</span>
            <span v-else class="muted small">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="packets" label="包数" width="80" />
        <el-table-column prop="bytes" label="字节" width="90" />
      </el-table>
      <div v-else-if="pcapSum" class="hint">这段抓包里没有可分析的流。</div>
    </el-card>

    <el-dialog v-model="dlg" title="导入 / 升级特征库" width="640px">
      <el-alert type="warning" :closable="false" show-icon style="margin-bottom:12px"
        title="只导入你有权使用的数据。域名词表可来自公开分类项目或自建抓包整理；不要搬运商业路由器的私有特征库。" />
      <el-radio-group v-model="mode" style="margin-bottom:10px">
        <el-radio-button label="paste">粘贴 JSON</el-radio-button>
        <el-radio-button label="zip">上传 ZIP</el-radio-button>
      </el-radio-group>

      <template v-if="mode === 'paste'">
        <el-input v-model="pasteText" type="textarea" :rows="12" placeholder='{"version":"20261030","files":{"bilibili":"bilibili.com\nb23.tv","qq":"qq.com"}}' />
        <div class="hint">files 的每个键是一组，通常就是应用名；值是该组的域名列表（一行一个，支持 include:）。</div>
      </template>
      <template v-else>
        <input type="file" accept=".zip" @change="onZip" />
        <div class="hint">ZIP 内每个文本文件是一组，文件名即应用名。兼容 v2ray domain-list 目录结构。</div>
      </template>

      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="importing" @click="doImport">导入</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type Stats = {
  version: string; apps: number; domains: number; updated_at: string;
  source?: string; empty_reason?: string;
};
type Cat = { category: string; count: number };
type App = { id: string; name: string; category: string; domains: number };

const st = reactive<Stats>({ version: '', apps: 0, domains: 0, updated_at: '' });
const note = ref('');
const categories = ref<Cat[]>([]);
const apps = ref<App[]>([]);
const activeCat = ref('');
const loading = ref(false);

const probeHost = ref('www.bilibili.com');
const probeResult = ref<any>(null);
const probing = ref(false);

const pcapInput = ref<HTMLInputElement | null>(null);
const analyzing = ref(false);
const pcapSum = ref<any>(null);
const pcapFlows = ref<any[]>([]);

const dlg = ref(false);
const mode = ref('paste');
const pasteText = ref('');
const zipFile = ref<File | null>(null);
const importing = ref(false);

function fmtTime(t?: string) {
  if (!t) return '-';
  const d = new Date(t);
  if (isNaN(d.getTime())) return t;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/appid/stats');
    Object.assign(st, res.data?.stats || {});
    note.value = res.data?.note || '';
    if (res.data?.loaded === false) note.value = '特征库尚未加载。导入数据后立即生效。';
    const c = await api.get('/api/v1/appid/categories');
    categories.value = c.data?.categories || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  } finally {
    loading.value = false;
  }
}

async function pickCat(cat: string) {
  activeCat.value = cat;
  try {
    const res = await api.get('/api/v1/appid/categories', { params: { category: cat } });
    apps.value = res.data?.apps || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  }
}

async function probe() {
  if (!probeHost.value.trim()) return;
  probing.value = true;
  try {
    const res = await api.get('/api/v1/appid/lookup', { params: { host: probeHost.value.trim() } });
    probeResult.value = res.data;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '查询失败');
  } finally {
    probing.value = false;
  }
}

async function testApp(row: App) {
  probeHost.value = row.id;
  // 用应用标识试查通常查不到（标识不是域名），提示用户换真实域名
  ElMessage.info('请在下方输入该应用的真实域名再查，例如 www.' + row.id + '.com');
}

function analyzePcap() {
  pcapInput.value?.click();
}

async function onPcap(e: Event) {
  const input = e.target as HTMLInputElement;
  const f = input.files?.[0];
  input.value = ''; // 允许重复选同一个文件
  if (!f) return;
  if (f.size > 32 * 1024 * 1024) {
    ElMessage.warning('文件超过 32MB，请先裁剪');
    return;
  }
  analyzing.value = true;
  try {
    const buf = await f.arrayBuffer();
    const res = await api.post('/api/v1/dpi/analyze-pcap', buf, {
      headers: { 'Content-Type': 'application/vnd.tcpdump.pcap' },
    });
    const d = res.data || {};
    pcapSum.value = d;
    // 已识别的排在前面，未识别的沉底，便于一眼看出识别率
    pcapFlows.value = (d.flows || []).slice().sort((a: any, b: any) => {
      const rank = (x: any) => (x.app ? 0 : x.host ? 1 : x.protocol !== 'Unknown' ? 2 : 3);
      return rank(a) - rank(b) || b.packets - a.packets;
    });
    const unk = (d.flows || []).filter((x: any) => x.protocol === 'Unknown').length;
    if (unk) ElMessage.warning(`${d.flow_count} 条流里有 ${unk} 条未能定性（多为加密或私有协议）`);
  } catch (err: any) {
    ElMessage.error(err?.response?.data?.detail || err?.message || '分析失败');
  } finally {
    analyzing.value = false;
  }
}

function onZip(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0];
  zipFile.value = f || null;
}

async function doImport() {
  importing.value = true;
  try {
    if (mode.value === 'paste') {
      let payload: any;
      try {
        payload = JSON.parse(pasteText.value);
      } catch {
        ElMessage.error('JSON 格式不正确');
        return;
      }
      const res = await api.post('/api/v1/appid/import', payload);
      reportResult(res.data);
    } else {
      if (!zipFile.value) { ElMessage.warning('请选择 ZIP 文件'); return; }
      const buf = await zipFile.value.arrayBuffer();
      const res = await api.post('/api/v1/appid/import', buf, {
        headers: { 'Content-Type': 'application/zip' },
      });
      reportResult(res.data);
    }
    dlg.value = false;
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e?.message || '导入失败');
  } finally {
    importing.value = false;
  }
}

function reportResult(d: any) {
  const s = d?.stats || {};
  const r = d?.rejected || {};
  const skipped = (r.regexp || 0) + (r.keyword || 0) + (r.invalid || 0) + (r.missing_include || 0);
  ElMessage.success(`已导入 ${s.apps || 0} 个应用 / ${s.domains || 0} 条域名` +
    (skipped ? `，跳过 ${skipped} 条` : ''));
}

onMounted(() => { load(); probe(); });
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.metrics { display: flex; gap: 28px; flex-wrap: wrap; }
.m { display: flex; flex-direction: column; gap: 2px; }
.m .k { font-size: 12px; color: #8b91a5; }
.m .v { font-size: 18px; font-weight: 600; color: #e6e9ef; font-variant-numeric: tabular-nums; }
.m .v.small { font-size: 12px; font-weight: 400; }
.cats { display: flex; gap: 8px; flex-wrap: wrap; }
.cat { padding: 6px 12px; border: 1px solid #2a2f3a; border-radius: 6px; cursor: pointer;
  display: flex; gap: 8px; align-items: baseline; font-size: 13px; color: #cfd3da; }
.cat:hover { border-color: #3d4655; }
.cat.on { border-color: #58a6ff; background: #16202e; }
.cat-n { color: #8b91a5; font-size: 12px; font-variant-numeric: tabular-nums; }
/* 指纹是定长字符串，折行会让人以为值被截断，所以强制单行 */
.ja4 { font-size: 11px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: block; }
.pcap-sum { display: flex; gap: 22px; flex-wrap: wrap; font-size: 13px; color: #cfd3da; }
.pcap-sum b { color: #e6e9ef; font-variant-numeric: tabular-nums; }
.probe { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.probe-out { font-size: 13px; color: #cfd3da; }
.hint { color: #8b91a5; font-size: 12px; margin-top: 8px; line-height: 1.6; }
.small { font-size: 12px; }
.muted { color: #6b7280; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; }
</style>
