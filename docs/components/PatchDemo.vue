<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/patch(Go)を可視化。
// 「パッチ化」: 画像を格子に切り、各パッチ=1トークン。サイズでトークン数が変わる。
// 「トークン会計」: 画像・動画のトークン数と計算コストを見積もる。

// --- パッチ化 ---
const IMG = 8; // 8×8 のおもちゃ画像
const patchSizes = [2, 4];
const psPick = ref(0);
const patchSize = computed(() => patchSizes[psPick.value]);
const perSide = computed(() => IMG / patchSize.value);
const numPatches = computed(() => perSide.value * perSide.value);

// 決定的な画像パターン(市松+グラデ)
function pixel(r: number, c: number): number {
  return ((r * 7 + c * 5) % 11) / 11;
}
const gridCells = computed(() => {
  const cells: { r: number; c: number; v: number; patch: number }[] = [];
  for (let r = 0; r < IMG; r++) {
    for (let c = 0; c < IMG; c++) {
      const pr = Math.floor(r / patchSize.value);
      const pc = Math.floor(c / patchSize.value);
      cells.push({ r, c, v: pixel(r, c), patch: pr * perSide.value + pc });
    }
  }
  return cells;
});

// --- トークン会計 ---
const scenarios = [
  { label: "画像1枚(224²)", dim: 224, patch: 16, frames: 1 },
  { label: "画像 patch8", dim: 224, patch: 8, frames: 1 },
  { label: "動画10秒 30fps", dim: 224, patch: 16, frames: 300 },
  { label: "動画60秒 30fps", dim: 224, patch: 16, frames: 1800 },
];
const scPick = ref(0);
const cur = computed(() => scenarios[scPick.value]);
const perImg = computed(() => {
  const n = cur.value.dim / cur.value.patch;
  return n * n;
});
const totalTokens = computed(() => perImg.value * cur.value.frames);
const attnCost = computed(() => totalTokens.value * totalTokens.value);
const fmtN = (n: number) => {
  if (n >= 1e12) return (n / 1e12).toFixed(1) + "兆";
  if (n >= 1e8) return (n / 1e8).toFixed(1) + "億";
  if (n >= 1e4) return (n / 1e4).toFixed(1) + "万";
  return "" + n;
};

const modes = [
  { key: "patch", label: "パッチ化" },
  { key: "budget", label: "トークン会計" },
] as const;
const mode = ref<"patch" | "budget">("patch");

const patchColors = ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p"];

const note = computed(() => {
  if (mode.value === "patch") {
    return `8×8 画像を ${patchSize.value}×${patchSize.value} パッチに切ると ${numPatches.value} 個のパッチ = ${numPatches.value} トークン。各パッチを平坦化して線形射影すれば、あとはテキストと同じトークン列。パッチを細かくするほどトークンが増える`;
  }
  return `${cur.value.label}: 1枚あたり ${perImg.value} トークン × ${cur.value.frames} フレーム = ${fmtN(totalTokens.value)} トークン。attention は n² なので計算量は ${fmtN(attnCost.value)}。${totalTokens.value > 100000 ? "長い動画はすぐ100万トークン窓を食い潰す" : "画像1枚なら軽いが、動画で跳ね上がる"}`;
});
</script>

<template>
  <DemoShell title="マルチモーダル入口(パッチ化)" badge-tone="neutral" :badge="mode === 'patch' ? numPatches + ' トークン' : fmtN(totalTokens)">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="mode = m.key">{{ m.label }}</span>
      </span>
      <span class="spacer" />
      <span v-if="mode === 'patch'" class="sd-seg">
        <span v-for="(p, i) in patchSizes" :key="p" class="sd-seg-opt" :class="{ on: psPick === i }" @click="psPick = i">{{ p }}×{{ p }}</span>
      </span>
    </div>

    <!-- パッチ化 -->
    <div v-if="mode === 'patch'" class="pa-patch">
      <div class="pa-grid" :style="{ '--n': IMG }">
        <div
          v-for="(cell, i) in gridCells"
          :key="i"
          class="pa-cell"
          :class="[
            'p-' + patchColors[cell.patch % patchColors.length],
            {
              'edge-r': (cell.c + 1) % patchSize === 0 && cell.c < IMG - 1,
              'edge-b': (cell.r + 1) % patchSize === 0 && cell.r < IMG - 1,
            },
          ]"
          :style="{ opacity: 0.35 + cell.v * 0.55 }"
        ></div>
      </div>
      <div class="pa-arrow mono">→ {{ numPatches }} トークン →</div>
      <div class="pa-tokens">
        <span v-for="n in numPatches" :key="n" class="pa-tok mono" :class="'p-' + patchColors[(n - 1) % patchColors.length]">P{{ n }}</span>
      </div>
    </div>

    <!-- トークン会計 -->
    <div v-else class="pa-budget">
      <div class="pa-sc-seg">
        <span v-for="(s, i) in scenarios" :key="s.label" class="pa-sc" :class="{ on: scPick === i }" @click="scPick = i">{{ s.label }}</span>
      </div>
      <div class="pa-calc">
        <div class="pa-calc-row">
          <span class="pa-calc-label">1枚のトークン</span>
          <span class="pa-calc-val mono">{{ perImg }}</span>
          <span class="pa-calc-note">({{ cur.dim }}/{{ cur.patch }})²</span>
        </div>
        <div class="pa-calc-row">
          <span class="pa-calc-label">× フレーム数</span>
          <span class="pa-calc-val mono">{{ cur.frames }}</span>
        </div>
        <div class="pa-calc-row total">
          <span class="pa-calc-label">総トークン</span>
          <span class="pa-calc-val mono">{{ fmtN(totalTokens) }}</span>
          <span class="pa-calc-note">窓100万に対し {{ (totalTokens / 1e6 * 100).toFixed(0) }}%</span>
        </div>
      </div>
    </div>

    <p class="pa-note">{{ note }}</p>

    <p class="pa-legend">
      画像を格子パッチに切り、各パッチを線形射影すれば、テキストと同じトークン列になる。
      あとは Transformer が種類を区別せず処理する。パッチの細かさと動画の長さがトークン数を決め、
      それがそのままマルチモーダルの計算コスト(attention は n²)になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.pa-patch {
  margin-top: 14px;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: center;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 16px;
  background-color: var(--vp-c-bg-soft);
}
.pa-grid {
  display: grid;
  grid-template-columns: repeat(var(--n), 18px);
  gap: 0;
  border: 2px solid var(--vp-c-text-3);
}
.pa-cell {
  width: 18px;
  height: 18px;
}
.pa-cell.edge-r {
  border-right: 1.5px solid var(--vp-c-bg);
}
.pa-cell.edge-b {
  border-bottom: 1.5px solid var(--vp-c-bg);
}
.pa-arrow {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.pa-tokens {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  max-width: 180px;
}
.pa-tok {
  font-size: 10px;
  padding: 3px 6px;
  border-radius: 3px;
  color: var(--vp-c-bg);
  font-weight: 700;
}
/* パッチ色(彩度を抑えたパレット) */
.p-a { background-color: #5b8fb0; } .p-b { background-color: #7a6c9c; }
.p-c { background-color: #4f9d7f; } .p-d { background-color: #b08a5b; }
.p-e { background-color: #a05b7a; } .p-f { background-color: #5b7ab0; }
.p-g { background-color: #9c7a6c; } .p-h { background-color: #6c9c5b; }
.p-i { background-color: #8f5bb0; } .p-j { background-color: #b05b5b; }
.p-k { background-color: #5bb0a0; } .p-l { background-color: #8b8b4f; }
.p-m { background-color: #5b6cb0; } .p-n { background-color: #b07a5b; }
.p-o { background-color: #7ab05b; } .p-p { background-color: #a05b9c; }
.pa-budget {
  margin-top: 14px;
}
.pa-sc-seg {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.pa-sc {
  font-size: 11.5px;
  padding: 3px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  cursor: pointer;
  color: var(--vp-c-text-2);
}
.pa-sc.on {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.pa-calc {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 14px;
  background-color: var(--vp-c-bg-soft);
}
.pa-calc-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 4px 0;
}
.pa-calc-row.total {
  border-top: 1px solid var(--vp-c-divider);
  margin-top: 4px;
  padding-top: 8px;
}
.pa-calc-label {
  width: 110px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.pa-calc-val {
  font-size: 15px;
  font-weight: 700;
  min-width: 60px;
}
.pa-calc-row.total .pa-calc-val {
  color: var(--vp-c-brand-1);
}
.pa-calc-note {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.pa-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.pa-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.mono {
  font-family: var(--vp-font-family-mono);
}
</style>
