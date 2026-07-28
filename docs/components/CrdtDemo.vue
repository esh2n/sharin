<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/crdt(Go)を移植。合流の順序を変えても値が変わらないこと、
// 2P-Set と OR-Set で「足し直せるか」が分かれることを見る。

type GC = Record<string, number>;
const NODES = ["a", "b", "c"];

function mergeGC(x: GC, y: GC): GC {
  const out: GC = { ...x };
  for (const k of Object.keys(y)) if (y[k] > (out[k] || 0)) out[k] = y[k];
  return out;
}
const valueGC = (g: GC) => Object.values(g).reduce((s, v) => s + v, 0);

const tab = ref<"counter" | "set">("counter");

// ---- 数え上げ ----
const local = ref<Record<string, GC>>({ a: { a: 3 }, b: { b: 5 }, c: { c: 2 } });
const ORDERS: string[][] = [
  ["a", "b", "c"],
  ["a", "c", "b"],
  ["b", "a", "c"],
  ["b", "c", "a"],
  ["c", "a", "b"],
  ["c", "b", "a"],
];
function bump(n: string) {
  local.value = { ...local.value, [n]: { ...local.value[n], [n]: (local.value[n][n] || 0) + 1 } };
}
const results = computed(() =>
  ORDERS.map((o) => {
    let m: GC = {};
    for (const n of o) m = mergeGC(m, local.value[n]);
    // 二度届いたことにする
    m = mergeGC(mergeGC(m, local.value[o[0]]), local.value[o[o.length - 1]]);
    return { order: o.join(" → "), value: valueGC(m) };
  }),
);
const allSame = computed(() => new Set(results.value.map((r) => r.value)).size === 1);

// ---- 集合 ----
interface Step {
  label: string;
  kind: "add" | "del";
  node: string;
  v: string;
}
const SCRIPT: Step[] = [
  { label: "a が x を足す", kind: "add", node: "a", v: "x" },
  { label: "a が x を消す", kind: "del", node: "a", v: "x" },
  { label: "a が x を足し直す", kind: "add", node: "a", v: "x" },
];
const step = ref(SCRIPT.length);

const twoP = computed(() => {
  const added = new Set<string>();
  const removed = new Set<string>();
  for (const s of SCRIPT.slice(0, step.value)) {
    if (s.kind === "add") added.add(s.v);
    else removed.add(s.v);
  }
  return [...added].filter((v) => !removed.has(v)).sort();
});
const orSet = computed(() => {
  const live: Record<string, string[]> = {};
  const seq: Record<string, number> = {};
  for (const s of SCRIPT.slice(0, step.value)) {
    if (s.kind === "add") {
      seq[s.node] = (seq[s.node] || 0) + 1;
      live[s.v] = [...(live[s.v] || []), `${s.node}${seq[s.node]}`];
    } else {
      delete live[s.v];
    }
  }
  return Object.keys(live).sort();
});

// 並行して足したものが残るか
const concurrent = computed(() => {
  // a と b が独立に x を足し、a だけが消す
  const aLive: Record<string, string[]> = { x: ["a1"] };
  const bLive: Record<string, string[]> = { x: ["b1"] };
  delete aLive["x"]; // a は自分の見えている印だけ消す
  const merged: Record<string, string[]> = {};
  for (const src of [aLive, bLive])
    for (const [v, tags] of Object.entries(src)) merged[v] = [...(merged[v] || []), ...tags];
  const orHas = (merged["x"] || []).length > 0;

  // 2P-Set なら、消した集合に入っているので消える
  const twoPHas = false;
  return { orHas, twoPHas, tags: merged["x"] || [] };
});

const badge = computed(() =>
  tab.value === "counter"
    ? allSame.value
      ? `6通りすべて ${results.value[0].value}`
      : "順序で値が割れている"
    : `2P-Set ${twoP.value.length} 要素 / OR-Set ${orSet.value.length} 要素`,
);
const badgeTone = computed<"ok" | "ng">(() =>
  tab.value === "counter" ? (allSame.value ? "ok" : "ng") : twoP.value.length === orSet.value.length ? "ok" : "ng",
);
</script>

<template>
  <DemoShell title="CRDT" :badge="badge" :badge-tone="badgeTone">
    <div class="cd-tabs">
      <button class="sd-btn" :class="tab === 'counter' ? 'sd-btn--primary' : ''" @click="tab = 'counter'">
        合流の順序
      </button>
      <button class="sd-btn" :class="tab === 'set' ? 'sd-btn--primary' : ''" @click="tab = 'set'">
        消したものを足し直す
      </button>
    </div>

    <!-- 数え上げ -->
    <div v-if="tab === 'counter'" class="cd-panel">
      <div class="cd-ctl">
        <span class="cd-note mono">各ノードが自分の要素だけを増やす</span>
        <button v-for="n in NODES" :key="n" class="sd-btn" @click="bump(n)">{{ n }} で +1</button>
      </div>
      <div class="cd-nodes">
        <div v-for="n in NODES" :key="n" class="cd-node mono">
          <span class="cd-nn">{{ n }}</span>
          <span class="cd-nv">{{ JSON.stringify(local[n]) }}</span>
        </div>
      </div>
      <div class="cd-orders">
        <div v-for="r in results" :key="r.order" class="cd-order mono">
          <span class="cd-oo">{{ r.order }}</span>
          <span class="cd-ov">{{ r.value }}</span>
        </div>
      </div>
      <div class="cd-verdict" :class="allSame ? 'ok' : 'bad'">
        <template v-if="allSame">
          6通りすべての合流順で同じ値になる。しかも最初と最後をもう一度合流させてあるので、
          二度届いても変わらないことも含まれている
        </template>
        <template v-else>順序で値が割れている</template>
      </div>
    </div>

    <!-- 集合 -->
    <div v-else class="cd-panel">
      <div class="cd-ctl">
        <span class="cd-note mono">同じ操作列を2つの型に与える</span>
        <button
          v-for="(s, i) in SCRIPT"
          :key="i"
          class="sd-btn"
          :class="step === i + 1 ? 'sd-btn--primary' : ''"
          @click="step = i + 1"
        >
          {{ i + 1 }}. {{ s.label }}
        </button>
      </div>
      <div class="cd-sets">
        <div class="cd-set" :class="twoP.length ? 'ok' : 'bad'">
          <em>2P-Set</em>
          <b class="mono">{{ twoP.length ? "{" + twoP.join(", ") + "}" : "{}" }}</b>
          <span>消した集合に入ったままなので、足し直しても出てこない</span>
        </div>
        <div class="cd-set" :class="orSet.length ? 'ok' : 'bad'">
          <em>OR-Set</em>
          <b class="mono">{{ orSet.length ? "{" + orSet.join(", ") + "}" : "{}" }}</b>
          <span>足し直すと新しい印がつくので、ちゃんと現れる</span>
        </div>
      </div>

      <div class="cd-conc">
        <div class="cd-conc-h mono">並行して足したものを、片方だけが消したら</div>
        <div class="cd-conc-b mono">
          a が x を足す(印 a1)・b が x を足す(印 b1)・a だけが x を消す → 合流
        </div>
        <div class="cd-sets">
          <div class="cd-set bad">
            <em>2P-Set</em><b class="mono">{}</b>
            <span>消した集合に入っているので、b が足したぶんも消える</span>
          </div>
          <div class="cd-set ok">
            <em>OR-Set</em><b class="mono">&#123;x&#125; 印 {{ concurrent.tags.join(", ") }}</b>
            <span>a は自分の印しか消していない。b の印は見ていないので残る</span>
          </div>
        </div>
      </div>

      <div class="cd-verdict" :class="step === 3 && !twoP.length ? 'bad' : 'ok'">
        <template v-if="step === 3 && !twoP.length">
          同じ操作列なのに結果が違う。消したことをどう表すかで、使える操作が決まる
        </template>
        <template v-else>操作を進めると、2つの型の差が出る</template>
      </div>
    </div>

    <p class="cd-legend">
      「合流の順序」では、3つのノードが勝手に増やしてから合流する。どの順で合流しても、
      二度合流させても、同じ値になる。「消したものを足し直す」では、同じ操作列を2つの型に与える。
      2P-Set は消した集合に入れる形なので足し直せず、並行して足したものまで消える。
      OR-Set は追加ごとの印を消す形なので、どちらも正しく残る。
    </p>
  </DemoShell>
</template>

<style scoped>
.cd-tabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.cd-panel {
  margin-top: 14px;
}
.cd-ctl {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.cd-note {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.cd-nodes {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.cd-node {
  flex: 1;
  min-width: 120px;
  display: flex;
  gap: 8px;
  align-items: baseline;
  border: 1px solid var(--vp-c-divider);
  padding: 5px 10px;
  background-color: var(--vp-c-bg-soft);
}
.cd-nn {
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.cd-nv {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
}
.cd-orders {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 4px;
}
.cd-order {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 10.5px;
  padding: 4px 9px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.cd-ov {
  font-weight: 700;
  color: var(--vp-c-green-1);
}
.cd-sets {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.cd-set {
  flex: 1;
  min-width: 200px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  border: 1px solid var(--vp-c-divider);
  padding: 8px 11px;
  background-color: var(--vp-c-bg-soft);
}
.cd-set em {
  font-style: normal;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.cd-set b {
  font-size: 14px;
}
.cd-set span:last-child {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
}
.cd-set.ok {
  border-color: var(--vp-c-green-1);
}
.cd-set.ok b {
  color: var(--vp-c-green-1);
}
.cd-set.bad {
  border-color: var(--vp-c-danger-1);
}
.cd-set.bad b {
  color: var(--vp-c-danger-1);
}
.cd-conc {
  margin-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 10px;
}
.cd-conc-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.cd-conc-b {
  font-size: 10px;
  color: var(--vp-c-text-3);
  margin: 3px 0 8px;
}
.cd-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.cd-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.cd-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.cd-legend {
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
