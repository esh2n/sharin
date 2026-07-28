<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/quant(Go)の外れ値まわりを移植。
// 512 要素のうち数個だけ桁違いに大きくして、3つの持ち方で誤差を比べる。

const N = 512;
const OUTLIER_VALUE = 25;
const POSITIONS = [7, 200, 431, 55, 118, 260, 333, 470];

// Go 側と厳密に同じ並びを作る(整数の線形合同法から 2 のべき乗で割る)。
function noisy(n: number): number[] {
  const mask = (1n << 64n) - 1n;
  const out: number[] = [];
  let s = 7n;
  for (let i = 0; i < n; i++) {
    s = (s * 6364136223846793005n + 1442695040888963407n) & mask;
    out.push((Number(s >> 40n) - 8388608) / 8388608);
  }
  return out;
}
const BASE = noisy(N);

const counts = [0, 1, 3, 8];
const countPick = ref(2);
const bitsOptions = [8, 4];
const bitPick = ref(0);

const count = computed(() => counts[countPick.value]);
const bits = computed(() => bitsOptions[bitPick.value]);
const marks = computed(() => POSITIONS.slice(0, count.value));

const data = computed(() => {
  const x = [...BASE];
  for (const i of marks.value) x[i] = OUTLIER_VALUE;
  return x;
});

function qmax(b: number) {
  return (1 << (b - 1)) - 1;
}
function step(x: number[], b: number) {
  const m = Math.max(...x.map(Math.abs), 0);
  return m / qmax(b) || 1;
}
function roundTo(v: number, s: number, b: number) {
  const q = qmax(b);
  return Math.max(-q, Math.min(q, Math.round(v / s))) * s;
}

// 大多数(外れ値でない要素)の平均誤差だけを見る。
function errOf(back: number[]) {
  const skip = new Set(marks.value);
  let sum = 0;
  let n = 0;
  for (let i = 0; i < N; i++) {
    if (skip.has(i)) continue;
    sum += Math.abs(data.value[i] - back[i]);
    n++;
  }
  return n ? sum / n : 0;
}

const whole = computed(() => {
  const s = step(data.value, bits.value);
  return { step: s.toFixed(5), err: errOf(data.value.map((v) => roundTo(v, s, bits.value))) };
});
const grouped = computed(() => {
  const size = 64;
  const back: number[] = [];
  let lo = Infinity;
  let hi = 0;
  for (let g = 0; g < N; g += size) {
    const part = data.value.slice(g, g + size);
    const s = step(part, bits.value);
    if (s > hi) hi = s;
    if (s < lo) lo = s;
    for (const v of part) back.push(roundTo(v, s, bits.value));
  }
  const label = lo === hi ? hi.toFixed(5) : `${lo.toFixed(5)}〜${hi.toFixed(5)}`;
  return { step: label, err: errOf(back) };
});
const split = computed(() => {
  const skip = new Set(marks.value);
  const rest = data.value.map((v, i) => (skip.has(i) ? 0 : v));
  const s = step(rest, bits.value);
  const back = rest.map((v, i) => (skip.has(i) ? data.value[i] : roundTo(v, s, bits.value)));
  return { step: s.toFixed(5), err: errOf(back) };
});
const clean = computed(() => {
  const s = step(BASE, bits.value);
  const back = BASE.map((v) => roundTo(v, s, bits.value));
  let sum = 0;
  const skip = new Set(marks.value);
  let n = 0;
  for (let i = 0; i < N; i++) {
    if (skip.has(i)) continue;
    sum += Math.abs(BASE[i] - back[i]);
    n++;
  }
  return n ? sum / n : 0;
});

const rows = computed(() => [
  { name: "全体で1つの scale", ...whole.value },
  { name: "64 要素ごとに scale", ...grouped.value },
  { name: "外れ値を抜いて別に持つ", ...split.value },
]);
const worstErr = computed(() => Math.max(...rows.value.map((r) => r.err)));

const badge = computed(
  () => `int${bits.value} ・ 外れ値 ${count.value} / ${N}`,
);
const note = computed(() => {
  if (!count.value)
    return `外れ値が無ければ、3つの持ち方に大きな差は出ない。値の幅が ±1 に収まっているので、全体で1つの scale でも格子は十分に細かい`;
  const ratio = whole.value.err / split.value.err;
  return `外れ値は ${((count.value / N) * 100).toFixed(1)}% しかないのに、全体で1つの scale にすると大多数の誤差が ${ratio.toFixed(0)} 倍になる。抜いて別に持つと ${clean.value.toFixed(5)}(外れ値が最初から無い場合)まで戻る`;
});
</script>

<template>
  <DemoShell title="外れ値と scale の配り方" :badge="badge" :badge-tone="count ? 'ng' : 'ok'">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="(c, i) in counts"
          :key="c"
          class="sd-seg-opt"
          :class="{ on: countPick === i }"
          @click="countPick = i"
          >外れ値 {{ c }}</span
        >
      </span>
      <span class="spacer" />
      <span class="sd-seg">
        <span
          v-for="(b, i) in bitsOptions"
          :key="b"
          class="sd-seg-opt"
          :class="{ on: bitPick === i }"
          @click="bitPick = i"
          >int{{ b }}</span
        >
      </span>
    </div>

    <div class="qo-strip">
      <span
        v-for="i in N"
        :key="i"
        class="qo-tick"
        :class="marks.includes(i - 1) ? 'out' : ''"
        :style="{ height: marks.includes(i - 1) ? '22px' : `${2 + Math.abs(BASE[i - 1]) * 8}px` }"
      />
    </div>
    <p class="qo-caption">
      512 要素の並び。赤い棒が {{ OUTLIER_VALUE }}、それ以外は ±1 の中に収まっている
    </p>

    <table class="qo-table">
      <thead>
        <tr>
          <th>scale の持ち方</th>
          <th class="num">格子の間隔</th>
          <th class="num">大多数の平均誤差</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(r, i) in rows" :key="r.name" :class="i === 2 ? 'best' : ''">
          <td>{{ r.name }}</td>
          <td class="num mono">{{ r.step }}</td>
          <td class="num mono">{{ r.err.toFixed(5) }}</td>
          <td class="bar-cell">
            <span class="qo-bar" :class="i === 2 ? 'ok' : 'bad'" :style="{ width: `${(r.err / worstErr) * 100}%` }" />
          </td>
        </tr>
      </tbody>
    </table>

    <div class="qo-note" :class="count ? 'bad' : 'ok'">{{ note }}</div>

    <p class="qo-legend">
      比べているのは外れ値そのものの誤差ではなく、外れ値でない大多数の誤差。全体で1つの scale を使うと、
      いちばん大きい値に合わせて格子の間隔が決まるので、±1 に収まっている大多数がその粗い格子に丸められる。
      64 要素ごとに区切ると被害はその区切りの中で止まり、抜いて別に持つと大多数は自分のレンジで格子を使える。
    </p>
  </DemoShell>
</template>

<style scoped>
.spacer {
  flex: 1;
}
.qo-strip {
  display: flex;
  align-items: flex-end;
  gap: 0;
  height: 24px;
  margin-top: 14px;
  border-bottom: 1px solid var(--vp-c-divider);
}
.qo-tick {
  flex: 1;
  background-color: var(--vp-c-text-3);
  opacity: 0.5;
}
.qo-tick.out {
  background-color: var(--vp-c-danger-1);
  opacity: 1;
}
.qo-caption {
  margin: 6px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.qo-table {
  width: 100%;
  margin-top: 14px;
  border-collapse: collapse;
  font-size: 12px;
}
.qo-table th {
  padding: 4px 8px;
  border-bottom: 1px solid var(--vp-c-divider);
  font-size: 10.5px;
  font-weight: 600;
  color: var(--vp-c-text-3);
  text-align: left;
}
.qo-table td {
  padding: 5px 8px;
  border-bottom: 1px solid var(--vp-c-divider);
  color: var(--vp-c-text-2);
}
.qo-table tr.best td {
  color: var(--vp-c-text-1);
  font-weight: 600;
}
.qo-table .num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.bar-cell {
  width: 34%;
}
.qo-bar {
  display: block;
  height: 8px;
  min-width: 2px;
}
.qo-bar.bad {
  background-color: var(--vp-c-danger-1);
}
.qo-bar.ok {
  background-color: var(--vp-c-green-1);
}
.qo-note {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.qo-note.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.qo-note.bad {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.qo-legend {
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
