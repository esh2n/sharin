import { defineConfig } from "vitest/config";

// フロント編の実装は実DOMへ patch を当てるため、テスト環境は happy-dom。
// 純粋な diff ロジックも DOM への適用結果も、同じ環境で検証できる。
export default defineConfig({
  test: {
    globals: true,
    environment: "happy-dom",
    include: ["**/*.test.ts"],
    coverage: {
      provider: "v8",
      include: ["**/*.ts"],
      exclude: ["**/*.test.ts", "vitest.config.ts"],
      thresholds: { lines: 80, functions: 80, branches: 80, statements: 80 },
    },
  },
});
