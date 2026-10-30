<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>虚拟专网 · PPTP / L2TP / OpenVPN 服务端</span>
          <div class="tools">
            <el-tag v-if="status" size="small" :type="status === 'active' ? 'success' : 'info'">
              服务状态：{{ status }}
            </el-tag>
            <el-button type="primary" size="small" :loading="saving" @click="save">保存并生效</el-button>
          </div>
        </div>
      </template>

      <el-tabs v-model="proto" @tab-change="onTab">
        <el-tab-pane label="PPTP" name="pptp" />
        <el-tab-pane label="L2TP / IPSec" name="l2tp" />
        <el-tab-pane label="OpenVPN" name="openvpn" />
      </el-tabs>

      <el-alert v-if="proto === 'pptp'" type="warning" :closable="false" show-icon style="margin-bottom:12px"
        title="PPTP 的 MS-CHAPv2 可被离线爆破，只适合存量设备。新部署请优先用 L2TP/IPSec 或 OpenVPN。" />
      <el-alert v-if="proto === 'l2tp'" type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="只跑 xl2tpd 不配 IPSec 等于把明文 L2TP 暴露在公网，因此强制要求预共享密钥（除非显式关闭）。" />

      <!-- 公共基础设置 -->
      <el-form label-width="110px" style="max-width:760px">
        <el-form-item label="启用">
          <el-switch v-model="cfg.enabled" active-text="保存后立即启停服务" />
        </el-form-item>

        <template v-if="proto !== 'openvpn'">
          <el-form-item label="服务端地址" required>
            <el-input v-model="cfg.server_ip" placeholder="192.168.20.1" style="width:220px" />
            <span class="hint">不能落在地址池内，否则与拨号客户端冲突</span>
          </el-form-item>
          <el-form-item label="监听端口">
            <el-input-number v-model="cfg.server_port" :min="1" :max="65535" controls-position="right" />
          </el-form-item>
          <el-form-item label="地址池" required>
            <el-input v-model="cfg.addr_pool" placeholder="192.168.20.100-192.168.20.150" />
            <div class="hint">支持多段用逗号分隔，单段可简写 192.168.20.100-200；不要包含 .0 / .255</div>
          </el-form-item>
        </template>

        <el-alert
        v-if="proto === 'openvpn' && tunOk === false"
        type="error"
        show-icon
        :closable="false"
        title="本机不能创建 TUN 设备，OpenVPN 连不起来"
        style="margin-top:10px"
      >
        配置可以保存，但服务启动会立即失败。LXC 容器需要在宿主机放行 tun：
        <code>lxc.cgroup2.devices.allow: c 10:200 rwm</code> 与
        <code>lxc.mount.entry: /dev/net/tun dev/net/tun none bind,create=file</code>
      </el-alert>

      <template v-if="proto === 'openvpn'">
          <el-form-item label="隧道网段" required>
            <el-input v-model="cfg.subnet" placeholder="10.8.0.0" style="width:160px" />
            <el-input v-model="cfg.mask" placeholder="255.255.255.0" style="width:160px; margin-left:8px" />
          </el-form-item>
          <el-form-item label="协议 / 端口">
            <el-select v-model="cfg.proto" style="width:110px">
              <el-option label="UDP" value="udp" />
              <el-option label="TCP" value="tcp" />
            </el-select>
            <el-input-number v-model="cfg.port" :min="1" :max="65535" controls-position="right" style="margin-left:8px" />
          </el-form-item>
          <el-form-item label="加密算法">
            <el-select v-model="cfg.cipher" style="width:220px">
              <el-option v-for="c in ciphers" :key="c" :label="c" :value="c" />
            </el-select>
            <span class="hint">DES / RC2 / BF 已被拒绝</span>
          </el-form-item>
          <el-form-item label="设备 / 拓扑">
            <el-select v-model="cfg.dev_type" style="width:110px">
              <el-option label="tun" value="tun" />
              <el-option label="tap" value="tap" />
            </el-select>
            <el-select v-model="cfg.topology" style="width:130px; margin-left:8px">
              <el-option label="subnet" value="subnet" />
              <el-option label="net30" value="net30" />
            </el-select>
          </el-form-item>
          <el-form-item label="认证方式">
            <el-radio-group v-model="cfg.method">
              <el-radio-button :value="0">证书</el-radio-button>
              <el-radio-button :value="1">证书 + 账号密码</el-radio-button>
              <el-radio-button :value="2">仅账号密码</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </template>

        <el-form-item v-if="proto === 'pptp'" label="MPPE 加密">
          <el-radio-group v-model="cfg.open_mppe">
            <el-radio-button :value="2">128 位（推荐）</el-radio-button>
            <el-radio-button :value="1">40 位</el-radio-button>
            <el-radio-button :value="0">不加密</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <el-form-item v-if="proto === 'l2tp'" label="IPSec 密钥">
          <el-input v-model="cfg.ipsec_secret" placeholder="至少 8 位" show-password style="width:260px" />
          <el-checkbox v-model="forceIPSec" style="margin-left:12px">强制 IPSec</el-checkbox>
        </el-form-item>

        <el-form-item v-if="proto !== 'openvpn'" label="DNS">
          <el-input v-model="cfg.dns1" placeholder="223.5.5.5" style="width:170px" />
          <el-input v-model="cfg.dns2" placeholder="119.29.29.29" style="width:170px; margin-left:8px" />
        </el-form-item>

        <el-form-item v-if="proto !== 'openvpn'" label="MTU / MRU">
          <el-input-number v-model="cfg.mtu" :min="1000" :max="1492" controls-position="right" />
          <el-input-number v-model="cfg.mru" :min="1000" :max="1492" controls-position="right" style="margin-left:8px" />
          <span class="hint">范围 1000-1492（PPTP/L2TP 头部开销）</span>
        </el-form-item>
      </el-form>

      <el-divider content-position="left">拨号账号</el-divider>
      <el-table :data="users" size="small" empty-text="还没有账号">
        <el-table-column label="用户名" width="180">
          <template #default="{ row }"><el-input v-model="row.username" size="small" /></template>
        </el-table-column>
        <el-table-column label="密码" width="180">
          <template #default="{ row }"><el-input v-model="row.password" size="small" show-password /></template>
        </el-table-column>
        <el-table-column label="固定地址（可选）" width="180">
          <template #default="{ row }"><el-input v-model="row.ip" size="small" placeholder="须在地址池内" /></template>
        </el-table-column>
        <el-table-column label="备注">
          <template #default="{ row }"><el-input v-model="row.comment" size="small" /></template>
        </el-table-column>
        <el-table-column width="70">
          <template #default="{ $index }">
            <el-button link type="danger" size="small" @click="users.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-button size="small" style="margin-top:10px" @click="users.push({ username: '', password: '', ip: '', comment: '' })">新增账号</el-button>

      <template v-if="proto === 'openvpn'">
        <el-divider content-position="left">客户端证书与档案</el-divider>
        <div class="issue-row">
          <el-input v-model="newClient" placeholder="客户端名（字母数字 _ . -，1-64 位）" style="max-width:300px" />
          <el-input v-model="remoteHost" placeholder="服务端地址，如 vpn.example.com" style="max-width:240px" />
          <el-button type="primary" size="small" :loading="issuing" @click="issueClient">签发并下载 .ovpn</el-button>
        </div>
        <div class="hint" style="margin-top:6px">
          每个名字对应一张独立证书：可按人吊销、审计里分得清谁是谁。多人共用一个名字等于共用同一份凭据。
        </div>

        <el-divider content-position="left">在线客户端（每 10 秒刷新）</el-divider>
        <el-table :data="ovpnClients" size="small" border empty-text="当前没有客户端连接">
          <el-table-column prop="common_name" label="客户端" width="160" />
          <el-table-column prop="real_address" label="来源地址" min-width="170" />
          <el-table-column prop="virtual_address" label="隧道地址" width="140" />
          <el-table-column label="收 / 发" width="170">
            <template #default="{ row }">{{ fmtBytes(row.bytes_received) }} / {{ fmtBytes(row.bytes_sent) }}</template>
          </el-table-column>
          <el-table-column prop="connected_since" label="连接时间" min-width="170" />
        </el-table>
      </template>

      <el-card v-if="lastConf" shadow="never" style="margin-top:16px">
        <template #header><span>生成的配置</span></template>
        <pre class="code">{{ lastConf }}</pre>
      </el-card>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

type User = { username: string; password: string; ip: string; comment: string };
type Cfg = Record<string, any>;

interface OvpnClient {
  common_name: string;
  real_address: string;
  virtual_address: string;
  bytes_received: number;
  bytes_sent: number;
  connected_since: string;
}

const proto = ref('pptp');
const cfg = ref<Cfg>({});
const users = ref<User[]>([]);
const status = ref('');
const saving = ref(false);
const lastConf = ref('');
const forceIPSec = ref(true);

const newClient = ref('');
const remoteHost = ref('');
const issuing = ref(false);
const tunOk = ref<boolean | null>(null);
const ovpnClients = ref<OvpnClient[]>([]);
let poll: number | undefined;

function fmtBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

async function refreshClients() {
  if (proto.value !== 'openvpn') return;
  try {
    const res = await api.get('/api/v1/vpn/openvpn/status');
    ovpnClients.value = res.data?.clients || [];
  } catch {
    ovpnClients.value = [];
  }
}

async function issueClient() {
  const name = newClient.value.trim();
  if (!/^[A-Za-z0-9_.-]{1,64}$/.test(name)) {
    ElMessage.error('客户端名只能包含字母数字与 _ . -，长度 1-64');
    return;
  }
  issuing.value = true;
  try {
    const res = await api.post('/api/v1/vpn/openvpn/client-conf', {
      name,
      remote: remoteHost.value.trim(),
    });
    const conf = res.data?.conf || '';
    if (!conf) throw new Error('服务端返回了空配置');
    const blob = new Blob([conf], { type: 'application/x-openvpn-profile' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${name}.ovpn`;
    a.click();
    URL.revokeObjectURL(url);
    ElMessage.success(`已生成 ${name}.ovpn（含该客户端独立证书）`);
  } catch (e: any) {
    const d = e?.response?.data;
    ElMessage.error(d?.detail || d?.message || e?.message || '签发失败');
  } finally {
    issuing.value = false;
  }
}

const ciphers = ['AES-256-GCM', 'AES-192-GCM', 'AES-128-GCM', 'AES-256-CBC', 'AES-192-CBC', 'AES-128-CBC'];

const endpoint = () => `/api/v1/vpn/${proto.value}/services`;

function defaults(): any {
  if (proto.value === 'pptp') {
    return { enabled: false, server_ip: '', server_port: 1723, addr_pool: '', dns1: '223.5.5.5', dns2: '119.29.29.29', open_mppe: 2, mtu: 1400, mru: 1400 };
  }
  if (proto.value === 'l2tp') {
    return { enabled: false, server_ip: '', server_port: 1701, addr_pool: '', dns1: '223.5.5.5', dns2: '119.29.29.29', ipsec_secret: '', force_ipsec: 1, mtu: 1400, mru: 1400 };
  }
  return { enabled: false, subnet: '10.8.0.0', mask: '255.255.255.0', proto: 'udp', port: 1194, tun_mtu: 1420, cipher: 'AES-256-GCM', dev_type: 'tun', topology: 'subnet', method: 0, dns1: '223.5.5.5', dns2: '119.29.29.29' };
}

async function loadTun() {
  try {
    const res = await api.get('/api/v1/vpn/openvpn/services');
    if (typeof res.data?.tun === 'boolean') tunOk.value = res.data.tun;
  } catch {
    tunOk.value = null;
  }
}

async function load() {
  lastConf.value = '';
  try {
    const res = await api.get(endpoint());
    const d = defaults();
    const c = { ...(res.data?.config || {}) };
    // 后端用 0 表示"未配置，走默认"。直接塞进 el-input-number 会被 min 钳成下界
    // （端口变成 1、MTU 变成 1000），保存时就把错误的默认值写进配置了。
    for (const k of ['server_port', 'port', 'mtu', 'mru', 'tun_mtu']) {
      if (!c[k]) c[k] = d[k];
    }
    cfg.value = { ...d, ...c };
    users.value = (res.data?.config?.users || []).map((u: any) => ({
      username: u.username || '', password: u.password || '', ip: u.ip || '', comment: u.comment || '',
    }));
    status.value = res.data?.status || '';
    forceIPSec.value = cfg.value.force_ipsec !== 0;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  }
}

function onTab() {
  load();
  loadTun();
  refreshClients();
}

async function save() {
  saving.value = true;
  try {
    const body: any = { ...cfg.value, users: users.value.filter((u) => u.username.trim()) };
    if (proto.value === 'l2tp') body.force_ipsec = forceIPSec.value ? 1 : 0;
    const res = await api.post(endpoint(), body);
    cfg.value = { ...cfg.value, ...(res.data?.config || {}) };
    status.value = res.data?.status || '';
    if (res.data?.conf) lastConf.value = res.data.conf;
    if (res.data?.applied) {
      ElMessage.success('已保存并重启服务');
    } else {
      ElMessage.warning('配置已保存，但服务未启动：' + (res.data?.error || '本机可能未安装对应服务端软件包'));
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败');
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  await load();
  await loadTun();
  await refreshClients();
  poll = window.setInterval(refreshClients, 10000);
});

onUnmounted(() => {
  if (poll) window.clearInterval(poll);
});
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.tools { display: flex; gap: 8px; align-items: center; }
.issue-row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.hint { color: #8b91a5; font-size: 12px; margin-left: 8px; }
.code { background: #0f1117; color: #d5dae6; padding: 12px; border-radius: 6px; font-size: 12px; overflow: auto; max-height: 320px; }
</style>
