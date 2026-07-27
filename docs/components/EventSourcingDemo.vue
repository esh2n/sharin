<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// messaging/eventsourcing(Go)を移植。イベントを追記し、畳み込んで残高を導く。
// 過去の version へ遡り、残高超過の出金が拒否される様子も見る。

type Ev = { type: "in" | "out"; amount: number; version: number };

// 初期のイベント列(決定的)。
const events = ref<Ev[]>([
  { type: "in", amount: 1000, version: 1 },
  { type: "in", amount: 500, version: 2 },
  { type: "out", amount: 300, version: 3 },
]);
const viewVersion = ref(3); // 表示する時点(0=最初, 末尾=現在)
const message = ref("");

// version までを畳んだ残高。
function stateAt(version: number): number {
  let bal = 0;
  for (const e of events.value) {
    if (e.version > version) break;
    bal += e.type === "in" ? e.amount : -e.amount;
  }
  return bal;
}
const currentBalance = computed(() => stateAt(events.value.length));
const viewBalance = computed(() => stateAt(viewVersion.value));
const isCurrent = computed(() => viewVersion.value === events.value.length);

function deposit(amount: number) {
  const v = events.value.length + 1;
  events.value = [...events.value, { type: "in", amount, version: v }];
  viewVersion.value = v;
  message.value = `+${amount} を追記(イベント v${v})`;
}
function withdraw(amount: number) {
  // コマンド検証: 現在残高を超える出金は拒否し、イベントを残さない。
  if (amount > currentBalance.value) {
    message.value = `出金 ${amount} は拒否。残高 ${currentBalance.value} を超える(イベントは残らない)`;
    return;
  }
  const v = events.value.length + 1;
  events.value = [...events.value, { type: "out", amount, version: v }];
  viewVersion.value = v;
  message.value = `−${amount} を追記(イベント v${v})`;
}
function reset() {
  events.value = [
    { type: "in", amount: 1000, version: 1 },
    { type: "in", amount: 500, version: 2 },
    { type: "out", amount: 300, version: 3 },
  ];
  viewVersion.value = 3;
  message.value = "";
}

const badge = computed(() => `残高 ${currentBalance.value}`);
</script>

<template>
  <DemoShell title="イベントソーシング(口座)" badge-tone="neutral" :badge="badge">
    <div class="es-actions">
      <button class="sd-btn" @click="deposit(500)">入金 +500</button>
      <button class="sd-btn" @click="deposit(1000)">入金 +1000</button>
      <button class="sd-btn" @click="withdraw(300)">出金 −300</button>
      <button class="sd-btn" @click="withdraw(5000)">出金 −5000(残高超過)</button>
      <span class="es-spacer" />
      <button class="sd-btn" @click="reset">最初に戻す</button>
    </div>

    <p v-if="message" class="es-msg mono">{{ message }}</p>

    <div class="es-cols">
      <!-- イベントログ -->
      <div class="es-log">
        <div class="es-log-head">イベントログ(追記専用)</div>
        <div
          v-for="e in events"
          :key="e.version"
          class="es-ev"
          :class="{ dim: e.version > viewVersion, in: e.type === 'in', out: e.type === 'out' }"
        >
          <span class="es-ev-v mono">v{{ e.version }}</span>
          <span class="es-ev-t mono">{{ e.type === "in" ? "Deposited" : "Withdrawn" }}</span>
          <span class="es-ev-a mono">{{ e.type === "in" ? "+" : "−" }}{{ e.amount }}</span>
        </div>
      </div>

      <!-- 導出された状態 -->
      <div class="es-state">
        <div class="es-log-head">畳み込んで導く状態</div>
        <div class="es-balance">
          <span class="es-balance-label">残高</span>
          <span class="es-balance-val mono">{{ viewBalance }}</span>
        </div>
        <div class="es-travel">
          <span class="es-travel-label mono">表示時点: v{{ viewVersion }}{{ isCurrent ? "(現在)" : "(過去)" }}</span>
          <input type="range" min="0" :max="events.length" v-model.number="viewVersion" class="es-range" />
        </div>
        <p class="es-hint">スライダーで過去の時点へ遡れる(タイムトラベル)。状態は保存せずイベントから導く</p>
      </div>
    </div>

    <p class="es-legend">
      保存しているのは残高でなくイベント列(追記専用)。現在残高はイベントを頭から畳み込んで導く。
      履歴が丸ごと残るので、任意の過去の時点に遡れる。出金コマンドは現在残高で検証され、超過なら
      拒否してイベントを残さない(意図は事実にしない)。長い履歴はスナップショットで畳み込みを省ける。
    </p>
  </DemoShell>
</template>

<style scoped>
.es-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.es-spacer {
  flex: 1;
}
.es-msg {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
  padding: 6px 10px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.es-cols {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-top: 14px;
}
.es-log,
.es-state {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px;
  background-color: var(--vp-c-bg-soft);
}
.es-log-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.es-ev {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  margin-bottom: 3px;
}
.es-ev.dim {
  opacity: 0.3;
}
.es-ev.in {
  border-left-color: var(--vp-c-green-1);
}
.es-ev.out {
  border-left-color: var(--vp-c-warning-1);
}
.es-ev-v {
  font-size: 11px;
  color: var(--vp-c-text-3);
  width: 28px;
}
.es-ev-t {
  font-size: 11px;
  color: var(--vp-c-text-2);
  flex: 1;
}
.es-ev-a {
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.es-balance {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 10px 0;
}
.es-balance-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.es-balance-val {
  font-size: 28px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
}
.es-travel {
  margin-top: 8px;
}
.es-travel-label {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.es-range {
  width: 100%;
  margin-top: 6px;
}
.es-hint {
  margin: 8px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
  line-height: 1.6;
}
.es-legend {
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
