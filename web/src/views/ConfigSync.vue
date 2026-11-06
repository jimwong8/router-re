<template>
  <div style="display:flex; flex-direction:column; gap:16px">
    <!-- 区块一：导出 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px">
          <span>配置导出</span>
          <div style="display:flex; gap:8px">
            <el-button type="primary" size="small" :loading="exporting" @click="exportConfig">导出当前配置</el-button>
            <el-button size="small" :disabled="!currentBundle" @click="downloadJson">下载 JSON</el-button>
          </div>
        </div>
      </template>

      <el-skeleton :rows="2" animated v-if="exporting" />

      <el-descriptions v-else-if="currentBundle" :column="3" border size="small">
        <el-descriptions-item label="版本">{{ currentBundle.version }}</el-descriptions-item>
        <el-descriptions-item label="主机名">{{ currentBundle.hostname }}</el-descriptions-item>
        <el-descriptions-item label="sections 数">{{ sectionCount }}</el-descriptions-item>
        <el-descriptions-item label="checksum" :span="3">
          <span class="mono">{{ checksumShort }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <el-empty v-else description="尚未导出配置" :image-size="60" />
    </el-card>

    <!-- 区块二：导入与差异 -->
    <el-card shadow="never">
      <template #header><span>导入与差异预览</span></template>

      <!-- 输入方式 -->
      <el-radio-group v-model="inputMode" size="small" style="margin-bottom:10px">
        <el-radio-button value="file">选择 JSON 文件</el-radio-button>
        <el-radio-button value="paste">粘贴 JSON</el-radio-button>
      </el-radio-group>

      <div v-if="inputMode === 'file'" style="margin-bottom:10px">
        <el-input v-model="importFileName" placeholder="选择 .json 文件" readonly>
          <template #append>
            <el-button @click="pickFile">浏览</el-button>
          </template>
        </el-input>
        <input ref="fileInput" type="file" accept=".json,application/json" style="display:none" @change="onFileChange" />
      </div>

      <el-input
        v-else
        type="textarea"
        v-model="pasteText"
        :rows="8"
        placeholder="粘贴 Bundle JSON，例：{&quot;version&quot;:1,&quot;sections&quot;:{...},&quot;checksum&quot;:&quot;...&quot;}"
      />

      <div style="margin-top:10px">
        <el-button type="primary" :loading="previewing" :disabled="!hasBundle" @click="previewDiff">预览差异</el-button>
      </div>

      <!-- 预览结果 -->
      <template v-if="preview">
        <div style="margin-top:14px; margin-bottom:8px; display:flex; align-items:center; gap:10px">
          <span style="font-weight:bold">差异预览</span>
          <el-tag :type="preview.checksum_ok ? 'success' : 'danger'" size="small">
            {{ preview.checksum_ok ? '校验通过' : '校验失败' }}
          </el-tag>
          <span style="color:#909399; font-size:12px">将替换 {{ (preview.sections ?? []).length }} 个 section</span>
        </div>

        <!-- dangerous 危险项 -->
        <div v-if="(preview.dangerous ?? []).length">
          <el-alert
            v-for="(d, i) in preview.dangerous"
            :key="i"
            type="error"
            :title="String(d)"
            :closable="false"
            show-icon
            style="margin-bottom:6px"
          />
        </div>

        <!-- changes 表格 -->
        <el-table v-if="(preview.changes ?? []).length" :data="preview.changes" size="small" border style="margin-top:8px">
          <el-table-column label="类型" width="110">
            <template #default="{ row }">
              <el-tag :type="kindType(row.kind)" size="small">{{ kindLabel(row.kind) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="section" label="sections" width="120" show-overflow-tooltip />
          <el-table-column prop="key" label="key" min-width="160" show-overflow-tooltip />
          <el-table-column prop="old" label="old" min-width="140" show-overflow-tooltip />
          <el-table-column prop="new" label="new" min-width="140" show-overflow-tooltip />
        </el-table>
        <el-empty v-else description="无可应用变更" :image-size="50" />
      </template>
    </el-card>

    <!-- 区块三：恢复 -->
    <el-card shadow="never">
      <template #header><span>配置恢复（破坏性）</span></template>
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        title="恢复会覆盖当前配置，网络/接口/防火墙等 section 恢复后若与本机不符可能导致失联。"
        style="margin-bottom:10px"
      />
      <el-checkbox v-model="confirmChecked">我确认恢复会覆盖当前配置</el-checkbox>
      <div style="margin-top:10px">
        <el-button
          type="danger"
          :loading="restoring"
          :disabled="!confirmChecked || !currentBundle"
          @click="doRestore"
        >执行恢复</el-button>
      </div>
      <el-alert v-if="restoreMsg" :title="restoreMsg" :type="restoreOk ? 'success' : 'error'" :closable="false" show-icon style="margin-top:10px" />
    </el-card>

    <!-- 区块四：HA 高可用 -->
    <el-card shadow="never">
      <template #header><span>HA 高可用配置</span></template>

      <el-form label-position="top" size="default">
        <el-form-item label="对端 URL">
          <el-input v-model="ha.peer_url" placeholder="https://peer-router:8443" clearable />
        </el-form-item>
        <el-form-item label="共享密钥（留空不修改）">
          <el-input v-model="ha.secret" type="password" show-password placeholder="不修改则留空" clearable />
        </el-form-item>
        <el-form-item label="同步 sections">
          <el-select
            v-model="ha.sync_keys"
            multiple
            filterable
            allow-create
            default-first-option
            placeholder="选择或输入 section 名"
            style="width:100%"
          >
            <el-option v-for="k in haSectionOptions" :key="k" :label="k" :value="k" />
          </el-select>
        </el-form-item>
      </el-form>

      <div style="display:flex; gap:8px; flex-wrap:wrap; margin-top:8px">
        <el-button type="primary" :loading="haSaving" @click="saveHaConfig">保存配置</el-button>
        <el-button :loading="haSyncing" @click="doHaSync">立即同步</el-button>
        <el-button :loading="haStatusLoading" @click="loadHaStatus">刷新状态</el-button>
      </div>

      <!-- HA 状态 -->
      <template v-if="haStatus">
        <el-descriptions :column="2" border size="small" style="margin-top:14px">
          <el-descriptions-item label="对端 URL">{{ haStatus.peer_url || '—' }}</el-descriptions-item>
          <el-descriptions-item label="最后同步时间">{{ haStatus.last_sync || '—' }}</el-descriptions-item>
          <el-descriptions-item label="最后结果">
            <el-tag :type="haStatus.last_result === 'ok' ? 'success' : 'danger'" size="small">
              {{ haStatus.last_result || '—' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="最后错误" v-if="haStatus.last_error" :span="2">
            <span style="color:#f56c6c">{{ haStatus.last_error }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </template>

      <el-alert
        v-if="haMsg"
        :title="haMsg"
        :type="haMsgOk ? 'success' : 'error'"
        :closable="false"
        show-icon
        style="margin-top:10px"
      />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { ElMessage } from 'element-plus';

interface Bundle {
  version: number;
  created_at?: number;
  hostname?: string;
  sections: Record<string, any>;
  checksum: string;
}

function token(): string {
  return `Bearer ${localStorage.getItem('router_token') || ''}`;
}

const currentBundle = ref<Bundle | null>(null);
const exporting = ref(false);
const previewing = ref(false);
const restoring = ref(false);
const restoreMsg = ref('');
const restoreOk = ref(false);

const inputMode = ref<'file' | 'paste'>('paste');
const fileInput = ref<HTMLInputElement | null>(null);
const importFileName = ref('');
const pasteText = ref('');

const preview = ref<{ sections?: string[]; changes?: any[]; dangerous?: string[]; checksum_ok?: boolean } | null>(null);
const confirmChecked = ref(false);

const sectionCount = computed(() => (currentBundle.value ? Object.keys(currentBundle.value.sections ?? {}).length : 0));
const checksumShort = computed(() => {
  const c = currentBundle.value?.checksum || '';
  return c.length > 16 ? c.slice(0, 16) + '…' : c;
});

function hasBundle(): boolean {
  if (inputMode.value === 'file') return !!currentBundle.value;
  return pasteText.value.trim().length > 0;
}

function parseInput(): Bundle | null {
  if (inputMode.value === 'file') return currentBundle.value;
  try {
    const obj = JSON.parse(pasteText.value);
    if (!obj || typeof obj !== 'object' || !obj.sections) {
      ElMessage.warning('JSON 无效：缺少 sections 字段');
      return null;
    }
    return obj as Bundle;
  } catch (e: any) {
    ElMessage.error(`JSON 解析失败：${e?.message || e}`);
    return null;
  }
}

async function exportConfig() {
  exporting.value = true;
  try {
    const res = await fetch('/api/v1/config/export', { headers: { Authorization: token() } });
    if (!res.ok) {
      const body = await safeJson(res);
      ElMessage.error(bundleErr(body, `导出失败（HTTP ${res.status}）`));
      return;
    }
    currentBundle.value = await res.json();
    ElMessage.success(`已导出，共 ${sectionCount.value} 个 sections`);
  } catch (e: any) {
    ElMessage.error(e?.message || '导出失败');
  } finally {
    exporting.value = false;
  }
}

function downloadJson() {
  if (!currentBundle.value) return;
  const blob = new Blob([JSON.stringify(currentBundle.value, null, 2)], { type: 'application/json;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `router-re-config-${Date.now()}.json`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function pickFile() {
  fileInput.value?.click();
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  importFileName.value = file.name;
  const reader = new FileReader();
  reader.onload = () => {
    try {
      const obj = JSON.parse(String(reader.result));
      if (!obj || typeof obj !== 'object' || !obj.sections) {
        throw new Error('缺少 sections 字段');
      }
      currentBundle.value = obj as Bundle;
      ElMessage.success('文件已加载');
    } catch (err: any) {
      ElMessage.error(`文件 JSON 无效：${err?.message || err}`);
    }
  };
  reader.onerror = () => ElMessage.error('文件读取失败');
  reader.readAsText(file);
}

async function previewDiff() {
  const bundle = parseInput();
  if (!bundle) return;
  previewing.value = true;
  preview.value = null;
  try {
    // 注意：preview 请求 body 是裸 bundle，不是包在字段里
    const res = await fetch('/api/v1/config/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: token() },
      body: JSON.stringify(bundle),
    });
    if (!res.ok) {
      const body = await safeJson(res);
      ElMessage.error(bundleErr(body, `预览失败（HTTP ${res.status}）`));
      return;
    }
    preview.value = await res.json();
  } catch (e: any) {
    ElMessage.error(e?.message || '预览失败');
  } finally {
    previewing.value = false;
  }
}

async function doRestore() {
  if (!currentBundle.value) return;
  restoring.value = true;
  restoreMsg.value = '';
  try {
    // 注意：restore 的 body 是 { confirm, bundle }，与 preview 的裸 bundle 不同
    const res = await fetch('/api/v1/config/restore', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: token() },
      body: JSON.stringify({ confirm: true, bundle: currentBundle.value }),
    });
    if (!res.ok) {
      const body = await safeJson(res);
      restoreMsg.value = bundleErr(body, `恢复失败（HTTP ${res.status}）`);
      restoreOk.value = false;
      return;
    }
    restoreMsg.value = '配置已恢复。';
    restoreOk.value = true;
  } catch (e: any) {
    restoreMsg.value = e?.message || '恢复失败';
    restoreOk.value = false;
  } finally {
    restoring.value = false;
  }
}

async function safeJson(res: Response): Promise<any> {
  try { return await res.json(); } catch { return null; }
}

function bundleErr(body: any, fallback: string): string {
  const parts: string[] = [];
  if (body?.message) parts.push(body.message);
  if (body?.detail) parts.push(String(body.detail));
  return parts.length ? parts.join('：') : fallback;
}

function kindType(kind: string): 'success' | 'danger' | 'warning' | 'info' {
  switch (kind) {
    case 'added': return 'success';
    case 'removed': return 'danger';
    case 'changed': return 'warning';
    default: return 'info';
  }
}
function kindLabel(kind: string): string {
  switch (kind) {
    case 'added': return '新增';
    case 'removed': return '删除';
    case 'changed': return '修改';
    default: return kind;
  }
}

// ── HA 高可用 ──
const ha = ref({ peer_url: '', secret: '', sync_keys: [] as string[] });
const haSaving = ref(false);
const haSyncing = ref(false);
const haStatusLoading = ref(false);
const haStatus = ref<{ peer_url?: string; last_sync?: string; last_result?: string; last_error?: string } | null>(null);
const haMsg = ref('');
const haMsgOk = ref(false);

const haSectionOptions = computed(() => {
  if (currentBundle.value) return Object.keys(currentBundle.value.sections ?? {});
  return ['network', 'firewall', 'qos', 'dns', 'dhcp', 'wan', 'system'];
});

function showError(body: any, fallback: string) {
  const parts: string[] = [];
  if (body?.code) parts.push(`[${body.code}]`);
  if (body?.message) parts.push(body.message);
  if (body?.detail) parts.push(String(body.detail));
  return parts.length ? parts.join(' ') : fallback;
}

async function loadHaConfig() {
  try {
    const res = await fetch('/api/v1/ha/config', { headers: { Authorization: token() } });
    if (res.ok) {
      const d = await res.json();
      ha.value = {
        peer_url: d.peer_url || '',
        secret: d.has_secret ? '••••••' : '',
        sync_keys: d.sync_keys || [],
      };
    }
  } catch { /* silent */ }
}

async function loadHaStatus() {
  haStatusLoading.value = true;
  try {
    const res = await fetch('/api/v1/ha/status', { headers: { Authorization: token() } });
    if (res.ok) {
      haStatus.value = await res.json();
    } else {
      haStatus.value = null;
    }
  } catch {
    haStatus.value = null;
  } finally {
    haStatusLoading.value = false;
  }
}

async function saveHaConfig() {
  haSaving.value = true;
  haMsg.value = '';
  haMsgOk.value = false;
  try {
    const body: Record<string, any> = { peer_url: ha.value.peer_url, sync_keys: ha.value.sync_keys };
    if (ha.value.secret && ha.value.secret !== '••••••') {
      body.secret = ha.value.secret;
    }
    const res = await fetch('/api/v1/ha/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: token() },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const b = await safeJson(res);
      haMsg.value = showError(b, `保存失败（HTTP ${res.status}）`);
      haMsgOk.value = false;
    } else {
      haMsg.value = 'HA 配置已保存';
      haMsgOk.value = true;
      ha.value.secret = '';
      loadHaStatus();
    }
  } catch (e: any) {
    haMsg.value = e?.message || '保存失败';
    haMsgOk.value = false;
  } finally {
    haSaving.value = false;
  }
}

async function doHaSync() {
  haSyncing.value = true;
  haMsg.value = '';
  haMsgOk.value = false;
  try {
    const res = await fetch('/api/v1/ha/sync', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: token() },
      body: JSON.stringify({}),
    });
    if (!res.ok) {
      const b = await safeJson(res);
      haMsg.value = showError(b, `同步失败（HTTP ${res.status}）`);
      haMsgOk.value = false;
    } else {
      haMsg.value = '同步指令已发送';
      haMsgOk.value = true;
      loadHaStatus();
    }
  } catch (e: any) {
    haMsg.value = e?.message || '同步失败';
    haMsgOk.value = false;
  } finally {
    haSyncing.value = false;
  }
}

// 初始化加载
loadHaConfig();
loadHaStatus();
</script>

<style scoped>
.mono {
  font-family: ui-monospace, SFMono-Regular, "Courier New", monospace;
  word-break: break-all;
}
</style>