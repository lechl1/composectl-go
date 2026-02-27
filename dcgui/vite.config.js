import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// Read base from environment (same variable used in svelte.config.js)
const rawBase = process.env.BASE_PATH || '';
const base = rawBase ? '/' + rawBase.replace(/^\/+|\/+$/g, '') : '';

// https://vite.dev/config/
export default defineConfig({
  base,
  plugins: [sveltekit()],
  define: {
    'process.env.BASE_PATH': JSON.stringify(base),
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8882',
        changeOrigin: true,
      },
    },
  },
});
