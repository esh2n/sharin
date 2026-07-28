<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/topology(Go)を移植。区画ごとに数えることと、
// 散らし方の違いが「1区画を失ったとき何個残るか」に出ることを見せる。

const ZONES = ["zone-a", "zone-b", "zone-c"];
const NODES = ZONES.flatMap((z) => [z + "-1", z + "-2"]);
const zoneOf = (node: string) => node.slice(0, node.lastIndexOf("-"));

const maxSkew = ref(1);
const total = ref(6);

interface Placement {
  node: string;
  pods: string[];
}

function place(skew: number, n: number): Placement[] {
  const nodes: Placement[] = NODES.map((name) => ({ node: name, pods: [] }));
  const countZone = (z: string) =>
    nodes.filter((x) => zoneOf(x.node) === z).reduce((a, x) => a + x.pods.length, 0);
  const skewAfter = (z: string) => {
    const counts = ZONES.map((zz) => countZone(zz) + (zz === z ? 1 : 0));
    return Math.max(...counts) - Math.min(...counts);
  };

  for (let i = 1; i <= n; i++) {
    let feasible = ZONES.filter((z) => skewAfter(z) <= skew);
    if (feasible.length === 0) feasible = ZONES;
    let best = feasible[0];
    for (const z of feasible) if (countZone(z) < countZone(best)) best = z;
    const inZone = nodes.filter((x) => zoneOf(x.node) === best);
    let node = inZone[0];
    for (const x of inZone) if (x.pods.length < node.pods.length) node = x;
    node.pods = [...node.pods, `web-${i}`];
  }
  return nodes;
}

const layout = computed(() => place(maxSkew.value, total.value));
const countZone = (z: string) =>
  layout.value.filter((x) => zoneOf(x.node) === z).reduce((a, x) => a + x.pods.length, 0);
const skew = computed(() => {
  const counts = ZONES.map(countZone);
  return Math.max(...counts) - Math.min(...counts);
});
const worstZoneLoss = computed(() => {
  const counts = ZONES.map(countZone);
  return total.value - Math.max(...counts);
});
const worstNodeLoss = computed(() => total.value - Math.max(...layout.value.map((x) => x.pods.length)));

// 比較用: 1区画に寄せた場合
const packedZoneLoss = 0;

const badge = computed(() => `偏り ${skew.value} ・ 1区画を失うと ${worstZoneLoss.value}/${total.value} 残る`);
const badgeTone = computed<"ok" | "ng">(() => (worstZoneLoss.value >= Math.ceil(total.value / 2) ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="topology spread" :badge="badge" :badge-tone="badgeTone">
    <div class="tp-row">
      <span class="tp-label">maxSkew(区画ごとの数の差の上限)</span>
      <span class="sd-seg">
        <span v-for="n in [1, 2, 6]" :key="n" class="sd-seg-opt" :class="{ on: maxSkew === n }" @click="maxSkew = n">{{ n }}</span>
      </span>
    </div>
    <div class="tp-row">
      <span class="tp-label">置く Pod の数</span>
      <span class="sd-seg">
        <span v-for="n in [3, 6, 9]" :key="n" class="sd-seg-opt" :class="{ on: total === n }" @click="total = n">{{ n }}</span>
      </span>
    </div>

    <div class="tp-zones">
      <div v-for="z in ZONES" :key="z" class="tp-zone">
        <div class="tp-zone-h">
          <span class="mono tp-zone-n">{{ z }}</span>
          <span class="mono tp-zone-c">{{ countZone(z) }} 個</span>
        </div>
        <div class="tp-nodes">
          <div v-for="n in layout.filter((x) => zoneOf(x.node) === z)" :key="n.node" class="tp-node">
            <span class="mono tp-node-n">{{ n.node }}</span>
            <span class="tp-pods">
              <span v-for="p in n.pods" :key="p" class="tp-pod mono">{{ p }}</span>
              <span v-if="n.pods.length === 0" class="tp-empty">(空)</span>
            </span>
          </div>
        </div>
      </div>
    </div>

    <div class="tp-loss">
      <div class="tp-loss-row">
        <span class="tp-loss-l">ノードを1台失うと</span>
        <span class="tp-track">
          <span v-for="i in total" :key="i" class="tp-slot" :class="i <= worstNodeLoss ? 'ok' : 'lost'" />
        </span>
        <span class="mono tp-loss-v">{{ worstNodeLoss }} / {{ total }} 残る</span>
      </div>
      <div class="tp-loss-row">
        <span class="tp-loss-l">区画を1つ失うと</span>
        <span class="tp-track">
          <span v-for="i in total" :key="i" class="tp-slot" :class="i <= worstZoneLoss ? 'ok' : 'lost'" />
        </span>
        <span class="mono tp-loss-v">{{ worstZoneLoss }} / {{ total }} 残る</span>
      </div>
      <div class="tp-loss-row">
        <span class="tp-loss-l">1区画に寄せていたら</span>
        <span class="tp-track">
          <span v-for="i in total" :key="i" class="tp-slot lost" />
        </span>
        <span class="mono tp-loss-v">{{ packedZoneLoss }} / {{ total }} 残る</span>
      </div>
    </div>

    <div class="tp-verdict" :class="worstZoneLoss >= Math.ceil(total / 2) ? 'ok' : 'bad'">
      <template v-if="maxSkew >= total">
        許容量が大きいので、制約としてはどんな偏りも許している。実装は少ない区画を選ぶので結果は散るが、制約が守っているわけではない
      </template>
      <template v-else-if="worstZoneLoss >= Math.ceil(total / 2)">
        区画をまたいで散っている。1区画を失っても {{ worstZoneLoss }} 個が残り、過半数を保てる
      </template>
      <template v-else>
        1区画に寄っている。その区画が落ちると {{ total - worstZoneLoss }} 個を一度に失う
      </template>
    </div>

    <p class="tp-legend">
      ノードを1台失う場合と、区画を1つ失う場合で、残る数がまるで違う。ノードに均等に置くだけでは、区画の障害には
      備えられない。偏りを数える単位が、そのまま何の障害に備えるかを決めている。maxSkew を大きくすると制約が
      緩み、散らばりを保証しなくなる。いちばん下の帯は、同じ数を1区画に寄せていた場合で、その区画が落ちると
      1つも残らない。
    </p>
  </DemoShell>
</template>

<style scoped>
.tp-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.tp-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  min-width: 210px;
}
.tp-zones {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.tp-zone {
  flex: 1;
  min-width: 160px;
  border: 1px solid var(--vp-c-brand-1);
  padding: 8px 10px;
  background-color: var(--vp-c-bg-soft);
}
.tp-zone-h {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 6px;
}
.tp-zone-n {
  font-size: 11.5px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
}
.tp-zone-c {
  margin-left: auto;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.tp-nodes {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.tp-node {
  border: 1px solid var(--vp-c-divider);
  padding: 5px 7px;
  background-color: var(--vp-c-bg);
}
.tp-node-n {
  display: block;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  margin-bottom: 3px;
}
.tp-pods {
  display: flex;
  gap: 3px;
  flex-wrap: wrap;
}
.tp-pod {
  font-size: 9.5px;
  padding: 1px 5px;
  border: 1px solid var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.tp-empty {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.tp-loss {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.tp-loss-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.tp-loss-l {
  font-size: 11px;
  color: var(--vp-c-text-2);
  min-width: 130px;
}
.tp-track {
  display: flex;
  gap: 3px;
  flex: 1;
}
.tp-slot {
  flex: 1;
  max-width: 24px;
  height: 12px;
  border: 1px solid var(--vp-c-divider);
}
.tp-slot.ok {
  background-color: var(--vp-c-green-1);
  border-color: var(--vp-c-green-1);
}
.tp-slot.lost {
  background-color: var(--vp-c-danger-soft);
  border-color: var(--vp-c-danger-1);
}
.tp-loss-v {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
  min-width: 82px;
  text-align: right;
}
.tp-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.tp-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.tp-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.tp-legend {
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
