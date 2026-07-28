<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/clock(Go)を移植。同じ筋書きを2つの時計で記録し、答えが割れる組を見る。

type Vec = Record<string, number>;
interface Ev {
  id: number;
  node: string;
  label: string;
  lam: number;
  vec: Vec;
}

const NODES = ["a", "b", "c"];

function build(): Ev[] {
  const lam: Record<string, number> = { a: 0, b: 0, c: 0 };
  const vec: Record<string, Vec> = { a: {}, b: {}, c: {} };
  const out: Ev[] = [];
  let seq = 0;

  const clone = (v: Vec): Vec => ({ ...v });
  const rec = (node: string, label: string): Ev => {
    seq++;
    const e = { id: seq, node, label, lam: lam[node], vec: clone(vec[node]) };
    out.push(e);
    return e;
  };
  const local = (n: string, label: string) => {
    lam[n]++;
    vec[n][n] = (vec[n][n] || 0) + 1;
    return rec(n, label);
  };
  const send = (n: string, label: string) => {
    const e = local(n, label);
    return { e, msg: { lam: e.lam, vec: clone(e.vec) } };
  };
  const recv = (n: string, m: { lam: number; vec: Vec }, label: string) => {
    if (m.lam > lam[n]) lam[n] = m.lam;
    lam[n]++;
    for (const k of Object.keys(m.vec)) {
      if (m.vec[k] > (vec[n][k] || 0)) vec[n][k] = m.vec[k];
    }
    vec[n][n] = (vec[n][n] || 0) + 1;
    return rec(n, label);
  };

  local("a", "a で更新");
  const { msg: m1 } = send("a", "b へ知らせる");
  local("b", "b で更新");
  local("c", "c で更新");
  recv("b", m1, "a の知らせを受けた");
  const { msg: m2 } = send("b", "c へ知らせる");
  local("a", "a でまた更新");
  recv("c", m2, "b の知らせを受けた");
  return out;
}

const events = build();

type Ord = "同じ" | "前" | "後" | "同時";
function compare(a: Vec, b: Vec): Ord {
  let aLess = false;
  let bLess = false;
  for (const k of new Set([...Object.keys(a), ...Object.keys(b)])) {
    const x = a[k] || 0;
    const y = b[k] || 0;
    if (x < y) aLess = true;
    if (x > y) bLess = true;
  }
  if (aLess && bLess) return "同時";
  if (aLess) return "前";
  if (bLess) return "後";
  return "同じ";
}
function byLamport(a: Ev, b: Ev): Ord {
  if (a.lam < b.lam) return "前";
  if (a.lam > b.lam) return "後";
  return "同じ";
}
const vecStr = (v: Vec) => "{" + NODES.filter((n) => v[n]).map((n) => `${n}:${v[n]}`).join(" ") + "}";

const pickA = ref(2); // a2(b へ知らせる)
const pickB = ref(3); // b1(b で更新)

function click(id: number) {
  if (pickA.value === id || pickB.value === id) return;
  pickA.value = pickB.value;
  pickB.value = id;
}
const ea = computed(() => events.find((e) => e.id === pickA.value)!);
const eb = computed(() => events.find((e) => e.id === pickB.value)!);
const lamAns = computed(() => byLamport(ea.value, eb.value));
const vecAns = computed(() => compare(ea.value.vec, eb.value.vec));
const split = computed(() => vecAns.value === "同時" && lamAns.value !== "同じ");

const sorted = computed(() => [...events].sort((x, y) => (x.lam !== y.lam ? x.lam - y.lam : x.node < y.node ? -1 : 1)));
const concurrentIds = computed(() => {
  const s = new Set<number>();
  for (let i = 0; i < events.length; i++)
    for (let j = i + 1; j < events.length; j++)
      if (compare(events[i].vec, events[j].vec) === "同時") {
        s.add(events[i].id);
        s.add(events[j].id);
      }
  return s;
});
const pairCount = computed(() => {
  let n = 0;
  for (let i = 0; i < events.length; i++)
    for (let j = i + 1; j < events.length; j++)
      if (compare(events[i].vec, events[j].vec) === "同時") n++;
  return n;
});

const badge = computed(() => `同時の組 ${pairCount.value} 件 / 出来事 ${events.length} 件`);
const rowsFor = (n: string) => events.filter((e) => e.node === n);
</script>

<template>
  <DemoShell title="論理時計とベクタークロック" :badge="badge" badge-tone="ng">
    <p class="lc-brief mono">
      出来事を2つ選ぶと、2つの時計それぞれの答えが出る。破線の枠は、誰かと同時に起きた出来事
    </p>

    <div class="lc-lines">
      <div v-for="n in NODES" :key="n" class="lc-line">
        <span class="lc-node mono">{{ n }}</span>
        <span class="lc-evs">
          <button
            v-for="e in rowsFor(n)"
            :key="e.id"
            class="lc-ev mono"
            :class="[
              pickA === e.id || pickB === e.id ? 'sel' : '',
              concurrentIds.has(e.id) ? 'conc' : '',
            ]"
            @click="click(e.id)"
          >
            <span class="lc-lam">{{ e.lam }}</span>
            <span class="lc-vec">{{ vecStr(e.vec) }}</span>
            <span class="lc-lab">{{ e.label }}</span>
          </button>
        </span>
      </div>
    </div>

    <div class="lc-cmp">
      <div class="lc-pair mono">
        <span>{{ ea.node }} / {{ ea.label }}</span>
        <span class="lc-vs">と</span>
        <span>{{ eb.node }} / {{ eb.label }}</span>
      </div>
      <div class="lc-answers">
        <div class="lc-ans">
          <em>Lamport(数1つ)</em>
          <b class="mono">{{ ea.lam }} と {{ eb.lam }}</b>
          <span>左は右より「{{ lamAns }}」</span>
        </div>
        <div class="lc-ans" :class="vecAns === '同時' ? 'conc' : ''">
          <em>ベクタ(ノードごと)</em>
          <b class="mono">{{ vecStr(ea.vec) }} と {{ vecStr(eb.vec) }}</b>
          <span>左は右より「{{ vecAns }}」</span>
        </div>
      </div>
    </div>

    <div class="lc-verdict" :class="split ? 'bad' : 'ok'">
      <template v-if="split">
        答えが割れている。Lamport は「{{ lamAns }}」と順序をつけたが、実際には互いを知らない。
        数を1つに押し込むと「比べられない」を表せないので、必ずどちらかが大きくなる
      </template>
      <template v-else-if="vecAns === '同時'">
        どちらの時計でも順序がついていない
      </template>
      <template v-else>
        因果のある組。この向きは両方の時計で一致する。原因は必ず小さい数を持つ
      </template>
    </div>

    <div class="lc-order">
      <span class="lc-ol mono">Lamport で一列に並べると</span>
      <span class="lc-seq">
        <span
          v-for="(e, i) in sorted"
          :key="e.id"
          class="lc-chip mono"
          :class="i > 0 && compare(sorted[i - 1].vec, e.vec) === '同時' ? 'break' : ''"
        >
          {{ e.node }}{{ e.lam }}
        </span>
      </span>
    </div>

    <p class="lc-legend">
      一列には必ず並ぶ。だが赤い印のところは、隣り合っているのに互いを知らない組になっている。
      並べられることと、その順序が因果を表すことは別になる。ベクタなら「同時」という3つ目の答えを
      持てるので、片方を黙って捨てずに済む。捨てずにどうまとめるかが、次の話になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.lc-brief {
  margin: 0 0 12px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.lc-lines {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.lc-line {
  display: flex;
  align-items: stretch;
  gap: 8px;
}
.lc-node {
  width: 16px;
  flex: none;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  display: flex;
  align-items: center;
}
.lc-evs {
  display: flex;
  gap: 5px;
  flex: 1;
  flex-wrap: wrap;
}
.lc-ev {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 1px;
  padding: 4px 9px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  cursor: pointer;
  text-align: left;
}
.lc-ev.conc {
  border-style: dashed;
  border-color: var(--vp-c-danger-1);
}
.lc-ev.sel {
  border-style: solid;
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.lc-lam {
  font-size: 13px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.lc-vec {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.lc-lab {
  font-size: 9.5px;
  color: var(--vp-c-text-2);
}
.lc-cmp {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 9px 12px;
  background-color: var(--vp-c-bg-soft);
}
.lc-pair {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.lc-vs {
  color: var(--vp-c-text-3);
}
.lc-answers {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.lc-ans {
  flex: 1;
  min-width: 200px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid var(--vp-c-divider);
  padding: 7px 10px;
  background-color: var(--vp-c-bg);
}
.lc-ans em {
  font-style: normal;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.lc-ans b {
  font-size: 12px;
  color: var(--vp-c-text-1);
}
.lc-ans span:last-child {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.lc-ans.conc {
  border-color: var(--vp-c-danger-1);
}
.lc-ans.conc span:last-child {
  color: var(--vp-c-danger-1);
  font-weight: 700;
}
.lc-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.lc-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.lc-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.lc-order {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.lc-ol {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.lc-seq {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.lc-chip {
  font-size: 10.5px;
  padding: 2px 7px;
  border: 1px solid var(--vp-c-divider);
  color: var(--vp-c-text-2);
}
.lc-chip.break {
  border-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.lc-legend {
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
