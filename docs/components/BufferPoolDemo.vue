<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 db/bufferpool のヒット/ミス計上をブラウザで再現するシミュレータ。
const CAPACITY = 4;

const order = ref<number[]>([]); // 先頭 = 最近使った(LRU)
const hits = ref(0);
const misses = ref(0);
const message = ref("");
let nextSeq = 0;

function access(id: number) {
  const cur = order.value.filter((p) => p !== id);
  if (cur.length !== order.value.length) {
    hits.value++;
  } else {
    misses.value++;
    if (cur.length >= CAPACITY) cur.pop();
  }
  cur.unshift(id);
  order.value = cur;
}

function sameTen() {
  for (let i = 0; i < 10; i++) access(0);
  message.value = "page 0 を10回読んだ。最初の1回だけミスで、あとは全部キャッシュヒット";
}

function seqTen() {
  for (let i = 0; i < 10; i++) access(nextSeq++);
  message.value = `page ${nextSeq - 10}〜${nextSeq - 1} を順に読んだ。初見のページは必ずミス`;
}

function randTen() {
  for (let i = 0; i < 10; i++) access(Math.floor(Math.random() * 20));
  message.value = "0〜19 からランダムに10回読んだ。容量4に対して母集団20なので、ほぼミス";
}

function reset() {
  order.value = [];
  hits.value = 0;
  misses.value = 0;
  message.value = "";
  nextSeq = 0;
}

const total = computed(() => hits.value + misses.value);
const rate = computed(() => (total.value ? hits.value / total.value : 0));
</script>

<template>
  <DemoShell
    title="バッファプール(容量4ページ)"
    :badge="total ? `ヒット率 ${(rate * 100).toFixed(0)}%` : '未計測'"
    :badge-tone="total ? (rate >= 0.5 ? 'ok' : 'ng') : 'neutral'"
  >
    <div class="sd-controls">
      <span class="bp-caption">アクセスパターン:</span>
      <button class="sd-btn sd-btn--primary" type="button" @click="sameTen">同じページ ×10</button>
      <button class="sd-btn sd-btn--primary" type="button" @click="seqTen">昇順に10ページ</button>
      <button class="sd-btn sd-btn--primary" type="button" @click="randTen">ランダム ×10</button>
      <span class="spacer"></span>
      <button class="sd-btn" type="button" @click="reset">リセット</button>
    </div>

    <div class="bp-grid">
      <div class="bp-cache">
        <p class="bp-label">キャッシュ(最近使った順)</p>
        <div class="bp-slots">
          <span v-for="i in CAPACITY" :key="i" class="bp-slot" :class="{ empty: !order[i - 1] && order[i - 1] !== 0 }">
            {{ order[i - 1] !== undefined ? `page ${order[i - 1]}` : "空き" }}
          </span>
        </div>
      </div>
      <div class="bp-stats">
        <p class="bp-label">集計</p>
        <div class="bp-row">
          <span>ヒット(メモリで完結)</span><strong class="ok">{{ hits }}</strong>
        </div>
        <div class="bp-row">
          <span>ミス(ディスクまで読む)</span><strong class="ng">{{ misses }}</strong>
        </div>
        <div class="bp-bar">
          <span class="bp-fill" :style="{ width: `${rate * 100}%` }" />
        </div>
      </div>
    </div>

    <p v-if="message" class="sd-msg">{{ message }}</p>
  </DemoShell>
</template>

<style scoped>
.bp-caption {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.bp-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 14px;
}
@media (max-width: 560px) {
  .bp-grid {
    grid-template-columns: 1fr;
  }
}
.bp-cache,
.bp-stats {
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
  padding: 10px 12px;
}
.bp-label {
  margin: 0 0 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.bp-slots {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.bp-slot {
  min-width: 64px;
  padding: 8px 6px;
  border-radius: 6px;
  background-color: var(--vp-c-default-soft);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  text-align: center;
}
.bp-slot.empty {
  background-color: transparent;
  border: 1px dashed var(--vp-c-divider);
  color: var(--vp-c-text-3);
}
.bp-row {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  padding: 2px 0;
}
.bp-row strong {
  font-family: var(--vp-font-family-mono);
}
.bp-row .ok {
  color: var(--vp-c-green-1);
}
.bp-row .ng {
  color: var(--vp-c-danger-1);
}
.bp-bar {
  margin-top: 8px;
  height: 8px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.bp-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-green-1);
  transition: width 0.2s;
}
</style>
