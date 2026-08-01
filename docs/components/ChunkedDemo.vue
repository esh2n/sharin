<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/chunked(Go)を移植。整数行列なので丸めが入らず、値の一致を厳密に見せられる。
type Mat = { rows: number; cols: number; data: number[] };
const mat = (rows: number, cols: number, data: number[]): Mat => ({ rows, cols, data });
const at = (m: Mat, r: number, c: number) => m.data[r * m.cols + c];

function mul(a: Mat, b: Mat): Mat {
  const out = mat(a.rows, b.cols, new Array(a.rows * b.cols).fill(0));
  for (let i = 0; i < a.rows; i++)
    for (let p = 0; p < a.cols; p++) {
      const av = at(a, i, p);
      if (!av) continue;
      for (let j = 0; j < b.cols; j++) out.data[i * out.cols + j] += av * at(b, p, j);
    }
  return out;
}
function tr(m: Mat): Mat {
  const out = mat(m.cols, m.rows, new Array(m.rows * m.cols).fill(0));
  for (let i = 0; i < m.rows; i++) for (let j = 0; j < m.cols; j++) out.data[j * out.cols + i] = at(m, i, j);
  return out;
}
function tril(m: Mat): Mat {
  const out = mat(m.rows, m.cols, [...m.data]);
  for (let i = 0; i < m.rows; i++) for (let j = i + 1; j < m.cols; j++) out.data[i * m.cols + j] = 0;
  return out;
}
const rowsOf = (m: Mat, from: number, to: number) =>
  mat(to - from, m.cols, m.data.slice(from * m.cols, to * m.cols));

// 章と同じ 4 トークン・次元 2 の例
const q = mat(4, 2, [1, 2, 0, 1, 3, 1, 2, 2]);
const k = mat(4, 2, [2, 1, 1, 3, 0, 2, 1, 1]);
const v = mat(4, 2, [1, 0, 0, 1, 2, 1, 1, 2]);

function chunked(c: number): Mat {
  if (c <= 0) c = 1;
  const l = q.rows, d = v.cols;
  const out = mat(l, d, new Array(l * d).fill(0));
  let state = mat(k.cols, v.cols, new Array(k.cols * v.cols).fill(0));
  for (let s = 0; s < l; s += c) {
    const e = Math.min(s + c, l);
    const qc = rowsOf(q, s, e), kc = rowsOf(k, s, e), vc = rowsOf(v, s, e);
    const inter = mul(qc, state);
    const intra = mul(tril(mul(qc, tr(kc))), vc);
    for (let i = s; i < e; i++)
      for (let j = 0; j < d; j++) out.data[i * d + j] = at(inter, i - s, j) + at(intra, i - s, j);
    const upd = mul(tr(kc), vc);
    state = mat(state.rows, state.cols, state.data.map((x, i) => x + upd.data[i]));
  }
  return out;
}

const chunk = ref(1);
const result = computed(() => chunked(chunk.value).data);
const reference = chunked(4).data;
const same = computed(() => result.value.every((x, i) => x === reference[i]));
const endLabel = computed(() =>
  chunk.value === 1 ? "線形と同じ" : chunk.value >= 4 ? "attention と同じ" : "その間",
);

// 計算量(系列長 1024・次元 64)
const L = 1024, D = 64;
const CS = [1, 32, 64, 128, 256, 1024];
const flops = computed(() =>
  CS.map((c) => ({
    c,
    state: 2 * L * D * D,
    score: 2 * L * c * D,
    total: 2 * L * D * D + 2 * L * c * D,
    area: c * c,
  })),
);
const maxTotal = computed(() => Math.max(...flops.value.map((f) => f.total)));
const fmt = (n: number) => n.toLocaleString();

const view = ref<"value" | "cost">("value");
const badge = computed(() =>
  view.value === "value" ? `チャンク ${chunk.value}(${endLabel.value})` : `系列長 ${L} / 次元 ${D}`,
);
</script>

<template>
  <DemoShell title="チャンクの大きさで attention と線形がつながる" :badge="badge">
    <div class="ck-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: view === 'value' }" @click="view = 'value'">値は変わらない</span>
        <span class="sd-seg-opt" :class="{ on: view === 'cost' }" @click="view = 'cost'">計算量は変わる</span>
      </span>
    </div>

    <template v-if="view === 'value'">
      <div class="ck-actions ck-row2">
        <span class="ck-label mono">チャンク {{ chunk }}</span>
        <span class="sd-seg">
          <span v-for="c in [1, 2, 3, 4]" :key="c" class="sd-seg-opt mono"
                :class="{ on: chunk === c }" @click="chunk = c">{{ c }}</span>
        </span>
        <span class="ck-end">{{ endLabel }}</span>
      </div>

      <p class="ck-setting mono">4 トークン / 次元 2 ・ 整数なので丸めが入らない</p>

      <div class="ck-out mono">
        <span v-for="(x, i) in result" :key="i" class="ck-cell">{{ x }}</span>
      </div>

      <div class="ck-verdict">
        <template v-if="same">
          チャンクを {{ chunk }} にしても、出力は 4 のときと 1 つも違わない。
          {{ chunk === 1 ? "状態を1つずつ育てる形" : chunk >= 4 ? "全対のスコアを取る形" : "その中間" }}
          だが、同じ答えに着く
        </template>
        <template v-else>値が変わった(実装の誤り)</template>
      </div>
    </template>

    <template v-else>
      <div class="ck-rows">
        <div v-for="f in flops" :key="f.c" class="ck-row">
          <span class="ck-c mono">C={{ f.c }}</span>
          <span class="ck-bar">
            <span class="ck-seg fix" :style="{ width: (f.state / maxTotal) * 100 + '%' }"></span>
            <span class="ck-seg var" :style="{ width: (f.score / maxTotal) * 100 + '%' }"></span>
          </span>
          <span class="ck-total mono">{{ fmt(f.total) }}</span>
        </div>
      </div>
      <div class="ck-key mono">
        <span><i class="ck-dot fix"></i>状態(C に依らない)</span>
        <span><i class="ck-dot var"></i>スコア(C に比例)</span>
      </div>

      <div class="ck-verdict">
        状態の側は {{ fmt(2 * L * D * D) }} のまま一度も動かない。動くのはスコアの側だけで、
        C=1024 の合計は C=1 の 16 倍を超える。値はどこでも同じなので、選ぶ基準は正しさでなく資源になる
      </div>
    </template>

    <p class="ck-note">
      softmax を外すと掛ける順を変えられる。先にスコアを作れば系列長の二乗の面積が要り、先に K と V を畳めば
      次元だけで決まる固定サイズの状態になる。チャンクの中は前者、チャンクをまたぐところは後者で運ぶと、
      その間を連続的に取れる。
    </p>
  </DemoShell>
</template>

<style scoped>
.ck-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.ck-row2 { margin-top: 12px; }
.ck-label { width: 96px; flex: none; font-size: 11.5px; color: var(--vp-c-text-2); }
.ck-end { font-size: 11px; color: var(--vp-c-text-3); }
.ck-setting { margin: 12px 0 0; font-size: 11px; color: var(--vp-c-text-3); }
.ck-out { display: flex; gap: 4px; margin-top: 12px; flex-wrap: wrap; }
.ck-cell {
  min-width: 34px; text-align: center; padding: 5px 6px;
  background-color: var(--vp-c-bg-soft); font-size: 12.5px; color: var(--vp-c-text-1);
}
.ck-rows { display: flex; flex-direction: column; gap: 6px; margin-top: 12px; }
.ck-row { display: flex; align-items: center; gap: 10px; }
.ck-c { width: 62px; flex: none; font-size: 11.5px; color: var(--vp-c-text-2); }
.ck-bar { flex: 1 1 auto; display: flex; height: 12px; background-color: var(--vp-c-default-soft); }
.ck-seg { display: block; height: 100%; }
.ck-seg.fix { background-color: var(--vp-c-text-3); }
.ck-seg.var { background-color: var(--vp-c-brand-1); }
.ck-total { width: 96px; flex: none; text-align: right; font-size: 11.5px; color: var(--vp-c-text-1); }
.ck-key { display: flex; gap: 16px; margin-top: 8px; font-size: 10px; color: var(--vp-c-text-3); flex-wrap: wrap; }
.ck-dot { display: inline-block; width: 8px; height: 8px; margin-right: 5px; }
.ck-dot.fix { background-color: var(--vp-c-text-3); }
.ck-dot.var { background-color: var(--vp-c-brand-1); }
.ck-verdict {
  margin-top: 14px; padding: 8px 12px; background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1); font-size: 12.5px; line-height: 1.6; color: var(--vp-c-text-1);
}
.ck-note {
  margin: 14px 0 0; padding-top: 12px; border-top: 1px solid var(--vp-c-divider);
  font-size: 12px; line-height: 1.7; color: var(--vp-c-text-2);
}
.mono { font-family: var(--vp-font-family-mono); }
</style>
