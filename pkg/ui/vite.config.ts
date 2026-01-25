import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "./",
  plugins: [react()],
  build: {
    outDir: "dist/admin",
    sourcemap: false,
    rollupOptions: {
      input: {
        main: "admin.html",
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/health": {
        target: "http://localhost:9810",
        changeOrigin: true,
      },
      "/stats": {
        target: "http://localhost:9810",
        changeOrigin: true,
      },
      "/cache": {
        target: "http://localhost:9810",
        changeOrigin: true,
      },
      "/api": {
        target: "http://localhost:9810",
        changeOrigin: true,
      },
    },
  },
});
