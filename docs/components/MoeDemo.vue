<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/moe(Go)のルータ + 負荷分散損失をブラウザに移植。
// 8 expert・top-2。トークンを 1 つずつ流すと expert の負荷が積み上がる。
// ルータを「偏った初期化」に切り替えると集中と損失の跳ね上がりが見える。

const DIM = 8;
const N_EXPERTS = 8;
const TOP_K = 2;
const N_TOKENS = 10;

function lcgMatrix(rows: number, cols: number, seed: number): number[][] {
  let s = BigInt(seed) * 6364136223846793005n + 1442695040888963407n;
  const out: number[][] = [];
  for (let r = 0; r < rows; r++) {
    const row: number[] = [];
    for (let c = 0; c < cols; c++) {
      s = (s * 6364136223846793005n + 1442695040888963407n) & 0xffffffffffffffffn;
      row.push(Number((s >> 40n) & 0xffffffn) / (1 << 24) - 0.5);
    }
    out.push(row);
  }
  return out;
}

const routerBase = lcgMatrix(DIM, N_EXPERTS, 7);
// 偏った初期化: expert 3 のスコアが常に大きい
const routerSkewed = routerBase.map((row) => row.map((v, e) => (e === 3 ? v + 2.0 : v)));

function tokenVec(i: number): number[] {
  let v = 0.29 + i * 0.111;
  const out: number[] = [];
  for (let d = 0; d < DIM; d++) {
    v = (v * 7.13 + 0.37) % 1.0;
    out.push(v * 2 - 1);
  }
  return out;
}

interface Frame {
  token: number;
  top: { expert: number; weight: number }[];
  loads: number[];
  loss: number;
  note: string;
}

function route(router: number[][], x: number[]): { expert: number; weight: number }[] {
  const scores = Array.from({ length: N_EXPERTS }, (_, e) => x.reduce((a, v, i) => a + v * router[i][e], 0));
  const order = scores.map((_, i) => i).sort((a, b) => scores[b] - scores[a]);
  const top = order.slice(0, TOP_K);
  const maxS = scores[top[0]];
  const exps = top.map((e) => Math.exp(scores[e] - maxS));
  const sum = exps.reduce((a, b) => a + b, 0);
  return top.map((e, i) => ({ expert: e, weight: exps[i] / sum }));
}

function loss(loads: number[]): number {
  const total = loads.reduce((a, b) => a + b, 0);
  if (total === 0) return 0;
  return N_EXPERTS * loads.reduce((a, n) => a + (n / total) ** 2, 0);
}

function simulate(router: number[][]): Frame[] {
  const loads = Array(N_EXPERTS).fill(0);
  const frames: Frame[] = [
    { token: 0, top: [], loads: [...loads], loss: 0, note: "10 トークンを 1 つずつルータに通す。各トークンはスコア上位 2 つの expert だけを使う" },
  ];
  for (let t = 0; t < N_TOKENS; t++) {
    const top = route(router, tokenVec(t));
    for (const a of top) loads[a.expert]++;
    const l = loss(loads);
    frames.push({
      token: t + 1,
      top,
      loads: [...loads],
      loss: l,
      note: `トークン ${t + 1}: expert ${top[0].expert}(重み ${top[0].weight.toFixed(2)})と expert ${top[1].expert}(${top[1].weight.toFixed(2)})が選ばれた。残り ${N_EXPERTS - TOP_K} 個は計算されない`,
    });
  }
  return frames;
}

const routers = [
  { key: "even", label: "均されたルータ", frames: simulate(routerBase) },
  { key: "skew", label: "偏った初期化", frames: simulate(routerSkewed) },
];
const pick = ref(0);
const at = ref(0);
function setPick(i: number) {
  pick.value = i;
  at.value = 0;
}

const frames = computed(() => routers[pick.value].frames);
const cur = computed(() => frames.value[at.value]);
const maxLoad = computed(() => Math.max(4, ...cur.value.loads));

const note = computed(() => {
  if (at.value === frames.value.length - 1 && pick.value === 1) {
    return `全トークン処理完了。偏った初期化では expert 3 に集中し、負荷分散損失は ${cur.value.loss.toFixed(2)} まで上がった(均等なら 1 付近)。学習ではこの値を損失に足して崩壊を防ぐ`;
  }
  return cur.value.note;
});

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.value.length - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frames.value.length - 1; }

const badge = computed(() => `token ${cur.value.token}/${N_TOKENS} · loss ${cur.value.loss.toFixed(2)}`);
</script>

<template>
  <DemoShell title="MoE(ルーティングと負荷)" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="mo-cfg mono">8 expert · top-2</span>
      <span class="spacer" />
      <span class="sd-seg">
        <span v-for="(r, i) in routers" :key="r.key" class="sd-seg-opt" :class="{ on: pick === i }" @click="setPick(i)">{{ r.label }}</span>
      </span>
    </div>

    <div class="mo-current" v-if="cur.top.length">
      <span class="mo-current-label">このトークンの行き先:</span>
      <span v-for="a in cur.top" :key="a.expert" class="mo-chip mono">E{{ a.expert }} × {{ a.weight.toFixed(2) }}</span>
    </div>
    <div class="mo-current" v-else>
      <span class="mo-current-label">まだトークンを流していない</span>
    </div>

    <div class="mo-loads">
      <div class="mo-loads-head">
        expert ごとの負荷(受け取ったトークン数)
        <span class="mono mo-loss">負荷分散損失 {{ cur.loss.toFixed(2) }}</span>
      </div>
      <div v-for="(n, e) in cur.loads" :key="e" class="mo-row" :class="{ hot: cur.top.some((a) => a.expert === e) }">
        <span class="mo-row-label mono">E{{ e }}</span>
        <span class="mo-track"><span class="mo-fill" :style="{ width: (n / maxLoad) * 100 + '%' }"></span></span>
        <span class="mo-val mono">{{ n }}</span>
      </div>
    </div>

    <p class="mo-note">{{ note }}</p>

    <div class="mo-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="mo-nav mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1トークン流す</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="mo-legend">
      各トークンが使うのは 8 expert 中 2 個だけ。総パラメータは 8 個ぶんメモリに載るが、
      計算は常に 2 個ぶんで済む。損失は均等で 1、集中で 8 に近づき、
      学習ではこの値を本来の損失に足してルータの崩壊(特定 expert への固執)を防ぐ。
    </p>
  </DemoShell>
</template>

<style scoped>
.mo-cfg {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.mo-current {
  margin-top: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  min-height: 28px;
}
.mo-current-label {
  font-size: 12.5px;
  color: var(--vp-c-text-2);
}
.mo-chip {
  font-size: 12px;
  padding: 3px 9px;
  border: 1px solid var(--vp-c-brand-1);
  border-radius: 3px;
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.mo-loads {
  margin-top: 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.mo-loads-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 7px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.mo-loss {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.mo-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 12px;
  border-left: 3px solid transparent;
  border-radius: 0;
}
.mo-row.hot {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg-soft);
}
.mo-row-label {
  width: 28px;
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.mo-track {
  flex: 1;
  height: 10px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.mo-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.mo-val {
  width: 24px;
  text-align: right;
  font-size: 11.5px;
}
.mo-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.mo-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.mo-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.mo-legend {
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
