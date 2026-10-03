import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  base: "/dashboard/",
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: {
    // `npm run dev` talks to a sandbox already running on :8080.
    proxy: {
      "/mock": "http://localhost:8080",
      "/payment_page": "http://localhost:8080",
    },
  },
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
});
