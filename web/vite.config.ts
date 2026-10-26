import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

const apiProxy = {
  '/api/v1': {
    target: 'http://127.0.0.1:9090',
    changeOrigin: true,
    rewrite: (path: string) => path.replace(/^\/api\/v1/, '/api/v1'),
  },
};

export default defineConfig({
  plugins: [vue()],
  server: { proxy: apiProxy },
  // `vite preview` serves the production build; enable the same proxy so the
  // built app can be exercised end-to-end without the dev server's transform.
  preview: { proxy: apiProxy },
});
