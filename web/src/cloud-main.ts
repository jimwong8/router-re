import { createApp } from 'vue';
import CloudConsole from './views/CloudConsole.vue';
import ElementPlus from 'element-plus';
import 'element-plus/dist/index.css';

const app = createApp(CloudConsole);
app.use(ElementPlus);
app.mount('#app');
