<template>
  <div>
    <el-card shadow="never">
      <template #header><span>安全增强</span></template>

      <el-tabs v-model="tab">
        <!-- ===== Tab 1: DDoS 防护 ===== -->
        <el-tab-pane label="DDoS 防护" name="ddos">
          <el-alert
            v-if="ddosSource"
            type="info"
            :closable="false"
            show-icon
            :title="`配置来源：${ddosSource}`"
            style="margin-bottom:12px"
          />

          <el-divider content-position="left">SYN Flood</el-divider>
          <el-form inline size="small" label-width="70px">
            <el-form-item label="PPS"><el-input-number v-model="cfg.syn_flood.pps" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="BPS"><el-input-number v-model="cfg.syn_flood.bps" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="Burst"><el-input-number v-model="cfg.syn_flood.burst" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="动作">
              <el-select v-model="cfg.syn_flood.action" style="width:130px">
                <el-option label="丢弃 drop" value="drop" /><el-option label="记录 log" value="log" /><el-option label="限速 ratelimit" value="ratelimit" />
              </el-select>
            </el-form-item>
          </el-form>

          <el-divider content-position="left">ICMP Flood</el-divider>
          <el-form inline size="small" label-width="70px">
            <el-form-item label="PPS"><el-input-number v-model="cfg.icmp_flood.pps" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="BPS"><el-input-number v-model="cfg.icmp_flood.bps" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="Burst"><el-input-number v-model="cfg.icmp_flood.burst" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="动作">
              <el-select v-model="cfg.icmp_flood.action" style="width:130px">
                <el-option label="丢弃 drop" value="drop" /><el-option label="记录 log" value="log" /><el-option label="限速 ratelimit" value="ratelimit" />
              </el-select>
            </el-form-item>
          </el-form>

          <el-divider content-position="left">UDP Flood</el-divider>
          <el-form inline size="small" label-width="70px">
            <el-form-item label="PPS"><el-input-number v-model="cfg.udp_flood.pps" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="BPS"><el-input-number v-model="cfg.udp_flood.bps" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="Burst"><el-input-number v-model="cfg.udp_flood.burst" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="动作">
              <el-select v-model="cfg.udp_flood.action" style="width:130px">
                <el-option label="丢弃 drop" value="drop" /><el-option label="记录 log" value="log" /><el-option label="限速 ratelimit" value="ratelimit" />
              </el-select>
            </el-form-item>
          </el-form>

          <el-divider content-position="left">ACK Flood</el-divider>
          <el-form inline size="small" label-width="70px">
            <el-form-item label="PPS"><el-input-number v-model="cfg.ack_flood.pps" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="BPS"><el-input-number v-model="cfg.ack_flood.bps" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="Burst"><el-input-number v-model="cfg.ack_flood.burst" :min="0" style="width:120px" /></el-form-item>
            <el-form-item label="动作">
              <el-select v-model="cfg.ack_flood.action" style="width:130px">
                <el-option label="丢弃 drop" value="drop" /><el-option label="记录 log" value="log" /><el-option label="限速 ratelimit" value="ratelimit" />
              </el-select>
            </el-form-item>
          </el-form>

          <el-divider content-position="left">其它</el-divider>
          <el-form inline size="small" label-width="130px">
            <el-form-item label="Ping of Death">
              <el-switch v-model="cfg.ping_of_death" />
            </el-form-item>
            <el-form-item label="端口扫描 PPS">
              <el-input-number v-model="cfg.port_scan_pps" :min="0" style="width:120px" />
            </el-form-item>
            <el-form-item label="端口扫描窗口">
              <el-input-number v-model="cfg.port_scan_window" :min="0" style="width:120px" />
            </el-form-item>
            <el-form-item label="封禁秒数">
              <el-input-number v-model="cfg.block_seconds" :min="0" style="width:120px" />
            </el-form-item>
          </el-form>

          <el-divider content-position="left">白名单</el-divider>
          <div v-for="(w, i) in cfg.whitelist" :key="i" style="display:flex; gap:8px; margin-bottom:6px; max-width:520px">
            <el-input v-model="cfg.whitelist[i]" placeholder="IP / CIDR，如 1.2.3.4 或 10.0.0.0/8" />
            <el-button :disabled="whitelistSaving" @click="removeWhitelist(i)">删除</el-button>
          </div>
          <el-empty v-if="!cfg.whitelist.length" description="暂无白名单条目" :image-size="40" style="padding-top:0" />
          <el-input
            v-model="newWhitelist"
            placeholder="输入 IP / CIDR 后点击添加，如 192.168.1.100"
            style="max-width:400px; margin-top:6px"
            @keyup.enter="addWhitelist"
          >
            <template #append><el-button :disabled="whitelistSaving" @click="addWhitelist">添加</el-button></template>
          </el-input>

          <!-- 后端拒绝 0.0.0.0/0 的 detail 会显示在这里 -->
          <el-alert
            v-if="ddosDetail"
            type="error"
            :closable="false"
            show-icon
            :title="ddosDetail"
            style="margin-top:12px"
          />

          <div style="margin-top:14px; display:flex; gap:10px">
            <el-button type="primary" :loading="ddosPreviewing" @click="previewDDoS">预览规则</el-button>
            <el-button type="primary" plain :loading="ddosApplying" @click="applyDDoS">应用</el-button>
          </div>

          <pre v-if="ddosNft" class="tool-pre">{{ ddosNft }}</pre>
          <el-alert
            v-if="ddosAppResp"
            :type="ddosAppResp.applied ? 'success' : 'danger'"
            :closable="false"
            show-icon
            :title="ddosAppResp.applied ? '已应用' : '应用失败'"
            style="margin-top:8px"
          />
          <pre v-if="ddosAppResp && ddosAppResp.output" class="tool-pre">{{ ddosAppResp.output }}</pre>
        </el-tab-pane>

        <!-- ===== Tab 2: 时间段策略 ===== -->
        <el-tab-pane label="时间段策略" name="timesched">
          <el-descriptions v-if="tsHeader" :column="3" border size="small" style="margin-bottom:12px">
            <el-descriptions-item label="当前时间">{{ tsHeader.now }}</el-descriptions-item>
            <el-descriptions-item label="时区">{{ tsHeader.timezone }}</el-descriptions-item>
            <el-descriptions-item label="规则数">{{ tsHeader.count }}</el-descriptions-item>
          </el-descriptions>

          <div style="margin-bottom:10px">
            <el-button type="primary" size="small" @click="openNewRule">新增规则</el-button>
            <el-button size="small" :loading="tsLoading" @click="loadTimeSchedule" style="margin-left:8px">刷新</el-button>
          </div>

          <el-table :data="tsRules" size="small" border style="width:100%" empty-text="暂无时间段规则">
            <el-table-column prop="name" label="名称" min-width="120" />
            <el-table-column label="星期" min-width="120">
              <template #default="{ row }">{{ fmtDays(row.days) }}</template>
            </el-table-column>
            <el-table-column label="时间段" width="120">
              <template #default="{ row }">{{ fmtMin(row.start_min) }} - {{ fmtMin(row.end_min) }}</template>
            </el-table-column>
            <el-table-column label="目标" min-width="120">
              <template #default="{ row }">{{ (row.targets || []).join(', ') || '全部' }}</template>
            </el-table-column>
            <el-table-column label="动作" width="100">
              <template #default="{ row }">{{ actionLabel(row.action) }}</template>
            </el-table-column>
            <el-table-column label="限速(kbps)" width="100">
              <template #default="{ row }">{{ row.rate_kbps || '—' }}</template>
            </el-table-column>
            <el-table-column label="启用" width="70">
              <template #default="{ row }">{{ row.enabled ? '是' : '否' }}</template>
            </el-table-column>
            <el-table-column label="当前生效" width="100">
              <template #default="{ row }">
                <el-tag :type="row.active_now ? 'success' : 'info'" size="small">{{ row.active_now ? '生效' : '未生效' }}</el-tag>
              </template>
            </el-table-column>
          </el-table>

          <!-- 后端拒绝跨零点的 detail 显示在这里 -->
          <el-alert v-if="tsDetail" type="error" :closable="false" show-icon :title="tsDetail" style="margin-top:12px" />

          <div v-if="tsNft" style="margin-top:12px">
            <div style="display:flex; gap:10px; margin-bottom:8px">
              <el-button type="primary" size="small" :loading="tsPreviewing" @click="previewTimeSchedule">预览规则</el-button>
              <el-button type="primary" plain size="small" :loading="tsApplying" @click="applyTimeSchedule">应用</el-button>
            </div>
            <pre class="tool-pre">{{ tsNft }}</pre>
            <el-alert
              v-if="tsAppResp"
              :type="tsAppResp.applied ? 'success' : 'danger'"
              :closable="false"
              show-icon
              :title="tsAppResp.applied ? '已应用' : '应用失败'"
              style="margin-top:8px"
            />
            <pre v-if="tsAppResp && tsAppResp.output" class="tool-pre">{{ tsAppResp.output }}</pre>
          </div>

          <!-- 新增规则弹窗 -->
          <el-dialog v-model="ruleDialog" title="新增时间段规则" width="560px">
            <el-form label-width="110px" size="small">
              <el-form-item label="规则名">
                <el-input v-model="newRule.name" placeholder="如：工作日限制办公网" />
              </el-form-item>
              <el-form-item label="生效星期">
                <el-checkbox-group v-model="newRule.days">
                  <el-checkbox :value="0">周日</el-checkbox>
                  <el-checkbox :value="1">周一</el-checkbox>
                  <el-checkbox :value="2">周二</el-checkbox>
                  <el-checkbox :value="3">周三</el-checkbox>
                  <el-checkbox :value="4">周四</el-checkbox>
                  <el-checkbox :value="5">周五</el-checkbox>
                  <el-checkbox :value="6">周六</el-checkbox>
                </el-checkbox-group>
              </el-form-item>
              <el-form-item label="开始时刻">
                <el-time-picker v-model="startTime" format="HH:mm" value-format="HH:mm" style="width:130px" />
              </el-form-item>
              <el-form-item label="结束时刻">
                <el-time-picker v-model="endTime" format="HH:mm" value-format="HH:mm" style="width:130px" />
              </el-form-item>
              <el-form-item label="目标">
                <el-input v-model="newRule.targetsText" placeholder="逗号分隔，如 ip:10.0.0.0/8、port:443；留空为全部" />
              </el-form-item>
              <el-form-item label="动作">
                <el-select v-model="newRule.action" style="width:180px">
                  <el-option label="阻断 block" value="block" />
                  <el-option label="限速 ratelimit" value="ratelimit" />
                  <el-option label="仅允许 allow_only" value="allow_only" />
                </el-select>
              </el-form-item>
              <el-form-item label="限速(kbps)">
                <el-input-number v-model="newRule.rate_kbps" :min="0" style="width:150px" />
              </el-form-item>
              <el-form-item label="启用">
                <el-switch v-model="newRule.enabled" />
              </el-form-item>
            </el-form>
            <div v-if="ruleError" style="margin-bottom:8px">
              <el-alert type="error" :closable="false" show-icon :title="ruleError" />
            </div>
            <template #footer>
              <el-button size="small" @click="ruleDialog = false">取消</el-button>
              <el-button size="small" type="primary" @click="saveNewRule">确定</el-button>
            </template>
          </el-dialog>
        </el-tab-pane>

        <!-- ===== Tab 3: Firewall Rules ===== -->
        <el-tab-pane label="防火墙规则" name="firewall">
          <el-alert v-if="fwDetail" type="error" :closable="false" show-icon :title="fwDetail" style="margin-bottom:12px" />
          <el-form inline size="small" label-width="70px" style="margin-bottom:10px">
            <el-form-item>
              <el-button type="primary" size="small" :loading="fwLoading" @click="loadFirewall">加载</el-button>
              <el-button type="primary" size="small" :loading="fwSaving" @click="saveFirewall" style="margin-left:8px">保存</el-button>
            </el-form-item>
          </el-form>
          <el-table :data="fwRules" size="small" border style="width:100%" empty-text="暂无防火墙规则">
            <el-table-column prop="Chain" label="链" width="100" />
            <el-table-column prop="Src" label="源地址" min-width="150">
              <template #default="scope">
                <el-input v-model="scope.row.Src" size="small" placeholder="如 10.0.0.0/8 或留空" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column prop="Dst" label="目的地址" min-width="150">
              <template #default="scope">
                <el-input v-model="scope.row.Dst" size="small" placeholder="如 192.168.1.0/24 或留空" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column prop="Proto" label="协议" width="100">
              <template #default="scope">
                <el-select v-model="scope.row.Proto" size="small" placeholder="任意" style="width:100%">
                  <el-option label="任意" value="" />
                  <el-option label="tcp" value="tcp" />
                  <el-option label="udp" value="udp" />
                  <el-option label="icmp" value="icmp" />
                </el-select>
              </template>
            </el-table-column>
            <el-table-column prop="DPort" label="目的端口" width="120">
              <template #default="scope">
                <el-input v-model="scope.row.DPort" size="small" placeholder="如 80 或 8000-9000" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column prop="Action" label="动作" width="120">
              <template #default="scope">
                <el-select v-model="scope.row.Action" size="small" style="width:100%">
                  <el-option label="接受 accept" value="accept" />
                  <el-option label="丢弃 drop" value="drop" />
                  <el-option label="拒绝 reject" value="reject" />
                </el-select>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="scope">
                <el-button size="small" type="danger" @click="removeFirewallRule(scope.$index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div style="display:flex; gap:8px; margin-top:10px">
            <el-button type="primary" size="small" @click="addFirewallRule">新增规则</el-button>
          </div>
          <div v-if="fwNft" style="margin-top:12px">
            <div style="font-weight:bold; margin-bottom:4px">渲染的 nftables：</div>
            <pre class="tool-pre">{{ fwNft }}</pre>
          </div>
        </el-tab-pane>

        <!-- ===== Tab 4: NAT Rules ===== -->
        <el-tab-pane label="NAT 规则" name="nat">
          <el-alert v-if="natDetail" type="error" :closable="false" show-icon :title="natDetail" style="margin-bottom:12px" />
          <el-form inline size="small" label-width="70px" style="margin-bottom:10px">
            <el-form-item>
              <el-button type="primary" size="small" :loading="natLoading" @click="loadNAT">加载</el-button>
              <el-button type="primary" size="small" :loading="natSaving" @click="saveNAT" style="margin-left:8px">保存</el-button>
            </el-form-item>
          </el-form>
          <el-table :data="natRules" size="small" border style="width:100%" empty-text="暂无 NAT 规则">
            <el-table-column prop="Type" label="类型" width="120">
              <template #default="scope">
                <el-select v-model="scope.row.Type" size="small" style="width:100%">
                  <el-option label="源 NAT (masquerade)" value="masquerade" />
                  <el-option label="端口映射 (portforward)" value="portforward" />
                </el-select>
              </template>
            </el-table-column>
            <el-table-column prop="Interface" label="出接口" width="120">
              <template #default="scope">
                <el-input v-model="scope.row.Interface" size="small" placeholder="如 eth0" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column prop="Proto" label="协议" width="90">
              <template #default="scope">
                <el-select v-model="scope.row.Proto" size="small" placeholder="任意" style="width:100%">
                  <el-option label="任意" value="" />
                  <el-option label="tcp" value="tcp" />
                  <el-option label="udp" value="udp" />
                </el-select>
              </template>
            </el-table-column>
            <el-table-column prop="SrcPort" label="源端口" width="120">
              <template #default="scope">
                <el-input v-model="scope.row.SrcPort" size="small" placeholder="如 80" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column prop="DstAddr" label="目的地址" width="140">
              <template #default="scope">
                <el-input v-model="scope.row.DstAddr" size="small" placeholder="如 192.168.1.10" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column prop="DstPort" label="目的端口" width="120">
              <template #default="scope">
                <el-input v-model="scope.row.DstPort" size="small" placeholder="如 80" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column prop="ToPort" label="映射端口" width="120">
              <template #default="scope">
                <el-input v-model="scope.row.ToPort" size="small" placeholder="如 8080" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="scope">
                <el-button size="small" type="danger" @click="removeNATRule(scope.$index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div style="display:flex; gap:8px; margin-top:10px">
            <el-button type="primary" size="small" @click="addNATRule">新增规则</el-button>
          </div>
          <div v-if="natNft" style="margin-top:12px">
            <div style="font-weight:bold; margin-bottom:4px">渲染的 nftables：</div>
            <pre class="tool-pre">{{ natNft }}</pre>
          </div>
        </el-tab-pane>

        <!-- ===== Tab 5: Webfilter ===== -->
        <el-tab-pane label="Web 过滤" name="webfilter">
          <el-alert v-if="wfDetail" type="error" :closable="false" show-icon :title="wfDetail" style="margin-bottom:12px" />
          <el-form inline size="small" label-width="70px" style="margin-bottom:10px">
            <el-form-item>
              <el-button type="primary" size="small" :loading="wfLoading" @click="loadWebfilter">加载规则</el-button>
              <el-button type="primary" size="small" :loading="wfSaving" @click="saveWebfilter" style="margin-left:8px">保存规则</el-button>
              <el-button type="success" size="small" :loading="wfApplying" @click="applyWebfilter" style="margin-left:8px">应用到系统</el-button>
              <el-button size="small" :loading="wfStatusLoading" @click="loadWebfilterStatus" style="margin-left:8px">应用状态</el-button>
            </el-form-item>
          </el-form>
          <el-table :data="wfRules" size="small" border style="width:100%" empty-text="暂无过滤规则">
            <el-table-column prop="Domain" label="域名 / IP" min-width="220">
              <template #default="scope">
                <el-input v-model="scope.row.Domain" size="small" placeholder="如 example.com 或 1.2.3.4" style="width:100%" />
              </template>
            </el-table-column>
            <el-table-column prop="Action" label="动作" width="120">
              <template #default="scope">
                <el-select v-model="scope.row.Action" size="small" style="width:100%">
                  <el-option label="拦截 block" value="block" />
                  <el-option label="允许 allow" value="allow" />
                </el-select>
              </template>
            </el-table-column>
            <el-table-column prop="Enabled" label="启用" width="80">
              <template #default="scope">
                <el-switch v-model="scope.row.Enabled" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="scope">
                <el-button size="small" type="danger" @click="removeWebfilterRule(scope.$index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div style="display:flex; gap:8px; margin-top:10px">
            <el-button type="primary" size="small" @click="addWebfilterRule">新增规则</el-button>
          </div>
          <div v-if="wfDnsmasq" style="margin-top:12px">
            <div style="font-weight:bold; margin-bottom:4px">dnsmasq 配置预览：</div>
            <pre class="tool-pre">{{ wfDnsmasq }}</pre>
          </div>
          <div v-if="wfApplyStatus" style="margin-top:12px">
            <div style="font-weight:bold; margin-bottom:4px">最近应用状态：</div>
            <pre class="tool-pre">{{ wfApplyStatus }}</pre>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue';
import { ElMessage } from 'element-plus';
import api from '../api';

function showError(e: any, fallback: string) {
  const d = e?.response?.data;
  if (d?.message) {
    ElMessage.error(d.detail ? `${d.message}：${d.detail}` : d.message);
  } else {
    ElMessage.error(fallback);
  }
}

function errDetail(e: any): string {
  const d = e?.response?.data;
  if (d?.detail) return String(d.detail);
  if (d?.message) return String(d.message);
  return '';
}

const tab = ref('timesched');
const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
function fmtDays(days: number[] | undefined): string {
  if (!days || !days.length) return '每天';
  return days.map(d => WEEKDAYS[d] ?? d).join(',');
}
function fmtMin(m: number): string {
  if (m == null) return '—';
  const h = Math.floor(m / 60);
  const mm = m % 60;
  return `${String(h).padStart(2, '0')}:${String(mm).padStart(2, '0')}`;
}
function actionLabel(a: string): string {
  switch (a) {
    case 'block': return '阻断';
    case 'ratelimit': return '限速';
    case 'allow_only': return '仅允许';
    default: return a;
  }
}

// ---- Tab 1: DDoS ----
const ddosSource = ref('');
const ddosDetail = ref('');
const ddosNft = ref('');
const ddosPreviewing = ref(false);
const ddosApplying = ref(false);
const ddosAppResp = ref<any>(null);
const newWhitelist = ref('');
const whitelistSaving = ref(false);

const cfg = reactive({
  syn_flood: { pps: 0, bps: 0, burst: 0, action: 'drop', log_prefix: '' },
  icmp_flood: { pps: 0, bps: 0, burst: 0, action: 'drop', log_prefix: '' },
  udp_flood: { pps: 0, bps: 0, burst: 0, action: 'drop', log_prefix: '' },
  ack_flood: { pps: 0, bps: 0, burst: 0, action: 'drop', log_prefix: '' },
  ping_of_death: false,
  port_scan_pps: 0,
  port_scan_window: 10,
  block_seconds: 600,
  whitelist: [] as string[],
});

async function loadDDoS() {
  try {
    const res = await api.get('/api/v1/security/ddos');
    const d = res.data || {};
    ddosSource.value = d.source || '';
    if (d.config) {
      // 防御：后端历史版本可能把 nil slice 序列化成 null，
      // 直接 Object.assign 会用 null 覆盖掉初始的 []，模板里 .length 立刻崩，
      // 而症状是整个 tab 空白 —— 极难定位。数组字段一律兜底成 []。
      const c = { ...d.config };
      if (!Array.isArray(c.whitelist)) c.whitelist = [];
      Object.assign(cfg, c);
    }
  } catch (e: any) {
    showError(e, '加载 DDoS 配置失败');
  }
}

function addWhitelist() {
  const v = newWhitelist.value.trim();
  if (!v) return;
  cfg.whitelist.push(v);
  newWhitelist.value = '';
}
function removeWhitelist(i: number) {
  cfg.whitelist.splice(i, 1);
}

async function previewDDoS() {
  ddosDetail.value = '';
  ddosNft.value = '';
  ddosAppResp.value = null;
  ddosPreviewing.value = true;
  try {
    const res = await api.post('/api/v1/security/ddos', { config: cfg });
    if (res.data?.nft) ddosNft.value = res.data.nft;
  } catch (e: any) {
    ddosDetail.value = errDetail(e) || '预览失败';
    showError(e, '预览 DDoS 规则失败');
  } finally {
    ddosPreviewing.value = false;
  }
}

async function applyDDoS() {
  ddosDetail.value = '';
  ddosAppResp.value = null;
  ddosApplying.value = true;
  try {
    const res = await api.post('/api/v1/security/ddos', { config: cfg, confirm: true });
    ddosAppResp.value = res.data || {};
    if (res.data?.nft) ddosNft.value = res.data.nft;
  } catch (e: any) {
    ddosDetail.value = errDetail(e) || '应用失败';
    showError(e, '应用 DDoS 配置失败');
  } finally {
    ddosApplying.value = false;
  }
}

// ---- Tab 2: 时间段策略 ----
const tsLoading = ref(false);
const tsHeader = ref<any>(null);
const tsRules = ref<any[]>([]);
const tsDetail = ref('');
const tsNft = ref('');
const tsPreviewing = ref(false);
const tsApplying = ref(false);
const tsAppResp = ref<any>(null);

const ruleDialog = ref(false);
const ruleError = ref('');
const startTime = ref('09:00');
const endTime = ref('18:00');
const newRule = reactive({
  name: '',
  days: [] as number[],
  targetsText: '',
  action: 'block',
  rate_kbps: 0,
  enabled: true,
});

async function loadTimeSchedule() {
  tsLoading.value = true;
  tsDetail.value = '';
  try {
    const res = await api.get('/api/v1/policy/timeschedule');
    const d = res.data || {};
    tsHeader.value = { now: d.now, timezone: d.timezone, count: d.count };
    tsRules.value = d.rules || [];
  } catch (e: any) {
    showError(e, '加载时间段策略失败');
  } finally {
    tsLoading.value = false;
  }
}

function timeToMin(t: string): number {
  const [h, m] = t.split(':').map(Number);
  return (h || 0) * 60 + (m || 0);
}

function openNewRule() {
  ruleError.value = '';
  newRule.name = '';
  newRule.days = [];
  newRule.targetsText = '';
  newRule.action = 'block';
  newRule.rate_kbps = 0;
  newRule.enabled = true;
  startTime.value = '09:00';
  endTime.value = '18:00';
  ruleDialog.value = true;
}

function saveNewRule() {
  ruleError.value = '';
  if (!newRule.name.trim()) { ruleError.value = '规则名不能为空'; return; }
  const sm = timeToMin(startTime.value);
  const em = timeToMin(endTime.value);
  if (em <= sm) { ruleError.value = '结束时刻必须晚于开始时刻（跨零点必须拆成两条规则：如 23:00-24:00 与 00:00-02:00）'; return; }
  const rule: any = {
    name: newRule.name.trim(),
    days: [...newRule.days],
    start_min: sm,
    end_min: em,
    targets: newRule.targetsText.split(',').map(s => s.trim()).filter(Boolean),
    action: newRule.action,
    rate_kbps: newRule.rate_kbps,
    enabled: newRule.enabled,
  };
  tsRules.value.push(rule);
  ruleDialog.value = false;
  // 重新加载 nft 预览
  previewTimeSchedule();
}

async function previewTimeSchedule() {
  tsDetail.value = '';
  tsNft.value = '';
  tsAppResp.value = null;
  tsPreviewing.value = true;
  try {
    const res = await api.post('/api/v1/policy/timeschedule', { rules: tsRules.value });
    if (res.data?.nft) tsNft.value = res.data.nft;
  } catch (e: any) {
    tsDetail.value = errDetail(e) || '预览失败';
    showError(e, '预览时间段规则失败');
  } finally {
    tsPreviewing.value = false;
  }
}

async function applyTimeSchedule() {
  tsDetail.value = '';
  tsAppResp.value = null;
  tsApplying.value = true;
  try {
    const res = await api.post('/api/v1/policy/timeschedule', { rules: tsRules.value, confirm: true });
    tsAppResp.value = res.data || {};
    if (res.data?.nft) tsNft.value = res.data.nft;
  } catch (e: any) {
    tsDetail.value = errDetail(e) || '应用失败';
    showError(e, '应用时间段规则失败');
  } finally {
    tsApplying.value = false;
  }
}

// ---------- Tab 3: Firewall Rules ----------
const fwLoading = ref(false);
const fwSaving = ref(false);
const fwDetail = ref('');
const fwRules = ref<{Chain: string; Src: string; Dst: string; Proto: string; DPort: string; Action: string}[]>([]);
const fwNft = ref('');

async function loadFirewall() {
  fwLoading.value = true;
  fwDetail.value = '';
  fwNft.value = '';
  try {
    const res = await api.get('/api/v1/firewall/rules');
    const d = res.data || {};
    fwRules.value = d.rules || [];
    if (d.nft) fwNft.value = d.nft;
  } catch (e: any) {
    fwDetail.value = errDetail(e) || '加载防火墙规则失败';
    showError(e, '加载防火墙规则失败');
  } finally {
    fwLoading.value = false;
  }
}

function addFirewallRule() {
  fwRules.value.push({ Chain: 'input', Src: '', Dst: '', Proto: '', DPort: '', Action: 'accept' });
}

function removeFirewallRule(index: number) {
  fwRules.value.splice(index, 1);
}

async function saveFirewall() {
  fwSaving.value = true;
  fwDetail.value = '';
  fwNft.value = '';
  try {
    const res = await api.post('/api/v1/firewall/rules', { rules: fwRules.value });
    const d = res.data || {};
    fwRules.value = d.rules || [];
    if (d.nft) fwNft.value = d.nft;
  } catch (e: any) {
    fwDetail.value = errDetail(e) || '保存防火墙规则失败';
    showError(e, '保存防火墙规则失败');
  } finally {
    fwSaving.value = false;
  }
}

// ---------- Tab 4: NAT Rules ----------
const natLoading = ref(false);
const natSaving = ref(false);
const natDetail = ref('');
const natRules = ref<{Type: string; Interface: string; Proto: string; SrcPort: string; DstAddr: string; DstPort: string; ToPort: string}[]>([]);
const natNft = ref('');

async function loadNAT() {
  natLoading.value = true;
  natDetail.value = '';
  natNft.value = '';
  try {
    const res = await api.get('/api/v1/nat/rules');
    const d = res.data || {};
    natRules.value = (d.rules || []).map((r: any) => ({
      Type: r.type ?? r.Type ?? 'masquerade',
      Interface: r.interface ?? r.Interface ?? '',
      Proto: r.proto ?? r.Proto ?? '',
      SrcPort: r.src_port ?? r.SrcPort ?? '',
      DstAddr: r.dst_addr ?? r.DstAddr ?? '',
      DstPort: r.dst_port ?? r.DstPort ?? '',
      ToPort: r.to_port ?? r.ToPort ?? '',
    }));
    if (d.nft) natNft.value = d.nft;
  } catch (e: any) {
    natDetail.value = errDetail(e) || '加载 NAT 规则失败';
    showError(e, '加载 NAT 规则失败');
  } finally {
    natLoading.value = false;
  }
}

function addNATRule() {
  natRules.value.push({ Type: 'masquerade', Interface: '', Proto: '', SrcPort: '', DstAddr: '', DstPort: '', ToPort: '' });
}

function removeNATRule(index: number) {
  natRules.value.splice(index, 1);
}

async function saveNAT() {
  natSaving.value = true;
  natDetail.value = '';
  natNft.value = '';
  try {
    const wire = natRules.value.map((r) => ({
      type: r.Type, interface: r.Interface, proto: r.Proto,
      src_port: r.SrcPort, dst_addr: r.DstAddr, dst_port: r.DstPort, to_port: r.ToPort,
    }));
    const res = await api.post('/api/v1/nat/rules', { rules: wire });
    const d = res.data || {};
    natRules.value = (d.rules || []).map((r: any) => ({
      Type: r.type ?? r.Type ?? 'masquerade',
      Interface: r.interface ?? r.Interface ?? '',
      Proto: r.proto ?? r.Proto ?? '',
      SrcPort: r.src_port ?? r.SrcPort ?? '',
      DstAddr: r.dst_addr ?? r.DstAddr ?? '',
      DstPort: r.dst_port ?? r.DstPort ?? '',
      ToPort: r.to_port ?? r.ToPort ?? '',
    }));
    if (d.nft) natNft.value = d.nft;
  } catch (e: any) {
    natDetail.value = errDetail(e) || '保存 NAT 规则失败';
    showError(e, '保存 NAT 规则失败');
  } finally {
    natSaving.value = false;
  }
}

// ---------- Tab 5: Webfilter ----------
const wfLoading = ref(false);
const wfSaving = ref(false);
const wfApplying = ref(false);
const wfStatusLoading = ref(false);
const wfDetail = ref('');
const wfRules = ref<{Domain: string; Action: string; Enabled: boolean}[]>([]);
const wfDnsmasq = ref('');
const wfApplyStatus = ref<string>('');

async function loadWebfilter() {
  wfLoading.value = true;
  wfDetail.value = '';
  wfDnsmasq.value = '';
  try {
    const res = await api.get('/api/v1/webfilter/rules');
    const d = res.data || {};
    wfRules.value = (d.rules || []).map((r: any) => ({
      Domain: r.domain ?? r.Domain ?? '',
      Action: r.action ?? r.Action ?? 'block',
      Enabled: r.enabled ?? r.Enabled ?? false,
    }));
    if (d.dnsmasq) wfDnsmasq.value = d.dnsmasq;
  } catch (e: any) {
    wfDetail.value = errDetail(e) || '加载 Web 过滤规则失败';
    showError(e, '加载 Web 过滤规则失败');
  } finally {
    wfLoading.value = false;
  }
}

function addWebfilterRule() {
  wfRules.value.push({ Domain: '', Action: 'block', Enabled: true });
}

function removeWebfilterRule(index: number) {
  wfRules.value.splice(index, 1);
}

async function saveWebfilter() {
  wfSaving.value = true;
  wfDetail.value = '';
  wfDnsmasq.value = '';
  try {
    const wire = wfRules.value.map((r) => ({ domain: r.Domain, action: r.Action, enabled: r.Enabled }));
    const res = await api.post('/api/v1/webfilter/rules', { rules: wire });
    const d = res.data || {};
    wfRules.value = (d.rules || []).map((r: any) => ({
      Domain: r.domain ?? r.Domain ?? '',
      Action: r.action ?? r.Action ?? 'block',
      Enabled: r.enabled ?? r.Enabled ?? false,
    }));
    if (d.dnsmasq) wfDnsmasq.value = d.dnsmasq;
    if (d.detail) {
      wfDetail.value = d.detail;
      if (d.applied) ElMessage.success(d.detail);
      else ElMessage.warning(d.detail);
    }
  } catch (e: any) {
    wfDetail.value = errDetail(e) || '保存 Web 过滤规则失败';
    showError(e, '保存 Web 过滤规则失败');
  } finally {
    wfSaving.value = false;
  }
}

async function applyWebfilter() {
  wfApplying.value = true;
  wfDetail.value = '';
  wfApplyStatus.value = '';
  try {
    const wire = wfRules.value.map((r) => ({ domain: r.Domain, action: r.Action, enabled: r.Enabled }));
    const res = await api.post('/api/v1/webfilter/apply', { rules: wire });
    const d = res.data || {};
    if (d.ok) {
      ElMessage.success('应用成功');
      if (d.script) wfApplyStatus.value = d.script;
    } else {
      wfDetail.value = '应用失败';
    }
  } catch (e: any) {
    wfDetail.value = errDetail(e) || '应用 Web 过滤失败';
    showError(e, '应用 Web 过滤失败');
  } finally {
    wfApplying.value = false;
  }
}

async function loadWebfilterStatus() {
  wfStatusLoading.value = true;
  wfApplyStatus.value = '';
  try {
    const res = await api.get('/api/v1/webfilter/apply-status');
    const d = res.data || {};
    const parts: string[] = [];
    parts.push(`ok: ${d.ok}`);
    if (d.error) parts.push(`error: ${d.error}`);
    if (d.script) parts.push(`script:\n${d.script}`);
    if (d.applied_at) parts.push(`applied_at: ${new Date(d.applied_at * 1000).toLocaleString()}`);
    wfApplyStatus.value = parts.join('\n');
  } catch (e: any) {
    wfApplyStatus.value = errDetail(e) || '获取应用状态失败';
    showError(e, '获取应用状态失败');
  } finally {
    wfStatusLoading.value = false;
  }
}

onMounted(() => { loadDDoS(); loadTimeSchedule(); });
</script>

<style scoped>
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
  margin-top: 8px;
}
</style>