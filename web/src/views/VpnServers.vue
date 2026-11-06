<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>虚拟专网 · PPTP / L2TP / OpenVPN / EasyTier / Tailscale</span>
          <div class="tools">
            <el-tag v-if="status" size="small" :type="status === 'active' ? 'success' : 'info'">
              服务状态：{{ status }}
            </el-tag>
            <el-button type="primary" size="small" :loading="saving" @click="save">保存并生效</el-button>
          </div>
        </div>
      </template>

      <el-tabs v-model="proto" @tab-change="onTab">
        <el-tab-pane label="PPTP" name="pptp">
          <el-form label-width="110px" style="max-width:760px">
            <el-form-item label="启用">
              <el-switch v-model="cfg.enabled" active-text="保存后立即启停服务" />
            </el-form-item>
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
            <el-form-item label="MPPE 加密">
              <el-radio-group v-model="cfg.open_mppe">
                <el-radio-button :value="2">128 位（推荐）</el-radio-button>
                <el-radio-button :value="1">40 位</el-radio-button>
                <el-radio-button :value="0">不加密</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="DNS">
              <el-input v-model="cfg.dns1" placeholder="223.5.5.5" style="width:170px" />
              <el-input v-model="cfg.dns2" placeholder="119.29.29.29" style="width:170px; margin-left:8px" />
            </el-form-item>
            <el-form-item label="MTU / MRU">
              <el-input-number v-model="cfg.mtu" :min="1000" :max="1492" controls-position="right" />
              <el-input-number v-model="cfg.mru" :min="1000" :max="1492" controls-position="right" style="margin-left:8px" />
              <span class="hint">范围 1000-1492（PPTP 头部开销）</span>
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
        </el-tab-pane>
        <el-tab-pane label="L2TP / IPSec" name="l2tp">
          <el-form label-width="110px" style="max-width:760px">
            <el-form-item label="启用">
              <el-switch v-model="cfg.enabled" active-text="保存后立即启停服务" />
            </el-form-item>
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
            <el-form-item label="IPSec 密钥">
              <el-input v-model="cfg.ipsec_secret" placeholder="至少 8 位" show-password style="width:260px" />
              <el-checkbox v-model="forceIPSec" style="margin-left:12px">强制 IPSec</el-checkbox>
              <span v-if="pskSet" class="hint" style="margin-left:12px">已配置，留空表示不修改</span>
              <span v-else-if="forceIPSec" class="hint" style="margin-left:12px; color:#f56c6c">未配置，启用时必须填写</span>
            </el-form-item>
            <el-form-item label="IPSec 本端身份 (leftid)">
              <el-input v-model="cfg.leftid" placeholder="可空，默认用服务端地址" style="width:260px" />
            </el-form-item>
            <el-form-item label="IPSec 对端身份 (rightid)">
              <el-input v-model="cfg.rightid" placeholder="可空" style="width:260px" />
            </el-form-item>
            <el-form-item label="内网段 (left_subnet)">
              <el-input v-model="cfg.left_subnet" placeholder="允许客户端访问的内网段 CIDR，可空" style="width:260px" />
            </el-form-item>
            <el-form-item label="最大会话数">
              <el-input-number v-model="cfg.max_sessions" :min="0" :max="1024" controls-position="right" />
              <span class="hint">0 表示不限</span>
            </el-form-item>
            <el-form-item label="DNS">
              <el-input v-model="cfg.dns1" placeholder="223.5.5.5" style="width:170px" />
              <el-input v-model="cfg.dns2" placeholder="119.29.29.29" style="width:170px; margin-left:8px" />
            </el-form-item>
            <el-form-item label="MTU / MRU">
              <el-input-number v-model="cfg.mtu" :min="1000" :max="1492" controls-position="right" />
              <el-input-number v-model="cfg.mru" :min="1000" :max="1492" controls-position="right" style="margin-left:8px" />
              <span class="hint">范围 1000-1492（L2TP 头部开销）</span>
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
        </el-tab-pane>
        <el-tab-pane label="OpenVPN" name="openvpn" />
        <el-tab-pane label="EasyTier 组网" name="easytier" />
        <el-tab-pane label="Tailscale" name="tailscale" />
      </el-tabs>

      <el-alert v-if="proto === 'pptp'" type="warning" :closable="false" show-icon style="margin-bottom:12px"
        title="PPTP 的 MS-CHAPv2 可被离线爆破，只适合存量设备。新部署请优先用 L2TP/IPSec 或 OpenVPN。" />
      <el-alert v-if="proto === 'l2tp'" type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="只跑 xl2tpd 不配 IPSec 等于把明文 L2TP 暴露在公网，因此强制要求预共享密钥（除非显式关闭）。" />

      <!-- 公共基础设置（PPTP / L2TP / OpenVPN 共用；EasyTier 有自己的一套字段） -->
      <template v-if="proto === 'openvpn'">
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
      </template>

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

      <template v-if="proto === 'easytier'">
        <el-alert v-if="etBinaryExists === false" type="error" :closable="false" show-icon
          title="本机没有 easytier-core" style="margin-bottom:12px">
          配置可以保存，但实例起不来。请把 easytier-core / easytier-cli 放进 PATH，
          或用 MGM_EASYTIER_BINARY / MGM_EASYTIER_CLI 指定绝对路径。
        </el-alert>
        <el-alert v-else-if="etTun === false" type="warning" :closable="false" show-icon
          title="本机不能创建 TUN，只能作为中继节点" style="margin-bottom:12px">
          不填「本机 IPv4」时 EasyTier 只转发流量、不建 TUN。
          <template v-if="etPrivileged === false">当前 mgmtd 不是以 root 运行，创建 tun 接口需要 CAP_NET_ADMIN；要建 TUN 请用 root 启动，或给 easytier-core 加 setcap。</template>
          <template v-else>容器需要宿主机放行 /dev/net/tun。</template>
        </el-alert>

        <el-form label-width="130px" style="max-width:820px">
          <el-form-item label="启用">
            <el-switch v-model="et.enabled" active-text="保存后立即启停" />
            <el-tag size="small" style="margin-left:12px" :type="etRunning ? 'success' : 'info'">
              {{ etRunning ? `运行中 pid ${etPid}` : '未运行' }}
            </el-tag>
            <span v-if="etVersion" class="hint">{{ etVersion }}</span>
          </el-form-item>
          <el-form-item label="网络名称" required>
            <el-input v-model="et.network_name" placeholder="例如 office-mesh" style="width:260px" />
          </el-form-item>
          <el-form-item label="网络密钥">
            <el-input v-model="et.network_secret" show-password style="width:260px"
              :placeholder="etSecretSet ? '已配置，留空表示不修改' : '至少 8 位'" />
            <span class="hint">所有节点必须一致；空密钥等于谁都能加入</span>
          </el-form-item>
          <el-form-item label="本机 IPv4">
            <el-input v-model="et.ipv4" placeholder="10.144.0.1/24" style="width:220px" />
            <span class="hint">留空 = 只中继，不建 TUN</span>
          </el-form-item>
          <el-form-item label="RPC 管理地址">
            <el-input v-model="et.rpc_portal" placeholder="127.0.0.1:15888" style="width:220px" />
            <span class="hint">只监听本机；暴露到公网等于交出节点管理权</span>
          </el-form-item>
          <el-form-item label="监听器">
            <el-input v-model="etListeners" type="textarea" :rows="2" placeholder="tcp://0.0.0.0:11010" />
            <div class="hint">每行一个；支持 tcp:// udp:// ws:// wss:// quic:// wg:// 或裸端口</div>
          </el-form-item>
          <el-form-item label="对等节点">
            <el-input v-model="etPeers" type="textarea" :rows="3" placeholder="tcp://public.easytier.cn:11010" />
            <div class="hint">每行一个；填了公共节点才能在没有公网入口时自动发现对端</div>
          </el-form-item>
          <el-form-item label="代理网段">
            <el-input v-model="etProxy" type="textarea" :rows="2" placeholder="192.168.1.0/24 或 10.0.0.0/24->192.168.50.0/24" />
          </el-form-item>
          <el-form-item label="公共中继">
            <el-input v-model="et.external_node" placeholder="tcp://public.easytier.cn:11010（可留空）" style="width:340px" />
          </el-form-item>
          <el-form-item label="角色">
            <el-checkbox v-model="et.enable_exit_node">作为出口节点（其他节点可经它上网）</el-checkbox>
            <el-checkbox v-model="et.relay_all_peer_traffic">中继其他节点的流量</el-checkbox>
          </el-form-item>
          <el-form-item>
            <el-button size="small" @click="previewEasy">预览配置</el-button>
            <el-button size="small" :loading="etLoading" @click="loadEasy">刷新状态</el-button>
            <el-button size="small" type="danger" plain @click="stopEasy">停止实例</el-button>
          </el-form-item>
        </el-form>

        <el-alert v-if="etErr" type="warning" :closable="false" show-icon :title="etErr" style="margin-bottom:12px" />

        <el-divider content-position="left">对等节点（每 10 秒刷新）</el-divider>
        <el-table :data="etPeersList" size="small" border empty-text="没有节点（实例未运行或还没有对端）">
          <el-table-column prop="ipv4" label="隧道地址" width="150" />
          <el-table-column prop="hostname" label="主机名" min-width="160" />
          <el-table-column label="链路" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="row.local ? 'info' : 'success'">{{ row.cost }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="延迟" width="100">
            <template #default="{ row }">{{ row.latency_ok ? `${row.latency_ms} ms` : '-' }}</template>
          </el-table-column>
          <el-table-column prop="loss" label="丢包" width="80" />
          <el-table-column prop="tunnel" label="传输" width="80" />
          <el-table-column prop="nat" label="NAT" width="130" />
          <el-table-column label="收 / 发" width="150">
            <template #default="{ row }">{{ row.rx }} / {{ row.tx }}</template>
          </el-table-column>
          <el-table-column prop="version" label="版本" min-width="140" />
        </el-table>
      </template>

      <template v-if="proto === 'tailscale'">
        <el-alert v-if="tsStatus && tsStatus.error" type="error" :closable="false" show-icon
          :title="tsStatus.error" style="margin-bottom:12px" />
        <el-alert v-if="tsSystemSocket" type="warning" :closable="false" show-icon style="margin-bottom:12px"
          title="正在操作系统自带的 tailscaled">
          在这里点「断开连接」会切断这台机器自己的 Tailscale，包括你可能正在使用的远程管理通道。
        </el-alert>

        <el-form label-width="130px" style="max-width:860px">
          <el-form-item label="启用">
            <el-switch v-model="ts.enabled" active-text="保存后执行 tailscale up / down" />
            <el-tag size="small" style="margin-left:12px"
              :type="tsStatus?.backend_state === 'Running' ? 'success' : 'info'">
              {{ tsStatus?.backend_state || '未知' }}
            </el-tag>
            <span v-if="tsStatus?.version" class="hint">{{ tsStatus.version }}</span>
          </el-form-item>
          <el-form-item label="节点名">
            <el-input v-model="ts.hostname" placeholder="留空用系统主机名" style="width:240px" />
            <span class="hint">只能是字母数字和连字符</span>
          </el-form-item>
          <el-form-item label="授权密钥">
            <el-input v-model="ts.auth_key" show-password style="width:360px"
              :placeholder="tsKeySet ? '已配置，留空表示不修改' : 'tskey-auth-...（留空则沿用已有登录）'" />
            <div class="hint">只写入不回显；节点已登录时留空即可，只会修改下面的开关</div>
          </el-form-item>
          <el-form-item label="通告子网">
            <el-input v-model="tsRoutes" type="textarea" :rows="2" placeholder="192.168.1.0/24" />
            <div class="hint">每行一个 IPv4 CIDR；宣告后还需要在管理台批准（否则状态里会有提示）</div>
          </el-form-item>
          <el-form-item label="角色与开关">
            <el-checkbox v-model="ts.advertise_exit_node">作为出口节点</el-checkbox>
            <el-checkbox v-model="ts.accept_routes">接受其他节点的子网路由</el-checkbox>
            <el-checkbox v-model="ts.accept_dns">使用 MagicDNS</el-checkbox>
            <el-checkbox v-model="ts.ssh">允许 Tailscale SSH 登录本机</el-checkbox>
          </el-form-item>
          <el-form-item label="控制面地址">
            <el-input v-model="ts.login_server" placeholder="https://controlplane.tailscale.com（自建 headscale 时改这里）" style="width:420px" />
          </el-form-item>
          <el-form-item label="重新认证">
            <el-checkbox v-model="ts.force_reauth">保存时强制重新走一次授权</el-checkbox>
          </el-form-item>
          <el-form-item>
            <el-button size="small" @click="previewTailscale">预览参数</el-button>
            <el-button size="small" :loading="tsLoading" @click="loadTailscale">刷新状态</el-button>
            <el-button size="small" type="danger" plain @click="downTailscale">断开连接</el-button>
          </el-form-item>
        </el-form>

        <el-alert v-if="tsArgs.length" type="info" :closable="false" style="margin-bottom:12px">
          <template #title><code>{{ tsArgs.join(' ') }}</code></template>
        </el-alert>

        <el-descriptions v-if="tsStatus?.reachable" :column="4" border size="small" style="margin-bottom:12px">
          <el-descriptions-item label="后端状态">{{ tsStatus.backend_state }}</el-descriptions-item>
          <el-descriptions-item label="网络">{{ tsStatus.tailnet || '-' }}</el-descriptions-item>
          <el-descriptions-item label="本机地址">{{ (tsStatus.self?.ips || []).join(', ') || '-' }}</el-descriptions-item>
          <el-descriptions-item label="在线节点">{{ tsStatus.online_count }} / {{ tsStatus.peer_count }}</el-descriptions-item>
        </el-descriptions>

        <el-alert v-for="(h, i) in (tsStatus?.health || [])" :key="i" type="warning" :closable="false"
          :title="h" style="margin-bottom:6px" />

        <el-divider content-position="left">节点列表（每 10 秒刷新）</el-divider>
        <el-table :data="tsPeers" size="small" border empty-text="没有节点（未登录或网络里只有本机）">
          <el-table-column prop="hostname" label="主机名" min-width="170" />
          <el-table-column prop="os" label="系统" width="90" />
          <el-table-column label="地址" min-width="170">
            <template #default="{ row }">{{ (row.ips || []).join(', ') }}</template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="row.online ? 'success' : 'info'">{{ row.online ? '在线' : '离线' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="relay" label="中继" width="80" />
          <el-table-column label="出口节点" width="90">
            <template #default="{ row }">{{ row.exit_node ? '是' : '-' }}</template>
          </el-table-column>
          <el-table-column label="收 / 发" width="160">
            <template #default="{ row }">{{ fmtBytes(row.rx_bytes || 0) }} / {{ fmtBytes(row.tx_bytes || 0) }}</template>
          </el-table-column>
          <el-table-column label="最近可见" width="170">
            <template #default="{ row }">{{ fmtLastSeen(row.last_seen) }}</template>
          </el-table-column>
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
import { ElMessage, ElMessageBox } from 'element-plus';
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
const pskSet = ref(false);

const newClient = ref('');
const remoteHost = ref('');
const issuing = ref(false);
const tunOk = ref<boolean | null>(null);
const ovpnClients = ref<OvpnClient[]>([]);
let poll: number | undefined;

// ---------- EasyTier ----------
type ETPeer = {
  ipv4: string; hostname: string; cost: string; latency_ms: number; latency_ok: boolean;
  loss: string; rx: string; tx: string; tunnel: string; nat: string; version: string; local: boolean;
};
const et = ref<Record<string, any>>({});
const etListeners = ref('');
const etPeers = ref('');
const etProxy = ref('');
const etPeersList = ref<ETPeer[]>([]);
const etRunning = ref(false);
const etPid = ref(0);
const etSecretSet = ref(false);
const etTun = ref<boolean | null>(null);
const etBinaryExists = ref<boolean | null>(null);
const etPrivileged = ref<boolean | null>(null);
const etVersion = ref('');
const etErr = ref('');
const etLoading = ref(false);

function splitLines(s: string): string[] {
  return s.split(/\r?\n/).map((x) => x.trim()).filter(Boolean);
}

function applyEasyStatus(st: any) {
  etRunning.value = !!st?.running;
  etPid.value = st?.pid || 0;
  status.value = st?.running ? 'active' : 'inactive';
}

async function loadEasy() {
  etLoading.value = true;
  try {
    const res = await api.get('/api/v1/vpn/easytier/config');
    const c = res.data?.config || {};
    et.value = {
      enabled: false, instance_name: 'default', network_name: '',
      ipv4: '', rpc_portal: '127.0.0.1:15888', external_node: '',
      enable_exit_node: false, relay_all_peer_traffic: false, ...c,
      network_secret: '',
    };
    etListeners.value = (c.listeners || []).join('\n');
    etPeers.value = (c.peers || []).join('\n');
    etProxy.value = (c.proxy_networks || []).join('\n');
    etSecretSet.value = !!res.data?.secret_set;
    etTun.value = typeof res.data?.tun === 'boolean' ? res.data.tun : null;
    etBinaryExists.value = typeof res.data?.binary_exists === 'boolean' ? res.data.binary_exists : null;
    etPrivileged.value = typeof res.data?.privileged === 'boolean' ? res.data.privileged : null;
    etVersion.value = res.data?.version || '';
    applyEasyStatus(res.data?.status);
    await refreshEasyPeers();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e?.message || '加载失败');
  } finally {
    etLoading.value = false;
  }
}

function easyBody(): Record<string, any> {
  return {
    ...et.value,
    listeners: splitLines(etListeners.value),
    peers: splitLines(etPeers.value),
    proxy_networks: splitLines(etProxy.value),
  };
}

async function refreshEasyPeers() {
  if (proto.value !== 'easytier') return;
  try {
    const res = await api.get('/api/v1/vpn/easytier/status');
    applyEasyStatus(res.data?.status);
    etPeersList.value = res.data?.peers || [];
    etErr.value = res.data?.error || '';
  } catch {
    etPeersList.value = [];
  }
}

async function saveEasy() {
  const res = await api.post('/api/v1/vpn/easytier/config', easyBody());
  et.value = { ...et.value, ...(res.data?.config || {}), network_secret: '' };
  etSecretSet.value = !!res.data?.secret_set;
  applyEasyStatus(res.data?.status);
  if (res.data?.rendered) lastConf.value = res.data.rendered;
  if (res.data?.applied) {
    ElMessage.success(et.value.enabled ? '已保存并启动 EasyTier' : '配置已保存（未启用）');
  } else {
    ElMessage.warning('配置已保存，但实例未启动：' + (res.data?.error || '本机可能没装 easytier-core'));
  }
  await refreshEasyPeers();
}

async function stopEasy() {
  try {
    await api.post('/api/v1/vpn/easytier/stop');
    ElMessage.success('已停止 EasyTier 实例');
    await refreshEasyPeers();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.message || '停止失败');
  }
}

async function previewEasy() {
  try {
    const res = await api.post('/api/v1/vpn/easytier/preview', easyBody());
    lastConf.value = res.data?.config || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e?.message || '预览失败');
  }
}

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
    return { enabled: false, server_ip: '', server_port: 1701, addr_pool: '', dns1: '223.5.5.5', dns2: '119.29.29.29', ipsec_secret: '', force_ipsec: 1, mtu: 1400, mru: 1400, leftid: '', rightid: '', left_subnet: '', max_sessions: 0 };
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

// ---------- Tailscale ----------
type TSNode = {
  id: string; hostname: string; dns_name: string; os: string; ips: string[];
  online: boolean; relay: string; exit_node: boolean; rx_bytes: number; tx_bytes: number;
  last_seen: string; is_self: boolean;
};
const ts = ref<Record<string, any>>({});
const tsStatus = ref<any>(null);
const tsPeers = ref<TSNode[]>([]);
const tsRoutes = ref('');
const tsKeySet = ref(false);
const tsSystemSocket = ref(false);
const tsArgs = ref<string[]>([]);
const tsLoading = ref(false);

function fmtLastSeen(v?: string): string {
  if (!v || v.startsWith('0001-01-01')) return '-';
  const s = v.replace('T', ' ').replace('Z', '');
  return s.length >= 19 ? s.slice(0, 19) : s;
}

async function loadTailscale() {
  tsLoading.value = true;
  try {
    const res = await api.get('/api/v1/vpn/tailscale/config');
    const c = res.data?.config || {};
    ts.value = {
      enabled: false, hostname: '', login_server: '',
      advertise_exit_node: false, accept_routes: false, accept_dns: true,
      ssh: false, force_reauth: false, ...c, auth_key: '',
    };
    tsRoutes.value = (c.advertise_routes || []).join('\n');
    tsKeySet.value = !!res.data?.key_set;
    tsSystemSocket.value = !!res.data?.system_socket;
    tsStatus.value = res.data?.status || null;
    tsPeers.value = (res.data?.status?.peers || []) as TSNode[];
    tsArgs.value = res.data?.up_args_preview || [];
    status.value = tsStatus.value?.backend_state === 'Running' ? 'active' : 'inactive';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.message || '加载失败');
  } finally {
    tsLoading.value = false;
  }
}

function tsBody(): Record<string, any> {
  return { ...ts.value, advertise_routes: splitLines(tsRoutes.value) };
}

async function refreshTailscaleStatus() {
  if (proto.value !== 'tailscale') return;
  try {
    const res = await api.get('/api/v1/vpn/tailscale/status');
    tsStatus.value = res.data?.status || null;
    tsPeers.value = (res.data?.status?.peers || []) as TSNode[];
    tsSystemSocket.value = !!res.data?.system_socket;
    status.value = tsStatus.value?.backend_state === 'Running' ? 'active' : 'inactive';
  } catch {
    tsPeers.value = [];
  }
}

async function saveTailscale() {
  const res = await api.post('/api/v1/vpn/tailscale/config', tsBody());
  ts.value = { ...ts.value, ...(res.data?.config || {}), auth_key: '' };
  tsKeySet.value = !!res.data?.key_set;
  tsStatus.value = res.data?.status || null;
  tsPeers.value = (res.data?.status?.peers || []) as TSNode[];
  tsArgs.value = res.data?.up_args || [];
  status.value = tsStatus.value?.backend_state === 'Running' ? 'active' : 'inactive';
  if (res.data?.applied) {
    ElMessage.success(ts.value.enabled ? '已保存并接入 Tailscale' : '已保存并断开 Tailscale');
  } else {
    ElMessage.warning('配置已保存，但未生效：' + (res.data?.error || ''));
  }
}

async function downTailscale() {
  try {
    await ElMessageBox.confirm(
      tsSystemSocket.value
        ? '这会断开本机自己的 Tailscale 连接，如果当前正通过 Tailscale 管理这台机器，会立即失联。确定继续？'
        : '确定断开这个 Tailscale 节点？',
      '断开连接', { type: 'warning', confirmButtonText: '断开', cancelButtonText: '取消' },
    );
  } catch {
    return;
  }
  try {
    await api.post('/api/v1/vpn/tailscale/down');
    ElMessage.success('已断开');
    await refreshTailscaleStatus();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.message || '断开失败');
  }
}

async function previewTailscale() {
  try {
    const res = await api.post('/api/v1/vpn/tailscale/preview', tsBody());
    tsArgs.value = res.data?.args || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.message || '预览失败');
  }
}

async function load() {
  lastConf.value = '';
  if (proto.value === 'easytier') {
    await loadEasy();
    return;
  }
  if (proto.value === 'tailscale') {
    await loadTailscale();
    return;
  }
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
    pskSet.value = !!res.data?.psk_set;
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
    if (proto.value === 'easytier') {
      await saveEasy();
      return;
    }
    if (proto.value === 'tailscale') {
      await saveTailscale();
      return;
    }
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
  poll = window.setInterval(() => {
    refreshClients();
    refreshEasyPeers();
    refreshTailscaleStatus();
  }, 10000);
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
