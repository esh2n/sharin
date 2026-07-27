<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/rope(Go)の RoPE をブラウザに移植。
// 「回転を見る」: 周波数の違う 3 ペアが、位置とともに別々の速さで回る。
// 「相対性」: 位置 m の Q と位置 n の K の内積が、全体をずらしても変わらない。

const DIM = 8;
const freqs = Array.from({ length: DIM / 2 }, (_, i) => Math.pow(10000, (-2 * i) / DIM));

function applyAt(x: number[], pos: number): number[] {
  const out = [...x];
  for (let i = 0; i * 2 + 1 < x.length && i < freqs.length; i++) {
    const th = pos * freqs[i];
    const sin = Math.sin(th), cos = Math.cos(th);
    const a = x[i * 2], b = x[i * 2 + 1];
    out[i * 2] = a * cos - b * sin;
    out[i * 2 + 1] = a * sin + b * cos;
  }
  return out;
}

function dot(a: number[], b: number[]): number {
  let s = 0;
  for (let i = 0; i < a.length; i++) s += a[i] * b[i];
  return s;
}

// 決定的な固定ベクトル(Go テストの vec と同じ漸化式)。
function vec(dim: number, seed: number): number[] {
  const out: number[] = [];
  let x = seed;
  for (let i = 0; i < dim; i++) {
    x = (x * 7.13 + 0.37) % 1.0;
    out.push(x * 2 - 1);
  }
  return out;
}
const Q = vec(DIM, 0.11);
const K = vec(DIM, 0.83);

// --- 回転を見る ---
const POSITIONS = [0, 1, 2, 3, 4, 5, 6, 7];
// 表示する 3 ペア: 高周波(ペア0)・中間(ペア1)・低周波(ペア3)
const shownPairs = [
  { idx: 0, label: "ペア0(高周波)", cls: "hi" },
  { idx: 1, label: "ペア1(中間)", cls: "mid" },
  { idx: 3, label: "ペア3(低周波)", cls: "lo" },
];

// --- 相対性 ---
const SHIFTS = [0, 1, 10, 100, 1000];
const M0 = 9, N0 = 5;

const modes = [
  { key: "rotate", label: "回転を見る" },
  { key: "relative", label: "相対性" },
] as const;
const mode = ref<"rotate" | "relative">("rotate");
const at = ref(0);
function setMode(m: "rotate" | "relative") {
  mode.value = m;
  at.value = 0;
}

const frameCount = computed(() => (mode.value === "rotate" ? POSITIONS.length : SHIFTS.length));

const pos = computed(() => POSITIONS[at.value]);
const RADII = [52, 42, 32]; // 重なっても全ペア見えるよう半径をずらす
const arrows = computed(() =>
  shownPairs.map((p, i) => {
    const th = pos.value * freqs[p.idx];
    return { ...p, r: RADII[i], x: Math.cos(th), y: Math.sin(th), deg: ((th * 180) / Math.PI) % 360 };
  }),
);

const shift = computed(() => SHIFTS[at.value]);
const rel = computed(() => {
  const s = shift.value;
  const score4 = dot(applyAt(Q, M0 + s), applyAt(K, N0 + s));
  const score1 = dot(applyAt(Q, N0 + 1 + s), applyAt(K, N0 + s));
  return { m: M0 + s, n: N0 + s, score4, score1 };
});

const note = computed(() => {
  if (mode.value === "rotate") {
    if (pos.value === 0) return "位置 0 では全ペアが回転ゼロ(恒等)。ここから位置が進むごとに、各ペアが自分の周波数ぶんだけ回る";
    return `位置 ${pos.value}。高周波のペア0 は 1 位置で約 57° 回り、低周波のペア3 はほぼ動かない。秒針と時針のように、1 つのベクトルが目盛りの違う複数の回転を同時に持つ`;
  }
  if (shift.value === 0) return "位置 9 の Q と位置 5 の K の内積(間隔 4)。ここから両方を同じだけ後ろへずらしていく";
  return `全体を +${shift.value} ずらした。絶対位置は (${rel.value.m}, ${rel.value.n}) に変わったが、間隔 4 のスコアは 1 桁も動かない。間隔 1 のスコアも同様に不変で、両者は異なる値のまま`;
});

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frameCount.value - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frameCount.value - 1; }

const badge = computed(() => (mode.value === "rotate" ? `位置 ${pos.value}` : `ずらし +${shift.value}`));
const fmt = (x: number) => x.toFixed(4);
</script>

<template>
  <DemoShell title="RoPE(回転位置埋め込み)" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="m in modes"
          :key="m.key"
          class="sd-seg-opt"
          :class="{ on: mode === m.key }"
          @click="setMode(m.key)"
          >{{ m.label }}</span
        >
      </span>
    </div>

    <!-- 回転を見る -->
    <div v-if="mode === 'rotate'" class="rp-panel">
      <svg viewBox="-70 -70 140 140" class="rp-svg" aria-label="回転の可視化">
        <circle cx="0" cy="0" r="56" class="rp-circle" />
        <line x1="-64" y1="0" x2="64" y2="0" class="rp-axis" />
        <line x1="0" y1="-64" x2="0" y2="64" class="rp-axis" />
        <g v-for="a in arrows" :key="a.idx">
          <line :x2="a.x * a.r" :y2="-a.y * a.r" x1="0" y1="0" class="rp-arrow" :class="a.cls" />
          <circle :cx="a.x * a.r" :cy="-a.y * a.r" r="3.5" class="rp-tip" :class="a.cls" />
        </g>
      </svg>
      <div class="rp-side">
        <div v-for="a in arrows" :key="a.idx" class="rp-freq" :class="a.cls">
          <span class="rp-freq-label">{{ a.label }}</span>
          <span class="mono">freq {{ freqs[a.idx].toFixed(3) }} / 角度 {{ a.deg.toFixed(0) }}°</span>
        </div>
      </div>
    </div>

    <!-- 相対性 -->
    <div v-else class="rp-rel">
      <div class="rp-rel-row head">
        <span>Q の位置 m</span><span>K の位置 n</span><span>間隔 4 のスコア</span><span>間隔 1 のスコア</span>
      </div>
      <div class="rp-rel-row mono">
        <span>{{ rel.m }}</span>
        <span>{{ rel.n }}</span>
        <span class="rp-score">{{ fmt(rel.score4) }}</span>
        <span class="rp-score alt">{{ fmt(rel.score1) }}</span>
      </div>
      <p class="rp-rel-note">スコア = dot(R(m)·q, R(n)·k)。位置が動いても間隔が同じなら同じ値になる</p>
    </div>

    <p class="rp-note">{{ note }}</p>

    <div class="rp-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="rp-nav mono">{{ at + 1 }} / {{ frameCount }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="rp-legend">
      RoPE は Q と K の次元ペアを位置に比例した角度で回す。回転の内積は角度差だけで決まるので、
      attention スコアは 2 トークンの間隔のみに依存する。「5 語前を見る」パターンが
      文のどこにいても同じに働くのはこの性質による。
    </p>
  </DemoShell>
</template>

<style scoped>
.rp-panel {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 14px;
  display: flex;
  align-items: center;
  gap: 18px;
  flex-wrap: wrap;
}
.rp-svg {
  width: 190px;
  height: 190px;
  flex: none;
}
.rp-circle {
  fill: none;
  stroke: var(--vp-c-divider);
}
.rp-axis {
  stroke: var(--vp-c-divider);
  stroke-dasharray: 3 3;
}
.rp-arrow {
  stroke-width: 2.5;
}
.rp-arrow.hi, .rp-tip.hi { stroke: var(--vp-c-brand-1); fill: var(--vp-c-brand-1); }
.rp-arrow.mid, .rp-tip.mid { stroke: var(--vp-c-purple-1); fill: var(--vp-c-purple-1); }
.rp-arrow.lo, .rp-tip.lo { stroke: var(--vp-c-green-1); fill: var(--vp-c-green-1); }
.rp-side {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 220px;
}
.rp-freq {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
  color: var(--vp-c-text-2);
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 4px 10px;
}
.rp-freq.hi { border-left-color: var(--vp-c-brand-1); }
.rp-freq.mid { border-left-color: var(--vp-c-purple-1); }
.rp-freq.lo { border-left-color: var(--vp-c-green-1); }
.rp-freq-label {
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.rp-rel {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.rp-rel-row {
  display: grid;
  grid-template-columns: 1fr 1fr 1.4fr 1.4fr;
  gap: 8px;
  padding: 10px 14px;
  font-size: 14px;
}
.rp-rel-row.head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.rp-score {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.rp-score.alt {
  color: var(--vp-c-purple-1);
}
.rp-rel-note {
  margin: 0;
  padding: 0 14px 10px;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.rp-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.rp-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.rp-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.rp-legend {
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
