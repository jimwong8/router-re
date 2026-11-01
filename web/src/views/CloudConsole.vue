<template>
  <div class="cloud-console">
    <!-- 登录 -->
    <div v-if="!token" class="login-wrap">
      <el-card class="login-card" shadow="never">
        <template #header>
          <span class="login-title">router-re 远程管理中心</span>
        </template>
        <el-form @submit.prevent="login">
          <el-form-item>
            <el-input v-model="username" placeholder="账号" prefix-icon="User" autocomplete="username" />
          </el-form-item>
          <el-form-item>
            <el-input v-model="password" type="password" placeholder="密码" prefix-icon="Lock" show-password autocomplete="current-password" @keyup.enter="login" />
          </el-form-item>
          <el-button type="primary" style="width:100%" :loading="logging" @click="login">登录</el-button>
        </el-form>
        <p class="muted">会话 12 小时，所有操作写审计。</p>
      </el-card>
    </div>

    <!-- 主界面 -->
    <template v-else>
      <header class="cc-header">
        <span class="cc-title">router-re 远程管理中心</span>
        <el-tag :type="onlineCount > 0 ? 'success' : 'info'" size="small">{{ onlineCount }}/{{ devices.length }} 在线</el-tag>
        <el-tag v-if="alertCount > 0" type="danger" size="small" style="margin-left:4px">{{ alertCount }} 告警</el-tag>
        <span class="sp"></span>
        <el-input v-model="searchQuery" placeholder="搜索设备名/IP" size="small" style="width:200px" clearable prefix-icon="Search" />
        <el-select v-model="statusFilter" size="small" style="width:100px;margin-left:8px" placeholder="状态">
          <el-option label="全部" value="" />
          <el-option label="在线" value="online" />
          <el-option label="离线" value="offline" />
        </el-select>
        <el-button size="small" style="margin-left:8px" @click="refreshAll" :loading="refreshing">刷新</el-button>
        <el-button size="small" type="danger" plain @click="logout">退出</el-button>
        <el-button size="small" plain @click="showChangePw=true">修改密码</el-button>
      </header>

      <main class="cc-main">
        <!-- 概览卡片 -->
        <div class="cards">
          <el-card shadow="never" class="card" @click="statusFilter=''; searchQuery=''">
            <div class="k">设备总数</div><div class="v">{{ devices.length }}</div>
          </el-card>
          <el-card shadow="never" class="card" @click="statusFilter='online'">
            <div class="k">在线</div><div class="v" style="color:var(--el-color-success)">{{ onlineCount }}</div>
          </el-card>
          <el-card shadow="never" class="card" @click="statusFilter='offline'">
            <div class="k">离线</div><div class="v" style="color:var(--el-color-info)">{{ devices.length - onlineCount }}</div>
          </el-card>
          <el-card shadow="never" class="card">
            <div class="k">告警</div><div class="v" style="color:var(--el-color-danger)">{{ alertCount }}</div>
          </el-card>
          <el-card shadow="never" class="card">
            <div class="k">服务运行时长</div><div class="v" style="font-size:18px">{{ dur(overview.server_uptime) }}</div>
          </el-card>
        </div>

        <!-- 批量操作栏 -->
        <el-card v-if="selectedDevices.length > 0" shadow="never" style="margin-bottom:12px;padding:8px 16px">
          <div class="row" style="justify-content:space-between">
            <span>已选 {{ selectedDevices.length }} 台设备</span>
            <div class="row">
              <el-button size="small" type="warning" @click="batchReboot" :loading="batchLoading">批量重启</el-button>
              <el-button size="small" @click="selectedDevices=[]">取消选择</el-button>
            </div>
          </div>
        </el-card>

        <!-- 选项卡 -->
        <el-tabs v-model="activeTab" type="card">
          <el-tab-pane label="概览" name="overview">
            <div class="cards" style="margin-bottom:20px">
              <el-card shadow="never" class="card" @click="activeTab='dev'">
                <div class="k">设备总数</div><div class="v">{{ devices.length }}</div>
              </el-card>
              <el-card shadow="never" class="card" @click="statusFilter='online'; activeTab='dev'">
                <div class="k">在线</div><div class="v" style="color:var(--el-color-success)">{{ onlineCount }}</div>
              </el-card>
              <el-card shadow="never" class="card" @click="statusFilter='offline'; activeTab='dev'">
                <div class="k">离线</div><div class="v" style="color:var(--el-color-info)">{{ devices.length - onlineCount }}</div>
              </el-card>
              <el-card shadow="never" class="card" @click="activeTab='alerts'">
                <div class="k">告警</div><div class="v" style="color:var(--el-color-danger)">{{ alertCount }}</div>
              </el-card>
              <el-card shadow="never" class="card">
                <div class="k">服务运行时长</div><div class="v" style="font-size:18px">{{ dur(overview.server_uptime) }}</div>
              </el-card>
              <el-card shadow="never" class="card">
                <div class="k">注册令牌</div><div class="v">{{ tokens.length }}</div>
              </el-card>
            </div>
            <!-- 设备健康概览 -->
            <el-card shadow="never" style="margin-bottom:16px">
              <template #header><span style="font-weight:600">设备健康状态</span></template>
              <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px">
                <div v-for="d in devices" :key="d.id" style="display:flex;align-items:center;gap:8px;padding:8px;border:1px solid var(--el-border-color);border-radius:8px">
                  <el-tag :type="d.online ? 'success' : 'info'" size="small" effect="dot">{{ d.online ? '在线' : '离线' }}</el-tag>
                  <div style="flex:1">
                    <div style="font-weight:600;font-size:13px">{{ d.name }}</div>
                    <div class="muted small">{{ d.model }} · {{ d.remote_ip || '—' }}</div>
                  </div>
                  <div style="text-align:right">
                    <div class="muted small">心跳 {{ ago(d.last_seen) }}</div>
                  </div>
                </div>
              </div>
            </el-card>
            <!-- 最近告警 -->
            <el-card shadow="never">
              <template #header><span style="font-weight:600">最近告警</span></template>
              <el-table v-if="alerts.length" :data="alerts.slice(0,10)" border stripe size="small" style="width:100%">
                <el-table-column prop="ts" label="时间" width="160">
                  <template #default="{ row }">{{ fmtTime(row.ts) }}</template>
                </el-table-column>
                <el-table-column prop="device" label="设备" width="140" />
                <el-table-column prop="level" label="级别" width="90">
                  <template #default="{ row }">
                    <el-tag :type="row.level==='critical'?'danger':(row.level==='warning'?'warning':'info')" size="small">{{ row.level }}</el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="message" label="消息" />
              </el-table>
              <div v-else class="muted" style="text-align:center;padding:20px">暂无告警</div>
            </el-card>
          </el-tab-pane>
          <el-tab-pane label="设备" name="dev">
            <el-table :data="filteredDevices" border stripe style="width:100%" @selection-change="onSelectionChange">
              <el-table-column type="selection" width="45" />
              <el-table-column label="类型" width="70" align="center">
                <template #default="{ row }">
                  <el-tag :color="deviceType(row).color" size="small" effect="plain" style="border:none;font-weight:bold">
                    {{ deviceType(row).label }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="90">
                <template #default="{ row }">
                  <el-tag :type="row.online ? 'success' : 'info'" size="small">
                    <span class="dot" :class="row.online ? 'on' : 'off'"></span>{{ row.online ? '在线' : '离线' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="name" label="名称" min-width="140">
                <template #default="{ row }">
                  <b>{{ row.name }}</b>
                  <div class="muted small">{{ row.id }}</div>
                </template>
              </el-table-column>
              <el-table-column prop="model" label="型号" width="120" />
              <el-table-column prop="version" label="版本" width="100" />
              <el-table-column prop="remote_ip" label="远端 IP" width="140" />
              <el-table-column label="最后心跳" width="110">
                <template #default="{ row }">{{ ago(row.last_seen) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="260">
                <template #default="{ row }">
                  <el-button size="small" @click="openDetail(row)">详情</el-button>
                  <el-button size="small" type="primary" plain @click="openHomepage(row)">主页</el-button>
                  <el-button size="small" @click="openExec(row)">执行</el-button>
                  <el-button size="small" type="danger" plain @click="delDevice(row)">移除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-if="!filteredDevices.length" class="muted" style="margin-top:12px">
              {{ devices.length ? '没有匹配的设备' : '还没有设备。先在「注册令牌」生成一个令牌，然后在路由终端上注册。' }}
            </p>
          </el-tab-pane>

          <el-tab-pane label="注册令牌" name="tok">
            <div class="row" style="margin-bottom:12px">
              <el-input v-model="tokNote" placeholder="备注（哪台设备）" style="flex:1;min-width:200px" />
              <el-input-number v-model="tokTTL" :min="1" :max="10080" style="width:120px" title="有效期（分钟）" />
              <el-button type="primary" @click="createToken">生成注册令牌</el-button>
            </div>
            <p class="muted" style="margin-bottom:12px">令牌只显示一次。设备上执行：<code>mgmtd --cloud-enroll-token &lt;令牌&gt; --cloud-url https://1.1ai.best</code></p>
            <el-table :data="tokens" border stripe style="width:100%">
              <el-table-column prop="note" label="备注">
                <template #default="{ row }">{{ row.note || '—' }}</template>
              </el-table-column>
              <el-table-column label="创建" width="140">
                <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column label="过期" width="140">
                <template #default="{ row }">{{ fmtTime(row.expires_at) }}</template>
              </el-table-column>
              <el-table-column label="状态" width="90">
                <template #default="{ row }">
                  <el-tag v-if="row.used_at" type="info" size="small">已使用</el-tag>
                  <el-tag v-else-if="row.expires_at*1000 < Date.now()" type="warning" size="small">已过期</el-tag>
                  <el-tag v-else type="success" size="small">可用</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="90">
                <template #default="{ row }">
                  <el-button size="small" type="danger" plain @click="delToken(row.id)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <el-tab-pane label="审计" name="aud">
            <div class="row" style="margin-bottom:12px;justify-content:space-between">
              <span class="muted">最近 {{ audit.length }} 条记录</span>
              <el-button size="small" @click="loadAudit">刷新</el-button>
            </div>
            <el-table :data="audit" border stripe style="width:100%">
              <el-table-column label="时间" width="150">
                <template #default="{ row }">{{ fmtTime(row.ts) }}</template>
              </el-table-column>
              <el-table-column prop="actor" label="操作者" width="140" />
              <el-table-column prop="device" label="设备" width="140">
                <template #default="{ row }">{{ row.device || '—' }}</template>
              </el-table-column>
              <el-table-column prop="action" label="动作" width="160" />
              <el-table-column label="结果" width="80">
                <template #default="{ row }">
                  <el-tag :type="row.result==='ok'?'success':(row.result==='deny'?'danger':'warning')" size="small">{{ row.result }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="detail" label="详情" />
            </el-table>
          </el-tab-pane>

          <el-tab-pane label="告警" name="alert">
            <el-table v-if="alerts.length" :data="alerts" border stripe style="width:100%">
              <el-table-column label="时间" width="150">
                <template #default="{ row }">{{ fmtTime(row.ts) }}</template>
              </el-table-column>
              <el-table-column prop="device" label="设备" width="140" />
              <el-table-column prop="level" label="级别" width="90">
                <template #default="{ row }">
                  <el-tag :type="row.level==='critical'?'danger':(row.level==='warning'?'warning':'info')" size="small">{{ row.level }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="message" label="消息" />
            </el-table>
            <el-empty v-else description="暂无告警" :image-size="80" />
          </el-tab-pane>

          <el-tab-pane label="能力边界" name="pol">
            <el-card shadow="never">
              <template #header>允许读取（GET）</template>
              <pre class="pol-pre">{{ (overview.policy?.reads || []).join('\n') }}</pre>
            </el-card>
            <el-card shadow="never" style="margin-top:12px">
              <template #header>允许写入（需二次确认）</template>
              <pre class="pol-pre">{{ (overview.policy?.writes || []).join('\n') }}</pre>
              <p class="muted" style="margin-top:10px">白名单在设备侧与云端各校验一次；不在表内的请求会被拒绝并写审计。</p>
            </el-card>
          </el-tab-pane>
        </el-tabs>

        <!-- 执行面板 -->
        <el-card v-if="execDev" shadow="never" style="margin-top:16px">
          <template #header>
            <span>执行 · {{ execDev.name }}{{ execDev.online ? '' : '（离线）' }}</span>
            <el-button size="small" style="float:right" @click="execDev = null">关闭</el-button>
          </template>
          <div class="row" style="margin-bottom:10px">
            <el-select v-model="execPath" style="flex:1;min-width:320px" filterable>
              <el-option v-for="p in execPaths" :key="p" :label="p" :value="p" />
            </el-select>
            <el-button type="primary" :loading="execLoading" @click="runExec">执行</el-button>
          </div>
          <el-input v-if="execIsWrite" v-model="execConfirm" placeholder="写操作需输入设备名确认" style="margin-bottom:10px" />
          <pre class="exec-out">{{ execOut }}</pre>
        </el-card>
      </main>

      <!-- 设备详情抽屉 -->
      <el-drawer v-model="detailVisible" title="设备详情" size="600px" :with-header="true">
        <div v-if="detailDev" class="detail-content">
          <el-descriptions :column="2" border size="small" style="margin-bottom:16px">
            <el-descriptions-item label="设备名">{{ detailDev.name }}</el-descriptions-item>
            <el-descriptions-item label="ID">{{ detailDev.id }}</el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag :type="detailDev.online ? 'success' : 'info'" size="small">{{ detailDev.online ? '在线' : '离线' }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="型号">{{ detailDev.model || '—' }}</el-descriptions-item>
            <el-descriptions-item label="版本">{{ detailDev.version || '—' }}</el-descriptions-item>
            <el-descriptions-item label="远端 IP">{{ detailDev.remote_ip || '—' }}</el-descriptions-item>
            <el-descriptions-item label="最后心跳">{{ ago(detailDev.last_seen) }}</el-descriptions-item>
          </el-descriptions>

          <el-card shadow="never" style="margin-bottom:12px" v-loading="detailLoading">
            <template #header>系统信息</template>
            <el-descriptions :column="2" border size="small" v-if="detailInfo">
              <el-descriptions-item label="主机名">{{ detailInfo.hostname || '—' }}</el-descriptions-item>
              <el-descriptions-item label="CPU">{{ detailInfo.cpu_model || '—' }}</el-descriptions-item>
              <el-descriptions-item label="CPU 核数">{{ detailInfo.cpu_cores || '—' }}</el-descriptions-item>
              <el-descriptions-item label="内存">{{ Math.round((detailInfo.mem_total_kb||0)/1024) }} MB (可用 {{ Math.round((detailInfo.mem_avail_kb||0)/1024) }} MB)</el-descriptions-item>
              <el-descriptions-item label="OS">{{ detailInfo.os_version || '—' }}</el-descriptions-item>
              <el-descriptions-item label="Go">{{ detailInfo.go_version || '—' }}</el-descriptions-item>
            </el-descriptions>
            <el-empty v-else description="加载中..." :image-size="40" />
          </el-card>

          <el-card shadow="never" style="margin-bottom:12px" v-loading="detailLoading">
            <template #header>实时状态</template>
            <el-row :gutter="12" v-if="detailStats">
              <el-col :span="8">
                <div class="detail-stat"><span class="k">CPU</span><span class="v">{{ (detailStats.cpu_percent||0).toFixed(1) }}%</span></div>
              </el-col>
              <el-col :span="8">
                <div class="detail-stat"><span class="k">内存</span><span class="v">{{ detailStats.mem_used_mb || 0 }}/{{ detailStats.mem_total_mb || 0 }} MB</span></div>
              </el-col>
              <el-col :span="8">
                <div class="detail-stat"><span class="k">负载</span><span class="v">{{ (detailStats.load1||0).toFixed(2) }}</span></div>
              </el-col>
              <el-col :span="8">
                <div class="detail-stat"><span class="k">连接数</span><span class="v">{{ detailStats.conntrack_count || 0 }}</span></div>
              </el-col>
              <el-col :span="8">
                <div class="detail-stat"><span class="k">↓ RX</span><span class="v">{{ formatBytes(detailStats.rx_bytes||0) }}</span></div>
              </el-col>
              <el-col :span="8">
                <div class="detail-stat"><span class="k">↑ TX</span><span class="v">{{ formatBytes(detailStats.tx_bytes||0) }}</span></div>
              </el-col>
            </el-row>
            <el-empty v-else description="加载中..." :image-size="40" />
          </el-card>

          <el-card shadow="never" style="margin-bottom:12px" v-loading="detailLoading">
            <template #header>接口</template>
            <el-table v-if="detailIfaces.length" :data="detailIfaces" border size="small" style="width:100%">
              <el-table-column prop="name" label="接口" width="120" />
              <el-table-column prop="ipaddr" label="IP" width="160" />
              <el-table-column label="RX" width="100">
                <template #default="{ row }">{{ formatBytes(row.rx_bytes) }}</template>
              </el-table-column>
              <el-table-column label="TX" width="100">
                <template #default="{ row }">{{ formatBytes(row.tx_bytes) }}</template>
              </el-table-column>
            </el-table>
            <el-empty v-else description="无接口数据" :image-size="40" />
          </el-card>

          <el-card shadow="never" style="margin-bottom:12px" v-loading="detailLoading">
            <template #header>WAN 状态</template>
            <el-table v-if="detailWan.length" :data="detailWan" border size="small" style="width:100%">
              <el-table-column prop="name" label="名称" width="100" />
              <el-table-column prop="interface" label="接口" width="90" />
              <el-table-column label="状态" width="80">
                <template #default="{ row }">
                  <el-tag :type="row.alive ? 'success' : 'danger'" size="small">{{ row.alive ? '在线' : '离线' }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="延迟" width="80">
                <template #default="{ row }">{{ row.alive ? row.latency_ms + 'ms' : '—' }}</template>
              </el-table-column>
            </el-table>
            <el-empty v-else description="无 WAN 数据" :image-size="40" />
          </el-card>

          <div class="row" style="margin-top:16px">
            <el-button type="primary" @click="openHomepage(detailDev)">打开主页</el-button>
            <el-button type="primary" @click="execFromDetail">远程执行</el-button>
            <el-button type="warning" @click="rebootDevice(detailDev)" :loading="rebootLoading">重启设备</el-button>
        <el-button type="primary" @click="upgradeDevice(detailDev)" :loading="upgradeLoading">在线升级</el-button>
          </div>
        </div>
      </el-drawer>
    <!-- 修改密码对话框 -->
    <el-dialog v-model="showChangePw" title="修改密码" width="400px">
      <el-form @submit.prevent="doChangePw">
        <el-form-item label="当前密码">
          <el-input v-model="cpOld" type="password" show-password />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="cpNew" type="password" show-password />
        </el-form-item>
        <el-form-item label="确认新密码">
          <el-input v-model="cpConfirm" type="password" show-password @keyup.enter="doChangePw" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showChangePw=false">取消</el-button>
        <el-button type="primary" :loading="cpLoading" @click="doChangePw">确认修改</el-button>
      </template>
    </el-dialog>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';

const token = ref<string>(localStorage.getItem('rr_cloud_token') || '');
const showChangePw = ref(false);
const cpOld = ref('');
const cpNew = ref('');
const cpConfirm = ref('');
const cpLoading = ref(false);
async function doChangePw() {
  if (!cpOld.value || !cpNew.value) { ElMessage.warning('请填写完整'); return; }
  if (cpNew.value !== cpConfirm.value) { ElMessage.error('两次新密码不一致'); return; }
  if (cpNew.value.length < 4) { ElMessage.error('新密码至少 4 位'); return; }
  cpLoading.value = true;
  try {
    await apiCall('/cloud/v1/change-password', { method:'POST', body: JSON.stringify({old_password: cpOld.value, new_password: cpNew.value}) });
    ElMessage.success('密码修改成功，请重新登录');
    showChangePw.value = false;
    cpOld.value = ''; cpNew.value = ''; cpConfirm.value = '';
    logout();
  } catch (e: any) {
    ElMessage.error(e?.message || '修改失败');
  } finally {
    cpLoading.value = false;
  }
}
const username = ref('admin');
const password = ref('');
const logging = ref(false);
const activeTab = ref('overview');
const devices = ref<any[]>([]);
const tokens = ref<any[]>([]);
const audit = ref<any[]>([]);
const overview = ref<any>({});
const alerts = ref<any[]>([]);
const searchQuery = ref('');
const statusFilter = ref('');
const selectedDevices = ref<any[]>([]);
const refreshing = ref(false);
const rebootLoading = ref(false);
const upgradeLoading = ref(false);
const batchLoading = ref(false);

// exec
const execDev = ref<any>(null);
const execPath = ref('/api/v1/system/info');
const execPaths = [
  '/api/v1/health', '/api/v1/system/info', '/api/v1/system/services', '/api/v1/system/disks',
  '/api/v1/system/processes', '/api/v1/system/time',
  '/api/v1/wan/status', '/api/v1/wan/links', '/api/v1/interfaces', '/api/v1/interfaces/config',
  '/api/v1/dns/config', '/api/v1/qos/limits', '/api/v1/conntrack', '/api/v1/dhcp/leases',
  '/api/v1/stats/current', '/api/v1/stats/interfaces', '/api/v1/clients',
  '/api/v1/threat/list', '/api/v1/threat/stats', '/api/v1/l7/rules', '/api/v1/l7/stats',
  '/api/v1/dpi/apps', '/api/v1/dpi/flow-stats', '/api/v1/dpi/tracker-stats',
  '/api/v1/alerts', '/api/v1/nat/rules', '/api/v1/route/table',
  '/api/v1/wireless/clients', '/api/v1/behavior/stats', '/api/v1/appid/stats',
  '/api/v1/firewall/rules', '/api/v1/tailscale/status', '/api/v1/easytier/status',
  'POST /api/v1/system/reboot', 'POST /api/v1/system/services/restart', 'POST /api/v1/dns/cache/clear',
];
const execConfirm = ref('');
const execOut = ref('选择一项后点「执行」。');
const execLoading = ref(false);

// detail drawer
const detailVisible = ref(false);
const detailDev = ref<any>(null);
const detailInfo = ref<any>(null);
const detailStats = ref<any>(null);
const detailIfaces = ref<any[]>([]);
const detailWan = ref<any[]>([]);
const detailLoading = ref(false);

// tokens
const tokNote = ref('');
const tokTTL = ref(60);

const onlineCount = computed(() => devices.value.filter(d => d.online).length);
const alertCount = computed(() => alerts.value.length);

const filteredDevices = computed(() => {
  let list = devices.value;
  if (statusFilter.value === 'online') list = list.filter(d => d.online);
  else if (statusFilter.value === 'offline') list = list.filter(d => !d.online);
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase();
    list = list.filter(d => (d.name || '').toLowerCase().includes(q) || (d.remote_ip || '').includes(q) || (d.id || '').includes(q));
  }
  return list;
});

const execIsWrite = computed(() => /^(POST|PUT|DELETE|PATCH) /.test(execPath.value));

async function apiCall(path: string, opt: any = {}) {
  const headers = Object.assign({'Content-Type':'application/json'}, opt.headers || {});
  if (token.value) headers['Authorization'] = 'Bearer ' + token.value;
  const r = await fetch(path, Object.assign({}, opt, { headers }));
  if (r.status === 401) { token.value = ''; localStorage.removeItem('rr_cloud_token'); throw new Error('会话已失效'); }
  const txt = await r.text();
  let js: any = null; try { js = JSON.parse(txt); } catch {}
  if (!r.ok) throw new Error((js && (js.detail || js.message)) || txt || ('HTTP ' + r.status));
  return js;
}

async function login() {
  logging.value = true;
  try {
    const r = await apiCall('/cloud/v1/login', { method:'POST', body: JSON.stringify({username:username.value, password:password.value}) });
    token.value = r.token;
    localStorage.setItem('rr_cloud_token', token.value);
    await refreshAll();
    ElMessage.success('登录成功');
  } catch (e: any) {
    ElMessage.error('登录失败：' + (e?.message || e));
  } finally { logging.value = false; }
}

function logout() { token.value = ''; localStorage.removeItem('rr_cloud_token'); }

async function refreshAll() {
  refreshing.value = true;
  try {
    overview.value = await apiCall('/cloud/v1/overview');
    const d = await apiCall('/cloud/v1/devices');
    devices.value = d.devices || [];
    const t = await apiCall('/cloud/v1/tokens');
    tokens.value = t.tokens || [];
    await loadAudit();
    await loadAlerts();
  } catch (e: any) { ElMessage.error(e?.message || '刷新失败'); }
  finally { refreshing.value = false; }
}

async function loadAudit() {
  try {
    const a = await apiCall('/cloud/v1/audit?limit=100');
    audit.value = a.entries || [];
  } catch { /* ignore */ }
}

async function loadAlerts() {
  const newAlerts: any[] = [];
  for (const d of devices.value) {
    if (!d.online) {
      newAlerts.push({ ts: d.last_seen || Date.now()/1000, device: d.name, level: 'warning', message: '设备离线' });
    }
  }
  for (const e of audit.value) {
    if (e.result === 'deny') {
      newAlerts.push({ ts: e.ts, device: e.device || '—', level: 'critical', message: e.detail || e.action });
    }
  }
  alerts.value = newAlerts.sort((a, b) => b.ts - a.ts).slice(0, 50);
}

function onSelectionChange(rows: any[]) { selectedDevices.value = rows; }

function openHomepage(row: any) {
  if (!row.online) {
    ElMessage.warning('设备离线，无法打开主页');
    return;
  }
  const token = localStorage.getItem('rr_cloud_token') || '';
  const url = '/cloud/v1/devices/' + row.id + '/ui/?token=' + encodeURIComponent(token);
  window.open(url, '_blank');
}

function openExec(row: any) { execDev.value = row; execConfirm.value = ''; execOut.value = '选择一项后点「执行」。'; }

async function runExec() {
  if (!execDev.value) return;
  execLoading.value = true;
  execOut.value = '执行中…';
  try {
    const parts = execPath.value.split(' ');
    const method = parts.length > 1 ? parts[0] : 'GET';
    const path = parts.length > 1 ? parts[1] : parts[0];
    const body: any = { method, path };
    if (execIsWrite.value && execConfirm.value) body.confirm = execConfirm.value;
    const r = await apiCall('/cloud/v1/devices/' + execDev.value.id + '/exec', { method:'POST', body: JSON.stringify(body) });
    let out = r.body ? atob(r.body) : '';
    try { out = JSON.stringify(JSON.parse(out), null, 2); } catch {}
    execOut.value = 'HTTP ' + r.status + (r.error ? ' · ' + r.error : '') + '\n\n' + out;
  } catch (e: any) {
    execOut.value = '失败：' + (e?.message || e);
  } finally { execLoading.value = false; }
}

async function delDevice(row: any) {
  if (!confirm('确认移除设备「' + row.name + '」？')) return;
  try { await apiCall('/cloud/v1/devices/' + row.id, { method:'DELETE' }); ElMessage.success('已移除'); refreshAll(); }
  catch (e: any) { ElMessage.error(e?.message || '失败'); }
}

async function createToken() {
  const ttl = tokTTL.value || 60;
  try {
    const r = await apiCall('/cloud/v1/tokens', { method:'POST', body: JSON.stringify({note: tokNote.value, ttl_minutes: ttl}) });
    ElMessage.success('令牌已生成，请立即复制');
    execOut.value = r.token + '\n\n设备上执行：\nmgmtd --cloud-enroll-token ' + r.token + ' --cloud-url https://1.1ai.best';
    execDev.value = null;
    refreshAll();
  } catch (e: any) { ElMessage.error(e?.message || '失败'); }
}

async function delToken(id: string) {
  if (!confirm('删除这个令牌？')) return;
  try { await apiCall('/cloud/v1/tokens/' + id, { method:'DELETE' }); refreshAll(); }
  catch (e: any) { ElMessage.error(e?.message || '失败'); }
}

// 设备详情
async function openDetail(row: any) {
  detailDev.value = row;
  detailVisible.value = true;
  detailInfo.value = null;
  detailStats.value = null;
  detailIfaces.value = [];
  detailWan.value = [];
  if (!row.online) return;
  detailLoading.value = true;
  try {
    const [info, stats, ifs, wan] = await Promise.all([
      apiCall('/cloud/v1/devices/' + row.id + '/exec', { method:'POST', body: JSON.stringify({method:'GET', path:'/api/v1/system/info'}) }),
      apiCall('/cloud/v1/devices/' + row.id + '/exec', { method:'POST', body: JSON.stringify({method:'GET', path:'/api/v1/stats/current'}) }),
      apiCall('/cloud/v1/devices/' + row.id + '/exec', { method:'POST', body: JSON.stringify({method:'GET', path:'/api/v1/interfaces'}) }),
      apiCall('/cloud/v1/devices/' + row.id + '/exec', { method:'POST', body: JSON.stringify({method:'GET', path:'/api/v1/wan/status'}) }),
    ]);
    detailInfo.value = info.body ? JSON.parse(atob(info.body)) : null;
    detailStats.value = stats.body ? JSON.parse(atob(stats.body)) : null;
    const ifsData = ifs.body ? JSON.parse(atob(ifs.body)) : null;
    detailIfaces.value = ifsData?.interfaces || [];
    const wanData = wan.body ? JSON.parse(atob(wan.body)) : null;
    detailWan.value = wanData?.status || [];
  } catch (e: any) {
    ElMessage.error('加载详情失败：' + (e?.message || e));
  } finally { detailLoading.value = false; }
}

function execFromDetail() {
  if (detailDev.value) { openExec(detailDev.value); detailVisible.value = false; }
}

async function rebootDevice(dev: any) {
  if (!dev) return;
  try {
    await ElMessageBox.confirm('确认重启设备「' + dev.name + '」？', '警告', { type: 'warning' });
  } catch { return; }
  rebootLoading.value = true;
  try {
    await apiCall('/cloud/v1/devices/' + dev.id + '/exec', { method:'POST', body: JSON.stringify({method:'POST', path:'/api/v1/system/reboot', confirm: dev.name}) });
    ElMessage.success('重启指令已发送');
  } catch (e: any) { ElMessage.error(e?.message || '失败'); }
  finally { rebootLoading.value = false; }
}

async function upgradeDevice(dev: any) {
  if (!dev) return;
  // 版本信息由云端解析：前端不拼下载地址、不自己算校验和，
  // 否则一个改过的页面就能把任意 URL 塞给设备的升级接口。
  let rel: any = null;
  try {
    const r = await apiCall('/cloud/v1/releases/latest');
    rel = r?.release || null;
  } catch { /* 拿不到版本也允许升级，只是提示少一点 */ }
  const desc = rel
    ? `将升级到 ${rel.version}（${(rel.size/1048576).toFixed(1)} MB）并重启服务。`
    : '将从云端下载最新版本并重启服务。';
  try {
    await ElMessageBox.confirm(
      '确认在线升级设备「' + dev.name + '」？\n' + desc + '\n升级期间设备会短暂断连。',
      '在线升级', { type: 'warning', confirmButtonText: '开始升级', cancelButtonText: '取消' }
    );
  } catch { return; }
  upgradeLoading.value = true;
  try {
    const r = await apiCall('/cloud/v1/devices/' + dev.id + '/upgrade', {
      method: 'POST',
      body: JSON.stringify({ confirm: dev.name }),
    });
    ElMessage.success('已下发 ' + (r?.version || '新版本') + '，设备正在重启');
  } catch (e: any) { ElMessage.error(e?.message || '升级失败'); }
  finally { upgradeLoading.value = false; }
}

async function batchReboot() {
  try {
    await ElMessageBox.confirm('确认重启 ' + selectedDevices.value.length + ' 台设备？', '批量重启', { type: 'warning' });
  } catch { return; }
  batchLoading.value = true;
  let ok = 0, fail = 0;
  for (const dev of selectedDevices.value) {
    try {
      await apiCall('/cloud/v1/devices/' + dev.id + '/exec', { method:'POST', body: JSON.stringify({method:'POST', path:'/api/v1/system/reboot', confirm: dev.name}) });
      ok++;
    } catch { fail++; }
  }
  batchLoading.value = false;
  ElMessage.success('重启完成：成功 ' + ok + '，失败 ' + fail);
  selectedDevices.value = [];
}

function fmtTime(ts: number) { if (!ts) return '—'; return new Date(ts * 1000).toLocaleString('zh-CN', { hour12:false }); }
function ago(ts: number) {
  if (!ts) return '从未';
  const s = Math.floor(Date.now()/1000) - ts;
  if (s < 60) return s + ' 秒前';
  if (s < 3600) return Math.floor(s/60) + ' 分钟前';
  if (s < 86400) return Math.floor(s/3600) + ' 小时前';
  return Math.floor(s/86400) + ' 天前';
}
function dur(sec: number) {
  if (!sec) return '—';
  const d = Math.floor(sec/86400), h = Math.floor(sec%86400/3600), m = Math.floor(sec%3600/60);
  return (d ? d + '天 ' : '') + (h ? h + '小时 ' : '') + m + '分';
}
function deviceType(row: any) {
  const model = (row.model || '').toLowerCase();
  const name = (row.name || '').toLowerCase();
  if (model.includes('ap') || name.includes('ap') || model.includes('access point')) {
    return { label: '无线 AP', icon: 'Wifi', color: '#409eff' };
  }
  if (model.includes('switch') || name.includes('switch') || model.includes('sw')) {
    return { label: '交换机', icon: 'Connection', color: '#67c23a' };
  }
  if (model.includes('router') || name.includes('router') || model.includes('ikuai') || model.includes('openwrt')) {
    return { label: '路由器', icon: 'Monitor', color: '#e6a23c' };
  }
  return { label: '网络设备', icon: 'Platform', color: '#909399' };
}
function formatBytes(b: number): string {
  if (!b) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0; let v = b;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return v.toFixed(1) + ' ' + units[i];
}

let refreshTimer: any = null;
onMounted(() => {
  if (token.value) refreshAll();
  refreshTimer = setInterval(() => { if (token.value) refreshAll(); }, 15000);
});
onUnmounted(() => { if (refreshTimer) clearInterval(refreshTimer); });
</script>

<style scoped>
.cloud-console { font-family: -apple-system, "Segoe UI", Roboto, "Noto Sans SC", sans-serif; }
.login-wrap { display:flex; align-items:center; justify-content:center; min-height:100vh; background: var(--el-bg-color-page); }
.login-card { width: 380px; }
.login-title { font-weight: 600; }
.muted { color: var(--el-text-color-secondary); font-size: 12px; }
.small { font-size: 11px; }
.cards { display:grid; grid-template-columns:repeat(auto-fit,minmax(140px,1fr)); gap:14px; margin-bottom:20px; }
.card { cursor:pointer; transition: transform .15s; }
.card:hover { transform: translateY(-2px); }
.card .k { color: var(--el-text-color-secondary); font-size: 12px; }
.card .v { font-size: 24px; font-weight: 600; margin-top: 4px; }
.row { display:flex; gap:10px; flex-wrap:wrap; align-items:center; }
.dot { display:inline-block; width:8px; height:8px; border-radius:50%; margin-right:6px; vertical-align:middle; }
.dot.on { background: var(--el-color-success); box-shadow: 0 0 6px rgba(103,194,58,.7); }
.dot.off { background: var(--el-color-info); }
.cc-header { display:flex; align-items:center; gap:8px; padding:14px 20px; background: var(--el-bg-color); border-bottom:1px solid var(--el-border-color); position:sticky; top:0; z-index:10; flex-wrap:wrap; }
.cc-title { font-size:15px; font-weight:600; }
.cc-main { padding:20px; max-width:1400px; margin:0 auto; }
.sp { flex:1; }
.pol-pre { background: var(--el-fill-color-lighter); border:1px solid var(--el-border-color); border-radius:8px; padding:12px; overflow:auto; max-height:420px; font-size:12px; margin:0; }
.exec-out { background: var(--el-fill-color-lighter); border:1px solid var(--el-border-color); border-radius:8px; padding:12px; overflow:auto; max-height:420px; font-size:12px; margin:0; }
.detail-content { padding: 0 4px; }
.detail-stat { display:flex; flex-direction:column; gap:2px; padding:8px; background: var(--el-fill-color-lighter); border-radius:6px; margin-bottom:8px; }
.detail-stat .k { color: var(--el-text-color-secondary); font-size:11px; }
.detail-stat .v { font-size:14px; font-weight:600; }
</style>
