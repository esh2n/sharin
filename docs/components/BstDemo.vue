<script setup lang="ts">
import { computed, ref } from "vue";
import BstNodeView, { type BstViewNode } from "./BstNodeView.vue";

// Go 版 data-structures/bst の JS ミラー。
interface BNode {
  key: number;
  left?: BNode;
  right?: BNode;
}

const MAX = 31;

let root: BNode | undefined;
let seq = 0;
let lastKey: number | null = null;

const version = ref(0);
const stats = ref({ n: 0, height: -1 });

function insert(key: number) {
  lastKey = key;
  if (!root) {
    root = { key };
    return;
  }
  let n = root;
  for (;;) {
    if (key === n.key) return;
    if (key < n.key) {
      if (!n.left) {
        n.left = { key };
        return;
      }
      n = n.left;
    } else {
      if (!n.right) {
        n.right = { key };
        return;
      }
      n = n.right;
    }
  }
}

function measure(n: BNode | undefined): { count: number; height: number } {
  if (!n) return { count: 0, height: -1 };
  const l = measure(n.left);
  const r = measure(n.right);
  return { count: 1 + l.count + r.count, height: 1 + Math.max(l.height, r.height) };
}

function refresh() {
  const m = measure(root);
  stats.value = { n: m.count, height: m.height };
  version.value++;
}

function addSequential() {
  insert(++seq);
  refresh();
}

function addRandom(n: number) {
  for (let i = 0; i < n; i++) insert(1 + Math.floor(Math.random() * 99));
  refresh();
}

function reset() {
  root = undefined;
  seq = 0;
  lastKey = null;
  refresh();
}

const full = computed(() => stats.value.n >= MAX);
const ideal = computed(() => (stats.value.n === 0 ? -1 : Math.ceil(Math.log2(stats.value.n + 1)) - 1));

const view = computed<BstViewNode | null>(() => {
  version.value;
  const snap = (n: BNode | undefined): BstViewNode | undefined =>
    n && { key: n.key, left: snap(n.left), right: snap(n.right), hot: n.key === lastKey };
  return snap(root) ?? null;
});
</script>

<template>
  <div class="bsd-demo">
    <div class="bsd-controls">
      <button class="bsd-btn brand" type="button" :disabled="full" @click="addSequential">
        昇順で1件挿入
      </button>
      <button class="bsd-btn brand" type="button" :disabled="full" @click="addRandom(1)">
        ランダムに1件挿入
      </button>
      <button class="bsd-btn" type="button" :disabled="full" @click="addRandom(5)">ランダム×5</button>
      <button class="bsd-btn" type="button" @click="reset">リセット</button>
    </div>
    <p class="bsd-stats">
      件数 {{ stats.n }} / 高さ <strong>{{ stats.height }}</strong>
      <template v-if="stats.n > 0">(この件数での理想は {{ ideal }})</template>
      <span v-if="full"> — 上限に達したのでリセットしてください</span>
    </p>
    <div class="bsd-tree">
      <BstNodeView v-if="view" :node="view" />
      <p v-else class="bsd-empty">まだ空です。挿入してみてください。</p>
    </div>
  </div>
</template>

<style scoped>
.bsd-demo {
  margin: 16px 0 24px;
  padding: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg-soft);
}
.bsd-controls {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.bsd-btn {
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-default-soft);
}
.bsd-btn.brand {
  font-weight: 600;
  color: var(--vp-button-brand-text);
  background-color: var(--vp-button-brand-bg);
}
.bsd-btn.brand:hover {
  background-color: var(--vp-button-brand-hover-bg);
}
.bsd-btn:disabled {
  opacity: 0.5;
}
.bsd-stats {
  margin: 10px 0;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.bsd-tree {
  overflow-x: auto;
  padding: 8px 0;
}
.bsd-empty {
  margin: 0;
  font-size: 13px;
  color: var(--vp-c-text-3);
}
</style>
