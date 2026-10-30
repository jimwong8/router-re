<template>
  <div>
    <el-card shadow="never">
      <template #header>
        <div class="hdr">
          <span>文件服务</span>
          <div class="tools">
            <el-button size="small" @click="router.push('/files')">文件管理</el-button>
            <el-button size="small" :loading="loading" @click="loadAll">刷新</el-button>
          </div>
        </div>
      </template>

      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="三个服务的共享根目录都必须位于 /etc/disk_user 内；密码不会回显，留空表示不修改。保存后可选「保存并应用」，但服务未安装时只会保存配置。" />

      <el-tabs v-model="tab">
        <el-tab-pane label="FTP" name="ftp" />
        <el-tab-pane label="Samba" name="samba" />
        <el-tab-pane label="HTTP" name="http" />
      </el-tabs>

      <div v-for="k in ['ftp', 'samba', 'http']" :key="k" v-show="tab === k">
        <div class="status-bar">
          <el-tag :type="st[k]?.running ? 'success' : 'info'" size="small">
            {{ st[k]?.running ? '运行中' : '未运行' }}
          </el-tag>
          <el-tag :type="st[k]?.installed ? 'success' : 'warning'" size="small" effect="plain">
            {{ st[k]?.installed ? '已安装 ' + st[k]?.path : '未安装（' + st[k]?.binary + '）' }}
          </el-tag>
          <span class="muted">配置文件：{{ confPath[k] }}</span>
        </div>

        <!-- FTP -->
        <el-form v-if="k === 'ftp'" label-width="130px" style="max-width:720px">
          <el-form-item label="启用">
            <el-switch v-model="ftp.enabled" />
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number v-model="ftp.port" :min="1" :max="65535" controls-position="right" style="width:150px" />
            <span class="hint">不能用 22 / 53 / 9090</span>
          </el-form-item>
          <el-form-item label="共享目录">
            <el-input v-model="ftp.root" style="width:340px" />
            <span class="hint">必须在 /etc/disk_user 内</span>
          </el-form-item>
          <el-form-item label="匿名访问">
            <el-switch v-model="ftp.anonymous" />
          </el-form-item>
          <template v-if="!ftp.anonymous">
            <el-form-item label="用户名">
              <el-input v-model="ftp.user" style="width:220px" />
            </el-form-item>
            <el-form-item label="密码">
              <el-input v-model="ftp.password" type="password" show-password style="width:220px"
                :placeholder="passwordSet.ftp ? '留空表示不修改' : '启用时必填'" />
            </el-form-item>
          </template>
          <el-form-item label="最大连接数">
            <el-input-number v-model="ftp.max_conn" :min="0" :max="1000" controls-position="right" style="width:150px" />
            <span class="hint">0 = 不限</span>
          </el-form-item>
          <el-form-item label="限速 (KB/s)">
            <el-input-number v-model="ftp.speed_limit" :min="0" :max="1048576" controls-position="right" style="width:150px" />
            <span class="hint">0 = 不限</span>
          </el-form-item>
        </el-form>

        <!-- Samba -->
        <el-form v-if="k === 'samba'" label-width="130px" style="max-width:720px">
          <el-form-item label="启用">
            <el-switch v-model="samba.enabled" />
          </el-form-item>
          <el-form-item label="工作组">
            <el-input v-model="samba.workgroup" maxlength="15" style="width:220px" />
          </el-form-item>
          <el-form-item label="共享名">
            <el-input v-model="samba.share_name" style="width:220px" />
          </el-form-item>
          <el-form-item label="共享目录">
            <el-input v-model="samba.root" style="width:340px" />
          </el-form-item>
          <el-form-item label="只读">
            <el-switch v-model="samba.read_only" />
          </el-form-item>
          <template v-if="!samba.read_only">
            <el-form-item label="用户名">
              <el-input v-model="samba.user" style="width:220px" />
            </el-form-item>
            <el-form-item label="密码">
              <el-input v-model="samba.password" type="password" show-password style="width:220px"
                :placeholder="passwordSet.samba ? '留空表示不修改' : '可写共享必填'" />
            </el-form-item>
          </template>
        </el-form>

        <!-- HTTP -->
        <el-form v-if="k === 'http'" label-width="130px" style="max-width:720px">
          <el-form-item label="启用">
            <el-switch v-model="http.enabled" />
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number v-model="http.port" :min="1" :max="65535" controls-position="right" style="width:150px" />
          </el-form-item>
          <el-form-item label="共享目录">
            <el-input v-model="http.root" style="width:340px" />
          </el-form-item>
          <el-form-item label="允许列目录">
            <el-switch v-model="http.listing" />
          </el-form-item>
        </el-form>

        <div class="actions">
          <el-button size="small" :loading="saving === k" @click="save(k, false)">保存</el-button>
          <el-button type="primary" size="small" :loading="applying === k" @click="save(k, true)">保存并应用</el-button>
        </div>

        <el-card shadow="never" style="margin-top:14px">
          <template #header><span>生成的配置（{{ confPath[k] }}）</span></template>
          <pre class="code">{{ conf[k] || '（暂无）' }}</pre>
        </el-card>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage } from 'element-plus';
import api from '../api';

const router = useRouter();
const tab = ref('ftp');
const loading = ref(false);
const saving = ref('');
const applying = ref('');

const ftp = reactive<any>({ enabled: false, port: 21, root: '/etc/disk_user', anonymous: false, user: '', password: '', max_conn: 10, speed_limit: 0 });
const samba = reactive<any>({ enabled: false, workgroup: 'WORKGROUP', share_name: 'disk', root: '/etc/disk_user', read_only: false, user: '', password: '' });
const http = reactive<any>({ enabled: false, port: 8088, root: '/etc/disk_user', listing: true });

const conf = reactive<Record<string, string>>({ ftp: '', samba: '', http: '' });
const confPath = reactive<Record<string, string>>({ ftp: '/etc/vsftpd.conf', samba: '/etc/samba/smb.conf', http: '/etc/lighttpd/lighttpd.conf' });
const st = reactive<Record<string, any>>({});
const passwordSet = reactive<Record<string, boolean>>({ ftp: false, samba: false, http: false });

const formOf: Record<string, any> = { ftp, samba, http };

async function loadOne(k: string) {
  try {
    const res = await api.get(`/api/v1/services/filesvc/${k}`);
    const c = res.data?.config || {};
    delete c.password;
    Object.assign(formOf[k], c);
    conf[k] = res.data?.conf || '';
    confPath[k] = res.data?.conf_path || confPath[k];
    st[k] = res.data?.status || {};
    passwordSet[k] = !!res.data?.password_set;
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '加载失败');
  }
}

async function loadAll() {
  loading.value = true;
  try {
    await Promise.all(['ftp', 'samba', 'http'].map(loadOne));
  } finally {
    loading.value = false;
  }
}

async function save(k: string, apply: boolean) {
  if (apply) applying.value = k;
  else saving.value = k;
  try {
    const body = { ...formOf[k], apply };
    if (!body.password) delete body.password;
    const res = await api.post(`/api/v1/services/filesvc/${k}`, body);
    conf[k] = res.data?.conf || conf[k];
    st[k] = res.data?.status || st[k];
    formOf[k].password = '';
    if (res.data?.applied) {
      ElMessage.success('已保存并启动');
    } else if (res.data?.note) {
      ElMessage.warning(res.data.note);
    } else {
      ElMessage.success('已保存');
    }
    await loadOne(k);
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败');
  } finally {
    saving.value = '';
    applying.value = '';
  }
}

onMounted(loadAll);
</script>

<style scoped>
.hdr { display: flex; justify-content: space-between; align-items: center; }
.tools { display: flex; gap: 8px; }
.status-bar { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; flex-wrap: wrap; }
.muted { color: #8b91a5; font-size: 12px; font-family: ui-monospace, Menlo, Consolas, monospace; }
.hint { color: #8b91a5; font-size: 12px; margin-left: 8px; }
.actions { display: flex; gap: 8px; margin-top: 4px; }
.code { background: #0f1117; color: #d5dae6; padding: 12px; border-radius: 6px; font-size: 12px; overflow: auto; max-height: 320px; margin: 0; }
</style>
