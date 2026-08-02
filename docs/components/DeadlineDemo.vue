<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// resilience/deadline(Go)を移植。時間は整数の刻みで、乱数は出てこない。

type Policy = "none" | "each" | "pass";

interface Chain {
  hops: number;
  tries: number;
  cost: number;
  budget: number;
  fails: number;
  policy: Policy;
}
interface LeafCall {
  start: number;
  wasted: boolean;
}
interface Result {
  leaf: number;
  total: number;
  elapsed: number;
  wasted: number;
  ok: boolean;
  calls: LeafCall[];
}

function run(c: Chain): Result {
  const res: Result = { leaf: 0, total: 0, elapsed: 0, wasted: 0, ok: false, calls: [] };
  if (c.hops < 1 || c.tries < 1) return res;
  let now = 0;
  let left = c.fails;

  const childUntil = (until: number) =>
    c.policy === "pass" ? until : c.policy === "each" ? now + c.budget : 0;

  const leaf = (): boolean => {
    res.leaf++;
    res.total++;
    const wasted = c.budget > 0 && c.policy !== "none" && now >= c.budget;
    if (wasted) res.wasted += c.cost;
    res.calls.push({ start: now, wasted });
    now += c.cost;
    if (left > 0) {
      left--;
      return false;
    }
    return true;
  };

  const call = (depth: number, until: number): boolean => {
    if (depth === c.hops - 1) return leaf();
    for (let i = 0; i < c.tries; i++) {
      if (until > 0 && now + c.cost > until) return false;
      res.total++;
      if (call(depth + 1, childUntil(until))) return true;
    }
    return false;
  };

  res.ok = call(0, c.policy === "none" ? 0 : c.budget);
  res.elapsed = now;
  return res;
}

const BROKEN = 1 << 20;

const hops = ref(4);
const tries = ref(3);
const budget = ref(45);
const policy = ref<Policy>("none");
const leafFails = ref(BROKEN);

const POLICIES = [
  ["none", "締め切り無し"],
  ["each", "段ごとに持つ"],
  ["pass", "下へ渡す"],
] as const;

const chain = computed<Chain>(() => ({
  hops: hops.value,
  tries: tries.value,
  cost: 10,
  budget: budget.value,
  fails: leafFails.value,
  policy: policy.value,
}));

const res = computed(() => run(chain.value));
const noDeadline = computed(() => run({ ...chain.value, policy: "none" }));

// 末端の呼び出しを時間軸に並べる。予算の線より右が、誰も待っていない仕事になる
const span = computed(() => Math.max(res.value.elapsed, budget.value, 10));
const marks = computed(() =>
  res.value.calls.map((c) => ({
    left: (c.start / span.value) * 100,
    width: (10 / span.value) * 100,
    wasted: c.wasted,
  })),
);
const budgetLine = computed(() => (budget.value / span.value) * 100);

const badge = computed(() => `末端 ${res.value.leaf} 回`);

const verdict = computed(() => {
  const r = res.value;
  if (policy.value === "none")
    return `${hops.value} 段で各段 ${tries.value} 回なら、末端は ${tries.value}^${hops.value - 1} = ${r.leaf} 回呼ばれる。入口の設定に書いてあるのは「${tries.value} 回まで」だけで、この掛け算はどの設定ファイルにも書かれていない`;
  if (policy.value === "each") {
    if (r.wasted === 0)
      return `いまの設定では、掛け算が予算 ${budget.value} に届いていない。段数か試行回数を増やすと、入口が諦めたあとも下が働き続ける形が出る`;
    return `入口の締め切りは ${budget.value} なのに、経過は ${r.elapsed} まで延びた。うち ${r.wasted} は入口が諦めたあとに始まった仕事で、返しても受け取る相手がいない。下の段が自分の呼ばれた時刻から測り直すからだ`;
  }
  return `締め切り無しなら末端 ${noDeadline.value.leaf} 回・経過 ${noDeadline.value.elapsed} のところ、下へ渡すと ${r.leaf} 回・経過 ${r.elapsed} で止まる。予算 ${budget.value} に末端の 1 回 10 が ${Math.floor(budget.value / 10)} 回しか入らないからで、段数を増やしてもここは変わらない`;
});
</script>

<template>
  <DemoShell title="締め切りの渡し方(3 段以上の呼び出し)" :badge="badge">
    <div class="dl-actions">
      <span class="sd-seg">
        <span v-for="[key, label] in POLICIES" :key="key" class="sd-seg-opt"
              :class="{ on: policy === key }" @click="policy = key">{{ label }}</span>
      </span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: leafFails === BROKEN }"
              @click="leafFails = BROKEN">末端は壊れている</span>
        <span class="sd-seg-opt" :class="{ on: leafFails === 4 }"
              @click="leafFails = 4">5 回目で通る</span>
      </span>
    </div>

    <div class="dl-knobs">
      <div class="dl-knob">
        <label for="dl-h">段数</label>
        <input id="dl-h" v-model.number="hops" type="range" min="2" max="6" step="1" />
        <span class="mono">{{ hops }}</span>
      </div>
      <div class="dl-knob">
        <label for="dl-t">各段が試す回数</label>
        <input id="dl-t" v-model.number="tries" type="range" min="1" max="5" step="1" />
        <span class="mono">{{ tries }}</span>
      </div>
      <div class="dl-knob" :class="{ off: policy === 'none' }">
        <label for="dl-b">予算</label>
        <input id="dl-b" v-model.number="budget" type="range" min="10" max="200" step="5"
               :disabled="policy === 'none'" />
        <span class="mono">{{ budget }}</span>
      </div>
    </div>

    <div class="dl-nums">
      <div class="dl-num">
        <span class="dl-k">末端への呼び出し</span>
        <span class="dl-v" :class="res.leaf < noDeadline.leaf ? 'good' : ''">{{ res.leaf }}</span>
        <span class="dl-was">締め切り無しなら {{ noDeadline.leaf }}</span>
      </div>
      <div class="dl-num">
        <span class="dl-k">経過した時間</span>
        <span class="dl-v" :class="policy !== 'none' && res.elapsed > budget ? 'cost' : ''">{{ res.elapsed }}</span>
        <span class="dl-was">{{ policy === "none" ? "締め切り無し" : `入口の締め切り ${budget}` }}</span>
      </div>
      <div class="dl-num">
        <span class="dl-k">誰も待っていない仕事</span>
        <span class="dl-v" :class="res.wasted > 0 ? 'cost' : ''">{{ res.wasted }}</span>
        <span class="dl-was">{{ res.ok ? "入口から見て成功" : "入口から見て失敗" }}</span>
      </div>
    </div>

    <div class="dl-line">
      <div class="dl-track">
        <span v-for="(m, i) in marks" :key="i" class="dl-call"
              :class="{ wasted: m.wasted }"
              :style="{ left: m.left + '%', width: m.width + '%' }" />
        <span v-if="policy !== 'none'" class="dl-cut" :style="{ left: budgetLine + '%' }" />
      </div>
    </div>
    <div class="dl-legend">
      1 本が末端の 1 回。左から右へ時間が進む。
      <span class="dl-tag">末端の呼び出し</span>
      <span v-if="policy !== 'none'" class="dl-tag wasted">入口が諦めたあと</span>
      <span v-if="policy !== 'none'" class="dl-tag cut">入口の締め切り</span>
    </div>

    <div class="dl-verdict">{{ verdict }}</div>

    <p class="dl-note">
      各段のリトライ設定は、単体で見るとどれも小さい。掛け算になるのは段をまたいだときで、
      その積はどの設定ファイルにも書かれていない。締め切りを時刻で下へ渡すと、
      段数によらず時間で上限が決まる。長さで渡すと、下の段が自分の時刻から測り直して延びる。
    </p>
  </DemoShell>
</template>

<style scoped>
.dl-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.dl-knobs { display: flex; gap: 16px; margin-top: 14px; flex-wrap: wrap; }
.dl-knob { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--vp-c-text-2); flex: 1; min-width: 190px; }
.dl-knob.off { opacity: 0.4; }
.dl-knob input[type="range"] { flex: 1; min-width: 80px; accent-color: var(--vp-c-brand-1); }
.mono { font-family: var(--vp-font-family-mono); font-size: 12px; color: var(--vp-c-text-1); font-variant-numeric: tabular-nums; }

.dl-nums { display: flex; gap: 10px; margin-top: 14px; flex-wrap: wrap; }
.dl-num { flex: 1; min-width: 145px; padding: 10px 12px; border: 1px solid var(--vp-c-divider); background-color: var(--vp-c-bg-soft); }
.dl-k { display: block; font-size: 10px; color: var(--vp-c-text-3); font-weight: 600; }
.dl-v { display: block; font-family: var(--vp-font-family-mono); font-size: 22px; line-height: 1.3; color: var(--vp-c-text-1); font-variant-numeric: tabular-nums; }
.dl-v.good { color: var(--vp-c-green-1); }
.dl-v.cost { color: var(--vp-c-yellow-1); }
.dl-was { display: block; font-size: 10.5px; color: var(--vp-c-text-3); }

.dl-line { margin-top: 14px; padding: 8px; border: 1px solid var(--vp-c-divider); }
.dl-track { position: relative; height: 34px; background-color: var(--vp-c-bg-soft); }
.dl-call { position: absolute; top: 4px; bottom: 4px; background-color: var(--vp-c-brand-soft); border-left: 1px solid var(--vp-c-brand-1); }
.dl-call.wasted { background-color: var(--vp-c-danger-soft); border-left-color: var(--vp-c-danger-1); }
.dl-cut { position: absolute; top: 0; bottom: 0; width: 2px; background-color: var(--vp-c-text-1); }

.dl-legend { margin-top: 6px; font-size: 10.5px; color: var(--vp-c-text-3); display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }
.dl-tag { padding: 1px 6px; font-weight: 600; color: var(--vp-c-text-2); background-color: var(--vp-c-default-soft); }
.dl-tag.wasted { color: var(--vp-c-danger-1); background-color: var(--vp-c-danger-soft); }
.dl-tag.cut { color: var(--vp-c-text-1); background-color: var(--vp-c-default-soft); }

.dl-verdict { margin-top: 14px; padding: 10px 12px; border: 1px solid var(--vp-c-divider); background-color: var(--vp-c-bg-soft); font-size: 12.5px; line-height: 1.7; color: var(--vp-c-text-1); }
.dl-note { margin: 12px 0 0; font-size: 12px; line-height: 1.75; color: var(--vp-c-text-2); }
</style>
