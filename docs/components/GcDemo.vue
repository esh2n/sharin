<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/gc(Go)の mark-sweep をブラウザで動かす移植版。
// ヒープを有向グラフとして持ち、tricolor マーキングで到達集合を求め、
// 残った white を掃く。ルートから辿れるかがすべてを決める。

type Col = "white" | "gray" | "black";
interface Node {
  id: string;
  x: number;
  y: number;
  root?: boolean;
}

// 固定レイアウト(決定的)。root→A↔B と root→C→D は生存、E↔F と G はゴミ。
const NODES: Node[] = [
  { id: "root", x: 46, y: 46, root: true },
  { id: "A", x: 138, y: 46 },
  { id: "B", x: 138, y: 122 },
  { id: "C", x: 226, y: 46 },
  { id: "D", x: 226, y: 122 },
  { id: "E", x: 46, y: 182 },
  { id: "F", x: 138, y: 182 },
  { id: "G", x: 258, y: 182 },
];
// 参照(有向辺)。root→C は付け外しできる。
const BASE_EDGES: [string, string][] = [
  ["A", "B"],
  ["B", "A"],
  ["C", "D"],
  ["E", "F"],
  ["F", "E"],
];
const ROOT_EDGES: [string, string][] = [["root", "A"]];
const ROOT_C: [string, string] = ["root", "C"];

const state = reactive({
  col: {} as Record<string, Col>,
  gray: [] as string[],
  removed: new Set<string>(),
  rootCOn: true,
  didSweep: false,
});

// いま有効な辺(消えたノードに繋がる辺は描かない)。
const edges = computed<[string, string][]>(() => {
  const es = [...ROOT_EDGES, ...BASE_EDGES];
  if (state.rootCOn) es.push(ROOT_C);
  return es.filter(([a, b]) => !state.removed.has(a) && !state.removed.has(b));
});

// 各ノードの参照先(有向辺から導出)。
function refsOf(id: string): string[] {
  return edges.value.filter(([a]) => a === id).map(([, b]) => b);
}

const liveNodes = computed(() => NODES.filter((n) => !state.removed.has(n.id)));
const roots = ["root"];

// マーキングを開始(全 white → ルートを gray)。
function restart() {
  state.col = {};
  for (const n of liveNodes.value) state.col[n.id] = "white";
  state.gray = [];
  for (const r of roots) {
    if (!state.removed.has(r)) {
      state.col[r] = "gray";
      state.gray.push(r);
    }
  }
  state.didSweep = false;
}

// gray を1つ black にし、その white の子を gray にする(tricolor の1手)。
function markStep() {
  if (state.gray.length === 0) return;
  const id = state.gray.shift() as string;
  state.col[id] = "black";
  for (const r of refsOf(id)) {
    if (state.col[r] === "white") {
      state.col[r] = "gray";
      state.gray.push(r);
    }
  }
}

// 残った white を回収する。
function sweep() {
  for (const n of liveNodes.value) {
    if (state.col[n.id] === "white") state.removed.add(n.id);
  }
  for (const n of liveNodes.value) state.col[n.id] = "white";
  state.didSweep = true;
}

function toggleRootC() {
  state.rootCOn = !state.rootCOn;
  restart();
}
function reset() {
  state.removed = new Set();
  state.rootCOn = true;
  restart();
}

restart(); // マウント時: ルートが gray、他は white(tricolor の初期状態)

const canMark = computed(() => state.gray.length > 0);
const canSweep = computed(() => state.gray.length === 0 && !state.didSweep && !allWhite.value);
const allWhite = computed(() => liveNodes.value.every((n) => state.col[n.id] === "white"));

const status = computed(() => {
  if (state.didSweep) return `sweep 完了 — 到達不能な ${8 - liveNodes.value.length} 個を回収`;
  if (canMark.value) return `mark 中 — gray(作業リスト): ${state.gray.join(", ")}`;
  return "mark 完了 — 到達集合が確定。sweep で white を回収";
});
const tone = computed<"ok" | "neutral">(() => (state.didSweep ? "ok" : "neutral"));

// 辺をノード境界で切り詰めた線分にする(矢印がノードに刺さって見える)。
function line(a: string, b: string) {
  const na = NODES.find((n) => n.id === a) as Node;
  const nb = NODES.find((n) => n.id === b) as Node;
  const dx = nb.x - na.x;
  const dy = nb.y - na.y;
  const len = Math.sqrt(dx * dx + dy * dy) || 1;
  const t = 27; // ノード半分ぶん詰める
  return {
    x1: na.x + (dx / len) * t,
    y1: na.y + (dy / len) * t,
    x2: nb.x - (dx / len) * t,
    y2: nb.y - (dy / len) * t,
  };
}
const edgeLines = computed(() => edges.value.map(([a, b]) => ({ key: `${a}-${b}`, ...line(a, b) })));
</script>

<template>
  <DemoShell title="gc(mark-sweep)" :badge="status" :badge-tone="tone">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" :disabled="!canMark" @click="markStep">mark 1手</button>
      <button class="sd-btn" :disabled="!canSweep" @click="sweep">sweep(白を回収)</button>
      <span class="spacer" />
      <button class="sd-btn" @click="toggleRootC">{{ state.rootCOn ? "root→C を外す" : "root→C を戻す" }}</button>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="gc-stage">
      <svg viewBox="0 0 300 214" class="gc-svg" role="img" aria-label="ヒープのオブジェクトグラフ">
        <defs>
          <marker id="gc-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M 0 1 L 9 5 L 0 9 z" fill="var(--vp-c-text-3)" />
          </marker>
        </defs>
        <line
          v-for="e in edgeLines"
          :key="e.key"
          :x1="e.x1"
          :y1="e.y1"
          :x2="e.x2"
          :y2="e.y2"
          class="gc-edge"
          marker-end="url(#gc-arrow)"
        />
        <g v-for="n in liveNodes" :key="n.id" :transform="`translate(${n.x},${n.y})`">
          <text v-if="n.root" class="gc-roottag" x="0" y="-22" text-anchor="middle">root</text>
          <rect x="-21" y="-15" width="42" height="30" rx="7" class="gc-node" :class="state.col[n.id]" />
          <text x="0" y="1" text-anchor="middle" dominant-baseline="middle" class="gc-nodetext" :class="state.col[n.id]">
            {{ n.id }}
          </text>
        </g>
      </svg>

      <div class="gc-side">
        <div class="gc-legend">
          <div class="gc-lrow"><span class="gc-sw white" /><b>white</b> 未到達(回収候補)</div>
          <div class="gc-lrow"><span class="gc-sw gray" /><b>gray</b> 到達・子は未走査</div>
          <div class="gc-lrow"><span class="gc-sw black" /><b>black</b> 走査済み(生存確定)</div>
        </div>
        <p class="gc-note">
          ルートから辿れる <code>A↔B</code>・<code>C→D</code> は生存。互いを指し合う <code>E↔F</code> は
          どのルートからも辿れないのでゴミ——<strong>参照カウントが漏らす循環</strong>を mark-sweep は回収できる。
          <code>root→C</code> を外すと <code>C</code>・<code>D</code> がまとめてゴミになる。
        </p>
      </div>
    </div>
  </DemoShell>
</template>

<style scoped>
.gc-stage {
  display: grid;
  grid-template-columns: 1fr 220px;
  gap: 16px;
  margin-top: 16px;
  align-items: start;
}
@media (max-width: 560px) {
  .gc-stage {
    grid-template-columns: 1fr;
  }
}
.gc-svg {
  width: 100%;
  height: auto;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
}
.gc-edge {
  stroke: var(--vp-c-text-3);
  stroke-width: 1.6;
}
.gc-node {
  stroke-width: 1.5;
  transition: fill 0.25s, stroke 0.25s;
}
.gc-node.white {
  fill: var(--vp-c-bg-alt);
  stroke: var(--vp-c-divider);
}
.gc-node.gray {
  fill: #9a9a9a;
  stroke: #7c7c7c;
}
.gc-node.black {
  fill: var(--vp-c-text-1);
  stroke: var(--vp-c-text-1);
}
.gc-nodetext {
  font-size: 13px;
  font-weight: 700;
  font-family: var(--vp-font-family-mono);
  fill: var(--vp-c-text-1);
}
.gc-nodetext.gray,
.gc-nodetext.black {
  fill: var(--vp-c-bg);
}
.gc-roottag {
  font-size: 10px;
  fill: var(--vp-c-brand-1);
  font-weight: 700;
}
.gc-side {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.gc-legend {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.gc-lrow {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.gc-lrow b {
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
}
.gc-sw {
  flex: 0 0 auto;
  width: 16px;
  height: 16px;
  border-radius: 4px;
  border: 1px solid var(--vp-c-divider);
}
.gc-sw.white {
  background-color: var(--vp-c-bg-alt);
}
.gc-sw.gray {
  background-color: #9a9a9a;
}
.gc-sw.black {
  background-color: var(--vp-c-text-1);
}
.gc-note {
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  margin: 0;
}
</style>
