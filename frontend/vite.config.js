import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// Base path for assets. GitHub Pages для проектного репо отдаёт сайт по
// `/<repo-name>/`, поэтому ассеты должны ссылаться оттуда же. В CI задаём
// VITE_BASE=/goronin/. Для локальной разработки и для деплоя в корень
// (свой домен через CNAME) base остаётся '/'.
const base = process.env.VITE_BASE || '/';

export default defineConfig({
  base,
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
  },
});
