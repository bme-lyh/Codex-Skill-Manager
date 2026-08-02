import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // Keep the output directory intact because this project never performs bulk
  // deletion. Stable names overwrite the previous build instead of leaving
  // obsolete hashed assets behind.
  build: {
    outDir: "dist",
    emptyOutDir: false,
    rollupOptions: {
      output: {
        entryFileNames: "assets/app.js",
        chunkFileNames: "assets/[name].js",
        assetFileNames: "assets/[name][extname]"
      }
    }
  },
  server: { port: 34115, strictPort: true }
});
