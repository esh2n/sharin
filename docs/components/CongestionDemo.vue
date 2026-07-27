<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// network/congestion(Go)を移植。cwnd ののこぎり波と、2 接続の公平収束を
// SVG の折れ線で描く。

type State = "ss" | "ca";
function simulate(ssthresh: number, capacity: number, rounds: number): number[] {
  let cwnd = 1;
  let sst = ssthresh;
  let state: State = "ss";
  const hist: number[] = [];
  for (let i = 0; i < rounds; i++) {
    if (cwnd > capacity) {
      // 損失: 半減して輻輳回避
      sst = Math.max(cwnd / 2, 1);
      cwnd = sst;
      state = "ca";
    } else if (state === "ss") {
      cwnd *= 2;
      if (cwnd >= sst) {
        cwnd = sst;
        state = "ca";
      }
    } else {
      cwnd += 1;
    }
    hist.push(cwnd);
  }
  return hist;
}
function simulateFairness(cap: number, a: number, b: number, rounds: number): { a: number[]; b: number[] } {
  const ha: number[] = [];
  const hb: number[] = [];
  for (let i = 0; i < rounds; i++) {
    a += 1;
    b += 1;
    if (a + b > cap) {
      a /= 2;
      b /= 2;
    }
    ha.push(a);
    hb.push(b);
  }
  return { a: ha, b: hb };
}

const modes = [
  { key: "sawtooth", label: "のこぎり波(1接続)" },
  { key: "fairness", label: "公平収束(2接続)" },
] as const;
const mode = ref<"sawtooth" | "fairness">("sawtooth");

const CAP_SAW = 32;
const CAP_FAIR = 40;
const saw = computed(() => simulate(16, CAP_SAW, 40));
const fair = computed(() => simulateFairness(CAP_FAIR, 30, 2, 60));

// SVG 座標系。
const W = 480;
const H = 180;
const PAD = 8;
function pathOf(hist: number[], maxY: number): string {
  const n = hist.length;
  return hist
    .map((v, i) => {
      const x = PAD + (i / (n - 1)) * (W - 2 * PAD);
      const y = H - PAD - (v / maxY) * (H - 2 * PAD);
      return `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
}
const maxYSaw = computed(() => Math.max(...saw.value, CAP_SAW) * 1.15);
const maxYFair = computed(() => Math.max(...fair.value.a, ...fair.value.b, CAP_FAIR) * 1.05);
const capYSaw = computed(() => H - PAD - (CAP_SAW / maxYSaw.value) * (H - 2 * PAD));

const badge = computed(() => (mode.value === "sawtooth" ? "容量を探る" : "公平へ収束"));

const fairEnd = computed(() => {
  const a = fair.value.a[fair.value.a.length - 1];
  const b = fair.value.b[fair.value.b.length - 1];
  return { a: a.toFixed(1), b: b.toFixed(1), ratio: (a / b).toFixed(2) };
});

const note = computed(() => {
  if (mode.value === "sawtooth")
    return "cwnd はスロースタートで倍々に立ち上がり、輻輳回避では 1 ずつ増える。容量(点線)を超えると損失で半減し、また増やす。容量ちょうどに留まらず、その周りを探り続けるのこぎり波になる";
  return `2 接続が容量 ${CAP_FAIR} を分け合う。初期は 30 対 2 と大きく偏るが、加算増加は差を保ち乗算減少は差を縮めるので、取り分が等しい方へ収束する(最終 ${fairEnd.value.a} 対 ${fairEnd.value.b}、比 ${fairEnd.value.ratio})`;
});
</script>

<template>
  <DemoShell title="輻輳制御(AIMD)" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="m in modes"
          :key="m.key"
          class="sd-seg-opt"
          :class="{ on: mode === m.key }"
          @click="mode = m.key"
          >{{ m.label }}</span
        >
      </span>
    </div>

    <div class="cg-chart">
      <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" class="cg-svg">
        <!-- のこぎり波 -->
        <template v-if="mode === 'sawtooth'">
          <line :x1="PAD" :y1="capYSaw" :x2="W - PAD" :y2="capYSaw" class="cg-cap" />
          <path :d="pathOf(saw, maxYSaw)" class="cg-line a" fill="none" />
        </template>
        <!-- 公平収束 -->
        <template v-else>
          <path :d="pathOf(fair.a, maxYFair)" class="cg-line a" fill="none" />
          <path :d="pathOf(fair.b, maxYFair)" class="cg-line b" fill="none" />
        </template>
      </svg>
      <div class="cg-legend-row">
        <template v-if="mode === 'sawtooth'">
          <span class="cg-tag"><span class="cg-swatch a"></span>cwnd</span>
          <span class="cg-tag"><span class="cg-swatch cap"></span>容量 {{ CAP_SAW }}</span>
        </template>
        <template v-else>
          <span class="cg-tag"><span class="cg-swatch a"></span>接続A(初期 30)</span>
          <span class="cg-tag"><span class="cg-swatch b"></span>接続B(初期 2)</span>
        </template>
      </div>
    </div>

    <p class="cg-note">{{ note }}</p>

    <p class="cg-legend">
      輻輳ウィンドウ cwnd は「一度に送ってよい量」。誰も空き帯域を知らないので、各接続が増やしては
      損失で減らして探る。スロースタートで手早く立ち上げ、輻輳回避で慎重に押し、損失で半減する(AIMD)。
      加算増加は接続間の差を保ち、乗算減少は差を縮めるので、中央の調整役なしに取り分が公平へ収束する。
    </p>
  </DemoShell>
</template>

<style scoped>
.cg-chart {
  margin-top: 16px;
}
.cg-svg {
  width: 100%;
  height: 180px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.cg-line {
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}
.cg-line.a {
  stroke: var(--vp-c-brand-1);
}
.cg-line.b {
  stroke: var(--vp-c-green-1);
}
.cg-cap {
  stroke: var(--vp-c-danger-1);
  stroke-width: 1;
  stroke-dasharray: 4 3;
  vector-effect: non-scaling-stroke;
}
.cg-legend-row {
  display: flex;
  gap: 16px;
  margin-top: 8px;
}
.cg-tag {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.cg-swatch {
  width: 14px;
  height: 3px;
  display: inline-block;
}
.cg-swatch.a { background-color: var(--vp-c-brand-1); }
.cg-swatch.b { background-color: var(--vp-c-green-1); }
.cg-swatch.cap { background-color: var(--vp-c-danger-1); }
.cg-note {
  margin: 12px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.cg-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
</style>
