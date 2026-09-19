import { defineConfig } from 'astro/config';
import node from '@astrojs/node';
import react from '@astrojs/react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  site: 'https://tap4furry.com',
  output: 'server', adapter: node({ mode: 'standalone' }), integrations: [react()],
  vite: {
    plugins: [tailwindcss()],
    server: { proxy: { '/api': { target: 'http://127.0.0.1:8080', rewrite: path => path.replace(/^\/api/, '') } } },
  },
});
