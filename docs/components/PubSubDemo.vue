<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// トピック型 Pub/Sub のファンアウトを可視化(Go 実装 messaging/pubsub の考え方を移植)。
// 1回の発行が購読者全員に届き、各自が独立カーソルで読む。遅い購読者は他を止めない。

type Start = "beginning" | "now";
interface Sub {
  id: number;
  start: Start;
  cursor: number;
  received: string[];
}

const COLORS = ["var(--vp-c-brand-2)", "var(--vp-c-green-2)", "var(--vp-c-purple-2)", "var(--vp-c-yellow-2)"];

const state = reactive({
  log: [] as string[], // トピックのメッセージ本体
  subs: [] as Sub[],
  nextSub: 1,
  seq: 0,
  note: "発行すると購読者全員のバックログが増える(ファンアウト)。各自が自分のペースで受信する",
});

function addSub(start: Start) {
  const id = state.nextSub++;
  // FromNow は今ある分を飛ばす。FromBeginning は先頭(0)から。
  const cursor = start === "now" ? state.log.length : 0;
  state.subs.push({ id, start, cursor, received: [] });
  state.note = `購読者 ${id} を追加(${start === "now" ? "FromNow: 以降だけ" : "FromBeginning: 過去も再生"})`;
}

function publish() {
  state.seq += 1;
  state.log.push("e" + state.seq);
  state.note = `e${state.seq} を発行。購読者全員のバックログに1件積まれた(発行は1回、配りは全員)`;
}

function receive(id: number) {
  const s = state.subs.find((x) => x.id === id);
  if (!s) return;
  const batch = state.log.slice(s.cursor);
  if (batch.length === 0) {
    state.note = `購読者 ${id}: 未読なし`;
    return;
  }
  s.received.push(...batch);
  s.cursor = state.log.length;
  state.note = `購読者 ${id} が ${batch.length}件受信。他の購読者のカーソルには影響しない(独立)`;
}

function removeSub(id: number) {
  state.subs = state.subs.filter((x) => x.id !== id);
}

function reset() {
  state.log = [];
  state.subs = [
    { id: 1, start: "beginning", cursor: 0, received: [] },
    { id: 2, start: "beginning", cursor: 0, received: [] },
  ];
  state.nextSub = 3;
  state.seq = 0;
  state.note = "発行すると購読者全員のバックログが増える(ファンアウト)。各自が自分のペースで受信する";
}
reset();

function backlog(s: Sub) {
  return state.log.length - s.cursor;
}
function colorOf(s: Sub) {
  return COLORS[(s.id - 1) % COLORS.length];
}
const badge = computed(() => `購読者 ${state.subs.length} / ログ ${state.log.length}件`);
</script>

<template>
  <DemoShell title="Pub/Sub(トピックのファンアウト)" :badge="badge" badge-tone="neutral">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" @click="publish">発行</button>
      <button class="sd-btn" @click="addSub('beginning')">購読者+ (FromBeginning)</button>
      <button class="sd-btn" @click="addSub('now')">購読者+ (FromNow)</button>
      <span class="spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="ps-topic">
      <div class="ps-label">トピック "news" のログ(追記され、消えない)</div>
      <div class="ps-log">
        <span v-for="(m, i) in state.log" :key="i" class="ps-cell">{{ m }}</span>
        <span v-if="state.log.length === 0" class="ps-empty">(空)</span>
      </div>
    </div>

    <div class="ps-subs">
      <div v-for="s in state.subs" :key="s.id" class="ps-sub" :style="{ borderLeftColor: colorOf(s) }">
        <div class="ps-sub-head">
          <span class="ps-sub-id">購読者 {{ s.id }}</span>
          <span class="ps-start">{{ s.start === "now" ? "FromNow" : "FromBeginning" }}</span>
          <span class="ps-backlog" :class="{ hot: backlog(s) > 0 }">未読 {{ backlog(s) }}</span>
        </div>
        <div class="ps-received">
          <span v-for="(m, i) in s.received" :key="i" class="ps-rcell">{{ m }}</span>
          <span v-if="s.received.length === 0" class="ps-empty">(受信なし)</span>
        </div>
        <div class="ps-actions">
          <button class="sd-btn sd-btn--sm" @click="receive(s.id)">受信</button>
          <button class="sd-btn sd-btn--sm" @click="removeSub(s.id)">解除</button>
        </div>
      </div>
    </div>

    <p class="sd-msg">{{ state.note }}</p>
    <div class="ps-legend">
      <span>発行1回 → 購読者全員のバックログ +1(ファンアウト)。キューは1件を1人が奪う</span>
      <span>各購読者は独立カーソル。遅い購読者がいても他は止まらない</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.ps-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin-bottom: 6px;
}
.ps-topic {
  margin-top: 14px;
}
.ps-log,
.ps-received {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  align-items: center;
}
.ps-cell {
  min-width: 28px;
  padding: 4px 8px;
  text-align: center;
  border: 1px solid var(--vp-c-brand-2);
  color: var(--vp-c-brand-1);
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  background-color: var(--vp-c-bg);
}
.ps-subs {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 10px;
  margin-top: 14px;
}
.ps-sub {
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  padding: 10px;
  background-color: var(--vp-c-bg);
}
.ps-sub-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ps-sub-id {
  font-size: 13px;
  font-weight: 600;
}
.ps-start {
  font-size: 10px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
.ps-backlog {
  margin-left: auto;
  font-size: 10px;
  font-weight: 700;
  padding: 1px 7px;
  border-radius: 8px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.ps-backlog.hot {
  background-color: var(--vp-c-yellow-soft);
  color: var(--vp-c-yellow-1);
}
.ps-received {
  margin-top: 8px;
  min-height: 20px;
}
.ps-rcell {
  min-width: 24px;
  padding: 2px 6px;
  text-align: center;
  border: 1px solid var(--vp-c-green-2);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
  font-size: 10px;
  font-family: var(--vp-font-family-mono);
}
.ps-actions {
  display: flex;
  gap: 6px;
  margin-top: 8px;
}
.sd-btn--sm {
  padding: 2px 10px;
  font-size: 11px;
}
.ps-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.ps-legend {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 12px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
