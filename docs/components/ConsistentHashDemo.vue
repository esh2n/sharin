<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// コンシステントハッシュのリングを可視化(Go 実装 distributed/consistenthash の考え方を移植)。
// 仮想ノード数を変えると負荷の偏りが、ノードを増減すると「動くキーが少ない」ことが見える。

// CRC32(IEEE)。Go の crc32.ChecksumIEEE と同じ多項式。似た短い文字列でもよく散らばる。
const crcTable = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();
function crc32(s: string): number {
  let c = 0xffffffff;
  for (let i = 0; i < s.length; i++) c = (crcTable[(c ^ s.charCodeAt(i)) & 0xff] ^ (c >>> 8)) >>> 0;
  return (c ^ 0xffffffff) >>> 0;
}

const ALL_NODES = ["A", "B", "C", "D", "E", "F"];
// リング上に描く点(見やすさ優先で少なめ)と、分布バー・移動数を測るキー(標本ノイズを
// 抑えるため多め)を分ける。バーは担当弧の実際の比率に近づき、章の説明と揃う。
const RING_KEYS = Array.from({ length: 150 }, (_, i) => "rk-" + i);
const DIST_COUNT = 3000;
const DIST_KEYS = Array.from({ length: DIST_COUNT }, (_, i) => "key-" + i);
const REPLICA_OPTS = [
  { key: 1, label: "1" },
  { key: 20, label: "20" },
  { key: 100, label: "100" },
];
const COLOR: Record<string, string> = {
  A: "var(--vp-c-brand-1)",
  B: "var(--vp-c-green-1)",
  C: "var(--vp-c-purple-1)",
  D: "var(--vp-c-yellow-1)",
  E: "var(--vp-c-red-1)",
  F: "var(--vp-c-text-2)",
};

const state = reactive({
  nodes: ["A", "B", "C"] as string[],
  replicas: 100,
  note: "仮想ノード数を変えると偏りが、ノードを増減すると『動くキーの少なさ』が見える",
});

interface VPoint {
  p: number;
  node: string;
}
function buildRing(nodes: string[], replicas: number): VPoint[] {
  const pts: VPoint[] = [];
  for (const n of nodes) for (let i = 0; i < replicas; i++) pts.push({ p: crc32(n + "#" + i), node: n });
  pts.sort((a, b) => a.p - b.p);
  return pts;
}
function ownerOf(pts: VPoint[], key: string): string {
  if (!pts.length) return "";
  const h = crc32(key);
  let lo = 0,
    hi = pts.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (pts[mid].p >= h) hi = mid;
    else lo = mid + 1;
  }
  return pts[lo === pts.length ? 0 : lo].node;
}

const ring = computed(() => buildRing(state.nodes, state.replicas));
// リング描画用: 少数キーの担当
const ringAssign = computed(() => {
  const m: Record<string, string> = {};
  for (const k of RING_KEYS) m[k] = ownerOf(ring.value, k);
  return m;
});
// 分布バー用: 多数キーの担当(担当弧の比率をよく表す)
const distAssign = computed(() => {
  const m: Record<string, string> = {};
  for (const k of DIST_KEYS) m[k] = ownerOf(ring.value, k);
  return m;
});
const counts = computed(() => {
  const c: Record<string, number> = {};
  for (const n of state.nodes) c[n] = 0;
  for (const k of DIST_KEYS) c[distAssign.value[k]]++;
  return c;
});

// リング座標。p(0..2^32) を上(12時)始まりの角度へ。
const R = 118;
const CX = 150;
const CY = 150;
function xy(p: number, radius: number) {
  const a = (p / 4294967296) * 2 * Math.PI - Math.PI / 2;
  return { x: CX + radius * Math.cos(a), y: CY + radius * Math.sin(a) };
}
const tickWidth = computed(() => (state.replicas >= 100 ? 1 : state.replicas >= 20 ? 1.5 : 3));
const vpoints = computed(() =>
  ring.value.map((vp) => {
    const in1 = xy(vp.p, R - 7);
    const out1 = xy(vp.p, R + 7);
    return { x1: in1.x, y1: in1.y, x2: out1.x, y2: out1.y, color: COLOR[vp.node] };
  }),
);
const keyDots = computed(() =>
  RING_KEYS.map((k) => {
    const pos = xy(crc32(k), R - 22);
    return { x: pos.x, y: pos.y, color: COLOR[ringAssign.value[k]] };
  }),
);

function measureMove(mutate: () => void) {
  const before = { ...distAssign.value };
  mutate();
  let moved = 0;
  for (const k of DIST_KEYS) if (distAssign.value[k] !== before[k]) moved++;
  return moved;
}

function setReplicas(v: number) {
  state.replicas = v;
  state.note = `仮想ノードを1台あたり ${v} 個に。多いほど担当弧が細かく分かれ、負荷が均等になる`;
}
function addNode() {
  const next = ALL_NODES.find((n) => !state.nodes.includes(n));
  if (!next) {
    state.note = "これ以上ノードを増やせない(デモ上限)";
    return;
  }
  const moved = measureMove(() => state.nodes.push(next));
  state.note = `ノード ${next} を追加。動いたキーは ${moved}/${DIST_COUNT}件だけ(mod なら大半が動く)`;
}
function removeNode() {
  if (state.nodes.length <= 2) {
    state.note = "これ以上減らせない(最低2台)";
    return;
  }
  const removed = state.nodes[state.nodes.length - 1];
  const moved = measureMove(() => state.nodes.pop());
  state.note = `ノード ${removed} を削除。動いたのは ${removed} が持っていた ${moved}/${DIST_COUNT}件だけ`;
}
function reset() {
  state.nodes = ["A", "B", "C"];
  state.replicas = 100;
  state.note = "仮想ノード数を変えると偏りが、ノードを増減すると『動くキーの少なさ』が見える";
}

const badge = computed(() => `${state.nodes.length}ノード / vnode ${state.replicas}`);
const maxCount = computed(() => Math.max(1, ...state.nodes.map((n) => counts.value[n])));
</script>

<template>
  <DemoShell title="コンシステントハッシュ・リング" :badge="badge" badge-tone="neutral">
    <div class="sd-controls">
      <div class="sd-seg">
        <span
          v-for="o in REPLICA_OPTS"
          :key="o.key"
          class="sd-seg-opt"
          :class="{ on: state.replicas === o.key }"
          @click="setReplicas(o.key)"
        >
          vnode {{ o.label }}
        </span>
      </div>
      <button class="sd-btn sd-btn--primary" @click="addNode">ノードを追加</button>
      <button class="sd-btn" @click="removeNode">ノードを削除</button>
      <span class="spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="ch-body">
      <svg class="ch-ring" viewBox="0 0 300 300" role="img" aria-label="ハッシュリング">
        <circle :cx="CX" :cy="CY" :r="R" class="ch-circle" />
        <line
          v-for="(v, i) in vpoints"
          :key="'v' + i"
          :x1="v.x1"
          :y1="v.y1"
          :x2="v.x2"
          :y2="v.y2"
          :stroke="v.color"
          :stroke-width="tickWidth"
        />
        <circle v-for="(d, i) in keyDots" :key="'k' + i" :cx="d.x" :cy="d.y" r="2.4" :fill="d.color" />
      </svg>

      <div class="ch-dist">
        <div v-for="n in state.nodes" :key="n" class="ch-row">
          <span class="ch-dot" :style="{ backgroundColor: COLOR[n] }" />
          <span class="ch-name">{{ n }}</span>
          <span class="ch-bar">
            <span
              class="ch-fill"
              :style="{ width: (counts[n] / maxCount) * 100 + '%', backgroundColor: COLOR[n] }"
            />
          </span>
          <span class="ch-num">{{ counts[n] }}件 ({{ Math.round((counts[n] / DIST_COUNT) * 100) }}%)</span>
        </div>
      </div>
    </div>

    <p class="sd-msg">{{ state.note }}</p>
    <div class="ch-legend">
      <span>外周の色付き線 = 仮想ノード(その物理ノードの担当開始点)</span>
      <span>内側の点 = キー。時計回りで最初に出会うノードの色に染まる</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.ch-body {
  display: flex;
  flex-wrap: wrap;
  gap: 18px;
  align-items: center;
  margin-top: 14px;
}
.ch-ring {
  width: 240px;
  height: 240px;
  flex: 0 0 auto;
}
.ch-circle {
  fill: none;
  stroke: var(--vp-c-divider);
  stroke-width: 1;
}
.ch-dist {
  flex: 1 1 220px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 220px;
}
.ch-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.ch-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: 0 0 auto;
}
.ch-name {
  width: 14px;
  font-weight: 600;
  color: var(--vp-c-text-1);
}
.ch-bar {
  flex: 1 1 auto;
  height: 10px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.ch-fill {
  display: block;
  height: 100%;
}
.ch-num {
  flex: 0 0 auto;
  color: var(--vp-c-text-3);
  font-variant-numeric: tabular-nums;
}
.ch-legend {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 10px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
