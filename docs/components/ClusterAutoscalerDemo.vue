<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/clusterautoscaler(Go)を移植。Pending Pod を見てノードを増やし、
// 空いたら集約して減らす。増やす前も減らす前も、スケジューラで確かめる。

const CAP = { cpu: 2000, mem: 2048 };
const MIN_NODES = 1;
const MAX_NODES = 4;
const BOOT = 3;
const SCALE_DOWN_UTIL = 40;

interface Spec {
  name: string;
  cpu: number;
  mem: number;
}
interface NodeT {
  name: string;
  used: { cpu: number; mem: number };
  pods: string[];
}

const nodes = ref<NodeT[]>([]);
const booting = ref<{ name: string; at: number }[]>([]);
const pending = ref<Spec[]>([]);
const specs = ref<Record<string, Spec>>({});
const place = ref<Record<string, string>>({});
const now = ref(0);
const seq = ref(0);
const nodeSeq = ref(0);
const log = ref<string[]>([]);

const freeOf = (n: NodeT) => ({ cpu: CAP.cpu - n.used.cpu, mem: CAP.mem - n.used.mem });
const utilOf = (n: NodeT) =>
  Math.floor((Math.floor((n.used.cpu * 100) / CAP.cpu) + Math.floor((n.used.mem * 100) / CAP.mem)) / 2);
const fitsOn = (s: Spec, n: NodeT) => {
  const f = freeOf(n);
  return s.cpu <= f.cpu && s.mem <= f.mem;
};

// filter で候補を絞り、score(BinPack=詰める)で選ぶ。前章と同じ手順。
function schedule(s: Spec, ns: NodeT[]): NodeT | null {
  const feasible = ns.filter((n) => fitsOn(s, n));
  if (feasible.length === 0) return null;
  const ranked = feasible
    .map((n) => ({
      n,
      score: Math.floor(
        (Math.floor(((n.used.cpu + s.cpu) * 100) / CAP.cpu) + Math.floor(((n.used.mem + s.mem) * 100) / CAP.mem)) / 2,
      ),
    }))
    .sort((a, b) => (b.score !== a.score ? b.score - a.score : a.n.name < b.n.name ? -1 : 1));
  const best = ranked[0].n;
  best.used = { cpu: best.used.cpu + s.cpu, mem: best.used.mem + s.mem };
  best.pods = [...best.pods, s.name];
  return best;
}

function freshNodes(except?: NodeT): NodeT[] {
  return nodes.value.filter((n) => n !== except).map((n) => ({ name: n.name, used: { cpu: 0, mem: 0 }, pods: [] }));
}
function podOrder(): string[] {
  return Object.keys(place.value).sort((a, b) => {
    const pa = specs.value[a], pb = specs.value[b];
    return pa.cpu !== pb.cpu ? pb.cpu - pa.cpu : a < b ? -1 : 1;
  });
}
function packInto(ns: NodeT[]): Record<string, string> | null {
  const out: Record<string, string> = {};
  for (const name of podOrder()) {
    const n = schedule(specs.value[name], ns);
    if (!n) return null;
    out[name] = n.name;
  }
  return out;
}

function submit(cpu: number, mem: number, label: string) {
  seq.value++;
  const s: Spec = { name: `${label}-${seq.value}`, cpu, mem };
  specs.value = { ...specs.value, [s.name]: s };
  const n = schedule(s, nodes.value);
  if (n) {
    place.value = { ...place.value, [s.name]: n.name };
    nodes.value = [...nodes.value];
  } else {
    pending.value = [...pending.value, s];
    log.value = [...log.value, `t=${now.value} ${s.name} は置き場所がない。Pending のまま`];
  }
}

function removeOne() {
  const names = Object.keys(place.value);
  if (names.length === 0) return;
  const victim = names.sort()[names.length - 1];
  const rest = { ...place.value };
  delete rest[victim];
  place.value = rest;
  const sp = { ...specs.value };
  delete sp[victim];
  specs.value = sp;
  const fresh = freshNodes();
  const packed = packInto(fresh);
  if (packed) {
    nodes.value = fresh;
    place.value = packed;
  }
  log.value = [...log.value, `t=${now.value} ${victim} を削除`];
}

function tick() {
  now.value++;
  const msgs: string[] = [];

  // ① 起動が終わったノードを使えるようにする。
  const still: { name: string; at: number }[] = [];
  for (const b of booting.value) {
    if (now.value >= b.at) {
      nodes.value = [...nodes.value, { name: b.name, used: { cpu: 0, mem: 0 }, pods: [] }];
      msgs.push(`t=${now.value} ${b.name} が使えるようになった`);
    } else still.push(b);
  }
  booting.value = still;

  // ② Pending を置けるだけ置く。
  const rest: Spec[] = [];
  for (const s of pending.value) {
    const n = schedule(s, nodes.value);
    if (n) place.value = { ...place.value, [s.name]: n.name };
    else rest.push(s);
  }
  pending.value = rest;
  nodes.value = [...nodes.value];

  // ③ 増やす。足す前に、新しい1台に載るかを確かめる。
  if (pending.value.length > 0 && nodes.value.length + booting.value.length < MAX_NODES) {
    const hypo: NodeT = { name: "hypothetical", used: { cpu: 0, mem: 0 }, pods: [] };
    if (pending.value.some((s) => fitsOn(s, hypo))) {
      nodeSeq.value++;
      const name = `node-${nodeSeq.value}`;
      booting.value = [...booting.value, { name, at: now.value + BOOT }];
      msgs.push(`t=${now.value} ${name} を追加(起動に ${BOOT} 周期かかる)`);
    } else {
      msgs.push(`t=${now.value} ノードを足しても Pending は解消しない。増やさない`);
    }
  }

  // ④ 減らす。消す前に、載っている Pod の行き先を確かめる。
  if (nodes.value.length > MIN_NODES && pending.value.length === 0) {
    for (const victim of nodes.value) {
      if (utilOf(victim) >= SCALE_DOWN_UTIL) continue;
      const fresh = freshNodes(victim);
      const packed = fresh.length > 0 ? packInto(fresh) : null;
      if (packed) {
        nodes.value = fresh;
        place.value = packed;
        msgs.push(`t=${now.value} ${victim.name} は使用率が低く、載っている Pod も他へ移せる。削除`);
        break;
      }
      msgs.push(`t=${now.value} ${victim.name} は使用率が低いが、載っている Pod の行き先がない。残す`);
      break;
    }
  }

  if (msgs.length) log.value = [...log.value, ...msgs];
}

function tick5() {
  for (let i = 0; i < 5; i++) tick();
}
function reset() {
  nodeSeq.value = MIN_NODES;
  nodes.value = Array.from({ length: MIN_NODES }, (_, i) => ({
    name: `node-${i + 1}`,
    used: { cpu: 0, mem: 0 },
    pods: [],
  }));
  booting.value = [];
  pending.value = [];
  specs.value = {};
  place.value = {};
  now.value = 0;
  seq.value = 0;
  log.value = [];
}
reset();

const total = computed(() => nodes.value.length + booting.value.length);
const badge = computed(() => `ノード ${nodes.value.length} + 起動中 ${booting.value.length} / Pending ${pending.value.length}`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  pending.value.length > 0 ? "ng" : Object.keys(place.value).length > 0 ? "ok" : "neutral",
);
</script>

<template>
  <DemoShell title="Cluster Autoscaler" :badge="badge" :badge-tone="badgeTone">
    <div class="ca-actions">
      <button class="sd-btn sd-btn--primary" @click="submit(500, 512, 'web')">小さいPod(500m/512Mi)</button>
      <button class="sd-btn" @click="submit(1500, 1536, 'batch')">大きいPod(1500m/1536Mi)</button>
      <button class="sd-btn" @click="submit(9000, 9000, 'huge')">1台に収まらないPod</button>
      <button class="sd-btn" @click="removeOne">Podを1つ消す</button>
    </div>
    <div class="ca-actions ca-second">
      <button class="sd-btn sd-btn--primary" @click="tick">1周期進める</button>
      <button class="sd-btn" @click="tick5">5周期進める</button>
      <span class="ca-spacer" />
      <span class="ca-clock mono">t={{ now }}</span>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <p class="ca-fixed mono">
      1台 {{ CAP.cpu }}m / {{ CAP.mem }}Mi ・ {{ MIN_NODES }}〜{{ MAX_NODES }}台 ・ 起動に {{ BOOT }} 周期 ・
      使用率 {{ SCALE_DOWN_UTIL }}% 未満が縮小の候補
    </p>

    <div class="ca-nodes">
      <div v-for="n in nodes" :key="n.name" class="ca-node">
        <div class="ca-node-h">
          <span class="mono ca-node-name">{{ n.name }}</span>
          <span class="ca-util mono">{{ utilOf(n) }}%</span>
        </div>
        <div class="ca-bar"><div class="ca-bar-fill" :style="{ width: utilOf(n) + '%' }" /></div>
        <div class="ca-pods">
          <span v-for="p in n.pods" :key="p" class="ca-pod mono">{{ p }}</span>
          <span v-if="n.pods.length === 0" class="ca-empty">(空)</span>
        </div>
      </div>
      <div v-for="b in booting" :key="b.name" class="ca-node ca-booting">
        <div class="ca-node-h">
          <span class="mono ca-node-name">{{ b.name }}</span>
          <span class="ca-util mono">起動中</span>
        </div>
        <div class="ca-boot mono">あと {{ b.at - now }} 周期で使えるようになる</div>
      </div>
    </div>

    <div class="ca-pending">
      <span class="ca-pending-h">Pending(置き場所がない)</span>
      <span v-for="s in pending" :key="s.name" class="ca-pod pend mono">{{ s.name }}</span>
      <span v-if="pending.length === 0" class="ca-empty">(なし)</span>
    </div>

    <div class="ca-verdict" :class="pending.length > 0 ? 'bad' : 'ok'">
      <template v-if="pending.length > 0">
        置けない Pod が {{ pending.length }} 件。周期を進めると、足せば解消するかを確かめてからノードを増やす
      </template>
      <template v-else-if="total > MIN_NODES">
        {{ nodes.length }} 台で全部の Pod が載っている。Pod を消して空きが増えると、集約して台数が減る
      </template>
      <template v-else>ノード {{ nodes.length }} 台。Pod を追加すると、空きが尽きたところで増える</template>
    </div>

    <div class="ca-log">
      <div class="ca-log-h">起きたこと</div>
      <div v-for="(l, i) in log.slice(-8)" :key="i" class="ca-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="ca-empty">(まだ何も起きていない)</div>
    </div>

    <p class="ca-legend">
      小さい Pod を 5 つ以上足すと空きが尽き、Pending が出る。周期を進めると、新しい1台に載るかを確かめてから
      ノードが増える。起動には時間がかかるので、その間 Pending は待つ。「1台に収まらないPod」は何台足しても
      置けないので、増やす判断そのものが見送られる。Pod を消して空きが増えると、載っている Pod の行き先を
      確かめたうえでノードが減る。増やすときも減らすときも、判断はスケジューラに委ねられている。
    </p>
  </DemoShell>
</template>

<style scoped>
.ca-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ca-second {
  margin-top: 8px;
}
.ca-spacer {
  flex: 1;
}
.ca-clock {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.ca-fixed {
  margin: 10px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.ca-nodes {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.ca-node {
  flex: 1;
  min-width: 160px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.ca-booting {
  border-style: dashed;
  opacity: 0.75;
}
.ca-node-h {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
}
.ca-node-name {
  font-size: 12px;
  font-weight: 700;
}
.ca-util {
  margin-left: auto;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.ca-bar {
  height: 8px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.ca-bar-fill {
  height: 100%;
  background-color: var(--vp-c-brand-1);
  transition: width 0.2s;
}
.ca-boot {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.ca-pods {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 7px;
}
.ca-pod {
  font-size: 10px;
  padding: 2px 6px;
  border: 1px solid var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.ca-pod.pend {
  border-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.ca-empty {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.ca-pending {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 12px;
  padding: 8px 12px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.ca-pending-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-right: 4px;
}
.ca-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.ca-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ca-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ca-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 54px;
}
.ca-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.ca-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.ca-legend {
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
