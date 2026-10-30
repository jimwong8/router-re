<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>磁盘管理</span>
          <div class="tools">
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
            <el-button size="small" @click="goFiles">文件管理</el-button>
          </div>
        </div>
      </template>

      <el-alert type="warning" :closable="false" show-icon style="margin-bottom:12px"
        title="格式化与改分区表会清空数据且不可恢复。系统盘分区已标记为禁止操作；其余操作都会先给出命令计划再执行。" />

      <div v-for="d in disks" :key="d.name" class="disk">
        <div class="disk-hdr">
          <span class="disk-name">{{ d.path }}</span>
          <el-tag size="small" type="info">{{ fmtSize(d.size_bytes) }}</el-tag>
          <span class="disk-model">{{ d.model || '未知型号' }}</span>
          <el-tag v-if="d.is_system" size="small" type="danger" effect="plain">系统盘</el-tag>
          <el-tag v-if="d.read_only" size="small" type="warning" effect="plain">只读</el-tag>
          <el-tag v-if="d.removable" size="small" effect="plain">可移动</el-tag>
          <span class="spacer" />
          <span class="disk-usage" v-if="d.mounted_count">
            已挂载 {{ d.mounted_count }} 个分区，已用 {{ fmtSize(d.used_bytes) }}，可用 {{ fmtSize(d.avail_bytes) }}
          </span>
          <el-button v-if="!d.is_system" size="small" @click="openPartition(d)">新建分区</el-button>
        </div>

        <el-table :data="d.partitions" size="small" empty-text="没有分区">
          <el-table-column prop="path" label="分区" width="130" />
          <el-table-column label="容量" width="110">
            <template #default="{ row }">{{ fmtSize(row.size_bytes) }}</template>
          </el-table-column>
          <el-table-column label="文件系统" width="110">
            <template #default="{ row }">
              <span v-if="row.fstype">{{ row.fstype }}</span>
              <el-tag v-else size="small" type="info">未识别</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="label" label="卷标" width="120" />
          <el-table-column prop="uuid" label="UUID" min-width="180" show-overflow-tooltip />
          <el-table-column label="挂载点" min-width="200">
            <template #default="{ row }">
              <span v-if="row.is_mounted" class="mono">{{ row.mount_point }}</span>
              <span v-else class="muted">未挂载</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="230" fixed="right">
            <template #default="{ row }">
              <el-button v-if="!row.is_mounted && row.fstype" link type="primary" size="small"
                @click="mount(row)">挂载</el-button>
              <el-button v-if="row.is_mounted" link type="warning" size="small"
                @click="umount(row)">卸载</el-button>
              <el-button link type="danger" size="small" :disabled="row.is_system || row.is_mounted"
                @click="openFormat(row)">格式化</el-button>
              <el-tag v-if="row.is_system" size="small" type="danger" effect="plain">禁止</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <el-empty v-if="!loading && !disks.length" description="没有识别到可管理的磁盘" />
    </el-card>

    <!-- 格式化：先看计划，再输入盘符确认 -->
    <el-dialog v-model="fmtDlg" title="格式化分区" width="560px">
      <el-alert type="error" :closable="false" show-icon :title="fmtPlan?.danger || ''" style="margin-bottom:12px" />
      <el-form label-width="90px">
        <el-form-item label="文件系统">
          <el-select v-model="fmtFs" style="width:150px" @change="previewFormat">
            <el-option label="ext4" value="ext4" />
            <el-option label="xfs" value="xfs" />
            <el-option label="vfat (FAT32)" value="vfat" />
          </el-select>
        </el-form-item>
        <el-form-item label="卷标">
          <el-input v-model="fmtLabel" maxlength="16" placeholder="可选，最多 16 字节" style="width:220px" @change="previewFormat" />
        </el-form-item>
        <el-form-item label="将执行">
          <pre class="code">{{ (fmtPlan?.command || []).join(' ') || '—' }}</pre>
        </el-form-item>
        <el-form-item label="确认">
          <el-input v-model="fmtConfirm" :placeholder="'输入 ' + fmtTarget + ' 以确认'" style="width:260px" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="fmtDlg = false">取消</el-button>
        <el-button type="danger" :disabled="fmtConfirm !== fmtTarget" :loading="fmtting" @click="doFormat">执行格式化</el-button>
      </template>
    </el-dialog>

    <!-- 新建分区 -->
    <el-dialog v-model="partDlg" title="新建分区" width="520px">
      <el-form label-width="110px">
        <el-form-item label="磁盘">{{ partDev }}</el-form-item>
        <el-form-item label="起始 (MiB)">
          <el-input-number v-model="partStart" :min="1" controls-position="right" />
        </el-form-item>
        <el-form-item label="结束 (MiB)">
          <el-input-number v-model="partEnd" :min="0" controls-position="right" />
          <span class="hint">0 = 用到剩余全部空间</span>
        </el-form-item>
        <el-form-item label="将执行">
          <pre class="code">{{ (partPlan?.command || []).join(' ') || '—' }}</pre>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="partDlg = false">取消</el-button>
        <el-button @click="previewPartition">预览</el-button>
        <el-button type="primary" :disabled="!partPlan" :loading="parting" @click="doPartition">执行</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

type Part = { name: string; path: string; size_bytes: number; fstype: string; label: string; uuid: string; mount_point: string; is_system: boolean; is_mounted: boolean };
type Disk = { name: string; path: string; size_bytes: number; model: string; removable: boolean; read_only: boolean; is_system: boolean; partitions: Part[]; mounted_count: number; used_bytes: number; avail_bytes: number };

const router = useRouter();
const disks = ref<Disk[]>([]);
const loading = ref(false);

const fmtDlg = ref(false);
const fmtTarget = ref('');
const fmtFs = ref('ext4');
const fmtLabel = ref('');
const fmtConfirm = ref('');
const fmtPlan = ref<any>(null);
const fmtting = ref(false);

const partDlg = ref(false);
const partDev = ref('');
const partStart = ref(1);
const partEnd = ref(0);
const partPlan = ref<any>(null);
const parting = ref(false);

function fmtSize(b: number) {
  if (!b) return '-';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = b;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(v >= 100 ? 0 : 1)} ${u[i]}`;
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/system/disks');
    disks.value = res.data?.disks || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  } finally {
    loading.value = false;
  }
}

function goFiles() { router.push('/files'); }

async function mount(row: Part) {
  try {
    const res = await api.post('/api/v1/system/disks/mount', { device: row.path });
    ElMessage.success('已挂载到 ' + (res.data?.mount_point || ''));
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '挂载失败');
  }
}

async function umount(row: Part) {
  try {
    await ElMessageBox.confirm(`卸载 ${row.mount_point}？正在读写该分区的程序会中断。`, '确认卸载', { type: 'warning' });
  } catch { return; }
  try {
    await api.post('/api/v1/system/disks/umount', { mount_point: row.mount_point });
    ElMessage.success('已卸载');
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '卸载失败');
  }
}

async function openFormat(row: Part) {
  fmtTarget.value = row.path;
  fmtFs.value = row.fstype === 'xfs' ? 'xfs' : row.fstype === 'vfat' ? 'vfat' : 'ext4';
  fmtLabel.value = '';
  fmtConfirm.value = '';
  fmtPlan.value = null;
  fmtDlg.value = true;
  await previewFormat();
}

async function previewFormat() {
  try {
    const res = await api.post('/api/v1/system/disks/format', {
      device: fmtTarget.value, fstype: fmtFs.value, label: fmtLabel.value, dry_run: true,
    });
    fmtPlan.value = res.data?.plan || null;
  } catch (e: any) {
    fmtPlan.value = null;
    ElMessage.error(e?.response?.data?.message || e?.message || '计划被拒绝');
  }
}

async function doFormat() {
  fmtting.value = true;
  try {
    const res = await api.post('/api/v1/system/disks/format', {
      device: fmtTarget.value, fstype: fmtFs.value, label: fmtLabel.value, confirmed: true,
    });
    if (res.data?.executed) {
      ElMessage.success('格式化完成');
      fmtDlg.value = false;
      await load();
    } else {
      ElMessage.warning(res.data?.note || '未执行');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '格式化失败');
  } finally {
    fmtting.value = false;
  }
}

function openPartition(d: Disk) {
  partDev.value = d.path;
  partStart.value = 1;
  partEnd.value = 0;
  partPlan.value = null;
  partDlg.value = true;
}

async function previewPartition() {
  try {
    const res = await api.post('/api/v1/system/disks/partition', {
      device: partDev.value, start_mib: partStart.value, end_mib: partEnd.value, dry_run: true,
    });
    partPlan.value = res.data?.plan || null;
  } catch (e: any) {
    partPlan.value = null;
    ElMessage.error(e?.response?.data?.message || e?.message || '计划被拒绝');
  }
}

async function doPartition() {
  parting.value = true;
  try {
    const res = await api.post('/api/v1/system/disks/partition', {
      device: partDev.value, start_mib: partStart.value, end_mib: partEnd.value, confirmed: true,
    });
    if (res.data?.executed) {
      ElMessage.success('分区已创建');
      partDlg.value = false;
      await load();
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '分区失败');
  } finally {
    parting.value = false;
  }
}

onMounted(load);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.disk { border: 1px solid #2a2f3a; border-radius: 8px; padding: 12px; margin-bottom: 14px; }
.disk-hdr { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; flex-wrap: wrap; }
.disk-name { font-weight: 600; font-family: ui-monospace, Menlo, Consolas, monospace; }
.disk-model { color: #8b91a5; font-size: 12px; }
.disk-usage { color: #8b91a5; font-size: 12px; }
.spacer { flex: 1; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; }
.muted { color: #6b7280; font-size: 12px; }
.code { background: #0f1117; color: #d5dae6; padding: 8px 10px; border-radius: 6px; font-size: 12px; overflow: auto; margin: 0; }
.hint { color: #8b91a5; font-size: 12px; margin-left: 8px; }
</style>
