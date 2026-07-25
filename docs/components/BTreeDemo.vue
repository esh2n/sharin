<script setup lang="ts">
import { computed, ref } from "vue";
import BTreeNodeView, { type ViewNode } from "./BTreeNodeView.vue";
import DemoShell from "./DemoShell.vue";

// Go 版 data-structures/btree の JS ミラー。t=2 (1ノード最大3キー) で可視化する。
const T = 2;
const MAX_KEYS = 120;

interface BNode {
  id: number;
  keys: number[];
  children: BNode[];
  leaf: boolean;
}

let nextId = 1;
const newNode = (leaf: boolean): BNode => ({ id: nextId++, keys: [], children: [], leaf });

let root = newNode(true);
let seq = 0;
let splitCount = 0;
let touchedNow: number[] = [];

const version = ref(0);
const touched = ref<number[]>([]);
const stats = ref({ keys: 0, height: 0, splits: 0, touchedCount: 0 });

function splitChild(parent: BNode, i: number) {
  const child = parent.children[i];
  const mid = child.keys[T - 1];
  const right = newNode(child.leaf);
  right.keys = child.keys.slice(T);
  if (!child.leaf) {
    right.children = child.children.slice(T);
    child.children = child.children.slice(0, T);
  }
  child.keys = child.keys.slice(0, T - 1);
  parent.keys.splice(i, 0, mid);
  parent.children.splice(i + 1, 0, right);
  splitCount++;
  touchedNow.push(right.id);
}

function insertNonFull(n: BNode, key: number) {
  touchedNow.push(n.id);
  let pos = n.keys.findIndex((k) => k >= key);
  if (pos === -1) pos = n.keys.length;
  if (n.keys[pos] === key) return;
  if (n.leaf) {
    n.keys.splice(pos, 0, key);
    return;
  }
  if (n.children[pos].keys.length === 2 * T - 1) {
    splitChild(n, pos);
    if (key > n.keys[pos]) pos++;
    else if (key === n.keys[pos]) return;
  }
  insertNonFull(n.children[pos], key);
}

function insert(key: number) {
  touchedNow = [];
  if (root.keys.length === 2 * T - 1) {
    const newRoot = newNode(false);
    newRoot.children = [root];
    root = newRoot;
    touchedNow.push(newRoot.id);
    splitChild(newRoot, 0);
  }
  insertNonFull(root, key);
}

function countKeys(n: BNode): number {
  return n.keys.length + n.children.reduce((a, c) => a + countKeys(c), 0);
}

function refresh() {
  let h = 0;
  for (let n = root; !n.leaf; n = n.children[0]) h++;
  stats.value = { keys: countKeys(root), height: h, splits: splitCount, touchedCount: touchedNow.length };
  touched.value = [...touchedNow];
  version.value++;
}

function addSequential(n: number) {
  for (let i = 0; i < n; i++) insert(++seq);
  refresh();
}

function addRandom(n: number) {
  for (let i = 0; i < n; i++) insert(1 + Math.floor(Math.random() * 999));
  refresh();
}

function reset() {
  root = newNode(true);
  seq = 0;
  splitCount = 0;
  touchedNow = [];
  refresh();
}

const full = computed(() => stats.value.keys >= MAX_KEYS);

const view = computed<ViewNode>(() => {
  version.value; // 依存を張って再描画をトリガする
  const snap = (n: BNode): ViewNode => ({
    id: n.id,
    keys: [...n.keys],
    children: n.children.map(snap),
  });
  return snap(root);
});
</script>

<template>
  <DemoShell title="B-Tree の挿入" :badge="`高さ ${stats.height}`" badge-tone="neutral">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" type="button" :disabled="full" @click="addSequential(1)">
        昇順で1件挿入
      </button>
      <button class="sd-btn sd-btn--primary" type="button" :disabled="full" @click="addRandom(1)">
        ランダムに1件挿入
      </button>
      <button class="sd-btn" type="button" :disabled="full" @click="addRandom(10)">ランダム×10</button>
      <span class="spacer"></span>
      <button class="sd-btn" type="button" @click="reset">リセット</button>
    </div>
    <p class="btd-stats">
      キー {{ stats.keys }} / 高さ {{ stats.height }} / 分割 {{ stats.splits }}回 /
      直前の挿入で触ったノード <strong>{{ stats.touchedCount }}</strong> 個(枠が光る)
      <span v-if="full">— 上限に達したのでリセットしてください</span>
    </p>
    <div class="btd-tree">
      <BTreeNodeView :node="view" :touched="touched" />
    </div>
  </DemoShell>
</template>

<style scoped>
.btd-stats {
  margin: 12px 0;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.btd-tree {
  overflow-x: auto;
  padding: 8px 0;
}
</style>
