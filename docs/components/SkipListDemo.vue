<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 skiplist の JS ミラー。挿入と、検索経路(飛ばし読み)の可視化。
const MAX_LEVEL = 5;

interface SNode {
  key: number;
  height: number;
}

// mulberry32 でシード付き乱数(表示が安定するよう)。
let seed = 12345;
function rand() {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) | 0;
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
}
function randomLevel() {
  let lvl = 1;
  while (lvl < MAX_LEVEL && rand() < 0.5) lvl++;
  return lvl;
}

const nodes = ref<SNode[]>([]);
const path = ref<Set<string>>(new Set()); // "key:level" の集合(検索経路)
const message = ref("");

function insert(key: number) {
  if (nodes.value.some((n) => n.key === key)) return;
  const n = { key, height: randomLevel() };
  const next = [...nodes.value, n].sort((a, b) => a.key - b.key);
  nodes.value = next;
}

function preset() {
  seed = 12345;
  nodes.value = [];
  for (const k of [1, 3, 6, 7, 9, 12, 17, 19, 21, 25]) insert(k);
  path.value = new Set();
  message.value = "10ノードを挿入。高さはコイン投げで決まっている";
}

const level = computed(() => Math.max(1, ...nodes.value.map((n) => n.height)));

// 検索経路を計算。現在位置(curKey)を段をまたいで保持するのが本物と同じ動き。
// 上の段で進んだ位置から下の段を続けるので、下段で最初から辿り直さない。
function search(key: number) {
  const p = new Set<string>();
  const sorted = nodes.value;
  let curKey = Number.NEGATIVE_INFINITY;
  let steps = 0;
  for (let lvl = level.value - 1; lvl >= 0; lvl--) {
    const onLevel = sorted.filter((n) => n.height > lvl);
    // その段で「現在位置より後ろ、かつ key 未満」のノードへ進む
    for (const n of onLevel) {
      if (n.key > curKey && n.key < key) {
        curKey = n.key;
        p.add(`${n.key}:${lvl}`);
        steps++;
      }
    }
  }
  const found = sorted.find((n) => n.key === key);
  if (found) p.add(`${key}:0`);
  path.value = p;
  message.value = found
    ? `${key} を検索: 上の段で飛ばして ${steps} 歩で到達(緑の経路)`
    : `${key} は無い(検索経路が空振り)`;
}

// 描画用: 段ごとのノード配置。各段には height>lvl のノードだけ現れる。
const grid = computed(() => {
  const rows = [];
  for (let lvl = level.value - 1; lvl >= 0; lvl--) {
    rows.push({
      lvl,
      cells: nodes.value.map((n) => ({
        key: n.key,
        present: n.height > lvl,
        onPath: path.value.has(`${n.key}:${lvl}`),
      })),
    });
  }
  return rows;
});

preset();
</script>

<template>
  <DemoShell title="スキップリスト" :badge="`${nodes.length}件 / ${level}段`" badge-tone="neutral">
    <div class="sd-controls">
      <span class="sk-caption">検索:</span>
      <button v-for="k in [7, 17, 25]" :key="k" class="sd-btn sd-btn--primary" type="button" @click="search(k)">
        {{ k }} を探す
      </button>
      <span class="spacer"></span>
      <button class="sd-btn" type="button" @click="preset">作り直す</button>
    </div>

    <p v-if="message" class="sd-msg">{{ message }}</p>

    <div class="sk-grid">
      <div v-for="row in grid" :key="row.lvl" class="sk-row">
        <span class="sk-lvl">L{{ row.lvl }}</span>
        <div class="sk-cells">
          <span
            v-for="c in row.cells"
            :key="c.key"
            class="sk-cell"
            :class="{ present: c.present, path: c.onPath }"
          >{{ c.present ? c.key : "" }}</span>
        </div>
      </div>
    </div>
  </DemoShell>
</template>

<style scoped>
.sk-caption {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.sk-grid {
  margin-top: 14px;
  overflow-x: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.sk-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sk-lvl {
  width: 24px;
  font-size: 11px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
.sk-cells {
  display: flex;
  gap: 3px;
}
.sk-cell {
  width: 30px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  background-color: transparent;
}
.sk-cell.present {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.sk-cell.path {
  background-color: var(--vp-c-green-1);
  color: var(--vp-c-bg);
  font-weight: 700;
}
</style>
