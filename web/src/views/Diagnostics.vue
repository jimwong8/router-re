<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px">
          <span>系统诊断</span>
          <div style="display:flex; gap:8px">
            <el-button type="primary" size="small" :loading="loading" @click="loadReport">生成诊断报告</el-button>
            <el-button size="small" :disabled="!report" :loading="exporting" @click="exportText">导出纯文本</el-button>
          </div>
        </div>
      </template>

      <!-- 汇总 -->
      <div v-if="report" style="margin-bottom:12px">
        <el-descriptions :column="3" border size="small">
          <el-descriptions-item label="主机名">{{ report.hostname }}</el-descriptions-item>
          <el-descriptions-item label="生成时间">{{ fmtTime(report.generated_at) }}</el-descriptions-item>
          <el-descriptions-item label="结果">
            <el-tag :type="(report.summary?.failed ?? 0) > 0 ? 'danger' : 'success'">
              共 {{ report.summary?.total ?? 0 }} 个检查项，{{ report.summary?.failed ?? 0 }} 个异常
            </el-tag>
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <!-- 加载中 -->
      <el-skeleton v-if="loading" :rows="6" animated style="margin-top:8px" />

      <!-- 报告分区 -->
      <el-collapse v-else-if="report" v-model="activeNames" style="margin-top:8px">
        <el-collapse-item v-for="(sec, i) in (report.sections ?? [])" :key="sec.name || i" :name="String(i)">
          <template #title>
            <div style="display:flex; align-items:center; gap:8px; flex:1">
              <span>{{ sec.title }} ({{ sec.name }})</span>
              <el-tag :type="sec.ok ? 'success' : 'danger'" size="small">
                {{ sec.ok ? '正常' : `异常（${(sec.errors ?? []).length} 项）` }}
              </el-tag>
            </div>
          </template>

          <!-- 错误列表 -->
          <el-alert
            v-for="(errMsg, ei) in (sec.errors ?? [])"
            :key="`err-${ei}`"
            type="error"
            :title="String(errMsg)"
            :closable="false"
            show-icon
            style="margin-bottom:6px"
          />

          <!-- items：命令原始输出，绝对不按 HTML 渲染，用文本插值 + pre -->
          <pre v-if="(sec.items ?? []).length > 0" class="sec-pre">{{ sec.items.join('\n') }}</pre>
          <div v-else-if="(sec.errors ?? []).length === 0" style="color:#909399; padding:4px 0">（无输出）</div>
        </el-collapse-item>
      </el-collapse>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { ElMessage } from 'element-plus';

interface DiagSection {
  name: string;
  title: string;
  ok: boolean;
  items: string[];
  errors: string[];
}
interface DiagReport {
  hostname: string;
  generated_at: number;
  sections?: DiagSection[];
  summary?: { total: number; failed: number };
}

const loading = ref(false);
const exporting = ref(false);
const report = ref<DiagReport | null>(null);
const activeNames = ref<string[]>([]);

function token(): string {
  return `Bearer ${localStorage.getItem('router_token') || ''}`;
}

function fmtTime(ts: number | undefined): string {
  if (!ts) return '—';
  try {
    return new Date(ts * 1000).toLocaleString();
  } catch {
    return String(ts);
  }
}

function errMsg(e: any): string {
  const data = e?.response?.data;
  if (!data) return e?.message || '请求失败';
  const parts: string[] = [];
  if (data.message) parts.push(data.message);
  if (data.detail) parts.push(String(data.detail));
  return parts.length ? parts.join('：') : data.message || '请求失败';
}

async function loadReport() {
  loading.value = true;
  try {
    const res = await fetch('/api/v1/diagnostics/report', {
      headers: { Authorization: token() },
    });
    if (!res.ok) {
      let body: any = null;
      try { body = await res.json(); } catch { /* 忽略 */ }
      const msg = body?.message || body?.detail || `请求失败（HTTP ${res.status}）`;
      ElMessage.error(msg);
      return;
    }
    report.value = await res.json();
    activeNames.value = ((report.value?.sections ?? []).map((_, i) => String(i)));
  } catch (e: any) {
    ElMessage.error(errMsg(e));
  } finally {
    loading.value = false;
  }
}

async function exportText() {
  if (!report.value) return;
  exporting.value = true;
  try {
    const res = await fetch('/api/v1/diagnostics/report/text', {
      headers: { Authorization: token() },
    });
    if (!res.ok) {
      let body: any = null;
      try { body = await res.json(); } catch { /* text/plain 可能不用 JSON */ }
      const msg = body?.message || body?.detail || `导出失败（HTTP ${res.status}）`;
      ElMessage.error(msg);
      return;
    }
    // 返回的是 text/plain，用 response.text() 而不是 .json()
    const text = await res.text();
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `diagnostics-${report.value.hostname || 'report'}.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    ElMessage.success('已导出纯文本报告');
  } catch (e: any) {
    ElMessage.error(errMsg(e));
  } finally {
    exporting.value = false;
  }
}
</script>

<style scoped>
.sec-pre {
  background: #0d1117;
  color: #c9d1d9;
  padding: 10px 12px;
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.55;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: ui-monospace, SFMono-Regular, "Courier New", monospace;
  margin: 0;
}
</style>