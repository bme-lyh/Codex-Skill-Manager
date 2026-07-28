import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // Keep the output directory intact. Windows release builds may have the
  // previous executable open, and this project never performs bulk deletion.
  build: { outDir: "dist", emptyOutDir: false },
  server: { port: 34115, strictPort: true }
});
