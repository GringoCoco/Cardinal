import { defineConfig } from "vite";
import preact from "@preact/preset-vite";

export default defineConfig({
  // Mini App живёт под /app/ на relayd
  base: "/app/",
  plugins: [preact()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
