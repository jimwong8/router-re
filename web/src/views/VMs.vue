<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>虚拟机（KVM）</span>
          <div class="tools">
            <el-button size="small" :loading="loading" @click="load">刷新</el-button>
            <el-button size="small" type="primary" :disabled="!cap.available" @click="openCreate">新建虚拟机</el-button>
          </div>
        </div>
      </template>

      <el-alert v-if="!cap.available" type="warning" :closable="false" show-icon
        :title="'当前环境不支持管理虚拟机：' + cap.reason" style="margin-bottom:12px" />
      <el-alert v-else type="success" :closable="false" show-icon
        :title="'KVM 可用：' + (cap.qemu_binary || '') + '，libvirt 已连接'" style="margin-bottom:12px" />

      <div class="caps">
        <span class="cap">/dev/kvm：<b :class="cap.has_device ? 'ok' : 'bad'">{{ cap.has_device ? '存在' : '缺失' }}</b></span>
        <span class="cap">可读写：<b :class="cap.device_writable ? 'ok' : 'bad'">{{ cap.device_writable ? '是' : '否' }}</b></span>
        <span class="cap">CPU 虚拟化：<b :class="cap.cpu_virt_flags ? 'ok' : 'bad'">{{ cap.cpu_virt_flags ? '支持' : '无 vmx/svm' }}</b></span>
        <span class="cap">qemu：<b :class="cap.has_qemu ? 'ok' : 'bad'">{{ cap.has_qemu ? '已安装' : '未安装' }}</b></span>
        <span class="cap">libvirt：<b :class="cap.libvirt_running ? 'ok' : 'bad'">{{ cap.libvirt_running ? '运行中' : (cap.has_virsh ? '未运行' : '未安装') }}</b></span>
      </div>

      <el-table :data="vms" size="small" v-loading="loading" empty-text="没有虚拟机">
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="stateType(row.state)">{{ row.state_text }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="vcpu" label="vCPU" width="80" />
        <el-table-column label="内存" width="100">
          <template #default="{ row }">{{ row.memory_mib ? row.memory_mib + ' MiB' : '-' }}</template>
        </el-table-column>
        <el-table-column prop="disk_path" label="磁盘" min-width="200" show-overflow-tooltip />
        <el-table-column label="自启" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="row.autostart ? 'success' : 'info'" effect="plain">
              {{ row.autostart ? '开' : '关' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button v-if="row.state !== 'running'" link type="primary" size="small" @click="act(row, 'start')">启动</el-button>
            <el-button v-else link type="warning" size="small" @click="act(row, 'shutdown')">关机</el-button>
            <el-button v-if="row.state === 'running'" link type="danger" size="small" @click="forceOff(row)">强制断电</el-button>
            <el-button link size="small" @click="act(row, row.autostart ? 'autostart-off' : 'autostart-on')">
              {{ row.autostart ? '取消自启' : '开机自启' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg" title="新建虚拟机" width="600px">
      <el-form label-width="110px">
        <el-form-item label="名称">
          <el-input v-model="spec.name" placeholder="字母数字开头，可用 . _ -" style="width:240px" />
        </el-form-item>
        <el-form-item label="vCPU">
          <el-input-number v-model="spec.vcpu" :min="1" :max="64" controls-position="right" />
        </el-form-item>
        <el-form-item label="内存 (MiB)">
          <el-input-number v-model="spec.memory_mib" :min="128" :max="1048576" :step="256" controls-position="right" />
        </el-form-item>
        <el-form-item label="磁盘">
          <el-input v-model="spec.disk_path" placeholder="/etc/disk_user/vms/xxx.qcow2" style="width:380px" />
          <div class="hint">必须已存在的磁盘镜像或块设备；本页不代建镜像文件</div>
        </el-form-item>
        <el-form-item label="安装 ISO">
          <el-input v-model="spec.iso" placeholder="可选，/path/to/install.iso" style="width:380px" />
        </el-form-item>
        <el-form-item label="网络">
          <el-input v-model="spec.network" placeholder="default" style="width:240px" />
        </el-form-item>
        <el-form-item label="开机自启">
          <el-switch v-model="spec.autostart" />
        </el-form-item>
        <el-form-item label="创建后启动">
          <el-switch v-model="startAfter" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="create">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

type Cap = {
  has_device: boolean; device_writable: boolean; cpu_virt_flags: boolean;
  has_qemu: boolean; qemu_binary?: string; has_virsh: boolean; libvirt_running: boolean;
  available: boolean; reason?: string;
};
type VM = {
  name: string; state: string; state_text: string; vcpu: number; memory_mib: number;
  disk_path?: string; autostart: boolean;
};

const cap = ref<Cap>({ has_device: false, device_writable: false, cpu_virt_flags: false, has_qemu: false, has_virsh: false, libvirt_running: false, available: false });
const vms = ref<VM[]>([]);
const loading = ref(false);

const dlg = ref(false);
const creating = ref(false);
const startAfter = ref(true);
const spec = reactive({ name: '', vcpu: 2, memory_mib: 1024, disk_path: '', iso: '', network: 'default', autostart: false });

function stateType(s: string) {
  if (s === 'running') return 'success';
  if (s === 'paused') return 'warning';
  if (s === 'crashed') return 'danger';
  return 'info';
}

async function load() {
  loading.value = true;
  try {
    const res = await api.get('/api/v1/vms');
    cap.value = res.data?.capability || cap.value;
    vms.value = res.data?.vms || [];
    if (res.data?.note && !cap.value.available) ElMessage.warning(res.data.note);
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  spec.name = '';
  spec.disk_path = '';
  spec.iso = '';
  dlg.value = true;
}

async function create() {
  creating.value = true;
  try {
    const res = await api.post('/api/v1/vms', { ...spec, start: startAfter.value });
    ElMessage.success('已创建 ' + res.data?.name);
    dlg.value = false;
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '创建失败');
  } finally {
    creating.value = false;
  }
}

async function act(row: VM, action: string) {
  try {
    await api.post(`/api/v1/vms/${encodeURIComponent(row.name)}`, { action });
    ElMessage.success('操作完成');
    await load();
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '操作失败');
  }
}

async function forceOff(row: VM) {
  try {
    await ElMessageBox.confirm(
      `强制断电会立刻终止「${row.name}」里的所有程序，未落盘的数据会丢。确认？`,
      '强制断电', { type: 'warning', confirmButtonText: '强制断电', cancelButtonText: '取消' });
  } catch { return; }
  await act(row, 'destroy');
}

onMounted(load);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.caps { display: flex; gap: 18px; flex-wrap: wrap; margin-bottom: 14px; font-size: 12px; color: #8b91a5; }
.cap b { font-weight: 600; }
.ok { color: #3fb950; }
.bad { color: #f85149; }
.hint { color: #8b91a5; font-size: 12px; margin-top: 2px; }
</style>
