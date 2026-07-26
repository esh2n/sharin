import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vitest/config";
import { vrtCompare } from "./vrt/commands";

// NOTE: VRT 専用の vitest 設定。playwright chromium の browser mode で各デモを実描画し、
//       reference PNG と pixelmatch で比較する。viewport は固定して描画を決定化する。
export default defineConfig({
  plugins: [vue()],
  test: {
    include: ["vrt/vrt.test.ts"],
    setupFiles: ["./vrt/setup.ts"],
    browser: {
      enabled: true,
      provider: "playwright",
      headless: true,
      instances: [{ browser: "chromium", viewport: { width: 720, height: 900 } }],
      commands: { vrtCompare },
    },
  },
});
