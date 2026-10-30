<template>
  <el-card shadow="never" style="margin-bottom:16px">
    <template #header>
      <div class="hdr">
        <span>其它协议客户端（PPTP / L2TP / OpenVPN）</span>
        <div class="tools">
          <el-button size="small" :loading="loading" @click="load">刷新</el-button>
          <el-button size="small" type="primary" @click="openNew">新增连接</el-button>
        </div>
      </div>
    </template>

    <el-alert v-if="note" type="info" :closable="false" show-icon :title="note" style="margin-bottom:12px" />

    <el-table :data="clients" size="small" v-loading="loading" empty-text="还没有 PPTP / L2TP / OpenVPN 客户端">
      <el-table-column label="类型" width="100">
        <template #default="{ row }">
          <el-tag size="small" effect="plain">{{ row.kind.toUpperCase() }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="name" label="名称" min-width="130" />
      <el-table-column label="服务器" min-width="200">
        <template #default="{ row }"><span class="mono">{{ row.server || row.remote || '-' }}</span></template>
      </el-table-column>
      <el-table-column prop="username" label="用户名" width="120" />
      <el-table-column label="凭据" width="100">
        <template #default="{ row }">
          <span v-if="row.kind === 'openvpn'" class="muted">配置内含</span>
          <el-tag v-else size="small" :type="row.has_password ? 'success' : 'danger'" effect="plain">
            {{ row.has_password ? '已设置' : '缺失' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="80">
        <template #default="{ row }">
          <el-tag size="small" :type="row.enabled ? 'success' : 'info'" effect="plain">{{ row.enabled ? '开' : '关' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="170" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="showPlan(row)">配置计划</el-button>
          <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="foot-note">{{ splitNote }}</div>
  </el-card>

  <el-dialog v-model="dlg" title="新增 VPN 客户端" width="640px">
    <el-form label-width="110px">
      <el-form-item label="协议">
        <el-radio-group v-model="form.kind">
          <el-radio-button label="pptp">PPTP</el-radio-button>
          <el-radio-button label="l2tp">L2TP</el-radio-button>
          <el-radio-button label="openvpn">OpenVPN</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <template v-if="form.kind !== 'openvpn'">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="hq-vpn" style="width:220px" />
          <span class="hint">字母数字与 . _ -，最长 32</span>
        </el-form-item>
        <el-form-item label="服务器">
          <el-input v-model="form.server" placeholder="vpn.example.com 或 1.2.3.4" style="width:260px" />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="form.username" style="width:220px" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.password" type="password" show-password style="width:220px"
            placeholder="不能含空格/换行" />
        </el-form-item>
        <el-form-item label="MTU">
          <el-input-number v-model="form.mtu" :min="576" :max="1500" :step="4" controls-position="right" />
          <span class="hint">留 0 用系统默认</span>
        </el-form-item>
        <el-form-item label="全局走 VPN">
          <el-switch v-model="form.route_all" />
          <span class="hint">打开会替换默认路由；关闭只建隧道，网段路由需自行指定</span>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.comment" style="width:100%" />
        </el-form-item>
      </template>

      <template v-else>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="留空则用服务器地址生成" style="width:260px" />
        </el-form-item>
        <el-form-item label="配置内容">
          <el-input v-model="form.ovpn" type="textarea" :rows="12" style="width:100%"
            placeholder="粘贴 .ovpn 文本内容" />
          <div class="hint warn">
            含 up / down / plugin / script-security 等会调用外部程序的指令会被直接拒绝——
            导入配置不应该等于允许执行脚本。
          </div>
        </el-form-item>
      </template>

      <el-form-item label="启用">
        <el-switch v-model="form.enabled" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="dlg = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="planDlg" title="配置计划（不会自动执行）" width="720px">
    <el-alert v-if="plan.missing && plan.missing.length" type="warning" :closable="false" show-icon
      :title="'缺少依赖：' + plan.missing.join('、') + ' —— 先装包，否则服务起不来'" style="margin-bottom:12px" />
    <div class="plan-sec">将写入的文件</div>
    <div v-for="f in plan.files" :key="f.path" class="plan-file">
      <div class="plan-path mono">{{ f.path }} <span class="muted">({{ f.mode }})</span></div>
      <pre class="code">{{ f.content }}</pre>
    </div>
    <div class="plan-sec" v-if="plan.commands && plan.commands.length">将执行的命令</div>
    <pre class="code">{{ (plan.commands || []).join('\n') }}</pre>
    <template #footer><el-button @click="planDlg = false">关闭</el-button></template>
  </el-dialog>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../../api';

type Client = {
  kind: string; name: string; server?: string; username?: string;
  enabled: boolean; has_password: boolean; comment?: string; remote?: string; note?: string;
};
type Plan = { files?: { path: string; content: string; mode: string }[]; commands?: string[]; missing?: string[] };

const clients = ref<Client[]>([]);
const note = ref('');
const splitNote = ref('');
const loading = ref(false);
const saving = ref(false);

const dlg = ref(false);
const planDlg = ref(false);
const plan = ref<Plan>({});

const form = reactive({
  kind: 'pptp', name: '', server: '', username: '', password: '',
  mtu: 0, route_all: true, comment: '', ovpn: '', enabled: true,
});

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/vpn/clients');
    clients.value = res.data?.clients || [];
    note.value = res.data?.note || '';
    splitNote.value = res.data?.split_note || '';
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  } finally {
    loading.value = false;
  }
}

function openNew() {
  Object.assign(form, {
    kind: 'pptp', name: '', server: '', username: '', password: '',
    mtu: 0, route_all: true, comment: '', ovpn: '', enabled: true,
  });
  dlg.value = true;
}

async function save() {
  saving.value = true;
  try {
    const body: any = form.kind === 'openvpn'
      ? { kind: 'openvpn', name: form.name, ovpn: form.ovpn, enabled: form.enabled }
      : {
        kind: form.kind, name: form.name, server: form.server, username: form.username,
        password: form.password, mtu: form.mtu, route_all: form.route_all,
        comment: form.comment, enabled: form.enabled,
      };
    const res = await api.post('/api/v1/vpn/clients', body);
    if (res.data?.saved) ElMessage.success('已保存：' + res.data.name);
    dlg.value = false;
    await load();
  } catch (e: any) {
    // 后端会指出具体哪一行被拒（例如 .ovpn 里的 up 指令）
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e?.message || '保存失败');
  } finally {
    saving.value = false;
  }
}

async function showPlan(row: Client) {
  try {
    const res = await api.get('/api/v1/vpn/clients/plan', { params: { name: row.name, kind: row.kind } });
    plan.value = res.data || {};
    planDlg.value = true;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.detail || e?.response?.data?.message || e?.message || '生成计划失败');
  }
}

async function remove(row: Client) {
  try {
    await ElMessageBox.confirm(`删除「${row.name}」？已写入的配置文件不会自动清理。`, '确认删除', { type: 'warning' });
  } catch { return; }
  try {
    await api.delete('/api/v1/vpn/clients', { params: { name: row.name, kind: row.kind } });
    ElMessage.success('已删除');
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '删除失败');
  }
}

onMounted(load);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.hint { color: #8b91a5; font-size: 12px; margin-left: 10px; }
.hint.warn { color: #d29922; display: block; margin: 6px 0 0; }
.foot-note { margin-top: 10px; color: #8b91a5; font-size: 12px; line-height: 1.6; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; }
.muted { color: #6b7280; font-size: 12px; }
.plan-sec { margin: 12px 0 6px; font-size: 13px; color: #cfd3da; }
.plan-file { margin-bottom: 10px; }
.plan-path { font-size: 12px; color: #8b91a5; margin-bottom: 4px; }
.code { margin: 0; padding: 10px 12px; background: #12151c; border: 1px solid #2a2f3a; border-radius: 6px;
  color: #d7dae0; font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px;
  white-space: pre-wrap; word-break: break-all; max-height: 240px; overflow: auto; }
</style>
