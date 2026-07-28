<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/scheduler(Go)を移植。filter(置けるか)と score(どこが良いか)の
// 2 段階を見せる。戦略を切り替えると score だけが変わり、filter は変わらない。

interface Res {
  cpu: number;
  mem: number;
}
interface NodeT {
  name: string;
  cap: Res;
  used: Res;
  taint: string; // "" なら汚れなし
  pods: string[];
}
interface PodT {
  name: string;
  req: Res;
  tolerates: string;
}
interface Verdict {
  node: string;
  fits: boolean;
  why: string;
}

type Strategy = "spread" | "binpack";

const strategy = ref<Strategy>("spread");
const nodes = ref<NodeT[]>([]);
const seq = ref(0);
const verdicts = ref<Verdict[]>([]);
const scores = ref<{ node: string; score: number }[]>([]);
const placed = ref<{ pod: string; node: string } | null>(null);
const pending = ref<string[]>([]);

function fresh(): NodeT[] {
  return [
    { name: "node-a", cap: { cpu: 2000, mem: 2048 }, used: { cpu: 0, mem: 0 }, taint: "", pods: [] },
    { name: "node-b", cap: { cpu: 2000, mem: 2048 }, used: { cpu: 0, mem: 0 }, taint: "", pods: [] },
    { name: "gpu-1", cap: { cpu: 4000, mem: 4096 }, used: { cpu: 0, mem: 0 }, taint: "hardware=gpu", pods: [] },
  ];
}

function free(n: NodeT): Res {
  return { cpu: n.cap.cpu - n.used.cpu, mem: n.cap.mem - n.used.mem };
}
function pct(a: number, b: number): number {
  return b <= 0 ? 100 : Math.floor((a * 100) / b);
}
function usage(n: NodeT): number {
  return Math.floor((pct(n.used.cpu, n.cap.cpu) + pct(n.used.mem, n.cap.mem)) / 2);
}

// filter: 置けないノードを落とす。可否だけを判定し、優劣はつけない。
function filter(p: PodT): { feasible: NodeT[]; verdicts: Verdict[] } {
  const feasible: NodeT[] = [];
  const vs: Verdict[] = [];
  for (const n of nodes.value) {
    const f = free(n);
    if (p.req.cpu > f.cpu || p.req.mem > f.mem) {
      vs.push({ node: n.name, fits: false, why: `空き不足(${f.cpu}m/${f.mem}Mi < ${p.req.cpu}m/${p.req.mem}Mi)` });
    } else if (n.taint !== "" && n.taint !== p.tolerates) {
      vs.push({ node: n.name, fits: false, why: `汚れ ${n.taint} を許容していない` });
    } else {
      feasible.push(n);
      vs.push({ node: n.name, fits: true, why: "" });
    }
  }
  return { feasible, verdicts: vs };
}

// score: 置いた後の使用率で点をつける。BinPack は高いほど、Spread は低いほど良い。
function score(p: PodT, n: NodeT): number {
  const after = { cpu: n.used.cpu + p.req.cpu, mem: n.used.mem + p.req.mem };
  const packed = Math.floor((pct(after.cpu, n.cap.cpu) + pct(after.mem, n.cap.mem)) / 2);
  return strategy.value === "binpack" ? packed : 100 - packed;
}

function schedule(req: Res, tolerates: string, label: string) {
  seq.value++;
  const p: PodT = { name: `${label}-${seq.value}`, req, tolerates };
  const { feasible, verdicts: vs } = filter(p);
  verdicts.value = vs;
  if (feasible.length === 0) {
    scores.value = [];
    placed.value = null;
    pending.value = [...pending.value, p.name];
    return;
  }
  const ranked = feasible
    .map((n) => ({ node: n.name, score: score(p, n) }))
    .sort((a, b) => (b.score !== a.score ? b.score - a.score : a.node < b.node ? -1 : 1));
  scores.value = ranked;
  const best = nodes.value.find((n) => n.name === ranked[0].node);
  if (!best) return;
  best.used = { cpu: best.used.cpu + p.req.cpu, mem: best.used.mem + p.req.mem };
  best.pods = [...best.pods, p.name];
  nodes.value = [...nodes.value];
  placed.value = { pod: p.name, node: best.name };
}

function addSmall() {
  schedule({ cpu: 500, mem: 512 }, "", "web");
}
function addBig() {
  schedule({ cpu: 1500, mem: 1536 }, "", "batch");
}
function addGpu() {
  schedule({ cpu: 1000, mem: 1024 }, "hardware=gpu", "train");
}
function reset() {
  nodes.value = fresh();
  seq.value = 0;
  verdicts.value = [];
  scores.value = [];
  placed.value = null;
  pending.value = [];
}
reset();

const total = computed(() => nodes.value.reduce((a, n) => a + n.pods.length, 0));
const badge = computed(() => `配置 ${total.value} / Pending ${pending.value.length}`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  pending.value.length > 0 ? "ng" : total.value > 0 ? "ok" : "neutral",
);
const strategyNote = computed(() =>
  strategy.value === "spread"
    ? "Spread: 空きの大きいノードを高く評価する。負荷が散る"
    : "BinPack: 置いた後の使用率が高いノードを高く評価する。1 台に詰まる",
);
</script>

<template>
  <DemoShell title="スケジューラ(Podの配置)" :badge="badge" :badge-tone="badgeTone">
    <div class="ps-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: strategy === 'spread' }" @click="strategy = 'spread'">Spread(散らす)</span>
        <span class="sd-seg-opt" :class="{ on: strategy === 'binpack' }" @click="strategy = 'binpack'">BinPack(詰める)</span>
      </span>
      <span class="ps-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>
    <p class="ps-note">{{ strategyNote }}</p>

    <div class="ps-actions ps-add">
      <button class="sd-btn sd-btn--primary" @click="addSmall">小さいPodを配置(500m/512Mi)</button>
      <button class="sd-btn" @click="addBig">大きいPodを配置(1500m/1536Mi)</button>
      <button class="sd-btn" @click="addGpu">GPU Podを配置(汚れを許容)</button>
    </div>

    <div class="ps-nodes">
      <div v-for="n in nodes" :key="n.name" class="ps-node">
        <div class="ps-node-h">
          <span class="mono ps-node-name">{{ n.name }}</span>
          <span v-if="n.taint" class="ps-taint mono">{{ n.taint }}</span>
          <span class="ps-usage mono">{{ usage(n) }}%</span>
        </div>
        <div class="ps-bar">
          <div class="ps-bar-fill" :style="{ width: usage(n) + '%' }" />
        </div>
        <div class="ps-cap mono">
          空き {{ free(n).cpu }}m / {{ free(n).mem }}Mi(容量 {{ n.cap.cpu }}m / {{ n.cap.mem }}Mi)
        </div>
        <div class="ps-pods">
          <span v-for="p in n.pods" :key="p" class="ps-pod mono">{{ p }}</span>
          <span v-if="n.pods.length === 0" class="ps-empty">(Pod なし)</span>
        </div>
      </div>
    </div>

    <div class="ps-cols">
      <div class="ps-col">
        <div class="ps-col-h">① filter — 置けるか(可否)</div>
        <div v-for="v in verdicts" :key="v.node" class="ps-row mono" :class="v.fits ? 'ok' : 'ng'">
          <span class="ps-row-n">{{ v.node }}</span>
          <span class="ps-row-t">{{ v.fits ? "候補に残る" : v.why }}</span>
        </div>
        <div v-if="verdicts.length === 0" class="ps-empty">(まだ配置していない)</div>
      </div>
      <div class="ps-col">
        <div class="ps-col-h">② score — どこが良いか(優劣)</div>
        <div v-for="(s, i) in scores" :key="s.node" class="ps-row mono" :class="i === 0 ? 'ok' : ''">
          <span class="ps-row-n">{{ s.node }}</span>
          <span class="ps-row-t">{{ s.score }} 点{{ i === 0 ? "(最高点 → ここに置く)" : "" }}</span>
        </div>
        <div v-if="scores.length === 0" class="ps-empty">(候補なし)</div>
      </div>
    </div>

    <div class="ps-verdict" :class="placed ? 'ok' : pending.length > 0 ? 'bad' : 'neutral'">
      <template v-if="placed">配置: {{ placed.pod }} を {{ placed.node }} に束縛。要求のぶん空きが減った</template>
      <template v-else-if="pending.length > 0">
        置けるノードがない: {{ pending[pending.length - 1] }} は Pending のまま(理由は左に表示)
      </template>
      <template v-else>Pod を配置すると、filter と score の過程が出る</template>
    </div>

    <p class="ps-legend">
      戦略を切り替えても filter の結果は変わらない。変わるのは score だけで、どこに置けるかでなく、どこを選ぶかが変わる。
      GPU ノードには汚れが付いているので、許容する Pod しか置けない。小さい Pod をいくつか置いてから大きい Pod を置くと、
      空きが足りずに filter で落ちる。要求は予約なので、Pod が実際に使っていなくても空きは戻らない。
    </p>
  </DemoShell>
</template>

<style scoped>
.ps-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ps-add {
  margin-top: 4px;
}
.ps-spacer {
  flex: 1;
}
.ps-note {
  margin: 8px 0 12px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.ps-nodes {
  display: flex;
  gap: 10px;
  margin-top: 16px;
  flex-wrap: wrap;
}
.ps-node {
  flex: 1;
  min-width: 190px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.ps-node-h {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
}
.ps-node-name {
  font-size: 12px;
  font-weight: 700;
}
.ps-taint {
  font-size: 9.5px;
  padding: 1px 5px;
  background-color: var(--vp-c-warning-soft);
  color: var(--vp-c-warning-1);
}
.ps-usage {
  margin-left: auto;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.ps-bar {
  height: 8px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.ps-bar-fill {
  height: 100%;
  background-color: var(--vp-c-brand-1);
  transition: width 0.2s;
}
.ps-cap {
  margin-top: 5px;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ps-pods {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 7px;
}
.ps-pod {
  font-size: 10px;
  padding: 2px 6px;
  border: 1px solid var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.ps-empty {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.ps-cols {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.ps-col {
  flex: 1;
  min-width: 240px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 92px;
}
.ps-col-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 6px;
}
.ps-row {
  display: flex;
  gap: 8px;
  font-size: 11px;
  padding: 2px 0;
  color: var(--vp-c-text-2);
}
.ps-row-n {
  min-width: 62px;
  font-weight: 700;
}
.ps-row.ok {
  color: var(--vp-c-green-1);
}
.ps-row.ng {
  color: var(--vp-c-danger-1);
}
.ps-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.ps-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ps-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ps-legend {
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
