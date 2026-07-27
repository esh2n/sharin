<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/layers(Go)の RMSNorm / SwiGLU をブラウザに移植。
// 「正規化」: 同じ入力を ×2 / +2 して、LayerNorm と RMSNorm の不変性の違いを見る。
// 「ゲート」: SwiGLU の 1 チャネルで、ゲート入力が値の通過量を決める様子を見る。

const BASE = [0.5, -1.5, 2.0, 1.0];

function layerNorm(x: number[]): number[] {
  const mean = x.reduce((a, b) => a + b, 0) / x.length;
  const varr = x.reduce((a, b) => a + (b - mean) ** 2, 0) / x.length;
  const inv = 1 / Math.sqrt(varr + 1e-6);
  return x.map((v) => (v - mean) * inv);
}
function rmsNorm(x: number[]): number[] {
  const ms = x.reduce((a, b) => a + b * b, 0) / x.length;
  const inv = 1 / Math.sqrt(ms + 1e-6);
  return x.map((v) => v * inv);
}
const silu = (v: number) => v / (1 + Math.exp(-v));

const normFrames = [
  { label: "元の入力", input: BASE, note: "基準となる入力。ここから入力をいじって、2 つの正規化の出力がどう動くかを見る" },
  {
    label: "×2(スケール)",
    input: BASE.map((v) => v * 2),
    note: "入力を 2 倍しても、どちらの出力も変わらない。分母(標準偏差 / RMS)も 2 倍になって打ち消すから。これが正規化の本来の役目",
  },
  {
    label: "+2(シフト)",
    input: BASE.map((v) => v + 2),
    note: "全要素に +2 すると、平均を引く LayerNorm は不変のまま、引かない RMSNorm は出力が変わる。RMSNorm はシフトの補正を放棄し、そのぶん計算を省いている",
  },
];

const GATES = [-6, -4, -2, 0, 2, 4, 6];
const VALUE = 5;

const modes = [
  { key: "norm", label: "正規化" },
  { key: "gate", label: "ゲート" },
] as const;
const mode = ref<"norm" | "gate">("norm");
const at = ref(0);
function setMode(m: "norm" | "gate") {
  mode.value = m;
  at.value = 0;
}

const frameCount = computed(() => (mode.value === "norm" ? normFrames.length : GATES.length));

const nf = computed(() => {
  const f = normFrames[at.value];
  return {
    ...f,
    ln: layerNorm(f.input),
    rms: rmsNorm(f.input),
    baseLn: layerNorm(BASE),
    baseRms: rmsNorm(BASE),
  };
});
const changed = (v: number, base: number) => Math.abs(v - base) > 1e-4;

const gf = computed(() => {
  const g = GATES[at.value];
  const s = silu(g);
  return { g, s, out: s * VALUE };
});
const gateNote = computed(() => {
  const { g, s } = gf.value;
  if (g <= -4) return "ゲート側が強い負 → SiLU がほぼ 0 → 値側が 5 を出していても、このチャネルはほぼ閉じる";
  if (g >= 4) return "ゲート側が強い正 → SiLU がほぼ素通し → 値側の 5 がゲート値倍されて流れる";
  return `ゲート入力 ${g} では SiLU(${g}) = ${s.toFixed(3)}。閉と開の間が滑らかにつながっていて、どれだけ通すかを入力ごとに変えられる`;
});

const note = computed(() => (mode.value === "norm" ? nf.value.note : gateNote.value));

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frameCount.value - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frameCount.value - 1; }

const badge = computed(() => (mode.value === "norm" ? normFrames[at.value].label : `gate = ${gf.value.g}`));
const fmt = (v: number) => (v >= 0 ? " " : "") + v.toFixed(3);
</script>

<template>
  <DemoShell title="RMSNorm / SwiGLU" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
    </div>

    <!-- 正規化 -->
    <div v-if="mode === 'norm'" class="ly-table">
      <div class="ly-row head">
        <span>系列</span><span v-for="i in 4" :key="i" class="num">x{{ i - 1 }}</span>
      </div>
      <div class="ly-row">
        <span>入力</span>
        <span v-for="(v, i) in nf.input" :key="i" class="num mono" :class="{ chg: changed(v, BASE[i]) }">{{ fmt(v) }}</span>
      </div>
      <div class="ly-row">
        <span>LayerNorm</span>
        <span v-for="(v, i) in nf.ln" :key="i" class="num mono" :class="{ chg: changed(v, nf.baseLn[i]) }">{{ fmt(v) }}</span>
      </div>
      <div class="ly-row">
        <span>RMSNorm</span>
        <span v-for="(v, i) in nf.rms" :key="i" class="num mono" :class="{ chg: changed(v, nf.baseRms[i]) }">{{ fmt(v) }}</span>
      </div>
      <p class="ly-sub">色付きの値 = 「元の入力」のときの出力から変わった値</p>
    </div>

    <!-- ゲート -->
    <div v-else class="ly-gate">
      <div class="ly-gate-flow">
        <div class="ly-cell">
          <span class="ly-cell-label">ゲート側の入力</span>
          <span class="ly-cell-val mono">{{ gf.g }}</span>
        </div>
        <span class="ly-arrow mono">→ SiLU →</span>
        <div class="ly-cell">
          <span class="ly-cell-label">ゲート係数</span>
          <span class="ly-cell-val mono">{{ gf.s.toFixed(3) }}</span>
        </div>
        <span class="ly-arrow mono">×</span>
        <div class="ly-cell">
          <span class="ly-cell-label">値側(固定)</span>
          <span class="ly-cell-val mono">{{ VALUE }}</span>
        </div>
        <span class="ly-arrow mono">=</span>
        <div class="ly-cell out">
          <span class="ly-cell-label">出力</span>
          <span class="ly-cell-val mono">{{ gf.out.toFixed(3) }}</span>
        </div>
      </div>
      <div class="ly-bar">
        <span class="ly-bar-label">通過量</span>
        <span class="ly-track"><span class="ly-fill" :style="{ width: Math.min(Math.max((gf.out / (VALUE * 6)) * 100, 0), 100) + '%' }"></span></span>
      </div>
    </div>

    <p class="ly-note">{{ note }}</p>

    <div class="ly-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="ly-nav mono">{{ at + 1 }} / {{ frameCount }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="ly-legend">
      RMSNorm は LayerNorm から平均減算を省いた簡略化で、違いはシフト不変性の有無に現れる。
      SwiGLU は FFN の各チャネルにゲートを付け、入力に応じて通過量を変える。
      どちらも 1 箇所の差は小さいが、層数と学習規模に掛け算されて効く部品になっている。
    </p>
  </DemoShell>
</template>

<style scoped>
.ly-table {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 6px 0 0;
}
.ly-row {
  display: grid;
  grid-template-columns: 110px repeat(4, 1fr);
  gap: 6px;
  padding: 6px 12px;
  font-size: 13px;
  align-items: center;
}
.ly-row.head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.ly-row > span:first-child {
  color: var(--vp-c-text-2);
  font-size: 12px;
}
.num {
  text-align: right;
  white-space: pre;
}
.num.chg {
  color: var(--vp-c-warning-1);
  font-weight: 700;
}
.ly-sub {
  margin: 4px 0 0;
  padding: 0 12px 10px;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.ly-gate {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.ly-gate-flow {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ly-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg);
  padding: 8px 12px;
}
.ly-cell.out {
  border-color: var(--vp-c-brand-1);
}
.ly-cell-label {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.ly-cell-val {
  font-size: 15px;
  font-weight: 700;
}
.ly-arrow {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.ly-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
}
.ly-bar-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.ly-track {
  flex: 1;
  height: 12px;
  background-color: var(--vp-c-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.ly-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.ly-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.ly-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.ly-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.ly-legend {
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
