import axios from 'axios';

const api = axios.create({
  baseURL: '',
  headers: {
    Authorization: `Bearer ${localStorage.getItem('router_token') || ''}`,
  },
});

// Response interceptor: on 401, clear token and redirect to login
api.interceptors.response.use(
  (res) => res,
  (err) => {
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
