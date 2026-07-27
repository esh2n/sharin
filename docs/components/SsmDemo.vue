<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/ssm(Go)をブラウザに移植。
// 「計算量」: 系列長を伸ばすと SSM 線形 / attention 二次のバーが開く。
// 「選択スキャン」: ゲート付きスキャンで、開いたトークンだけ状態に効く。

const seqLens = [8, 32, 128, 512, 2048];
const clen = ref(2);

const attnOps = computed(() => {
  const n = seqLens[clen.value];
  return n * n;
});
const ssmOps = computed(() => seqLens[clen.value]);
const maxOps = 2048 * 2048;
const fmtOps = (n: number) => (n >= 1e6 ? (n / 1e6).toFixed(1) + "M" : n >= 1000 ? (n / 1000).toFixed(1) + "k" : "" + n);

// 選択スキャン
const DECAY = 0.8;
const X = [4, 4, 4, 4, 4, 4];
const GATE = [1, 0, 0, 1, 0, 0];

interface Frame {
  t: number;
  h: number;
  gate: number;
  note: string;
}

function buildFrames(): Frame[] {
  const out: Frame[] = [{ t: -1, h: 0, gate: 0, note: "状態 h = 0 から開始。入力列は全部同じ 4 だが、ゲート列で取り込みが変わる" }];
  let h = 0;
  for (let t = 0; t < X.length; t++) {
    const g = GATE[t];
    h = DECAY * h + g * 0.1 * X[t];
    out.push({
      t,
      h,
      gate: g,
      note:
        g === 1
          ? `t=${t}: ゲート開(1)。入力 4 を状態に取り込む → h が増える`
          : `t=${t}: ゲート閉(0)。入力を無視し、状態は減衰(×${DECAY})だけ → h が減る`,
    });
  }
  return out;
}
const frames = buildFrames();
const at = ref(0);
const maxH = Math.max(...frames.map((f) => f.h));

const modes = [
  { key: "ops", label: "計算量" },
  { key: "scan", label: "選択スキャン" },
] as const;
const mode = ref<"ops" | "scan">("ops");
function setMode(m: "ops" | "scan") {
  mode.value = m;
  at.value = 0;
}

const cur = computed(() => frames[at.value]);
const note = computed(() =>
  mode.value === "ops"
    ? `系列長 ${seqLens[clen.value]}。attention は全ペアで ${fmtOps(attnOps.value)} 回、SSM は状態更新 ${fmtOps(ssmOps.value)} 回。比は ${seqLens[clen.value]} 倍で、長いほど開く`
    : cur.value.note,
);

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.length - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frames.length - 1; }

const badge = computed(() => (mode.value === "ops" ? `n = ${seqLens[clen.value]}` : cur.value.t < 0 ? "開始" : `t = ${cur.value.t}`));
// 対数バー
const logBar = (n: number) => (Math.log10(Math.max(n, 1)) / Math.log10(maxOps)) * 100;
</script>

<template>
  <DemoShell title="SSM(線形時間と選択)" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
      <span v-if="mode === 'ops'" class="spacer" />
      <span v-if="mode === 'ops'" class="sd-seg">
        <span v-for="(n, i) in seqLens" :key="n" class="sd-seg-opt" :class="{ on: clen === i }" @click="clen = i">{{ n }}</span>
      </span>
    </div>

    <!-- 計算量 -->
    <div v-if="mode === 'ops'" class="ss-ops">
      <div class="ss-op-row">
        <span class="ss-op-label">attention(n²)</span>
        <span class="ss-track"><span class="ss-fill attn" :style="{ width: logBar(attnOps) + '%' }"></span></span>
        <span class="ss-op-val mono">{{ fmtOps(attnOps) }}</span>
      </div>
      <div class="ss-op-row">
        <span class="ss-op-label">SSM(n)</span>
        <span class="ss-track"><span class="ss-fill ssm" :style="{ width: logBar(ssmOps) + '%' }"></span></span>
        <span class="ss-op-val mono">{{ fmtOps(ssmOps) }}</span>
      </div>
      <p class="ss-sub">バーは対数スケール。SSM は 1 個の状態を更新するだけなので、系列長にそのまま線形</p>
    </div>

    <!-- 選択スキャン -->
    <div v-else class="ss-scan">
      <div class="ss-seq">
        <div v-for="(xt, i) in X" :key="i" class="ss-cell" :class="{ open: GATE[i] === 1, active: cur.t === i }">
          <span class="ss-cell-x mono">x={{ xt }}</span>
          <span class="ss-cell-g mono" :class="GATE[i] === 1 ? 'g1' : 'g0'">gate={{ GATE[i] }}</span>
        </div>
      </div>
      <div class="ss-state">
        <span class="ss-state-label">状態 h</span>
        <span class="ss-track"><span class="ss-fill ssm" :style="{ width: (cur.h / maxH) * 100 + '%' }"></span></span>
        <span class="ss-op-val mono">{{ cur.h.toFixed(3) }}</span>
      </div>
    </div>

    <p class="ss-note">{{ note }}</p>

    <div class="ss-foot" v-if="mode === 'scan'">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="ss-nav mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1ステップ進む</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="ss-legend">
      SSM は全ペアを見る代わりに 1 個の状態を系列に沿って更新するので、計算が系列長に線形になる。
      固定の更新則では入力を選り分けられないが、Mamba の選択的 SSM は取り込み量を入力ごとに変えるゲートで
      「重要なトークンだけ状態に残す」を線形コストのまま実現する。
    </p>
  </DemoShell>
</template>

<style scoped>
.ss-ops {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.ss-op-row,
.ss-state {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0;
}
.ss-op-label,
.ss-state-label {
  width: 120px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.ss-track {
  flex: 1;
  height: 14px;
  background-color: var(--vp-c-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.ss-fill {
  display: block;
  height: 100%;
}
.ss-fill.attn { background-color: var(--vp-c-danger-1); }
.ss-fill.ssm { background-color: var(--vp-c-green-1); }
.ss-op-val {
  width: 60px;
  text-align: right;
  font-size: 12px;
}
.ss-sub {
  margin: 8px 0 0;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.ss-scan {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.ss-seq {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.ss-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg);
  padding: 6px 10px;
}
.ss-cell.open {
  border-color: var(--vp-c-green-1);
}
.ss-cell.active {
  box-shadow: 0 0 0 2px var(--vp-c-brand-1);
}
.ss-cell-x {
  font-size: 12px;
  font-weight: 700;
}
.ss-cell-g {
  font-size: 10.5px;
}
.ss-cell-g.g1 { color: var(--vp-c-green-1); }
.ss-cell-g.g0 { color: var(--vp-c-text-3); }
.ss-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.ss-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.ss-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.ss-legend {
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
