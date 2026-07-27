<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// observability/metrics(Go)をブラウザに移植。
// 応答時間の分布をヒストグラムに数え、平均と p50/p90/p99 がどこに来るかを見る。
// 遅いリクエストの割合を変えると、平均はあまり動かないのに p99 が跳ね上がる。

const BOUNDS = [2, 5, 10, 25, 50, 100, 250, 500, 1000]; // ms のバケット上限
const N = 1000; // リクエスト数

// 決定的擬似乱数(RetryDemo と同じ LCG)。
function makeRand(seed: number) {
  let s = BigInt(seed) * 2862933555777941757n + 1n;
  return () => {
    s = (s * 6364136223846793005n + 1442695040888963407n) & 0xffffffffffffffffn;
    return Number((s >> 33n) & 0xffffffffn);
  };
}

const slowOptions = [
  { pct: 0, label: "遅い 0%" },
  { pct: 2, label: "遅い 2%" },
  { pct: 5, label: "遅い 5%" },
  { pct: 10, label: "遅い 10%" },
] as const;
const slowPick = ref(1); // 既定 2%
const slowPct = computed(() => slowOptions[slowPick.value].pct);

// バケットに数える + sum/total を作る。
type Hist = { counts: number[]; sum: number; total: number };
function build(pct: number): Hist {
  const counts = new Array(BOUNDS.length + 1).fill(0);
  let sum = 0;
  const rand = makeRand(20260727);
  for (let i = 0; i < N; i++) {
    const isSlow = rand() % 1000 < pct * 10;
    const x = isSlow ? 300 + (rand() % 600) : 2 + (rand() % 8); // 遅い:300-899, 速い:2-9ms
    let b = 0;
    while (b < BOUNDS.length && x > BOUNDS[b]) b++;
    counts[b]++;
    sum += x;
  }
  return { counts, sum, total: N };
}

// Go の Quantile と同じ:累積 + バケット内線形補間、+Inf は最大有限上限で頭打ち。
function quantile(h: Hist, q: number): number {
  if (h.total === 0) return 0;
  const rank = q * h.total;
  let cum = 0;
  for (let i = 0; i < h.counts.length; i++) {
    const next = cum + h.counts[i];
    if (next >= rank) {
      if (i === BOUNDS.length) return BOUNDS[BOUNDS.length - 1];
      const lower = i > 0 ? BOUNDS[i - 1] : 0;
      const upper = BOUNDS[i];
      const frac = (rank - cum) / h.counts[i];
      return lower + (upper - lower) * frac;
    }
    cum = next;
  }
  return BOUNDS[BOUNDS.length - 1];
}

const hist = computed(() => build(slowPct.value));
const maxBar = computed(() => Math.max(...hist.value.counts, 1));
const mean = computed(() => hist.value.sum / hist.value.total);
const p50 = computed(() => quantile(hist.value, 0.5));
const p90 = computed(() => quantile(hist.value, 0.9));
const p99 = computed(() => quantile(hist.value, 0.99));

const labels = computed(() => [...BOUNDS.map((b) => "≤" + b), "+∞"]);

const badge = computed(() => `p99 ${Math.round(p99.value)}ms`);

// p99 が平均の何倍か。テールが埋もれているかの目安。
const tailRatio = computed(() => p99.value / Math.max(mean.value, 0.01));
const note = computed(() => {
  if (slowPct.value === 0)
    return `全リクエストが速い(2-9ms)。平均 ${mean.value.toFixed(1)}ms と p99 ${Math.round(p99.value)}ms がほぼ一致。分布が揃っていれば平均でも困らない`;
  return `遅いのは ${slowPct.value}% だけ。平均は ${mean.value.toFixed(1)}ms で大きく動かないのに、p99 は ${Math.round(p99.value)}ms(平均の ${tailRatio.value.toFixed(0)} 倍)。少数の遅さが平均に埋もれ、p99 にだけ現れる`;
});

function fmt(x: number): string {
  return x >= 100 ? Math.round(x).toString() : x.toFixed(1);
}
const stats = computed(() => [
  { key: "平均", val: fmt(mean.value), hot: false },
  { key: "p50", val: fmt(p50.value), hot: false },
  { key: "p90", val: fmt(p90.value), hot: false },
  { key: "p99", val: fmt(p99.value), hot: tailRatio.value > 5 },
]);
</script>

<template>
  <DemoShell title="メトリクスとヒストグラム" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="(o, i) in slowOptions"
          :key="o.pct"
          class="sd-seg-opt"
          :class="{ on: slowPick === i }"
          @click="slowPick = i"
          >{{ o.label }}</span
        >
      </span>
    </div>

    <div class="mt-chart">
      <div class="mt-bars">
        <div v-for="(c, i) in hist.counts" :key="i" class="mt-col">
          <span class="mt-count mono" v-if="c > 0">{{ c }}</span>
          <span
            class="mt-bar"
            :class="{ slow: i >= 6 && c > 0 }"
            :style="{ height: (c / maxBar) * 100 + '%' }"
          ></span>
          <span class="mt-label mono">{{ labels[i] }}</span>
        </div>
      </div>
      <div class="mt-axis">応答時間(ms)のバケット →</div>
    </div>

    <div class="mt-stats">
      <div v-for="s in stats" :key="s.key" class="mt-stat" :class="{ hot: s.hot }">
        <span class="mt-stat-k">{{ s.key }}</span>
        <span class="mt-stat-v mono">{{ s.val }}<small>ms</small></span>
      </div>
    </div>

    <p class="mt-note">{{ note }}</p>

    <p class="mt-legend">
      1000 件の応答時間をバケットに数えた分布。速いリクエスト(左の山)と遅いリクエスト(右端)。
      遅い割合を上げると、平均はほとんど動かないのに p99 が跳ね上がる。ユーザが体感するのは
      遅い方なので、応答時間は平均でなく p99 のような分位点で見る。分位点は各台のバケットを
      合算してから出す(各台の p99 を平均しても全体の p99 にはならない)。
    </p>
  </DemoShell>
</template>

<style scoped>
.mt-chart {
  margin-top: 16px;
}
.mt-bars {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  height: 150px;
}
.mt-col {
  flex: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
}
.mt-count {
  font-size: 10px;
  color: var(--vp-c-text-3);
  margin-bottom: 2px;
}
.mt-bar {
  width: 100%;
  background-color: var(--vp-c-brand-1);
  border-radius: 0;
  min-height: 1px;
  transition: height 0.15s;
}
.mt-bar.slow {
  background-color: var(--vp-c-danger-1);
}
.mt-label {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  margin-top: 3px;
  white-space: nowrap;
}
.mt-axis {
  margin-top: 6px;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
  text-align: right;
}
.mt-stats {
  display: flex;
  gap: 8px;
  margin-top: 16px;
}
.mt-stat {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 10px 6px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.mt-stat.hot {
  border-color: var(--vp-c-danger-1);
}
.mt-stat-k {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.mt-stat-v {
  font-size: 18px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.mt-stat.hot .mt-stat-v {
  color: var(--vp-c-danger-1);
}
.mt-stat-v small {
  font-size: 10px;
  font-weight: 400;
  color: var(--vp-c-text-3);
  margin-left: 1px;
}
.mt-note {
  margin: 12px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.mt-legend {
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
