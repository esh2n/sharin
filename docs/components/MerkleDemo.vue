<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/antientropy(Go)を移植。区画 16、key は 32 件。
// 要約は Go と同じ FNV-1a(64bit)なので、区画の割り当ても同じになる。

const BUCKETS = 16;
const KEYS = Array.from({ length: 32 }, (_, i) => `key${i}`);
const CANDIDATES = ["key5", "key12", "key20"];

const M = (1n << 64n) - 1n;
const PRIME = 1099511628211n;
const OFFSET = 14695981039346656037n;

function hash(s: string): bigint {
  let h = OFFSET;
  for (const byte of new TextEncoder().encode(s)) {
    h = (h ^ BigInt(byte)) & M;
    h = (h * PRIME) & M;
  }
  return h;
}
function mix(a: bigint, b: bigint): bigint {
  let h = OFFSET;
  h = ((h ^ a) & M) * PRIME & M;
  h = ((h ^ b) & M) * PRIME & M;
  return h;
}
const bucketOf = (k: string) => Number(hash(k) % BigInt(BUCKETS));

const changed = ref<Record<string, boolean>>({ key5: true });

// 2台ぶんの木を組む。b 側だけ、印を付けた key の値が新しくなっている。
function build(side: "a" | "b"): bigint[] {
  const nodes = new Array<bigint>(2 * BUCKETS).fill(0n);
  const byBucket: string[][] = Array.from({ length: BUCKETS }, () => []);
  for (const k of KEYS) byBucket[bucketOf(k)].push(k);
  for (let i = 0; i < BUCKETS; i++) {
    let h = 0n;
    for (const k of byBucket[i].slice().sort()) {
      const newer = side === "b" && changed.value[k];
      const v = newer ? "あたらしい" : k.replace("key", "v");
      h = mix(h, hash(`${k}=${v}@${newer ? 2 : 1}`));
    }
    nodes[BUCKETS + i] = h;
  }
  for (let i = BUCKETS - 1; i >= 1; i--) {
    const l = nodes[2 * i];
    const r = nodes[2 * i + 1];
    if (l === 0n && r === 0n) continue;
    nodes[i] = mix(l, r);
  }
  return nodes;
}

const walk = computed(() => {
  const a = build("a");
  const b = build("b");
  const state = new Array<"skip" | "same" | "diff">(2 * BUCKETS).fill("skip");
  const diffBuckets: number[] = [];
  let compared = 0;
  const go = (i: number) => {
    compared++;
    if (a[i] === b[i]) {
      state[i] = "same";
      return;
    }
    state[i] = "diff";
    if (i >= BUCKETS) {
      diffBuckets.push(i - BUCKETS);
      return;
    }
    go(2 * i);
    go(2 * i + 1);
  };
  go(1);
  const want = new Set(diffBuckets);
  const sent = KEYS.filter((k) => want.has(bucketOf(k))).length;
  return { state, diffBuckets, compared, sent };
});

const LEVELS = [1, 2, 4, 8, 16];
const rowOf = (level: number) => {
  const start = 2 ** level;
  return Array.from({ length: LEVELS[level] }, (_, i) => start + i);
};

const marked = computed(() => CANDIDATES.filter((k) => changed.value[k]));
const badge = computed(() => `違い ${marked.value.length} 件 / ${KEYS.length} 件中`);
const verdict = computed(() => {
  if (!marked.value.length)
    return "根が一致したので、そこで打ち切る。中身は1件も送らずに「同じ」と分かる";
  const w = walk.value;
  return `違う区画は ${w.diffBuckets.join("、")} 番。要約を ${w.compared} 個比べて、中身は ${w.sent} 件だけ送れば直る。全件で比べると ${KEYS.length} 件送ることになる`;
});
</script>

<template>
  <DemoShell title="差分の突き合わせ" :badge="badge" :badge-tone="marked.length ? 'neutral' : 'ok'">
    <div class="mk-actions">
      <span class="mk-label">片方だけ書き換える</span>
      <button
        v-for="k in CANDIDATES"
        :key="k"
        class="sd-btn"
        :class="changed[k] ? 'sd-btn--primary' : ''"
        @click="changed = { ...changed, [k]: !changed[k] }"
      >
        {{ k }}<span class="mk-sub">区画 {{ bucketOf(k) }}</span>
      </button>
      <span class="mk-gap" />
      <button class="sd-btn" @click="changed = {}">そろえる</button>
    </div>

    <div class="mk-tree">
      <div v-for="(n, level) in LEVELS" :key="level" class="mk-row">
        <span class="mk-level mono">{{ level === 4 ? "区画" : level === 0 ? "根" : `${level} 段目` }}</span>
        <span
          v-for="i in rowOf(level)"
          :key="i"
          class="mk-node"
          :class="walk.state[i]"
        >{{ level === 4 ? i - 16 : "" }}</span>
      </div>
    </div>

    <div class="mk-legendrow">
      <span class="mk-key same">一致したので打ち切り</span>
      <span class="mk-key diff">違うので降りる</span>
      <span class="mk-key skip">見ていない</span>
    </div>

    <div class="mk-table">
      <div class="mk-cell head">やり方</div>
      <div class="mk-cell head num">比べた数</div>
      <div class="mk-cell head num">送った件数</div>
      <div class="mk-cell">全件で比べる</div>
      <div class="mk-cell num mono bad">{{ KEYS.length }}</div>
      <div class="mk-cell num mono bad">{{ KEYS.length }}</div>
      <div class="mk-cell strong">木で降りる</div>
      <div class="mk-cell num mono ok">{{ walk.compared }}</div>
      <div class="mk-cell num mono ok">{{ walk.sent }}</div>
    </div>

    <div class="mk-verdict" :class="marked.length ? 'work' : 'ok'">{{ verdict }}</div>

    <p class="mk-note">
      32 件の key を 16 区画に割り振り、区画ごとの要約を葉にして木を組んでいる。片方だけ値を
      書き換えると、その区画から根までの要約が変わる。突き合わせは根から降りて、一致したところで
      打ち切るので、見ないまま済む枝が大きく残る。違いを増やすほど降りる枝が増え、木の得は減っていく。
    </p>
  </DemoShell>
</template>

<style scoped>
.mk-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.mk-label {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.mk-gap {
  flex: 1;
  min-width: 8px;
}
.mk-sub {
  font-size: 9px;
  opacity: 0.7;
  margin-left: 6px;
}
.mk-tree {
  margin-top: 14px;
  padding: 10px 12px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.mk-row {
  display: flex;
  align-items: center;
  gap: 3px;
  margin-bottom: 4px;
}
.mk-row:last-child {
  margin-bottom: 0;
}
.mk-level {
  width: 52px;
  flex: none;
  font-size: 9px;
  color: var(--vp-c-text-3);
}
.mk-node {
  flex: 1;
  height: 18px;
  line-height: 18px;
  text-align: center;
  font-size: 8.5px;
  font-family: var(--vp-font-family-mono);
  border: 1px solid transparent;
}
.mk-node.same {
  border-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.mk-node.diff {
  border-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.mk-node.skip {
  border-color: var(--vp-c-divider);
  color: var(--vp-c-text-3);
  background-color: transparent;
}
.mk-legendrow {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
  margin-top: 8px;
}
.mk-key {
  font-size: 10px;
  color: var(--vp-c-text-3);
  padding-left: 14px;
  position: relative;
}
.mk-key::before {
  content: "";
  position: absolute;
  left: 0;
  top: 0.35em;
  width: 9px;
  height: 9px;
  border: 1px solid var(--vp-c-divider);
}
.mk-key.same::before {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.mk-key.diff::before {
  border-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.mk-table {
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: 0 24px;
  margin-top: 14px;
  font-size: 12.5px;
}
.mk-cell {
  padding: 6px 0;
  border-bottom: 1px solid var(--vp-c-divider);
  color: var(--vp-c-text-2);
}
.mk-cell.head {
  font-size: 10.5px;
  font-weight: 600;
  color: var(--vp-c-text-3);
  border-bottom-color: var(--vp-c-text-3);
  padding-bottom: 4px;
}
.mk-cell.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
  min-width: 72px;
}
.mk-cell.strong {
  color: var(--vp-c-text-1);
  font-weight: 600;
}
.mk-cell.ok {
  color: var(--vp-c-green-1);
  font-weight: 600;
}
.mk-cell.bad {
  color: var(--vp-c-danger-1);
}
.mk-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.mk-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.mk-verdict.work {
  border-left-color: var(--vp-c-brand-1);
  color: var(--vp-c-text-1);
}
.mk-note {
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
