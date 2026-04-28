import { defineConfig } from "vite";
import { resolve } from "path";

export default defineConfig({
  build: {
    lib: {
      entry: resolve(__dirname, "src/index.ts"),
      name: "NoStripeTax",
      fileName: "nostripetax",
      formats: ["es", "umd"],
    },
    rollupOptions: {
      output: {
        // Ensure the UMD build attaches to window.NoStripeTax
        globals: {},
      },
    },
    sourcemap: true,
    minify: "esbuild",
  },
});
