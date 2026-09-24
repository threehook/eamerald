import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // forwards to the laadpalen-api backend, so the browser only ever calls its own origin.
      "/api": "http://localhost:8787",
    },
  },
});
