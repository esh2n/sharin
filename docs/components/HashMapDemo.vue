<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 data-structures/hashmap の JS ミラー。バケット配列とチェイン、リサイズを可視化。
const MAX_LOAD = 0.75;

// splitmix 風の整数ハッシュ(Go 版 HashInt と同じ考え方)。
function hashInt(n: number): number {
  // 32bit で近似(BigInt を避けて見やすさ優先)。
  let x = n | 0;
  x = Math.imul(x ^ (x >>> 16), 0x45d9f3b);
  x = Math.imul(x ^ (x >>> 16), 0x45d9f3b);
  x = x ^ (x >>> 16);
  return x >>> 0;
}

const slots = ref<number[][]>(Array.from({ length: 8 }, () => []));
const count = ref(0);
const lastResize = ref(false);
const message = ref("");
let seq = 0;

const loadFactor = computed(() => count.value / slots.value.length);

function put(key: number) {
  const idx = hashInt(key) % slots.value.length;
  const next = slots.value.map((b) => [...b]);
  if (!next[idx].includes(key)) {
    next[idx].push(key);
    count.value++;
  }
  slots.value = next;
  lastResize.value = false;
  message.value = `キー ${key} → hash % ${slots.value.length} = バケット ${idx} に追加`;

  if (count.value / slots.value.length > MAX_LOAD) {
    resize();
  }
}

function resize() {
  const oldLen = slots.value.length;
  const next: number[][] = Array.from({ length: oldLen * 2 }, () => []);
  for (const bucket of slots.value) {
    for (const key of bucket) {
      next[hashInt(key) % next.length].push(key);
    }
  }
  slots.value = next;
  lastResize.value = true;
  message.value = `負荷率が ${MAX_LOAD} を超えたのでバケットを ${oldLen} → ${oldLen * 2} に倍増、全要素を配り直した(rehash)`;
}

function addOne() {
  put(Math.floor(Math.random() * 1000));
}
function addFive() {
  for (let i = 0; i < 5; i++) put(seq++ * 7 + Math.floor(Math.random() * 5));
}
function reset() {
  slots.value = Array.from({ length: 8 }, () => []);
  count.value = 0;
  seq = 0;
  lastResize.value = false;
  message.value = "";
}

const maxChain = computed(() => Math.max(0, ...slots.value.map((b) => b.length)));
</script>

<template>
  <DemoShell
    title="ハッシュマップのバケット"
    :badge="`負荷率 ${loadFactor.toFixed(2)}`"
    :badge-tone="loadFactor > MAX_LOAD ? 'ng' : 'neutral'"
  >
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" type="button" @click="addOne">1件追加</button>
      <button class="sd-btn sd-btn--primary" type="button" @click="addFive">5件追加</button>
      <span class="spacer"></span>
      <span class="hm-stat">{{ count }}件 / {{ slots.length }}バケット / 最長チェイン {{ maxChain }}</span>
      <button class="sd-btn" type="button" @click="reset">リセット</button>
    </div>

    <p v-if="message" class="sd-msg">{{ message }}</p>

    <div class="hm-grid" :class="{ resized: lastResize }">
      <div v-for="(bucket, i) in slots" :key="i" class="hm-bucket">
        <span class="hm-idx">{{ i }}</span>
        <div class="hm-chain">
          <span v-for="k in bucket" :key="k" class="hm-item">{{ k }}</span>
          <span v-if="!bucket.length" class="hm-empty">·</span>
        </div>
      </div>
    </div>
  </DemoShell>
</template>

<style scoped>
.hm-stat {
  font-size: 12px;
  color: var(--vp-c-text-2);
  font-variant-numeric: tabular-nums;
}
.hm-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(80px, 1fr));
  gap: 6px;
  margin-top: 14px;
  transition: outline-color 0.4s;
  outline: 2px solid transparent;
  outline-offset: 4px;
  border-radius: 6px;
}
.hm-grid.resized {
  outline-color: var(--vp-c-brand-1);
}
.hm-bucket {
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  padding: 4px;
  min-height: 52px;
}
.hm-idx {
  font-size: 10px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
.hm-chain {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  margin-top: 2px;
}
.hm-item {
  padding: 1px 5px;
  border-radius: 4px;
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-family: var(--vp-font-family-mono);
  font-size: 11px;
}
.hm-empty {
  color: var(--vp-c-text-3);
}
</style>
