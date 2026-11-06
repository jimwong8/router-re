<template>
  <div>
    <!--  ACL 卡片 -->
    <el-card shadow="never" style="margin-bottom: 20px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>访问控制列表 (ACL)</span>
          <el-button type="primary" size="small" :loading="saving" @click="saveRules">保存下发</el-button>
        </div>
      </template>

      <el-table :data="rules" style="width:100%; margin-bottom:12px" row-key="id">
        <el-table-column prop="id" label="ID" width="50" />
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="src_ip" label="源 IP（任意为null）" />
        <el-table-column prop="dst_ip" label="目的 IP" />
        <el-table-column prop="dst_port" label="目的端口" />
        <el-table-column prop="proto" label="协议" />
        <el-table-column prop="action" label="动作" />
        <el-table-column prop="enabled" label="启用" width="70" />
        <el-table-column label="操作" width="160">
          <template #default="scope">
            <el-button type="primary" link size="small" @click="up(scope.$index)">上移</el-button>
            <el-button type="primary" link size="small" @click="down(scope.$index)">下移</el-button>
            <el-button type="danger" link size="small" @click="remove(scope.$index)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-form :inline="true" @keyup.enter="addRule">
        <el-form-item label="源 IP">
          <el-input v-model="form.src_ip" placeholder="留空为 any" />
        </el-form-item>
        <el-form-item label="目的 IP">
          <el-input v-model="form.dst_ip" placeholder="留空为 any" />
        </el-form-item>
        <el-form-item label="目的端口">
          <el-input v-model.number="form.dst_port" placeholder="0-65535" />
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="form.proto" style="width:110px">
            <el-option label="tcp" value="tcp" />
            <el-option label="udp" value="udp" />
            <el-option label="icmp" value="icmp" />
            <el-option label="any" value="any" />
          </el-select>
        </el-form-item>
        <el-form-item label="动作">
          <el-select v-model="form.action" style="width:110px">
            <el-option label="accept" value="accept" />
            <el-option label="drop" value="drop" />
            <el-option label="reject" value="reject" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="备注名" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="small" @click="addRule">新增</el-button>
        </el-form-item>
      </el-form>

      <el-input
        v-if="nftText"
        v-model="nftText"
        type="textarea"
        rows="8"
        style="width:100%; margin-top:12px; font-family:monospace; font-size:12px"
        readonly
        placeholder="保存下发后显示下发的 nft 文本"
      />
    </el-card>

    <!--  攻击防护卡片 -->
    <el-card shadow="never" style="margin-bottom:20px">
      <template #header>
        <div style="display:flex; justify-content:space-between; align-items:center">
          <span>攻击防护（/api/v1/security/attack-guard）</span>
          <el-button type="primary" size="small" :loading="attackGuardSaving" @click="saveAttackGuard">保存下发</el-button>
        </div>
      </template>
      <el-form :model="attackGuardForm" label-width="130px">
        <el-form-item label="启用">
          <el-switch v-model="attackGuardForm.enabled" />
        </el-form-item>
        <el-form-item label="SYN Flood PPS">
          <el-input-number v-model="attackGuardForm.syn_flood_pps" :min="1" :max="1000000" style="width:160px" />
          <span style="color:#909399; font-size:12px; margin-left:8px">每秒 SYN 包阈值</span>
        </el-form-item>
        <el-form-item label="ICMP Flood PPS">
          <el-input-number v-model="attackGuardForm.icmp_flood_pps" :min="1" :max="1000000" style="width:160px" />
          <span style="color:#909399; font-size:12px; margin-left:8px">每秒 ICMP 包阈值</span>
        </el-form-item>
        <el-form-item label="单 IP 连接限制">
          <el-input-number v-model="attackGuardForm.conn_limit_per_ip" :min="1" :max="1000000" style="width:160px" />
          <span style="color:#909399; font-size:12px; margin-left:8px">每 IP 最大并发连接数</span>
        </el-form-item>
      </el-form>
      <el-alert v-if="attackGuardError" :title="attackGuardError" type="error" :closable="false" style="margin:12px 0" />
      <div v-if="attackGuardNft" style="margin-top:12px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px">
          <span>nftables 规则</span>
          <el-button type="primary" link size="small" @click="copyAttackGuardNft">复制</el-button>
        </div>
        <pre style="background:#1e1e1e; color:#d4d4d4; padding:12px; border-radius:4px; min-height:80px; white-space:pre-wrap; word-break:break-all">{{ attackGuardNft }}</pre>
      </div>
    </el-card>

    <!--  ARP 防护卡片 -->
    <el-card shadow="never">
      <template #header><span>ARP 欺骗防护</span></template>

      <el-form :inline="true">
        <el-form-item label="接口">
          <el-select v-model="arpIface" placeholder="选择接口" style="width:180px">
            <el-option v-for="ifc in ifaces" :key="ifc.name" :label="ifc.name" :value="ifc.name" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="small" :loading="arpLoading" @click="applyArp">开启防护</el-button>
        </el-form-item>
      </el-form>

      <el-table
        v-if="Object.keys(arpSysctls).length"
        :data="Object.entries(arpSysctls).map(([k, v]) => ({ key: k, value: v }))"
        style="width:100%; margin-top:12px"
      >
        <el-table-column prop="key" label="参数" />
        <el-table-column prop="value" label="值" />
      </el-table>

      <el-table :data="neighbors" style="width:100%; margin-top:16px">
        <!-- 后端 /security/arp/neighbors 返回 {dev,dst,lladdr,state}；
             早先写成 prop="ip"/"mac" 导致 IP/MAC 两列全空（视觉验证时发现）。 -->
        <el-table-column prop="dev" label="接口" width="110" />
        <el-table-column prop="dst" label="IP 地址" />
        <el-table-column prop="lladdr" label="MAC 地址" />
        <el-table-column prop="state" label="状态" width="120" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '../api';

interface Rule {
  id: number;
  name: string;
  src_ip: string;
  dst_ip: string;
  dst_port: number;
  proto: string;
  action: string;
  enabled: boolean;
}

const rules = ref<Rule[]>([]);
const nftText = ref('');
const saving = ref(false);
const form = ref({
  src_ip: '', dst_ip: '', dst_port: 0, proto: 'tcp', action: 'accept', name: '',
});

const attackGuardForm = ref({
  enabled: false,
  syn_flood_pps: 1000,
  icmp_flood_pps: 1000,
  conn_limit_per_ip: 100,
});
const attackGuardSaving = ref(false);
const attackGuardError = ref('');
const attackGuardNft = ref('');

async function loadAttackGuard() {
  try {
    const res = await api.get('/api/v1/security/attack-guard');
    const cfg = res.data?.config || {};
    attackGuardForm.value = {
      enabled: !!cfg.enabled,
      syn_flood_pps: cfg.syn_flood_pps || 1000,
      icmp_flood_pps: cfg.icmp_flood_pps || 1000,
      conn_limit_per_ip: cfg.conn_limit_per_ip || 100,
    };
    attackGuardNft.value = res.data?.nft || '';
  } catch (e: any) {
    attackGuardError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(attackGuardError.value);
  }
}

async function saveAttackGuard() {
  attackGuardSaving.value = true;
  attackGuardError.value = '';
  try {
    const res = await api.post('/api/v1/security/attack-guard', attackGuardForm.value);
    const d = res.data || {};
    attackGuardNft.value = d.nft || '';
    if (d.applied) {
      ElMessage.success(d.detail || '攻击防护已下发');
    } else {
      ElMessage.warning(d.detail || '已保存但未下发');
    }
  } catch (e: any) {
    attackGuardError.value = e?.response?.data?.detail || e?.response?.data?.message || e.message;
    ElMessage.error(attackGuardError.value);
  } finally {
    attackGuardSaving.value = false;
  }
}

function copyAttackGuardNft() {
  navigator.clipboard.writeText(attackGuardNft.value || '').then(() => {
    ElMessage.success('已复制到剪贴板');
  }).catch(() => {
    ElMessage.error('复制失败');
  });
}

const arpIface = ref('');
const arpSysctls = ref<Record<string, string>>({});
const arpLoading = ref(false);
const neighbors = ref<any[]>([]);
const ifaces = ref<Array<{ name: string }>>([]);

async function loadRules() {
  try {
    const res = await api.get('/api/v1/acl/rules');
    rules.value = res.data?.rules || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function addRule() {
  if (!form.value.proto || !form.value.action) {
    ElMessage.error('请选择协议与动作');
    return;
  }
  const next = Math.max(0, ...rules.value.map((r) => r.id)) + 1;
  rules.value.push({
    id: next,
    name: form.value.name,
    src_ip: form.value.src_ip,
    dst_ip: form.value.dst_ip,
    dst_port: form.value.dst_port,
    proto: form.value.proto,
    action: form.value.action,
    enabled: true,
  });
  form.value = { src_ip: '', dst_ip: '', dst_port: 0, proto: 'tcp', action: 'accept', name: '' };
}

function remove(i: number) {
  rules.value.splice(i, 1);
}

function up(i: number) {
  if (i === 0) return;
  [rules.value[i], rules.value[i - 1]] = [rules.value[i - 1], rules.value[i]];
}

function down(i: number) {
  if (i === rules.value.length - 1) return;
  [rules.value[i], rules.value[i + 1]] = [rules.value[i + 1], rules.value[i]];
}

async function saveRules() {
  if (!rules.value.length) {
    ElMessage.error('至少添加一条规则');
    return;
  }
  try {
    await ElMessageBox.confirm('确定下发 ACL 规则到 nftables？', '确认', { type: 'warning' });
  } catch {
    return;
  }
  saving.value = true;
  try {
    const res = await api.post('/api/v1/acl/rules', { rules: rules.value });
    nftText.value = res.data?.nft || '';
    if (res.data?.applied) {
      ElMessage.success('ACL 已下发');
      await loadRules();
    } else {
      ElMessage.warning('渲染成功但未应用（仅开发模式）');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    saving.value = false;
  }
}

async function loadArpStatus() {
  try {
    const res = await api.get('/api/v1/security/arp');
    if (res.data?.iface) arpIface.value = res.data.iface;
  } catch {}
  try {
    const res = await api.get('/api/v1/security/arp/neighbors');
    neighbors.value = res.data?.neighbors || [];
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  }
}

async function applyArp() {
  if (!arpIface.value) {
    ElMessage.error('请选择接口');
    return;
  }
  arpLoading.value = true;
  try {
    const res = await api.post('/api/v1/security/arp', { iface: arpIface.value });
    arpSysctls.value = res.data?.sysctls || {};
    if (res.data?.applied) {
      ElMessage.success(`ARP 防护已开启（${arpIface.value}）`);
    } else {
      ElMessage.warning('仅渲染未应用');
    }
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e.message);
  } finally {
    arpLoading.value = false;
  }
}

async function loadIfaces() {
  try {
    const res = await api.get('/api/v1/stats/interfaces');
    ifaces.value = (res.data?.interfaces || []).filter((i: any) => i.name !== 'lo');
  } catch {}
}

onMounted(() => {
  loadRules();
  loadIfaces();
  loadArpStatus();
  loadAttackGuard();
});
</script>

<style scoped>
</style>