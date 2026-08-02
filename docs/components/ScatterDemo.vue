<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// resilience/scatter(Go)を移植。壁時計も乱数生成器も使わないので、
// Go のテストと同じ数字が出る。

const Fast = 10;
const FastWidth = 10;
const Slow = 200;
const SlowEvery = 20;

const TWO64 = 1n << 64n;
const TWO63 = 1n << 63n;
function lcg(seed: number): bigint {
  let s = (BigInt(seed) * 6364136223846793005n + 1442695040888963407n) % TWO64;
  if (s >= TWO63) s -= TWO64;
  if (s < 0n) s = -s;
  return s;
}
function took(node: number, attempt: number): number {
  const s = lcg(node * 31 + attempt * 7919);
  if (s % BigInt(SlowEvery) === 0n) return Slow;
  return Fast + Number(s % BigInt(FastWidth));
}

// 各台が返り終える時刻。hedgeAfter を過ぎた台には 2 本目を投げる。
function finishes(n: number, hedgeAfter: number) {
  const done: number[] = [];
  const hedged: boolean[] = [];
  let sent = 0;
  let slows = 0;
  for (let i = 0; i < n; i++) {
    let first = took(i, 0);
    sent++;
    if (first >= Slow) slows++;
    let usedSecond = false;
    if (hedgeAfter > 0 && first > hedgeAfter) {
      const second = hedgeAfter + took(i, 1);
      sent++;
      usedSecond = true;
      if (second < first) first = second;
    }
    done.push(first);
    hedged.push(usedSecond);
  }
  return { done, hedged, sent, slows };
}
const maxOf = (xs: number[]) => xs.reduce((a, b) => (b > a ? b : a), 0);
const kth = (xs: number[], k: number) =>
  k === 0 ? 0 : [...xs].sort((a, b) => a - b)[k - 1];

// 誰か 1 台が遅い割合。1-(1-p)^n を 1 万分率で。
function tail(n: number): number {
  const p = 1 - 1 / SlowEvery;
  return Math.round((1 - Math.pow(p, n)) * 10000);
}
const pct = (v: number) => `${Math.floor(v / 100)}.${String(v % 100).padStart(2, "0")}%`;

const view = ref<"grow" | "cut" | "hedge">("grow");
const VIEWS = [
  ["grow", "台数を増やす"],
  ["cut", "揃う前に打ち切る"],
  ["hedge", "遅い台にもう1本"],
] as const;

const n = ref(100);
const k = ref(90);
const after = ref(20);

const run = computed(() => {
  const hedgeAfter = view.value === "hedge" ? after.value : 0;
  const count = view.value === "grow" ? n.value : 100;
  const f = finishes(count, hedgeAfter);
  const cut = view.value === "cut" ? Math.min(k.value, count) : count;
  return {
    ...f,
    count,
    wait: view.value === "cut" ? kth(f.done, cut) : maxOf(f.done),
    got: cut,
  };
});

// 全部待つ場合を基準に置いて、何が縮んで何が増えたかを見せる
const base = computed(() => {
  const f = finishes(run.value.count, 0);
  return { wait: maxOf(f.done), sent: f.sent, got: run.value.count };
});

const bars = computed(() => {
  const { done, hedged } = run.value;
  const cutAt = view.value === "cut" ? run.value.wait : Infinity;
  return done.map((d, i) => ({
    h: Math.max(2, Math.round((d / Slow) * 100)),
    slow: d >= Slow,
    hedged: hedged[i],
    dropped: view.value === "cut" && d > cutAt,
  }));
});

const badge = computed(() => `揃うまで ${run.value.wait}`);

const verdict = computed(() => {
  if (view.value === "grow")
    return `${run.value.count} 台に配ると、誰かが遅い応答に当たる割合は ${pct(tail(run.value.count))} になる。1 台あたりの ${pct(tail(1))} は変えていない。変わったのは、それを何回引くかだけだ。数台の時点でもう天井に届くので、台数を減らしても効かない`;
  if (view.value === "cut")
    return `${run.value.count} 台のうち ${run.value.got} 台で打ち切ると、揃うまでが ${base.value.wait} から ${run.value.wait} になる。投げた本数は ${run.value.sent} 本のままで、相手の仕事は減っていない。減ったのは、こちらが待つ時間と手元に集まる答えの数だ`;
  return `${after.value} を過ぎた ${run.value.sent - base.value.sent} 台に 2 本目を投げると、揃うまでが ${base.value.wait} から ${run.value.wait} になる。答えは ${run.value.got} 件のままで減っていない。増えたのは投げた本数で、${base.value.sent} から ${run.value.sent} 本になった`;
});
</script>

<template>
  <DemoShell title="配って集める(裾の切り方)" :badge="badge">
    <div class="sc-actions">
      <span class="sd-seg">
        <span v-for="[key, label] in VIEWS" :key="key" class="sd-seg-opt"
              :class="{ on: view === key }" @click="view = key">{{ label }}</span>
      </span>
    </div>

    <div class="sc-knob">
      <template v-if="view === 'grow'">
        <label for="sc-n">配る台数</label>
        <input id="sc-n" v-model.number="n" type="range" min="1" max="200" step="1" />
        <span class="mono">{{ n }}</span>
      </template>
      <template v-else-if="view === 'cut'">
        <label for="sc-k">何台揃えば打ち切るか</label>
        <input id="sc-k" v-model.number="k" type="range" min="1" max="100" step="1" />
        <span class="mono">{{ k }} / 100</span>
      </template>
      <template v-else>
        <label for="sc-a">何を過ぎたら2本目を投げるか</label>
        <input id="sc-a" v-model.number="after" type="range" min="1" max="200" step="1" />
        <span class="mono">{{ after }}</span>
      </template>
    </div>

    <div class="sc-nums">
      <div class="sc-num">
        <span class="sc-k">揃うまで</span>
        <span class="sc-v" :class="run.wait < base.wait ? 'good' : ''">{{ run.wait }}</span>
        <span class="sc-was">全部待つと {{ base.wait }}</span>
      </div>
      <div class="sc-num">
        <span class="sc-k">集まった答え</span>
        <span class="sc-v" :class="run.got < base.got ? 'cost' : ''">{{ run.got }}</span>
        <span class="sc-was">配った台数 {{ run.count }}</span>
      </div>
      <div class="sc-num">
        <span class="sc-k">投げた本数</span>
        <span class="sc-v" :class="run.sent > base.sent ? 'cost' : ''">{{ run.sent }}</span>
        <span class="sc-was">遅かった台 {{ run.slows }}</span>
      </div>
    </div>

    <div class="sc-bars">
      <span v-for="(b, i) in bars" :key="i" class="sc-bar"
            :class="{ slow: b.slow, hedged: b.hedged, dropped: b.dropped }"
            :style="{ height: b.h + '%' }" />
    </div>
    <div class="sc-legend">
      1 本が 1 台。高いほど返るのが遅い。
      <span class="sc-tag slow">遅い応答</span>
      <span v-if="view === 'hedge'" class="sc-tag hedged">2 本目を投げた</span>
      <span v-if="view === 'cut'" class="sc-tag dropped">打ち切りに間に合わなかった</span>
    </div>

    <div class="sc-verdict">{{ verdict }}</div>

    <p class="sc-note">
      1 台を速くする工夫は、配る台数が増えると効きにくくなる。全部揃うのを待つ形では、
      速く返った台は待っているだけで、遅い 1 台が全体の時間を決めるからだ。
      打ち切りは答えを払って時間を買い、2 本目は下流の仕事を払って時間を買う。
      遅い理由が下流の混雑なら、2 本目は混雑を増やす側に回る。
    </p>
  </DemoShell>
</template>

<style scoped>
.sc-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.sc-knob { display: flex; align-items: center; gap: 10px; margin-top: 14px; font-size: 12px; color: var(--vp-c-text-2); flex-wrap: wrap; }
.sc-knob input[type="range"] { flex: 1; min-width: 160px; accent-color: var(--vp-c-brand-1); }
.mono { font-family: var(--vp-font-family-mono); font-size: 12px; color: var(--vp-c-text-1); }

.sc-nums { display: flex; gap: 10px; margin-top: 14px; flex-wrap: wrap; }
.sc-num { flex: 1; min-width: 130px; padding: 10px 12px; border: 1px solid var(--vp-c-divider); background-color: var(--vp-c-bg-soft); }
.sc-k { display: block; font-size: 10px; color: var(--vp-c-text-3); font-weight: 600; }
.sc-v { display: block; font-family: var(--vp-font-family-mono); font-size: 22px; line-height: 1.3; color: var(--vp-c-text-1); font-variant-numeric: tabular-nums; }
.sc-v.good { color: var(--vp-c-green-1); }
.sc-v.cost { color: var(--vp-c-yellow-1); }
.sc-was { display: block; font-size: 10.5px; color: var(--vp-c-text-3); }

.sc-bars { display: flex; align-items: flex-end; gap: 1px; height: 96px; margin-top: 14px; padding: 6px; border: 1px solid var(--vp-c-divider); overflow-x: auto; }
.sc-bar { flex: 1 0 3px; min-width: 3px; background-color: var(--vp-c-brand-soft); }
.sc-bar.slow { background-color: var(--vp-c-danger-1); }
.sc-bar.hedged { background-color: var(--vp-c-green-1); }
.sc-bar.dropped { background-color: var(--vp-c-text-3); opacity: 0.35; }

.sc-legend { margin-top: 6px; font-size: 10.5px; color: var(--vp-c-text-3); display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }
.sc-tag { padding: 1px 6px; font-weight: 600; }
.sc-tag.slow { color: var(--vp-c-danger-1); background-color: var(--vp-c-danger-soft); }
.sc-tag.hedged { color: var(--vp-c-green-1); background-color: var(--vp-c-green-soft); }
.sc-tag.dropped { color: var(--vp-c-text-2); background-color: var(--vp-c-default-soft); }

.sc-verdict { margin-top: 14px; padding: 10px 12px; border: 1px solid var(--vp-c-divider); background-color: var(--vp-c-bg-soft); font-size: 12.5px; line-height: 1.7; color: var(--vp-c-text-1); }
.sc-note { margin: 12px 0 0; font-size: 12px; line-height: 1.75; color: var(--vp-c-text-2); }
</style>
