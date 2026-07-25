<script setup lang="ts">
import { ref } from "vue";
import DemoShell from "./DemoShell.vue";

// リバースプロキシのラウンドロビン分散を可視化。
const BACKENDS = ["backend-1", "backend-2", "backend-3"];
const counts = ref<number[]>([0, 0, 0]);
const active = ref<number | null>(null);
const log = ref<{ n: number; to: string }[]>([]);
const reqNo = ref(0);
let next = 0;

function send() {
  const idx = next % BACKENDS.length;
  next++;
  reqNo.value++;
  active.value = idx;
  const c = [...counts.value];
  c[idx]++;
  counts.value = c;
  log.value = [{ n: reqNo.value, to: BACKENDS[idx] }, ...log.value].slice(0, 6);
}

function reset() {
  counts.value = [0, 0, 0];
  active.value = null;
  log.value = [];
  next = 0;
  reqNo.value = 0;
}
</script>

<template>
  <DemoShell title="リバースプロキシ + ラウンドロビン" :badge="`${reqNo} リクエスト`" badge-tone="neutral">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" type="button" @click="send">リクエストを送る</button>
      <span class="spacer"></span>
      <button class="sd-btn" type="button" @click="reset">リセット</button>
    </div>

    <div class="px-flow">
      <div class="px-node client">
        <span class="px-node-label">クライアント</span>
        <span class="px-node-sub">127.0.0.1</span>
      </div>
      <div class="px-arrow">
        <span class="px-arrow-line" :class="{ pulse: active !== null }"></span>
        <span class="px-arrow-tag">X-Forwarded-For: 127.0.0.1 を付与</span>
      </div>
      <div class="px-node proxy" :class="{ pulse: active !== null }">
        <span class="px-node-label">プロキシ</span>
        <span class="px-node-sub">L7 / round-robin</span>
      </div>
      <div class="px-fan">
        <div
          v-for="(b, i) in BACKENDS"
          :key="b"
          class="px-node backend"
          :class="{ hit: active === i }"
        >
          <span class="px-node-label">{{ b }}</span>
          <span class="px-node-count">{{ counts[i] }} 件</span>
        </div>
      </div>
    </div>

    <div v-if="log.length" class="px-log">
      <span v-for="e in log" :key="e.n" class="px-log-item">#{{ e.n }} → {{ e.to }}</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.px-flow {
  display: grid;
  grid-template-columns: auto auto auto 1.4fr;
  align-items: center;
  gap: 12px;
  margin-top: 16px;
}
@media (max-width: 720px) {
  .px-flow {
    grid-template-columns: 1fr;
  }
}
.px-node {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
  transition: border-color 0.2s, box-shadow 0.2s;
  min-width: 110px;
}
.px-node-label {
  font-size: 13px;
  font-weight: 600;
}
.px-node-sub,
.px-node-count {
  font-size: 11px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
.px-node.proxy.pulse {
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 0 0 1px var(--vp-c-brand-1);
}
.px-arrow {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
}
.px-arrow-line {
  width: 100%;
  min-width: 40px;
  height: 2px;
  background-color: var(--vp-c-divider);
}
.px-arrow-line.pulse {
  background-color: var(--vp-c-brand-1);
}
.px-arrow-tag {
  font-size: 10px;
  color: var(--vp-c-text-3);
  text-align: center;
}
.px-fan {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.px-node.backend {
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}
.px-node.backend.hit {
  border-color: var(--vp-c-green-1);
  box-shadow: 0 0 0 1px var(--vp-c-green-1);
}
.px-node.backend.hit .px-node-count {
  color: var(--vp-c-green-1);
  font-weight: 700;
}
.px-log {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 14px;
}
.px-log-item {
  padding: 2px 8px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  font-family: var(--vp-font-family-mono);
  font-size: 11px;
  color: var(--vp-c-text-2);
}
</style>
