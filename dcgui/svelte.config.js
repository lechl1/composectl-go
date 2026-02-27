import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Read base path from environment at build time. Example: BASE_PATH="/myapp"
const rawBase = process.env.BASE_PATH || '';
// Normalize: empty => '', otherwise ensure leading slash and no trailing slash
const base = rawBase
  ? '/' + rawBase.replace(/^\/+|\/+$/g, '')
  : '';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  // Consult https://svelte.dev/docs#compile-time-svelte-preprocess
  // for more information about preprocessors
  preprocess: vitePreprocess(),

  kit: {
    adapter: adapter({ fallback: 'index.html' }),
    paths: {
      base
    }
  }
};

export default config;

