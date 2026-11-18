import { defineConfig } from "vite";
import path from "node:path";

export default defineConfig({
  root: path.resolve(import.meta.dirname),
  base: "/manage/",
  esbuild: {
    jsx: "automatic",
    jsxImportSource: "preact",
    target: "es2022",
  },
  build: {
    outDir: path.resolve(import.meta.dirname, "../internal/app/static"),
    emptyOutDir: true,
    target: "es2022",
    sourcemap: false,
    rollupOptions: {
      input: {
        main: path.resolve(import.meta.dirname, "index.html"),
      },
    },
  },
});
