<script setup lang="ts">
import { computed, ref } from "vue";
import BTreeNodeView, { type ViewNode } from "./BTreeNodeView.vue";
import DemoShell from "./DemoShell.vue";

// Go 版 db/btreestore の JS ミラー。t=2 の小さな木で「検索のページ読み回数」を見せる。
const T = 2;

interface BNode {
  id: number;
  keys: number[];
  children: BNode[];
  leaf: boolean;
}

let nextId = 1;
const newNode = (leaf: boolean): BNode => ({ id: nextId++, keys: [], children: [], leaf });

let root = newNode(true);
const version = ref(0);
const path = ref<number[]>([]);
const searchKey = ref(10);
const result = ref<{ key: number; found: boolean; reads: number } | null>(null);

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
}

function insertNonFull(n: BNode, key: number) {
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
  if (root.keys.length === 2 * T - 1) {
    const newRoot = newNode(false);
    newRoot.children = [root];
    root = newRoot;
    splitChild(newRoot, 0);
  }
  insertNonFull(root, key);
}

function build() {
  nextId = 1;
  root = newNode(true);
  for (let i = 1; i <= 20; i++) insert(i);
  path.value = [];
  result.value = null;
  version.value++;
}

function get() {
  const key = searchKey.value;
  const p: number[] = [];
  let n = root;
  let found = false;
  for (;;) {
    p.push(n.id);
    let i = n.keys.findIndex((k) => k >= key);
    if (i === -1) i = n.keys.length;
    if (n.keys[i] === key) {
      found = true;
      break;
    }
    if (n.leaf) break;
    n = n.children[i];
  }
  path.value = p;
  result.value = { key, found, reads: p.length };
  version.value++;
}

build();

const view = computed<ViewNode | null>(() => {
  version.value;
  const snap = (n: BNode): ViewNode => ({ id: n.id, keys: [...n.keys], children: n.children.map(snap) });
  return root.keys.length || root.children.length ? snap(root) : null;
});

const badge = computed(() => (result.value ? `ページ読み ${result.value.reads}回` : undefined));
</script>

<template>
  <DemoShell title="B-Tree の検索とページ読み" :badge="badge" badge-tone="neutral">
    <div class="sd-controls">
      <label class="bp-key">
        検索キー: <strong>{{ searchKey }}</strong>
        <input v-model.number="searchKey" type="range" min="1" max="20" step="1" />
      </label>
      <button class="sd-btn sd-btn--primary" type="button" @click="get">検索する</button>
      <span class="spacer"></span>
      <button class="sd-btn" type="button" @click="build">木を作り直す</button>
    </div>

    <p v-if="result" class="sd-msg">
      キー {{ result.key }} は{{ result.found ? "見つかった" : "無かった" }}。
      根から葉まで <strong>{{ result.reads }}</strong> ページ読んだ(光った経路)。
      同じ検索を繰り返せば、これらのページはキャッシュに残るので2回目からディスクに触らない。
    </p>

    <div class="bp-tree">
      <BTreeNodeView v-if="view" :node="view" :touched="path" />
    </div>
  </DemoShell>
</template>

<style scoped>
.bp-key {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.bp-key input {
  accent-color: var(--vp-c-brand-1);
}
.bp-tree {
  overflow-x: auto;
  padding: 12px 0 4px;
}
</style>
