<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// resilience/loadbalancer(Go)をブラウザに移植。
// 12 台に 120 リクエストを振り、方式ごとに「いちばん混んだ台」がどこまで
// 高くなるかを積み上げグラフで比べる。P2C がランダムよりずっと平らで、
// 最少接続にかなり近いことを見る。

const N = 12; // 台数
const REQ = 120; // リクエスト数
const AVG = REQ / N; // 平均負荷 = 10

// 決定的擬似乱数(RetryDemo と同じ LCG)。
function makeRand(seed: number) {
  let s = BigInt(seed) * 2862933555777941757n + 1n;
  return () => {
    s = (s * 6364136223846793005n + 1442695040888963407n) & 0xffffffffffffffffn;
    return Number((s >> 33n) & 0xffffffffn);
  };
}

const strategies = [
  { key: "rr", label: "ラウンドロビン" },
  { key: "least", label: "最少接続" },
  { key: "p2c", label: "P2C" },
  { key: "random", label: "ランダム" },
] as const;
type StratKey = (typeof strategies)[number]["key"];

// 各方式について、1 リクエストごとの active[] スナップショット列を作る。
function simulate(strat: StratKey): number[][] {
  const active = new Array(N).fill(0);
  const frames: number[][] = [active.slice()];
  const rand = makeRand(20260727);
  for (let step = 0; step < REQ; step++) {
    let pick = 0;
    if (strat === "rr") {
      pick = step % N;
    } else if (strat === "least") {
      let best = 0;
      for (let i = 1; i < N; i++) if (active[i] < active[best]) best = i;
      pick = best;
    } else if (strat === "random") {
      pick = rand() % N;
    } else {
      // p2c: 異なる 2 台の軽い方
      const i = rand() % N;
      let j = rand() % (N - 1);
      if (j >= i) j++;
      pick = active[j] < active[i] ? j : i;
    }
    active[pick]++;
    frames.push(active.slice());
  }
  return frames;
}

// 4 方式ぶんを事前計算(決定的なのでマウント時に固定)。
const allFrames: Record<StratKey, number[][]> = {
  rr: simulate("rr"),
  least: simulate("least"),
  p2c: simulate("p2c"),
  random: simulate("random"),
};

const stratPick = ref<number>(2); // 既定 P2C
const strat = computed(() => strategies[stratPick.value].key);
const frames = computed(() => allFrames[strat.value]);
const at = ref(REQ); // 既定は最後(全リクエスト振り終えた状態)

function setStrat(i: number) {
  stratPick.value = i;
  // at は保持(方式を跨いで同じ進捗で比べられる)。
}

const cur = computed(() => frames.value[at.value]);
const curMax = computed(() => Math.max(...cur.value, 1));
const scaleMax = computed(() => {
  // グラフの縦軸は全方式・全フレームの最大に合わせて固定(方式間で高さを比較できる)。
  let m = AVG;
  for (const k of Object.keys(allFrames) as StratKey[]) {
    for (const v of allFrames[k][REQ]) if (v > m) m = v;
  }
  return m;
});
const sent = computed(() => at.value);

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < REQ);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value -= 10; if (at.value < 0) at.value = 0; }
function next() { if (canNext.value) at.value += 10; if (at.value > REQ) at.value = REQ; }
function last() { at.value = REQ; }

const badge = computed(() => `最大負荷 ${curMax.value} / 平均 ${AVG}`);

const note = computed(() => {
  const over = curMax.value - AVG;
  if (at.value === 0) return `${N} 台すべて空。ここから ${REQ} 個のリクエストを振っていく`;
  if (strat.value === "rr")
    return `ラウンドロビン: 順番に回すだけ。同じ重さのリクエストなら綺麗に均等になる(最大 ${curMax.value})。実際は重さがばらつくと崩れる`;
  if (strat.value === "least")
    return `最少接続: 毎回いちばん空いた台へ。偏りは最小(最大 ${curMax.value})。ただし全台を見る必要があり、複数のLBが並ぶと同じ台に殺到する`;
  if (strat.value === "p2c")
    return `P2C: 2 台だけ無作為に見て軽い方へ。全台を見ないのに最大は ${curMax.value}(平均+${over})。ランダムよりずっと平ら、最少接続にも肉薄`;
  return `ランダム: 毎回 1 台を無作為に。最も混んだ台は平均を大きく超える(最大 ${curMax.value}、平均+${over})。ボールを箱に投げると必ず飛び抜けた箱が出る`;
});
</script>

<template>
  <DemoShell title="ロードバランサ" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="(s, i) in strategies"
          :key="s.key"
          class="sd-seg-opt"
          :class="{ on: stratPick === i }"
          @click="setStrat(i)"
          >{{ s.label }}</span
        >
      </span>
    </div>

    <div class="lb-chart">
      <div class="lb-scale">
        <span class="lb-avg-line" :style="{ bottom: (AVG / scaleMax) * 100 + '%' }">
          <span class="lb-avg-label mono">平均 {{ AVG }}</span>
        </span>
      </div>
      <div class="lb-bars">
        <div v-for="(v, i) in cur" :key="i" class="lb-bar-col">
          <span
            class="lb-bar"
            :class="{ hot: v > AVG * 1.2, peak: v === curMax && v > AVG }"
            :style="{ height: (v / scaleMax) * 100 + '%' }"
          ></span>
          <span class="lb-bar-n mono">{{ v }}</span>
        </div>
      </div>
    </div>

    <p class="lb-note">{{ note }}</p>

    <div class="lb-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀ 10</button>
      <span class="lb-nav mono">{{ sent }} / {{ REQ }} 件</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">10 ▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">10件振る</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="lb-legend">
      12 台に 120 リクエスト(平均 10)を振ったときの、各台の積み上がり。方式を切り替えて、
      いちばん高い棒(最も混んだ台)の高さを比べてほしい。ランダムは平均から大きくはみ出し、
      P2C は 2 台を見るだけでほぼ平らになる。最少接続は最も平らだが全台を見る必要があり、
      複数のロードバランサが並ぶと同じ台への殺到(群集効果)を招く。
    </p>
  </DemoShell>
</template>

<style scoped>
.lb-chart {
  position: relative;
  margin-top: 16px;
  height: 180px;
}
.lb-scale {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.lb-avg-line {
  position: absolute;
  left: 0;
  right: 0;
  height: 0;
  border-top: 1px dashed var(--vp-c-brand-1);
  opacity: 0.6;
}
.lb-avg-label {
  position: absolute;
  right: 0;
  top: -14px;
  font-size: 10px;
  color: var(--vp-c-brand-1);
}
.lb-bars {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: flex-end;
  gap: 3px;
}
.lb-bar-col {
  flex: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
}
.lb-bar {
  width: 100%;
  background-color: var(--vp-c-brand-1);
  border-radius: 0;
  min-height: 1px;
  transition: height 0.15s;
}
.lb-bar.peak {
  box-shadow: inset 0 3px 0 0 var(--vp-c-text-1);
}
.lb-bar.hot {
  background-color: var(--vp-c-danger-1);
}
.lb-bar-n {
  font-size: 10px;
  color: var(--vp-c-text-3);
  margin-top: 2px;
}
.lb-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.lb-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
  flex-wrap: wrap;
}
.lb-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 64px;
  text-align: center;
}
.lb-legend {
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
