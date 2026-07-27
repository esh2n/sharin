<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// messaging/cqrs(Go)を移植。書き込み側のイベントログと、そこから導く
// 3 つの読みモデル、そして CatchUp までの遅れ(結果整合)を見せる。

type Kind = "placed" | "paid" | "cancelled";
type Ev = { kind: Kind; orderID: string; customer: string; amount: number; version: number };

// 初期のイベント列と、少し遅れた読みカーソル(結果整合を最初から見せる)。
function seed(): Ev[] {
  return [
    { kind: "placed", orderID: "o1", customer: "alice", amount: 1000, version: 1 },
    { kind: "paid", orderID: "o1", customer: "alice", amount: 1000, version: 2 },
    { kind: "placed", orderID: "o2", customer: "bob", amount: 500, version: 3 },
  ];
}
const events = ref<Ev[]>(seed());
const cursor = ref(2); // 読み取り側が処理済みの version(初期は 1 遅れ)
const message = ref("");
const nextId = ref(3);
const custs = ["alice", "bob", "carol"];
const amts = [500, 1000, 300];

function statusOf(id: string): Kind | "" {
  let st: Kind | "" = "";
  for (const e of events.value) if (e.orderID === id) st = e.kind;
  return st;
}
function placedOrders(): string[] {
  const ids: string[] = [];
  for (const e of events.value) if (e.kind === "placed") ids.push(e.orderID);
  return ids.filter((id) => statusOf(id) === "placed");
}

function place() {
  const id = "o" + (nextId.value + 1);
  const c = custs[nextId.value % 3];
  const a = amts[nextId.value % 3];
  nextId.value++;
  events.value = [...events.value, { kind: "placed", orderID: id, customer: c, amount: a, version: events.value.length + 1 }];
  message.value = `注文 ${id}(${c}, ${a})を追加`;
}
function pay() {
  const p = placedOrders();
  if (!p.length) {
    message.value = "支払える注文がない(placed のものだけ支払える)";
    return;
  }
  const id = p[p.length - 1];
  const o = events.value.find((e) => e.orderID === id && e.kind === "placed")!;
  events.value = [...events.value, { kind: "paid", orderID: id, customer: o.customer, amount: o.amount, version: events.value.length + 1 }];
  message.value = `注文 ${id} を支払い(書き込み側に反映、読みモデルはまだ遅れる)`;
}
function catchUp() {
  cursor.value = events.value.length;
  message.value = "読み取り側が CatchUp。全ビューが最新に追いついた";
}
function reset() {
  events.value = seed();
  cursor.value = 2;
  nextId.value = 3;
  message.value = "";
}

// 読みモデルは cursor までのイベントだけを畳む。
const view = computed(() => {
  const status: Record<string, string> = {};
  const spend: Record<string, number> = {};
  const detail: Record<string, { c: string; a: number }> = {};
  let revenue = 0;
  for (const e of events.value) {
    if (e.version > cursor.value) break;
    if (e.kind === "placed") {
      status[e.orderID] = "placed";
      detail[e.orderID] = { c: e.customer, a: e.amount };
    } else if (e.kind === "paid") {
      status[e.orderID] = "paid";
      const d = detail[e.orderID];
      if (d) { spend[d.c] = (spend[d.c] ?? 0) + d.a; revenue += d.a; }
    } else {
      status[e.orderID] = "cancelled";
    }
  }
  return { status, spend, revenue };
});
const lag = computed(() => events.value.length - cursor.value);

const badge = computed(() => (lag.value > 0 ? `読みが ${lag.value} 遅れ` : "整合済み"));
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (lag.value > 0 ? "ng" : "ok"));
</script>

<template>
  <DemoShell title="CQRS(注文)" :badge="badge" :badge-tone="badgeTone">
    <div class="cq-actions">
      <button class="sd-btn" @click="place">注文する</button>
      <button class="sd-btn" @click="pay">最新を支払う</button>
      <button class="sd-btn sd-btn--primary" @click="catchUp">読み取り側 CatchUp</button>
      <span class="cq-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <p v-if="message" class="cq-msg mono">{{ message }}</p>

    <div class="cq-grid">
      <!-- 書き込み側 -->
      <div class="cq-col write">
        <div class="cq-head">書き込み側(イベントログ)</div>
        <div
          v-for="e in events"
          :key="e.version"
          class="cq-ev"
          :class="{ pending: e.version > cursor }"
        >
          <span class="cq-ev-v mono">v{{ e.version }}</span>
          <span class="cq-ev-b mono">{{ e.kind }} {{ e.orderID }}</span>
          <span v-if="e.version > cursor" class="cq-ev-tag mono">未処理</span>
        </div>
      </div>

      <!-- 読み取り側(3 射影) -->
      <div class="cq-col read">
        <div class="cq-head">読み取り側(3 つの読みモデル)</div>
        <div class="cq-proj">
          <div class="cq-proj-t mono">① 注文状況</div>
          <div class="cq-proj-b mono">
            <span v-for="(st, id) in view.status" :key="id" class="cq-chip" :class="st">{{ id }}:{{ st }}</span>
          </div>
        </div>
        <div class="cq-proj">
          <div class="cq-proj-t mono">② 顧客ごとの購入額</div>
          <div class="cq-proj-b mono">
            <span v-for="(amt, c) in view.spend" :key="c" class="cq-chip">{{ c }}:{{ amt }}</span>
            <span v-if="Object.keys(view.spend).length === 0" class="cq-empty">(まだなし)</span>
          </div>
        </div>
        <div class="cq-proj">
          <div class="cq-proj-t mono">③ 総売上</div>
          <div class="cq-proj-b mono revenue">{{ view.revenue }}</div>
        </div>
      </div>
    </div>

    <p class="cq-legend">
      書き込み側はコマンドを検証してイベントを流す。読み取り側は同じイベント列を畳んで、注文状況・顧客ごとの
      購入額・総売上という 3 つの読みモデルを同時に作る。1 つのイベントから用途別のビューが何個でも組める。
      CatchUp する前は読みモデルが書き込みに遅れる(未処理・結果整合)。CatchUp で追いつく。読みと書きを
      分けることで、それぞれを独立に最適化できる代わりに、この遅れを受け入れる。
    </p>
  </DemoShell>
</template>

<style scoped>
.cq-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.cq-spacer {
  flex: 1;
}
.cq-msg {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
  padding: 6px 10px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.cq-grid {
  display: grid;
  grid-template-columns: 1fr 1.2fr;
  gap: 14px;
  margin-top: 14px;
}
.cq-col {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px;
  background-color: var(--vp-c-bg-soft);
}
.cq-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.cq-ev {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  border-left: 3px solid var(--vp-c-green-1);
  border-radius: 0;
  margin-bottom: 3px;
}
.cq-ev.pending {
  border-left-color: var(--vp-c-warning-1);
  opacity: 0.75;
}
.cq-ev-v {
  font-size: 11px;
  color: var(--vp-c-text-3);
  width: 26px;
}
.cq-ev-b {
  font-size: 11px;
  color: var(--vp-c-text-1);
  flex: 1;
}
.cq-ev-tag {
  font-size: 10px;
  color: var(--vp-c-warning-1);
  font-weight: 700;
}
.cq-proj {
  margin-bottom: 10px;
}
.cq-proj-t {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.cq-proj-b {
  display: flex;
  gap: 5px;
  flex-wrap: wrap;
}
.cq-chip {
  padding: 2px 7px;
  font-size: 11px;
  border-radius: 0;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.cq-chip.paid {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.cq-chip.cancelled {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.cq-empty {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cq-proj-b.revenue {
  font-size: 22px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
}
.cq-legend {
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
