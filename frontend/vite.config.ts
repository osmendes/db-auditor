import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The browser calls this dev server on the same origin; Compose sets its API target.
const apiProxyTarget =
  (globalThis as { process?: { env?: Record<string, string> } }).process?.env
    ?.VITE_API_PROXY_TARGET ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: true,
    port: 5173,
    headers: {
      "Content-Security-Policy":
        "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://api.fontshare.com https://fonts.googleapis.com; img-src 'self' data:; font-src 'self' https://cdn.fontshare.com https://fonts.gstatic.com; connect-src 'self' ws: wss: http://localhost:8080 http://127.0.0.1:8080; object-src 'none'; base-uri 'self'; frame-ancestors 'none'",
    },
    proxy: {
      "/api": { target: apiProxyTarget, changeOrigin: true },
      "/health": { target: apiProxyTarget, changeOrigin: true },
      "/ready": { target: apiProxyTarget, changeOrigin: true },
      "/metrics": { target: apiProxyTarget, changeOrigin: true },
    },
  },
});
