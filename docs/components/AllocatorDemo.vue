<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/allocator(Go)を移植。ヒープをブロック列で可視化し、
// 確保・解放・断片化・併合(coalesce)を見せる。

interface Block {
  offset: number;
  size: number;
  free: boolean;
}
const SIZE = 100;

function seed(): Block[] {
  // 断片化した初期状態: 20 を 5 つ確保し、A・C を解放した形。
  return [
    { offset: 0, size: 20, free: true }, // A(解放済み)
    { offset: 20, size: 20, free: false }, // B
    { offset: 40, size: 20, free: true }, // C(解放済み)
    { offset: 60, size: 20, free: false }, // D
    { offset: 80, size: 20, free: true }, // 末尾の空き
  ];
}
const blocks = ref<Block[]>(seed());
const message = ref("初期状態は断片化している(空き60だが連続20まで)");

function coalesce() {
  const merged: Block[] = [];
  for (const b of blocks.value) {
    const last = merged[merged.length - 1];
    if (last && last.free && b.free) last.size += b.size;
    else merged.push({ ...b });
  }
  blocks.value = merged;
}
function alloc(n: number) {
  for (let i = 0; i < blocks.value.length; i++) {
    const b = blocks.value[i];
    if (!b.free || b.size < n) continue;
    if (b.size > n) {
      const rem: Block = { offset: b.offset + n, size: b.size - n, free: true };
      blocks.value = [
        ...blocks.value.slice(0, i),
        { offset: b.offset, size: n, free: false },
        rem,
        ...blocks.value.slice(i + 1),
      ];
    } else {
      blocks.value = blocks.value.map((x, j) => (j === i ? { ...x, free: false } : x));
    }
    message.value = `${n} を確保(offset ${b.offset})。first-fit で最初の足る空きに配置`;
    return;
  }
  message.value = `${n} の確保に失敗。空きは総量 ${freeBytes.value} あるが連続は ${largest.value} まで(外部断片化)`;
}
function freeBlock(i: number) {
  const b = blocks.value[i];
  if (b.free) return;
  blocks.value = blocks.value.map((x, j) => (j === i ? { ...x, free: true } : x));
  coalesce();
  message.value = `offset ${b.offset} を解放し、隣り合う空きを併合(coalesce)`;
}
function reset() {
  blocks.value = seed();
  message.value = "初期状態は断片化している(空き60だが連続20まで)";
}

const freeBytes = computed(() => blocks.value.filter((b) => b.free).reduce((a, b) => a + b.size, 0));
const largest = computed(() => blocks.value.filter((b) => b.free).reduce((m, b) => Math.max(m, b.size), 0));
const fragmentation = computed(() => (freeBytes.value === 0 ? 0 : 1 - largest.value / freeBytes.value));

const badge = computed(() => `連続最大 ${largest.value} / 空き ${freeBytes.value}`);
</script>

<template>
  <DemoShell title="メモリアロケータ(free list)" badge-tone="neutral" :badge="badge">
    <div class="al-actions">
      <button class="sd-btn" @click="alloc(20)">確保 20</button>
      <button class="sd-btn" @click="alloc(30)">確保 30</button>
      <button class="sd-btn" @click="alloc(60)">確保 60</button>
      <span class="al-hint">使用中ブロックをクリックで解放</span>
      <span class="al-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="al-heap">
      <div
        v-for="(b, i) in blocks"
        :key="b.offset"
        class="al-block"
        :class="{ used: !b.free, free: b.free }"
        :style="{ width: (b.size / SIZE) * 100 + '%' }"
        @click="freeBlock(i)"
      >
        <span class="al-block-label mono">{{ b.free ? "空き" : "使用" }} {{ b.size }}</span>
      </div>
    </div>
    <div class="al-ruler mono"><span>0</span><span>{{ SIZE }}</span></div>

    <div class="al-metrics">
      <div class="al-metric">
        <span class="al-m-k">空き総量</span><span class="al-m-v mono">{{ freeBytes }}</span>
      </div>
      <div class="al-metric">
        <span class="al-m-k">連続最大</span><span class="al-m-v mono">{{ largest }}</span>
      </div>
      <div class="al-metric" :class="{ hot: fragmentation > 0.3 }">
        <span class="al-m-k">断片化</span><span class="al-m-v mono">{{ (fragmentation * 100).toFixed(0) }}%</span>
      </div>
    </div>

    <p class="al-msg mono">{{ message }}</p>

    <p class="al-legend">
      ヒープを空き/使用中のブロック列で管理する。確保は足る最初の空きを first-fit で貸し、大きすぎれば分割する。
      確保と解放を繰り返すと、空きの総量は足りても連続領域が細切れになり、大きな確保が失敗する(外部断片化)。
      使用中ブロックを解放すると隣り合う空きが併合され、大きな空きが復活する。連続最大と空き総量が別物なのが肝だ。
    </p>
  </DemoShell>
</template>

<style scoped>
.al-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.al-hint {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.al-spacer {
  flex: 1;
}
.al-heap {
  display: flex;
  margin-top: 16px;
  height: 44px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.al-block {
  display: flex;
  align-items: center;
  justify-content: center;
  border-right: 1px solid var(--vp-c-bg);
  cursor: pointer;
  transition: background-color 0.15s;
  min-width: 0;
}
.al-block:last-child {
  border-right: none;
}
.al-block.used {
  background-color: var(--vp-c-brand-1);
}
.al-block.free {
  background-color: var(--vp-c-default-soft);
  cursor: default;
}
.al-block-label {
  font-size: 10.5px;
  white-space: nowrap;
  overflow: hidden;
}
.al-block.used .al-block-label {
  color: #fff;
}
.al-block.free .al-block-label {
  color: var(--vp-c-text-3);
}
.al-ruler {
  display: flex;
  justify-content: space-between;
  margin-top: 3px;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.al-metrics {
  display: flex;
  gap: 10px;
  margin-top: 12px;
}
.al-metric {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.al-metric.hot {
  border-color: var(--vp-c-danger-1);
}
.al-m-k {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.al-m-v {
  font-size: 18px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.al-metric.hot .al-m-v {
  color: var(--vp-c-danger-1);
}
.al-msg {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
  padding: 6px 10px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.al-legend {
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
