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

        <!-- Tab 4: 抓包 -->
        <el-tab-pane label="抓包" name="capture">
          <el-form inline size="small">
            <el-form-item label="网卡">
              <el-input v-model="capture.interface" placeholder="如 eth0 / br-lan" style="width:160px" />
            </el-form-item>
            <el-form-item label="过滤表达式">
              <el-input v-model="capture.filter" placeholder="tcpdump BPF，如 host 192.168.1.1" style="width:260px" />
            </el-form-item>
            <el-form-item label="时长(s)">
              <el-input-number v-model="capture.seconds" :min="1" :max="600" style="width:100px" />
            </el-form-item>
            <el-form-item label="最大字节">
              <el-input-number v-model="capture.max_bytes" :min="1" style="width:140px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="captureLoading" @click="runCaptureStart">开始抓包</el-button>
            </el-form-item>
          </el-form>

          <div v-if="captureStartResult" style="margin-bottom:8px">
            <el-tag v-if="captureStartResult.id" type="success">任务 ID：{{ captureStartResult.id }}</el-tag>
            <el-tag v-if="captureStartResult.status" type="info">状态：{{ captureStartResult.status }}</el-tag>
          </div>

          <el-button type="text" size="small" :disabled="!captureListLoading && captureList.length === 0" @click="refreshCaptureList">刷新列表</el-button>
          <el-table v-if="captureList.length" :data="captureList" style="width:100%" size="small">
            <el-table-column prop="id" label="任务 ID" width="220" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="scope">
                <el-tag v-if="scope.row.status === 'running'" type="warning">运行中</el-tag>
                <el-tag v-else-if="scope.row.status === 'completed'" type="success">已完成</el-tag>
                <el-tag v-else-if="scope.row.status === 'stopped'" type="danger">已停止</el-tag>
                <el-tag v-else-if="scope.row.status === 'error'" type="error">错误</el-tag>
                <el-tag v-else>{{ scope.row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="interface" label="网卡" width="100" />
            <el-table-column prop="bytes_written" label="文件大小" width="120">
              <template #default="scope">{{ (scope.row.bytes_written || 0) | bytes }}</template>
            </el-table-column>
            <el-table-column label="操作" width="220">
              <template #default="scope">
                <el-button
                  type="primary"
                  size="small"
                  :disabled="scope.row.status !== 'running'"
                  @click="stopCapture(scope.row.id)"
                >停止</el-button>
                <el-button
                  type="text"
                  size="small"
                  :disabled="scope.row.status === 'running' || scope.row.status === ''"
                  @click="downloadCapture(scope.row.id)"
                >下载</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="captureList.length === 0 && !captureListLoading" style="color:#909399;font-size:12px;margin-top:6px">暂无抓包任务</div>
        </el-tab-pane>

        <!-- Tab 5: IPerf3 -->
        <el-tab-pane label="IPerf3" name="iperf">
          <el-form inline size="small">
            <el-form-item label="目标主机">
              <el-input v-model="iperf.host" placeholder="如 192.168.1.100" style="width:180px" />
            </el-form-item>
            <el-form-item label="端口">
              <el-input-number v-model="iperf.port" :min="1" :max="65535" style="width:100px" />
            </el-form-item>
            <el-form-item label="时长(s)">
              <el-input-number v-model="iperf.duration" :min="1" :max="300" style="width:100px" />
            </el-form-item>
            <el-form-item label="协议">
              <el-select v-model="iperf.protocol" style="width:100px">
                <el-option label="TCP" value="tcp" />
                <el-option label="UDP" value="udp" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="iperfLoading" @click="runIperf">开始测速</el-button>
            </el-form-item>
          </el-form>

          <el-button type="text" size="small" @click="refreshIperfStatus">刷新状态</el-button>

          <div v-if="iperfResult" style="margin-top:12px">
            <el-tag v-if="iperfResult.ok === true" type="success">成功</el-tag>
            <el-tag v-else-if="iperfResult.ok === false" type="danger">失败</el-tag>
            <el-table v-if="iperfResult.ok || iperfResult.error" :data="[iperfResult]" style="width:100%" size="small">
              <el-table-column prop="host" label="目标" width="140" />
              <el-table-column prop="port" label="端口" width="80" />
              <el-table-column prop="protocol" label="协议" width="80" />
              <el-table-column prop="duration" label="时长(s)" width="80" />
              <el-table-column prop="bandwidth_bps" label="带宽(bps)" width="140">
                <template #default="scope">{{ scope.row.bandwidth_bps ? (scope.row.bandwidth_bps / 1e6).toFixed(2) + ' Mbps' : 'N/A' }}</template>
              </el-table-column>
              <el-table-column prop="bytes" label="传输字节" width="120" />
              <el-table-column prop="retransmits" label="重传" width="80" />
              <el-table-column prop="lost_percent" label="丢失率(%)" width="100" />
              <el-table-column prop="jitter_ms" label="抖动(ms)" width="100" />
            </el-table>
            <div v-if="iperfResult.error" style="color:#f56c6c;font-size:12px;margin-top:6px">错误：{{ iperfResult.error }}</div>
          </div>
        </el-tab-pane>

        <!-- Tab 6: 子网计算 -->
        <el-tab-pane label="子网计算" name="subnet">
          <el-form inline size="small">
            <el-form-item label="CIDR">
              <el-input v-model="subnet.cidr" placeholder="如 192.168.1.0/24" style="width:200px" @keyup.enter="calcSubnet" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="calcSubnet">计算</el-button>
            </el-form-item>
          </el-form>

          <el-table v-if="subnetResult" :data="[subnetResult]" style="width:100%" size="small">
            <el-table-column prop="network" label="网络地址" width="160" />
            <el-table-column prop="broadcast" label="广播地址" width="160" />
            <el-table-column prop="netmask" label="子网掩码" width="140" />
            <el-table-column prop="prefix_len" label="前缀长度" width="100" />
            <el-table-column prop="first_host" label="首可用主机" width="160" />
            <el-table-column prop="last_host" label="末可用主机" width="160" />
            <el-table-column prop="host_count" label="可用主机数" width="120" />
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

const tab = ref('ping');

// 后端错误格式普遍 {code, message, detail}，detail 往往才是真正原因（如"含非法字符"），一起显示。
function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

// 提取接口返回里除了 result 之外的元信息（command / elapsed_ms / note）
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

// 字节数可读化过滤器（模板中使用）
function bytes(n: number): string {
  if (!n) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(n) / Math.log(k));
  return parseFloat((n / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
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

// ========================================================
// 抓包（Capture）
// ========================================================
const capture = reactive({
  interface: 'eth0',
  filter: '',
  seconds: 60,
  max_bytes: 32 * 1024 * 1024,
});
const captureLoading = ref(false);
const captureStartResult = ref<any>(null);
const captureList: any[] = [];
const captureListLoading = ref(false);
let capturePollTimer: ReturnType<typeof setInterval> | null = null;

async function runCaptureStart() {
  captureLoading.value = true;
  captureStartResult.value = null;
  try {
    const res = await api.post('/api/v1/tools/capture/start', {
      interface: capture.interface,
      filter: capture.filter,
      seconds: capture.seconds,
      max_bytes: capture.max_bytes,
    });
    const d = res.data || {};
    captureStartResult.value = d;
    if (d.id) {
      startCapturePoll();
    }
    ElMessage.success(`抓包已启动，任务 ID：${d.id}`);
  } catch (e: any) {
    showError(e, '启动抓包失败');
  } finally {
    captureLoading.value = false;
  }
}

async function refreshCaptureList() {
  captureListLoading.value = true;
  try {
    const res = await api.get('/api/v1/tools/capture/status');
    const d = res.data || {};
    captureList.length = 0;
    captureList.push(...(d.captures || []));
  } catch (e: any) {
    showError(e, '获取抓包列表失败');
  } finally {
    captureListLoading.value = false;
  }
}

function startCapturePoll() {
  if (capturePollTimer) {
    clearInterval(capturePollTimer);
  }
  capturePollTimer = setInterval(() => {
    refreshCaptureList();
  }, 3000);
}

async function stopCapture(id: string) {
  try {
    const res = await api.post(`/api/v1/tools/capture/${encodeURIComponent(id)}/stop`);
    const d = res.data || {};
    ElMessage.success(d.message || '抓包已停止');
    await refreshCaptureList();
  } catch (e: any) {
    showError(e, '停止抓包失败');
  }
}

async function downloadCapture(id: string) {
  try {
    const res = await api.get(`/api/v1/tools/capture/${encodeURIComponent(id)}/download`, {
      responseType: 'blob',
    });
    const blob = res.data as Blob;
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${id}.pcap`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    window.URL.revokeObjectURL(url);
  } catch (e: any) {
    showError(e, '下载抓包文件失败');
  }
}

// ========================================================
// IPerf3
// ========================================================
const iperf = reactive({
  host: '127.0.0.1',
  port: 5201,
  duration: 10,
  protocol: 'tcp',
});
const iperfLoading = ref(false);
const iperfResult = ref<any>(null);
let iperfPollTimer: ReturnType<typeof setInterval> | null = null;

async function runIperf() {
  iperfLoading.value = true;
  try {
    const res = await api.post('/api/v1/tools/iperf/run', {
      host: iperf.host,
      port: iperf.port,
      duration: iperf.duration,
      protocol: iperf.protocol,
    });
    const d = res.data || {};
    iperfResult.value = d;
    if (d.ok) {
      ElMessage.success('IPerf3 测速完成');
    } else {
      ElMessage.error(d.error || 'IPerf3 测速失败');
    }
  } catch (e: any) {
    showError(e, 'IPerf3 测速失败');
  } finally {
    iperfLoading.value = false;
  }
}

async function refreshIperfStatus() {
  try {
    const res = await api.get('/api/v1/tools/iperf/status');
    const d = res.data || {};
    iperfResult.value = d;
  } catch (e: any) {
    showError(e, '获取 IPerf3 状态失败');
  }
}

// ========================================================
// 子网计算（Subnet）
// ========================================================
const subnet = reactive({
  cidr: '192.168.1.0/24',
});
const subnetResult = ref<any>(null);

async function calcSubnet() {
  if (!subnet.cidr.trim()) {
    ElMessage.warning('请输入 CIDR');
    return;
  }
  try {
    const res = await api.get('/api/v1/tools/subnet', {
      params: { cidr: subnet.cidr.trim() },
    });
    const d = res.data || {};
    subnetResult.value = d;
    ElMessage.success('子网计算完成');
  } catch (e: any) {
    showError(e, '子网计算失败');
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