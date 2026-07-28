<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// observability/timeseries(Go)を移植。同じ生データに違う整列と集約を当てて、
// 順序と窓の長さで結論が変わるところを見る。

// ---- ヒストグラム(observability/metrics の移植。足せることが肝) ----
const BOUNDS = [5, 10, 25, 50, 100, 250, 500, 1000];

interface Hist {
  counts: number[];
  sum: number;
  total: number;
}
function newHist(): Hist {
  return { counts: new Array(BOUNDS.length + 1).fill(0), sum: 0, total: 0 };
}
function observe(h: Hist, x: number) {
  let i = 0;
  while (i < BOUNDS.length && x > BOUNDS[i]) i++;
  h.counts[i]++;
  h.sum += x;
  h.total++;
}
function merge(hs: Hist[]): Hist {
  const out = newHist();
  for (const h of hs) {
    for (let i = 0; i < out.counts.length; i++) out.counts[i] += h.counts[i];
    out.sum += h.sum;
    out.total += h.total;
  }
  return out;
}
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
      return lower + (upper - lower) * ((rank - cum) / h.counts[i]);
    }
    cum = next;
  }
  return BOUNDS[BOUNDS.length - 1];
}

// ---- 場面1: 分位点を取る順序 ----
const LAT_PODS = [
  { name: "pod-a", slowShare: 0, calls: 1000 },
  { name: "pod-b", slowShare: 0, calls: 1000 },
  { name: "pod-c", slowShare: 0.1, calls: 1000 },
];
function latencyHist(slowShare: number, calls: number): Hist {
  const h = newHist();
  const slow = Math.round(calls * slowShare);
  for (let i = 0; i < calls - slow; i++) observe(h, 4);
  for (let i = 0; i < slow; i++) observe(h, 900);
  return h;
}
const latHists = computed(() => LAT_PODS.map((p) => latencyHist(p.slowShare, p.calls)));
const perPodP99 = computed(() => latHists.value.map((h) => quantile(h, 0.99)));
const wrongP99 = computed(() => perPodP99.value.reduce((a, b) => a + b, 0) / perPodP99.value.length);
const rightP99 = computed(() => quantile(merge(latHists.value), 0.99));

// ---- 場面2: 平均が隠す ----
const CPU_STEPS = 24;
const cpuSeries = computed(() => {
  const out: { name: string; hot: boolean; points: number[] }[] = [];
  for (let i = 0; i < 9; i++) {
    out.push({
      name: `pod-${i + 1}`,
      hot: false,
      points: Array.from({ length: CPU_STEPS }, (_, t) => 9 + ((t * 7 + i * 13) % 5)),
    });
  }
  out.push({
    name: "pod-hot",
    hot: true,
    points: Array.from({ length: CPU_STEPS }, (_, t) => (t < 6 ? 20 + t * 12 : 92 + (t % 4))),
  });
  return out;
});
type Reducer = "mean" | "max" | "p99";
const reducer = ref<Reducer>("mean");
function reduceAt(vals: number[], r: Reducer): number {
  if (r === "max") return Math.max(...vals);
  if (r === "p99") {
    const s = [...vals].sort((a, b) => a - b);
    return s[Math.min(s.length - 1, Math.ceil(0.99 * s.length) - 1)];
  }
  return vals.reduce((a, b) => a + b, 0) / vals.length;
}
const cpuReduced = computed(() =>
  Array.from({ length: CPU_STEPS }, (_, t) => reduceAt(cpuSeries.value.map((s) => s.points[t]), reducer.value)),
);
const THRESHOLD = 65;
const cpuBreaches = computed(() => cpuReduced.value.filter((v) => v >= THRESHOLD).length);

// ---- 場面3: 窓がスパイクを隠す ----
const RAW_STEPS = 60; // 10 秒ごと、合計 600 秒
const rawSpike = Array.from({ length: RAW_STEPS }, (_, i) => (i === 30 ? 610 : 10 + (i % 3)));
const windowSize = ref(1); // 窓に入る点の数
type SpikeAligner = "mean" | "max";
const spikeAligner = ref<SpikeAligner>("mean");
const spikeAligned = computed(() => {
  const w = windowSize.value;
  const out: number[] = [];
  for (let i = 0; i < RAW_STEPS; i += w) {
    const chunk = rawSpike.slice(i, i + w);
    out.push(spikeAligner.value === "max" ? Math.max(...chunk) : chunk.reduce((a, b) => a + b, 0) / chunk.length);
  }
  return out;
});
const spikePeak = computed(() => Math.max(...spikeAligned.value));

// ---- 図の描画 ----
const W = 560;
const H = 120;
function path(points: number[], maxY: number): string {
  if (points.length === 0) return "";
  const step = points.length === 1 ? W : W / (points.length - 1);
  return points
    .map((v, i) => `${i === 0 ? "M" : "L"}${(i * step).toFixed(1)},${(H - (v / maxY) * H).toFixed(1)}`)
    .join(" ");
}

const scenario = ref<"quantile" | "mean" | "window">("quantile");
const badge = computed(() => {
  if (scenario.value === "quantile") return `p99: 正 ${rightP99.value.toFixed(0)}ms / 誤 ${wrongP99.value.toFixed(0)}ms`;
  if (scenario.value === "mean") return `閾値 ${THRESHOLD}% 超え ${cpuBreaches.value} 回`;
  return `見えるスパイクの高さ ${spikePeak.value.toFixed(0)}`;
});
const badgeTone = computed<"ok" | "ng">(() => {
  if (scenario.value === "quantile") return "ng";
  if (scenario.value === "mean") return cpuBreaches.value === 0 ? "ng" : "ok";
  return spikePeak.value < 100 ? "ng" : "ok";
});
const windowLabel = computed(() => `${windowSize.value * 10} 秒`);
</script>

<template>
  <DemoShell title="時系列の整列と集約" :badge="badge" :badge-tone="badgeTone">
    <div class="ts-tabs">
      <button class="sd-btn" :class="scenario === 'quantile' ? 'sd-btn--primary' : ''" @click="scenario = 'quantile'">
        分位点を取る順序
      </button>
      <button class="sd-btn" :class="scenario === 'mean' ? 'sd-btn--primary' : ''" @click="scenario = 'mean'">
        平均が隠すもの
      </button>
      <button class="sd-btn" :class="scenario === 'window' ? 'sd-btn--primary' : ''" @click="scenario = 'window'">
        窓が隠すもの
      </button>
    </div>

    <!-- 場面1 -->
    <div v-if="scenario === 'quantile'" class="ts-panel">
      <p class="ts-brief mono">
        3つの Pod がそれぞれ 1000 件を捌いた。pod-a と pod-b は全件 4ms、pod-c だけ 1 割が 900ms。
        全体の p99 を知りたい
      </p>
      <div class="ts-pods">
        <div v-for="(p, i) in LAT_PODS" :key="p.name" class="ts-pod mono">
          <span>{{ p.name }}</span>
          <span class="ts-pod-v">この系列の p99 = {{ perPodP99[i].toFixed(0) }}ms</span>
        </div>
      </div>

      <div class="ts-compare">
        <div class="ts-way bad">
          <div class="ts-way-h mono">ALIGN_PERCENTILE_99 → REDUCE_MEAN</div>
          <div class="ts-way-sub">先に系列ごとの p99 にしてから平均する</div>
          <div class="ts-way-v">{{ wrongP99.toFixed(0) }} ms</div>
          <div class="ts-way-note">遅い1台の値が、速い2台に薄められた</div>
        </div>
        <div class="ts-way ok">
          <div class="ts-way-h mono">ALIGN_DELTA → REDUCE_PERCENTILE_99</div>
          <div class="ts-way-sub">分布のまま足してから p99 を取る</div>
          <div class="ts-way-v">{{ rightP99.toFixed(0) }} ms</div>
          <div class="ts-way-note">3000 件ぶんの分布から取った、本当の p99</div>
        </div>
      </div>
      <p class="ts-verdict bad">
        同じデータから {{ (rightP99 / wrongP99).toFixed(1) }} 倍違う数字が出ている。
        分位点は足せないので、潰すのはいちばん最後にする
      </p>
    </div>

    <!-- 場面2 -->
    <div v-else-if="scenario === 'mean'" class="ts-panel">
      <p class="ts-brief mono">
        10 個の Pod の CPU 使用率。9 個は 10% 前後で、pod-hot だけが途中から 90% 台に張り付く。
        警告の閾値は {{ THRESHOLD }}%
      </p>
      <div class="ts-controls">
        <span class="ts-label mono">crossSeriesReducer</span>
        <button
          v-for="r in (['mean', 'max', 'p99'] as const)"
          :key="r"
          class="sd-btn"
          :class="reducer === r ? 'sd-btn--primary' : ''"
          @click="reducer = r"
        >
          REDUCE_{{ r.toUpperCase() }}
        </button>
      </div>
      <svg class="ts-chart" :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none">
        <line :x1="0" :x2="W" :y1="H - (THRESHOLD / 100) * H" :y2="H - (THRESHOLD / 100) * H" class="ts-thresh" />
        <path v-for="s in cpuSeries" :key="s.name" :d="path(s.points, 100)" class="ts-faint" />
        <path :d="path(cpuReduced, 100)" class="ts-bold" />
      </svg>
      <div class="ts-axis mono"><span>0%</span><span>閾値 {{ THRESHOLD }}%(横線)</span><span>100%</span></div>
      <p class="ts-verdict" :class="cpuBreaches === 0 ? 'bad' : 'ok'">
        <template v-if="cpuBreaches === 0">
          太線が一度も閾値に届いていない。9 個の低い値に薄められて、張り付いている 1 個が消えた
        </template>
        <template v-else>
          太線が {{ cpuBreaches }} 回、閾値を超えた。困っているところがあるかを知りたいなら、この見方になる
        </template>
      </p>
    </div>

    <!-- 場面3 -->
    <div v-else class="ts-panel">
      <p class="ts-brief mono">
        10 秒ごとの値。ふだんは 10 前後で、途中の 10 秒だけ 610 まで跳ねる
      </p>
      <div class="ts-controls">
        <span class="ts-label mono">alignmentPeriod</span>
        <input v-model.number="windowSize" type="range" min="1" max="30" step="1" class="ts-range" />
        <span class="ts-label mono">{{ windowLabel }}</span>
        <span class="ts-gap" />
        <button
          v-for="a in (['mean', 'max'] as const)"
          :key="a"
          class="sd-btn"
          :class="spikeAligner === a ? 'sd-btn--primary' : ''"
          @click="spikeAligner = a"
        >
          ALIGN_{{ a.toUpperCase() }}
        </button>
      </div>
      <svg class="ts-chart" :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none">
        <path :d="path(rawSpike, 650)" class="ts-faint" />
        <path :d="path(spikeAligned, 650)" class="ts-bold" />
      </svg>
      <div class="ts-axis mono">
        <span>薄い線が生データ</span><span>太線が整列後({{ spikeAligned.length }} 点)</span><span>上端 650</span>
      </div>
      <p class="ts-verdict" :class="spikePeak < 100 ? 'bad' : 'ok'">
        <template v-if="spikePeak < 100">
          窓 {{ windowLabel }} の平均では、跳ねの高さが {{ spikePeak.toFixed(0) }} まで落ちた。10 秒の事象が
          {{ windowLabel }} に薄められて消えている
        </template>
        <template v-else>
          跳ねが {{ spikePeak.toFixed(0) }} として残っている。窓を広げるか、最大でなく平均で取ると消える
        </template>
      </p>
    </div>

    <p class="ts-legend">
      3つの場面はどれも、データではなく見方の話になっている。分位点は潰す順序で、CPU はどの問いを投げるかで、
      スパイクは窓の長さで結論が変わる。Cloud Monitoring では、これが perSeriesAligner と crossSeriesReducer と
      alignmentPeriod という3つの設定として画面に並んでいる。
    </p>
  </DemoShell>
</template>

<style scoped>
.ts-tabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.ts-panel {
  margin-top: 14px;
}
.ts-brief {
  margin: 0 0 10px;
  font-size: 11px;
  line-height: 1.7;
  color: var(--vp-c-text-3);
}
.ts-pods {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.ts-pod {
  flex: 1;
  min-width: 150px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid var(--vp-c-divider);
  padding: 7px 10px;
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  background-color: var(--vp-c-bg-soft);
}
.ts-pod-v {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.ts-compare {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.ts-way {
  flex: 1;
  min-width: 220px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.ts-way.bad {
  border-color: var(--vp-c-danger-1);
}
.ts-way.ok {
  border-color: var(--vp-c-green-1);
}
.ts-way-h {
  font-size: 10px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.ts-way-sub {
  font-size: 10px;
  color: var(--vp-c-text-3);
  margin-top: 2px;
}
.ts-way-v {
  font-size: 24px;
  font-weight: 700;
  margin: 8px 0 4px;
  font-variant-numeric: tabular-nums;
}
.ts-way.bad .ts-way-v {
  color: var(--vp-c-danger-1);
}
.ts-way.ok .ts-way-v {
  color: var(--vp-c-green-1);
}
.ts-way-note {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
}
.ts-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.ts-label {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ts-gap {
  flex: 1;
}
.ts-range {
  width: 160px;
}
.ts-chart {
  width: 100%;
  height: 120px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  display: block;
}
.ts-faint {
  fill: none;
  stroke: var(--vp-c-text-3);
  stroke-width: 1;
  opacity: 0.35;
  vector-effect: non-scaling-stroke;
}
.ts-bold {
  fill: none;
  stroke: var(--vp-c-brand-1);
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}
.ts-thresh {
  stroke: var(--vp-c-danger-1);
  stroke-width: 1;
  stroke-dasharray: 4 3;
  vector-effect: non-scaling-stroke;
}
.ts-axis {
  display: flex;
  justify-content: space-between;
  margin-top: 3px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.ts-verdict {
  margin: 12px 0 0;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.ts-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ts-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ts-legend {
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
