<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/embed(Go)をブラウザに移植。
// 「意味検索」: 2次元に配置した単語ベクトルにクエリを当て、コサイン順で並べる。
// 「総当たりの限界」: 件数を増やすと比較回数が線形に膨らみ ANN が要る。

// 2次元の単語ベクトル(向きが意味を表すおもちゃ)
const words = [
  { key: "cat", vec: [0.9, 0.3] },
  { key: "kitten", vec: [0.85, 0.45] },
  { key: "dog", vec: [0.7, 0.5] },
  { key: "puppy", vec: [0.65, 0.62] },
  { key: "car", vec: [0.2, 0.95] },
  { key: "truck", vec: [0.1, 0.98] },
];

// クエリのプリセット(角度で表現)
const queries = [
  { label: "猫っぽく", vec: [0.9, 0.3] },
  { label: "犬っぽく", vec: [0.68, 0.55] },
  { label: "乗り物っぽく", vec: [0.15, 0.96] },
];
const qPick = ref(0);

function cosine(a: number[], b: number[]): number {
  let dot = 0, na = 0, nb = 0;
  for (let i = 0; i < a.length; i++) {
    dot += a[i] * b[i];
    na += a[i] * a[i];
    nb += b[i] * b[i];
  }
  return na === 0 || nb === 0 ? 0 : dot / (Math.sqrt(na) * Math.sqrt(nb));
}

const query = computed(() => queries[qPick.value].vec);
const ranked = computed(() =>
  words
    .map((w) => ({ key: w.key, score: cosine(query.value, w.vec), vec: w.vec }))
    .sort((a, b) => b.score - a.score),
);

// SVG 座標(0..1 を 0..160 に、y は反転)
const sx = (x: number) => x * 150 + 5;
const sy = (y: number) => 160 - y * 150 - 5;

// --- 総当たりの限界 ---
const counts = [1000, 100000, 10000000, 1000000000];
const cntPick = ref(0);
const fmtN = (n: number) => (n >= 1e9 ? (n / 1e9).toFixed(0) + "B" : n >= 1e6 ? (n / 1e6).toFixed(0) + "M" : n >= 1000 ? (n / 1000).toFixed(0) + "k" : "" + n);
// 1回の比較を仮に 100ns として総当たり時間
const bruteMs = (n: number) => (n * 100) / 1e6;
const annMs = (n: number) => (Math.log2(n) * 5000 * 100) / 1e6; // log n × 定数

const modes = [
  { key: "search", label: "意味検索" },
  { key: "scale", label: "総当たりの限界" },
] as const;
const mode = ref<"search" | "scale">("search");

const curCount = computed(() => counts[cntPick.value]);
const badge = computed(() => (mode.value === "search" ? queries[qPick.value].label : fmtN(curCount.value)));
const note = computed(() => {
  if (mode.value === "search") {
    const top = ranked.value[0];
    return `クエリ「${queries[qPick.value].label}」に最も近いのは ${top.key}(類似度 ${top.score.toFixed(2)})。キーワードは一致していないのに、ベクトルの向きの近さで意味的に並ぶ。これが同義・言い換えを拾える理由`;
  }
  const bm = bruteMs(curCount.value);
  const am = annMs(curCount.value);
  return `${fmtN(curCount.value)} 件のとき、総当たりは 1 クエリ ${bm >= 1000 ? (bm / 1000).toFixed(1) + "秒" : bm.toFixed(1) + "ms"}(件数に線形)。ANN は約 ${am.toFixed(1)}ms(log に比例)。${curCount.value >= 1e7 ? "この規模では総当たりは実用にならず、ANN が必須" : "小規模なら総当たりでも足りる"}`;
});
</script>

<template>
  <DemoShell title="埋め込みとベクトル検索" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="mode = m.key">{{ m.label }}</span>
      </span>
      <span class="spacer" />
      <span v-if="mode === 'search'" class="sd-seg">
        <span v-for="(q, i) in queries" :key="q.label" class="sd-seg-opt" :class="{ on: qPick === i }" @click="qPick = i">{{ q.label }}</span>
      </span>
      <span v-else class="sd-seg">
        <span v-for="(c, i) in counts" :key="c" class="sd-seg-opt" :class="{ on: cntPick === i }" @click="cntPick = i">{{ fmtN(c) }}</span>
      </span>
    </div>

    <!-- 意味検索 -->
    <div v-if="mode === 'search'" class="em-search">
      <svg viewBox="0 0 165 165" class="em-svg" aria-label="意味空間">
        <line x1="5" y1="160" x2="160" y2="160" class="em-axis" />
        <line x1="5" y1="160" x2="5" y2="5" class="em-axis" />
        <!-- クエリ方向 -->
        <line x1="5" y1="160" :x2="sx(query[0])" :y2="sy(query[1])" class="em-query-line" />
        <g v-for="w in ranked" :key="w.key">
          <circle :cx="sx(w.vec[0])" :cy="sy(w.vec[1])" r="4" class="em-dot" :class="{ top: w.key === ranked[0].key }" />
          <text :x="sx(w.vec[0]) + 6" :y="sy(w.vec[1]) + 3" class="em-label">{{ w.key }}</text>
        </g>
      </svg>
      <div class="em-rank">
        <div class="em-rank-head">類似度ランキング</div>
        <div v-for="(w, i) in ranked" :key="w.key" class="em-rank-row" :class="{ top: i === 0 }">
          <span class="em-rank-key mono">{{ i + 1 }}. {{ w.key }}</span>
          <span class="em-rank-track"><span class="em-rank-fill" :style="{ width: Math.max(w.score, 0) * 100 + '%' }"></span></span>
          <span class="em-rank-score mono">{{ w.score.toFixed(2) }}</span>
        </div>
      </div>
    </div>

    <!-- 総当たりの限界 -->
    <div v-else class="em-scale">
      <div class="em-scale-row">
        <span class="em-scale-label">総当たり O(n)</span>
        <span class="em-track"><span class="em-fill brute" :style="{ width: Math.min(bruteMs(curCount) / bruteMs(1e9) * 100, 100) + '%' }"></span></span>
        <span class="em-scale-val mono">{{ bruteMs(curCount) >= 1000 ? (bruteMs(curCount) / 1000).toFixed(1) + 's' : bruteMs(curCount).toFixed(1) + 'ms' }}</span>
      </div>
      <div class="em-scale-row">
        <span class="em-scale-label">ANN O(log n)</span>
        <span class="em-track"><span class="em-fill ann" :style="{ width: Math.min(annMs(curCount) / bruteMs(1e9) * 100, 100) + '%' }"></span></span>
        <span class="em-scale-val mono">{{ annMs(curCount).toFixed(1) }}ms</span>
      </div>
      <p class="em-sub">1 クエリあたりの検索時間(1 比較 = 100ns 仮定)。総当たりは件数に比例して伸びる</p>
    </div>

    <p class="em-note">{{ note }}</p>

    <p class="em-legend">
      文をベクトルにすると、意味の近さが向きの近さ(コサイン類似度)で測れる。キーワード一致でなく
      意味で引けるので RAG の検索に使う。ただし総当たりは件数に線形で billion 件では破綻するため、
      実物は近似最近傍(HNSW など)で全件を見ずに近傍へたどり着く。
    </p>
  </DemoShell>
</template>

<style scoped>
.em-search {
  margin-top: 14px;
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
  align-items: flex-start;
}
.em-svg {
  width: 200px;
  height: 200px;
  flex: none;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.em-axis {
  stroke: var(--vp-c-divider);
}
.em-query-line {
  stroke: var(--vp-c-brand-1);
  stroke-width: 1.5;
  stroke-dasharray: 3 2;
}
.em-dot {
  fill: var(--vp-c-text-3);
}
.em-dot.top {
  fill: var(--vp-c-brand-1);
}
.em-label {
  font-size: 8px;
  fill: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
}
.em-rank {
  flex: 1;
  min-width: 220px;
}
.em-rank-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  margin-bottom: 6px;
}
.em-rank-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 2px 4px;
  border-left: 3px solid transparent;
  border-radius: 0;
}
.em-rank-row.top {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg-soft);
}
.em-rank-key {
  width: 72px;
  font-size: 11.5px;
}
.em-rank-track {
  flex: 1;
  height: 10px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.em-rank-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.em-rank-score {
  width: 34px;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.em-scale {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.em-scale-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0;
}
.em-scale-label {
  width: 110px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.em-track {
  flex: 1;
  height: 14px;
  background-color: var(--vp-c-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.em-fill {
  display: block;
  height: 100%;
}
.em-fill.brute { background-color: var(--vp-c-danger-1); }
.em-fill.ann { background-color: var(--vp-c-green-1); }
.em-scale-val {
  width: 60px;
  text-align: right;
  font-size: 11.5px;
}
.em-sub {
  margin: 8px 0 0;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.em-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.em-legend {
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
