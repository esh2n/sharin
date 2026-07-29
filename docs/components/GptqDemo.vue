<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/gptq(Go)を移植。入力は整数の線形合同法から作るので、章の数字と一致する。

const D = 64;
const N = 256;
const MASK = (1n << 64n) - 1n;

function rndF(seed: bigint) {
  let s = seed | 1n;
  return () => {
    s = (s * 6364136223846793005n + 1442695040888963407n) & MASK;
    return (Number((s >> 40n) & 0xffffffn) - 8388608) / 8388608;
  };
}
function correlated(share: number): number[][] {
  const r = rndF(7n);
  const x = Array.from({ length: D }, () => new Array<number>(N));
  for (let k = 0; k < N; k++) {
    const base = r();
    for (let i = 0; i < D; i++) x[i][k] = share * base + (1 - share) * r();
  }
  return x;
}
const WEIGHTS = (() => {
  const r = rndF(31n);
  return Array.from({ length: D }, () => r());
})();

function hessian(x: number[][], damp: number): number[][] {
  const h = Array.from({ length: D }, () => new Array<number>(D).fill(0));
  for (let i = 0; i < D; i++)
    for (let j = 0; j < D; j++) {
      let s = 0;
      for (let k = 0; k < N; k++) s += x[i][k] * x[j][k];
      h[i][j] = s / N;
    }
  let mean = 0;
  for (let i = 0; i < D; i++) mean += h[i][i];
  mean /= D;
  for (let i = 0; i < D; i++) h[i][i] += damp * mean;
  return h;
}
function inverse(a: number[][]): number[][] {
  const d = a.length;
  const m = a.map((row, i) => {
    const r = new Array<number>(2 * d).fill(0);
    for (let j = 0; j < d; j++) r[j] = row[j];
    r[d + i] = 1;
    return r;
  });
  for (let c = 0; c < d; c++) {
    let p = c;
    for (let r = c + 1; r < d; r++) if (Math.abs(m[r][c]) > Math.abs(m[p][c])) p = r;
    [m[c], m[p]] = [m[p], m[c]];
    const piv = m[c][c];
    if (piv === 0) continue;
    for (let k = 0; k < 2 * d; k++) m[c][k] /= piv;
    for (let r = 0; r < d; r++) {
      if (r === c || m[r][c] === 0) continue;
      const f = m[r][c];
      for (let k = 0; k < 2 * d; k++) m[r][k] -= f * m[c][k];
    }
  }
  return m.map((r) => r.slice(d));
}
function cholUpper(a: number[][]): number[][] {
  const d = a.length;
  const l = Array.from({ length: d }, () => new Array<number>(d).fill(0));
  for (let i = 0; i < d; i++)
    for (let j = 0; j <= i; j++) {
      let s = a[i][j];
      for (let k = 0; k < j; k++) s -= l[i][k] * l[j][k];
      if (i === j) {
        l[i][i] = Math.sqrt(s <= 0 ? 1e-12 : s);
        continue;
      }
      l[i][j] = s / l[j][j];
    }
  const u = Array.from({ length: d }, () => new Array<number>(d).fill(0));
  for (let i = 0; i < d; i++) for (let j = 0; j < d; j++) u[i][j] = l[j][i];
  return u;
}
const clamp = (v: number, q: number) => Math.max(-q, Math.min(q, v));
function scaleOf(w: number[], bits: number) {
  return Math.max(...w.map(Math.abs)) / ((1 << (bits - 1)) - 1) || 1;
}
function roundToNearest(w: number[], bits: number) {
  const s = scaleOf(w, bits);
  const q = (1 << (bits - 1)) - 1;
  return w.map((v) => clamp(Math.round(v / s), q) * s);
}
function spread(w: number[], plan: number[][], bits: number) {
  const s = scaleOf(w, bits);
  const qm = (1 << (bits - 1)) - 1;
  const work = w.slice();
  const out = new Array<number>(w.length);
  for (let i = 0; i < work.length; i++) {
    const q = clamp(Math.round(work[i] / s), qm) * s;
    out[i] = q;
    const err = (work[i] - q) / plan[i][i];
    for (let j = i + 1; j < work.length; j++) work[j] -= err * plan[i][j];
  }
  return out;
}
function outputs(w: number[], x: number[][]) {
  const out = new Array<number>(N).fill(0);
  for (let k = 0; k < N; k++) {
    let s = 0;
    for (let i = 0; i < D; i++) s += w[i] * x[i][k];
    out[k] = s;
  }
  return out;
}
function rms(a: number[], b: number[]) {
  let s = 0;
  for (let i = 0; i < a.length; i++) {
    const d = a[i] - b[i];
    s += d * d;
  }
  return Math.sqrt(s / a.length);
}

const SHARES = [0, 0.5, 0.7, 0.9, 0.95];
const sharePick = ref(3);
const BITS = [2, 3, 4];
const bitPick = ref(1);

const result = computed(() => {
  const share = SHARES[sharePick.value];
  const bits = BITS[bitPick.value];
  const x = correlated(share);
  const plan = cholUpper(inverse(hessian(x, 0.01)));
  const rtn = roundToNearest(WEIGHTS, bits);
  const fix = spread(WEIGHTS, plan, bits);
  const base = outputs(WEIGHTS, x);
  return {
    share,
    bits,
    rtn,
    fix,
    wRTN: rms(WEIGHTS, rtn),
    wFix: rms(WEIGHTS, fix),
    oRTN: rms(base, outputs(rtn, x)),
    oFix: rms(base, outputs(fix, x)),
    moved: WEIGHTS.filter((_, i) => rtn[i] !== fix[i]).length,
  };
});

const gain = computed(() => result.value.oRTN / result.value.oFix);
const badge = computed(() => `共通成分 ${result.value.share} ・ int${result.value.bits}`);
const verdict = computed(() => {
  const r = result.value;
  if (gain.value < 1.05)
    return `入力どうしが似ていないので、肩代わりできる先が無い。配っても出力のずれはほとんど変わらない(${gain.value.toFixed(2)}倍)`;
  return `重みのずれは ${r.wRTN.toFixed(5)} から ${r.wFix.toFixed(5)} へ増えているのに、出力のずれは ${gain.value.toFixed(2)} 倍小さくなっている。最寄りに丸めるのをやめて、出力を守っている`;
});
const bar = (v: number, max: number) => `${Math.max(2, (v / max) * 100)}%`;
</script>

<template>
  <DemoShell title="誤差を配る量子化" :badge="badge" :badge-tone="gain > 1.05 ? 'ok' : 'neutral'">
    <div class="gp-actions">
      <span class="gp-label">入力の共通成分</span>
      <span class="sd-seg">
        <span
          v-for="(s, i) in SHARES"
          :key="s"
          class="sd-seg-opt"
          :class="{ on: sharePick === i }"
          @click="sharePick = i"
          >{{ s }}</span
        >
      </span>
      <span class="gp-gap" />
      <span class="sd-seg">
        <span
          v-for="(b, i) in BITS"
          :key="b"
          class="sd-seg-opt"
          :class="{ on: bitPick === i }"
          @click="bitPick = i"
          >int{{ b }}</span
        >
      </span>
    </div>

    <div class="gp-strip">
      <span
        v-for="i in D"
        :key="i"
        class="gp-tick"
        :class="result.rtn[i - 1] !== result.fix[i - 1] ? 'moved' : ''"
      />
    </div>
    <p class="gp-caption">
      64 個の重み。色が付いているのが、素朴な丸めとは別の格子点へ行ったもの({{ result.moved }} 個)
    </p>

    <div class="gp-table">
      <div class="gp-cell head">丸め方</div>
      <div class="gp-cell head num">重みのずれ</div>
      <div class="gp-cell head num">出力のずれ</div>
      <div class="gp-cell head"></div>

      <div class="gp-cell">素朴に丸める</div>
      <div class="gp-cell num mono ok">{{ result.wRTN.toFixed(5) }}</div>
      <div class="gp-cell num mono bad">{{ result.oRTN.toFixed(5) }}</div>
      <div class="gp-cell barcell">
        <span class="gp-bar bad" :style="{ width: bar(result.oRTN, Math.max(result.oRTN, result.oFix)) }" />
      </div>

      <div class="gp-cell strong">誤差を配る</div>
      <div class="gp-cell num mono bad">{{ result.wFix.toFixed(5) }}</div>
      <div class="gp-cell num mono ok">{{ result.oFix.toFixed(5) }}</div>
      <div class="gp-cell barcell">
        <span class="gp-bar ok" :style="{ width: bar(result.oFix, Math.max(result.oRTN, result.oFix)) }" />
      </div>
    </div>

    <div class="gp-verdict" :class="gain > 1.05 ? 'ok' : 'flat'">{{ verdict }}</div>

    <p class="gp-note">
      共通成分は、入力どうしがどれくらい一緒に動くかを表す。一緒に動く入力どうしなら、片方の重みを
      減らして他方を増やしても出力はほとんど変わるので、丸めた誤差を肩代わりさせられる。0 にすると
      肩代わりできる先が消え、素朴な丸めと同じところに落ち着く。緑と赤が列ごとに入れ替わっているのは、
      重みのずれを増やして出力のずれを減らしているからになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.gp-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.gp-label {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.gp-gap {
  flex: 1;
  min-width: 8px;
}
.gp-strip {
  display: flex;
  gap: 2px;
  height: 16px;
  margin-top: 14px;
}
.gp-tick {
  flex: 1;
  background-color: var(--vp-c-divider);
}
.gp-tick.moved {
  background-color: var(--vp-c-brand-1);
}
.gp-caption {
  margin: 6px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.gp-table {
  display: grid;
  grid-template-columns: auto auto auto 1fr;
  gap: 0 20px;
  margin-top: 14px;
  font-size: 12.5px;
}
.gp-cell {
  padding: 6px 0;
  border-bottom: 1px solid var(--vp-c-divider);
  color: var(--vp-c-text-2);
}
.gp-cell.head {
  font-size: 10.5px;
  font-weight: 600;
  color: var(--vp-c-text-3);
  border-bottom-color: var(--vp-c-text-3);
  padding-bottom: 4px;
}
.gp-cell.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.gp-cell.strong {
  color: var(--vp-c-text-1);
  font-weight: 600;
}
.gp-cell.ok {
  color: var(--vp-c-green-1);
  font-weight: 600;
}
.gp-cell.bad {
  color: var(--vp-c-danger-1);
}
.barcell {
  display: flex;
  align-items: center;
  min-width: 90px;
}
.gp-bar {
  display: block;
  height: 8px;
}
.gp-bar.ok {
  background-color: var(--vp-c-green-1);
}
.gp-bar.bad {
  background-color: var(--vp-c-danger-1);
}
.gp-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.gp-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.gp-verdict.flat {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.gp-note {
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
