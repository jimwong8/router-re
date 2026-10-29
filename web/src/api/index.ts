import axios from 'axios';

const api = axios.create({ baseURL: '' });

// 关键：token 必须在每次请求时读取，不能在模块加载时算一次。
// 登录页写 localStorage 后是 SP-alternative 路由跳转（不重新加载页面），
// 如果 Authorization 用模块级静态头，登录后所有请求带的仍是登录前的空 token
// → 全部 401 → 拦截器清 token 跳回登录页，表现为"登录成功立刻被踢回登录界面"。
api.interceptors.request.use((cfg) => {
  const token = localStorage.getItem('router_token') || '';
  if (token) cfg.headers.Authorization = `Bearer ${token}`;
  else delete cfg.headers.Authorization;
  return cfg;
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
        // 静默跳转会让人以为是页面出问题；明确提示是登录态过期
        const reason = body?.detail && /expire/i.test(String(body.detail)) ? '登录已过期，请重新登录' : '登录已失效，请重新登录';
        window.alert(reason);
        window.location.href = '/login';
      }
    }
    return Promise.reject(err);
  },
);

export default api;
