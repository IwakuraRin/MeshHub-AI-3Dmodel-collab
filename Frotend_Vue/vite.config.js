/*
|--------------------------------------------------------------------------
| Vite 构建配置
|--------------------------------------------------------------------------
| 启用 Vue 和 Tailwind，并把前端产物输出给 Wails 嵌入使用。
|--------------------------------------------------------------------------
 */
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    outDir: "../backend_go/frontend_dist",
    emptyOutDir: true
  },
  server: {
    strictPort: false
  }
});
