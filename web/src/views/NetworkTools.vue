<template>
  <div>
    <el-card shadow="never">
      <template #header><span>网络工具</span></template>

      <el-tabs v-model="tab">
        <!-- Tab 1: Ping -->
        <el-tab-pane label="Ping" name="ping">
          <el-form inline size="small">
            <el-form-item label="目标">
              <el-input v-model="ping.target" placeholder="IP / 域名" style="width:220px" @keyup.enter="runPing" />
            </el-form-item>
            <el-form-item label="次数">
              <el-input-number v-model="ping.count" :min="1" :max="20" style="width:120px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="pingLoading" @click="runPing">开始</el-button>
            </el-form-item>
          </el-form>

          <el-alert
            v-if="pingResult"
            :type="pingResult.ok ? 'success' : 'warning'"
            :title="pingResult.summary || '完成'"
            :closable="false"
            show-icon
            style="margin-bottom:8px"
          />
          <div v-if="pingMeta.command" class="cmd-line">命令：{{ pingMeta.command }}</div>
          <div v-if="pingMeta.elapsed_ms != null" class="cmd-line">耗时：{{ pingMeta.elapsed_ms }} ms</div>
          <div v-if="pingMeta.note" class="cmd-line">备注：{{ pingMeta.note }}</div>

          <pre v-if="pingResult && pingResult.lines && pingResult.lines.length" class="tool-pre">{{ pingResult.lines.join('\n') }}</pre>
        </el-tab-pane>

        <!-- Tab 2: 路由追踪 -->
        <el-tab-pane label="路由追踪" name="traceroute">
          <el-form inline size="small">
            <el-form-item label="目标">
              <el-input v-model="traceroute.target" placeholder="IP / 域名" style="width:220px" @keyup.enter="runTraceroute" />
            </el-form-item>
            <el-form-item label="最大跳数">
              <el-input-number v-model="traceroute.max_hops" :min="1" :max="30" style="width:120px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="trLoading" @click="runTraceroute">开始</el-button>
            </el-form-item>
          </el-form>

          <el-alert
            v-if="trResult"
            :type="trResult.ok ? 'success' : 'warning'"
            :title="trResult.summary || '完成'"
            :closable="false"
            show-icon
            style="margin-bottom:8px"
          />
          <div v-if="trMeta.command" class="cmd-line">命令：{{ trMeta.command }}</div>
          <div v-if="trMeta.note" class="cmd-line">备注：{{ trMeta.note }}</div>

          <pre v-if="trResult && trResult.lines && trResult.lines.length" class="tool-pre">{{ trResult.lines.join('\n') }}</pre>
        </el-tab-pane>

        <!-- Tab 3: 端口探测 -->
        <el-tab-pane label="端口探测" name="portprobe">
          <el-form inline size="small">
            <el-form-item label="目标">
              <el-input v-model="port.target" placeholder="IP / 域名" style="width:220px" @keyup.enter="runPort" />
            </el-form-item>
            <el-form-item label="端口">
              <el-input-number v-model="port.port" :min="1" :max="65535" style="width:130px" />
            </el-form-item>
            <el-form-item label="超时(s)">
              <el-input-number v-model="port.timeout" :min="1" :max="10" style="width:100px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="portLoading" @click="runPort">开始</el-button>
            </el-form-item>
          </el-form>

          <div v-if="portResult" style="margin-bottom:8px">
            <el-tag v-if="portResult.state === 'open'" type="success">开放</el-tag>
            <el-tag v-else-if="portResult.state === 'closed_or_filtered'" type="warning">关闭或被过滤</el-tag>
            <el-tag v-else-if="portResult.state === 'tool_missing'" type="danger">工具未安装</el-tag>
            <el-tag v-else type="info">{{ portResult.state }}</el-tag>
          </div>
          <div v-if="portResult && portResult.note" class="cmd-line">备注：{{ portResult.note }}</div>
          <div v-if="portMeta.command" class="cmd-line">命令：{{ portMeta.command }}</div>
          <div v-if="portMeta.elapsed_ms != null" class="cmd-line">耗时：{{ portMeta.elapsed_ms }} ms</div>

          <pre v-if="portResult && portResult.output && portResult.output.length" class="tool-pre">{{ portResult.output.join('\n') }}</pre>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const tab = ref('ping');

// 后端错误格式普遍 {message, detail}，detail 往往才是真正原因（如"含非法字符"），一起显示。
function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

// 提取接口返回里除了 result 之外的元信息（command / elapsed_ms / note）
// 各端点的字段位置不同，这里集中处理：
//   ping/traceroute -> { result:{command,...}, elapsed_ms, note }
//   portprobe       -> { state, command, output, elapsed_ms, note }（没有 result 包裹）
// 之前只读 data.command，于是 ping/traceroute 的"实际执行命令"永远是空白，
// 用户看不到到底跑了什么。
function meta(data: any): { command?: string; elapsed_ms?: number; note?: string } {
  if (!data || typeof data !== 'object') return {};
  const m: any = {};
  const inner = data.result && typeof data.result === 'object' ? data.result : {};
  const cmd = inner.command ?? data.command;
  if (cmd != null && cmd !== '') m.command = cmd;
  if (data.elapsed_ms != null) m.elapsed_ms = data.elapsed_ms;
  if (data.note) m.note = data.note;
  return m;
}

// ---- Ping ----
const ping = ref({ target: '127.0.0.1', count: 4 });
const pingLoading = ref(false);
const pingResult = ref<any>(null);
const pingMeta = ref<any>({});

async function runPing() {
  pingLoading.value = true;
  pingResult.value = null;
  pingMeta.value = {};
  try {
    const res = await api.post('/api/v1/diagnostics/ping', {
      target: ping.value.target,
      count: ping.value.count,
    });
    const d = res.data || {};
    pingResult.value = d.result ?? d;
    pingMeta.value = meta(d);
  } catch (e: any) {
    showError(e, 'Ping 失败');
  } finally {
    pingLoading.value = false;
  }
}

// ---- 路由追踪 ----
const traceroute = ref({ target: '8.8.8.8', max_hops: 15 });
const trLoading = ref(false);
const trResult = ref<any>(null);
const trMeta = ref<any>({});

async function runTraceroute() {
  trLoading.value = true;
  trResult.value = null;
  trMeta.value = {};
  try {
    const res = await api.post('/api/v1/diagnostics/traceroute', {
      target: traceroute.value.target,
      max_hops: traceroute.value.max_hops,
    });
    const d = res.data || {};
    trResult.value = d.result ?? d;
    trMeta.value = meta(d);
  } catch (e: any) {
    showError(e, '路由追踪失败');
  } finally {
    trLoading.value = false;
  }
}

// ---- 端口探测 ----
const port = ref({ target: '127.0.0.1', port: 443, timeout: 3 });
const portLoading = ref(false);
const portResult = ref<any>(null);
const portMeta = ref<any>({});

async function runPort() {
  portLoading.value = true;
  portResult.value = null;
  portMeta.value = {};
  try {
    const res = await api.post('/api/v1/diagnostics/portprobe', {
      target: port.value.target,
      port: port.value.port,
      timeout: port.value.timeout,
    });
    const d = res.data || {};
    portResult.value = d.result ?? d;
    portMeta.value = meta(d);
  } catch (e: any) {
    showError(e, '端口探测失败');
  } finally {
    portLoading.value = false;
  }
}
</script>

<style scoped>
.cmd-line {
  color: #909399;
  font-size: 12px;
  margin: 2px 0;
  word-break: break-all;
}
.tool-pre {
  background: #f5f7fa;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.5;
  font-family: ui-monospace, SFMono-Regular, "Courier New", monospace;
  overflow: auto;
  max-height: 400px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 4px 0 0 0;
}
</style>
