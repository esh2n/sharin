// NOTE: components/*Demo.vue を import.meta.glob で全部拾い、light / dark 両テーマで
//       実描画して reference と pixelmatch 比較する。デモを足せば自動で VRT の対象に
//       なるので登録漏れが起きない(arekore packages/ui/vrt の思想を VitePress/Vue へ移植)。
import { commands } from "@vitest/browser/context";
import { createApp, type Component } from "vue";
import { afterEach, describe, expect, test } from "vitest";

declare module "@vitest/browser/context" {
  interface BrowserCommands {
    vrtCompare: (demo: string, variant: string, theme: string) => Promise<void>;
  }
}

// NOTE: 台の幅は VitePress のドキュメント本文幅(.vp-doc の既定)に合わせる。
//       デモは本文中に置かれるので、その幅で成立することを VRT 自体が検証する。
const STAGE_WIDTH_PX = 688;

interface Variant {
  name: string;
  props: Record<string, unknown>;
}

// NOTE: 同じデモを別 props で複数回使うものは、実際の本文での使われ方に合わせて
//       variant を列挙する(1 variant = 1 基準画像)。ここに無いデモは props 無しの
//       "default" 1枚。必須 prop を持つデモをここに登録し忘れると、下の warn ガードで
//       VRT が落ちる(登録漏れを仕組みで検知する)。
const VRT_VARIANTS: Record<string, Variant[]> = {
  // RaftDemo は seed 固定 LCG。vrt=固定 tick 数で自動 tick を止めて静止画を撮る。
  RaftDemo: [{ name: "default", props: { vrt: 30 } }],
  IdGenDemo: [
    { name: "uuidv4", props: { kind: "uuidv4" } },
    { name: "uuidv7", props: { kind: "uuidv7" } },
    { name: "ulid", props: { kind: "ulid" } },
    { name: "snowflake", props: { kind: "snowflake" } },
  ],
  RateLimitDemo: [
    { name: "token-bucket", props: { algo: "token-bucket" } },
    { name: "leaky-bucket", props: { algo: "leaky-bucket" } },
    { name: "fixed-window", props: { algo: "fixed-window" } },
    { name: "sliding-window-log", props: { algo: "sliding-window-log" } },
  ],
  SamplingDemo: [
    { name: "temperature", props: { controls: ["temperature"] } },
    { name: "topk", props: { controls: ["topk"] } },
    { name: "topp", props: { controls: ["topp"] } },
    { name: "minp", props: { controls: ["minp"] } },
    { name: "all", props: { controls: ["temperature", "topk", "topp", "minp"] } },
  ],
};

const modules = import.meta.glob<{ default: Component }>("../components/*Demo.vue", {
  eager: true,
});

const demoOf = (modulePath: string): string => {
  const match = modulePath.match(/\/([^/]+)\.vue$/);
  if (!match?.[1]) throw new Error(`デモ名を取れません: ${modulePath}`);
  return match[1];
};

const variantsOf = (demo: string): Variant[] =>
  VRT_VARIANTS[demo] ?? [{ name: "default", props: {} }];

afterEach(() => {
  document.body.replaceChildren();
});

for (const [modulePath, module] of Object.entries(modules)) {
  const demo = demoOf(modulePath);
  describe(demo, () => {
    for (const variant of variantsOf(demo)) {
      for (const theme of ["light", "dark"] as const) {
        test(`${variant.name} (${theme})`, async () => {
          document.documentElement.classList.toggle("dark", theme === "dark");

          const stage = document.createElement("div");
          stage.id = "vrt-stage";
          stage.style.width = `${STAGE_WIDTH_PX}px`;
          stage.style.padding = "24px";
          stage.style.background = "var(--vp-c-bg)";
          stage.style.color = "var(--vp-c-text-1)";
          stage.style.fontFamily =
            "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif";
          document.body.appendChild(stage);

          // NOTE: props 不足や型不一致は「デモを正しく描けていない」サイン。warn を捕まえて
          //       テストを落とす(必須 prop を持つデモの variant 登録漏れを仕組みで検知)。
          const propWarnings: string[] = [];
          const app = createApp(module.default, variant.props);
          app.config.warnHandler = (msg) => {
            if (/Missing required prop|Invalid prop/.test(msg)) propWarnings.push(msg);
          };
          app.mount(stage);

          await document.fonts.ready;
          await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));

          try {
            expect(propWarnings, `props が不足/不一致: ${demo}/${variant.name}`).toEqual([]);
            await commands.vrtCompare(demo, variant.name, theme);
          } finally {
            app.unmount();
          }
        });
      }
    }
  });
}
