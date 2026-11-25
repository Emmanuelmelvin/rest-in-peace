import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development, the playground host (go run ./cmd/playground) listens on :8090.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://localhost:8090",
      "/p": "http://localhost:8090",
    },
  },
});
