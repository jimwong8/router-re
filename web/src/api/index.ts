import axios from 'axios';

const api = axios.create({
  baseURL: '',
  headers: {
    Authorization: `Bearer ${localStorage.getItem('router_token') || ''}`,
  },
});

// 后端错误体是 {code, message, detail}：message 是英文类别（forbidden /
// unauthorized / invalid JSON body），detail 才是给人看的中文说明
// （"当前角色无权执行该操作"）。而页面里 153 处调用点统一是
//     ElMessage.error(e?.response?.data?.message || e.message)
// 于是用户看到的是 "forbidden"——既看不懂发生了什么，也不知道该怎么办。
//
// 在这里把 detail 提到 message 位置上，一次覆盖所有调用点。不写 err.message
// 是因为那些调用点优先读 data.message，改了 err.message 不生效。
// 已确认没有代码依赖 message 的文本做分支判断。
//
// 401 仍然清 token 跳登录。
api.interceptors.response.use(
  (res) => res,
  (err) => {
    const body = err?.response?.data;
    if (body && typeof body === 'object') {
      const detail = typeof body.detail === 'string' ? body.detail.trim() : '';
      const msg = typeof body.message === 'string' ? body.message.trim() : '';
      if (detail && detail !== msg) {
        body.detail = msg || detail; // 保留英文类别，便于排查
        body.message = detail; // 页面显示的是这个
      }
    }
    if (err?.response?.status === 401) {
      localStorage.removeItem('router_token');
      if (window.location.pathname !== '/login') {
        window.location.href = '/login';
      }
    }
    return Promise.reject(err);
  },
);

export default api;
