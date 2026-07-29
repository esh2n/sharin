<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/prune(Go)を移植。入力は整数の線形合同法から作るので、章の数字と一致する。

const D = 64;
const N = 512;
const MASK = (1n << 64n) - 1n;

function rndF(seed: bigint) {
  let s = seed | 1n;
  return () => {
    s = (s * 6364136223846793005n + 1442695040888963407n) & MASK;
    return (Number((s >> 40n) & 0xffffffn) - 8388608) / 8388608;
  };
}
function inputs(share: number): number[][] {
  const r = rndF(7n);
  const x = Array.from({ length: D }, () => new Array<number>(N));
  for (let k = 0; k < N; k++) {
    const base = r();
    for (let i = 0; i < D; i++) {
      const scale = i % 2 === 1 ? 0.05 : 1;
      x[i][k] = scale * (share * base + (1 - share) * r());
    }
  }
  return x;
}
const WEIGHTS = (() => {
  const r = rndF(31n);
  return Array.from({ length: D }, (_, i) => {
    const v = r();
    return i % 2 === 0 ? v * 0.1 : v;
  });
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
function smallest(score: number[], k: number): number[] {
  const idx = score.map((_, i) => i);
  idx.sort((a, b) => score[a] - score[b] || a - b);
  return idx.slice(0, Math.min(k, idx.length));
}
function byMagnitude(w: number[], k: number) {
  const out = w.slice();
  for (const i of smallest(w.map(Math.abs), k)) out[i] = 0;
  return out;
}
function saliency(w: number[], inv: number[][]) {
  return w.map((v, i) => (v * v) / (2 * inv[i][i]));
}
function bySaliencyOnly(w: number[], inv: number[][], k: number) {
  const out = w.slice();
  for (const i of smallest(saliency(w, inv), k)) out[i] = 0;
  return out;
}
function bySaliency(w: number[], hinv: number[][], k: number) {
  const cur = w.slice();
  const inv = hinv.map((r) => r.slice());
  const dead = new Array<boolean>(D).fill(false);
  for (let step = 0; step < Math.min(k, D); step++) {
    let q = -1;
    let best = Infinity;
    for (let i = 0; i < D; i++) {
      if (dead[i] || inv[i][i] <= 0) continue;
      const l = (cur[i] * cur[i]) / (2 * inv[i][i]);
      if (l < best) {
        q = i;
        best = l;
      }
    }
    if (q < 0) break;
    const f = cur[q] / inv[q][q];
    for (let j = 0; j < D; j++) if (!dead[j] && j !== q) cur[j] -= f * inv[j][q];
    cur[q] = 0;
    dead[q] = true;
    const p = inv[q][q];
    for (let i = 0; i < D; i++) {
      if (dead[i]) continue;
      for (let j = 0; j < D; j++) {
        if (dead[j]) continue;
        inv[i][j] -= (inv[i][q] * inv[q][j]) / p;
      }
    }
  }
  return cur;
}
function outErr(w: number[], p: number[], x: number[][]) {
  let s = 0;
  for (let k = 0; k < N; k++) {
    let a = 0;
    let b = 0;
    for (let i = 0; i < D; i++) {
      a += w[i] * x[i][k];
      b += p[i] * x[i][k];
    }
    const d = a - b;
    s += d * d;
  }
  return Math.sqrt(s / N);
}

const SHARES = [0, 0.3, 0.7, 0.9];
const sharePick = ref(3);
const CUTS = [8, 16, 32, 48];
const cutPick = ref(2);

const result = computed(() => {
  const x = inputs(SHARES[sharePick.value]);
  const inv = inverse(hessian(x, 0.01));
  const k = CUTS[cutPick.value];
  const mag = byMagnitude(WEIGHTS, k);
  const sal = bySaliencyOnly(WEIGHTS, inv, k);
  const obs = bySaliency(WEIGHTS, inv, k);
  return {
    k,
    rows: [
      { name: "大きさで選ぶ", cut: mag, err: outErr(WEIGHTS, mag, x) },
      { name: "効きで選ぶ(補正なし)", cut: sal, err: outErr(WEIGHTS, sal, x) },
      { name: "効きで選んで補う", cut: obs, err: outErr(WEIGHTS, obs, x) },
    ],
  };
});
const swapped = computed(() =>
  result.value.rows.map((r) => {
    const base = result.value.rows[0].cut;
    let n = 0;
    for (let i = 0; i < D; i++) if ((base[i] === 0) !== (r.cut[i] === 0)) n++;
    return n;
  }),
);
const worst = computed(() => Math.max(...result.value.rows.map((r) => r.err)));
const best = computed(() => Math.min(...result.value.rows.map((r) => r.err)));
const badge = computed(
  () => `共通成分 ${SHARES[sharePick.value]} ・ ${result.value.k} / ${D} 個を消す`,
);
const backfires = computed(() => result.value.rows[1].err > result.value.rows[0].err);
const verdict = computed(() =>
  backfires.value
    ? "補える前提の見積もりで選んだのに補っていないので、素朴に大きさで選ぶより悪くなっている。見積もりと手順は対で意味を持つ"
    : "入力どうしが似ていないので肩代わりできる先が少ない。選び方を変えるだけで効き、補正を足してもあまり変わらない",
);
</script>

<template>
  <DemoShell title="枝刈り" :badge="badge" :badge-tone="backfires ? 'ng' : 'neutral'">
    <div class="pr-actions">
      <span class="pr-label">入力の共通成分</span>
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
      <span class="pr-gap" />
      <span class="pr-label">消す数</span>
      <span class="sd-seg">
        <span
          v-for="(c, i) in CUTS"
          :key="c"
          class="sd-seg-opt"
          :class="{ on: cutPick === i }"
          @click="cutPick = i"
          >{{ c }}</span
        >
      </span>
    </div>

    <div class="pr-rows">
      <div v-for="(r, i) in result.rows" :key="r.name" class="pr-row" :class="{ best: r.err === best }">
        <span class="pr-name">{{ r.name }}</span>
        <span class="pr-strip">
          <span v-for="j in D" :key="j" class="pr-tick" :class="r.cut[j - 1] === 0 ? 'cut' : ''" />
        </span>
        <span class="pr-num mono" :class="r.err === best ? 'ok' : r.err === worst ? 'bad' : ''">
          {{ r.err.toFixed(5) }}
        </span>
        <span class="pr-barcell">
          <span class="pr-bar" :class="r.err === best ? 'ok' : 'bad'" :style="{ width: `${(r.err / worst) * 100}%` }" />
        </span>
      </div>
    </div>
    <p class="pr-caption">
      横の並びが 64 個の重み。色の付いたところが消したもの。右の数字が出力のずれ。
      大きさで選ぶのと比べて、効きで選ぶと {{ swapped[1] }} か所、補正まで入れると {{ swapped[2] }} か所が入れ替わる
    </p>

    <div class="pr-verdict" :class="backfires ? 'bad' : 'ok'">{{ verdict }}</div>

    <p class="pr-note">
      共通成分は、入力どうしがどれくらい一緒に動くかを表す。消す損の見積もりは「残りで肩代わりできる
      ほど安い」という形をしているので、補うことを前提にした値段になっている。だから共通成分を上げて
      いくと、補正なしの列だけが跳ね上がる。補うところまで入れれば、どの設定でもいちばん小さい。
      共通成分を 0 に戻すと消す位置そのものが大きく入れ替わり、逆に 0.9 では数か所しか変わらない。
      強い相関のもとでは、差の出どころは選び方ではなく補うかどうかだけになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.pr-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.pr-label {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.pr-gap {
  flex: 1;
  min-width: 8px;
}
.pr-rows {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.pr-row {
  display: grid;
  grid-template-columns: 150px 1fr 62px 70px;
  align-items: center;
  gap: 10px;
  font-size: 12px;
}
.pr-name {
  color: var(--vp-c-text-2);
}
.pr-row.best .pr-name {
  color: var(--vp-c-text-1);
  font-weight: 600;
}
.pr-strip {
  display: flex;
  gap: 1px;
  height: 14px;
}
.pr-tick {
  flex: 1;
  background-color: var(--vp-c-divider);
}
.pr-tick.cut {
  background-color: var(--vp-c-brand-1);
}
.pr-num {
  text-align: right;
  font-variant-numeric: tabular-nums;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.pr-num.ok {
  color: var(--vp-c-green-1);
  font-weight: 600;
}
.pr-num.bad {
  color: var(--vp-c-danger-1);
}
.pr-barcell {
  display: flex;
  align-items: center;
}
.pr-bar {
  display: block;
  height: 8px;
  min-width: 2px;
}
.pr-bar.ok {
  background-color: var(--vp-c-green-1);
}
.pr-bar.bad {
  background-color: var(--vp-c-danger-1);
}
.pr-caption {
  margin: 8px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.pr-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.pr-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.pr-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.pr-note {
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
