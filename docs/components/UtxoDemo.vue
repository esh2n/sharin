<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// chain/utxo(Go)の「送金 = 消費と生成」「残高 = 集計」「二重支払いは一意消費で防ぐ」を
// 決定的シナリオで。coinbase → 送金 → お釣り → 二重支払いの試み、と進む。

interface Utxo {
  id: string; // (tx,index) の短縮表示
  owner: "Alice" | "Bob";
  amount: number;
}
interface Frame {
  utxos: Utxo[];
  spent: string[]; // この手番で消費された UTXO id
  born: string[]; // この手番で生まれた UTXO id
  actor: string;
  action: string;
  detail: string; // input/output の内訳
  fee: number | null;
  accepted: boolean | null; // null = 送金以外 / true = 受理 / false = 拒否
  note: string;
}

function build(): Frame[] {
  const frames: Frame[] = [];
  let utxos: Utxo[] = [];
  let seq = 0;

  const snap = (f: Omit<Frame, "utxos">) => {
    frames.push({ utxos: utxos.map((u) => ({ ...u })), ...f });
  };

  // 0. 初期状態
  snap({
    spent: [],
    born: [],
    actor: "",
    action: "初期状態",
    detail: "UTXO セットは空。残高という数字はどこにもない",
    fee: null,
    accepted: null,
    note: "残高は保存されない。あるのは「未使用出力」の集合だけ。今は空っぽ",
  });

  // 1. coinbase → Alice 50
  const cb = "cb#0";
  utxos = [{ id: cb, owner: "Alice", amount: 50 }];
  snap({
    spent: [],
    born: [cb],
    actor: "coinbase",
    action: "無から Alice に 50",
    detail: "input: なし / output: [50 → Alice]",
    fee: null,
    accepted: null,
    note: "coinbase は input を持たず、無から出力を生む(マイニング報酬の入口)。Alice あての UTXO が 1 つできた。残高 = それを数えて 50",
  });

  // 2. Alice → Bob 30(fee 0, お釣り 20)
  const t1a = "t1#0";
  const t1b = "t1#1";
  utxos = [
    { id: t1a, owner: "Bob", amount: 30 },
    { id: t1b, owner: "Alice", amount: 20 },
  ];
  snap({
    spent: [cb],
    born: [t1a, t1b],
    actor: "Alice",
    action: "Bob へ 30 を送金",
    detail: "input: cb#0(50, Alice 署名) / output: [30 → Bob][20 → Alice お釣り]",
    fee: 0,
    accepted: true,
    note: "50 を丸ごと消費し、Bob への 30 と自分へのお釣り 20 を作る。元の cb#0 は集合から消えた——もう二度と使えない。これが二重支払い防止の核心",
  });

  // 3. Bob → Alice 10(fee 2, お釣り 18)
  const t2a = "t2#0";
  const t2b = "t2#1";
  utxos = [
    { id: t1b, owner: "Alice", amount: 20 },
    { id: t2a, owner: "Alice", amount: 10 },
    { id: t2b, owner: "Bob", amount: 18 },
  ];
  snap({
    spent: [t1a],
    born: [t2a, t2b],
    actor: "Bob",
    action: "Alice へ 10 を送金(手数料 2)",
    detail: "input: t1#0(30, Bob 署名) / output: [10 → Alice][18 → Bob お釣り]",
    fee: 2,
    accepted: true,
    note: "Bob の 30 を消費。10 を Alice へ、18 をお釣りに戻す。差の 2 が手数料としてマイナーへ。入力 30 = 出力 28 + 手数料 2",
  });

  // 4. 二重支払いの試み: Alice が既に消費済みの cb#0 をもう一度使おうとする
  seq++;
  snap({
    spent: [],
    born: [],
    actor: "Alice",
    action: "cb#0 をもう一度使おうとする",
    detail: "input: cb#0 → 参照先が UTXO セットに存在しない",
    fee: null,
    accepted: false,
    note: "手番 2 で cb#0 は消費され、集合から消えている。同じ出力を指す取引は「存在しない input」として弾かれる——履歴を走査しなくても、集合に無いだけで二重支払いは防がれる",
  });

  return frames;
}

const frames = build();
const at = ref(0);
const cur = computed(() => frames[at.value]);

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.length - 1);
function first() {
  at.value = 0;
}
function prev() {
  if (canPrev.value) at.value--;
}
function next() {
  if (canNext.value) at.value++;
}
function last() {
  at.value = frames.length - 1;
}

const balA = computed(() =>
  cur.value.utxos.filter((u) => u.owner === "Alice").reduce((s, u) => s + u.amount, 0),
);
const balB = computed(() =>
  cur.value.utxos.filter((u) => u.owner === "Bob").reduce((s, u) => s + u.amount, 0),
);

const done = computed(() => at.value === frames.length - 1);
const badge = computed(() => {
  if (!done.value) return `step ${at.value + 1}`;
  return "二重支払いは弾かれた";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (done.value ? "ok" : "neutral"));

const acc = computed<"ok" | "rejected" | null>(() => {
  if (cur.value.accepted === null) return null;
  return cur.value.accepted ? "ok" : "rejected";
});
</script>

<template>
  <DemoShell title="utxo(送金 = 消費と生成)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="ux-hint">coinbase → 送金 → お釣り → 二重支払いの試み</span>
      <span class="spacer" />
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
    </div>

    <div class="ux-grid">
      <div class="ux-panel">
        <div class="ux-panel-head">UTXO セット（未使用出力）</div>
        <div class="ux-body">
          <div v-if="cur.utxos.length === 0" class="ux-empty">(空)</div>
          <div
            v-for="u in cur.utxos"
            :key="u.id"
            class="ux-utxo"
            :class="[u.owner === 'Alice' ? 'a' : 'b', { born: cur.born.includes(u.id) }]"
          >
            <span class="ux-utxo-id">{{ u.id }}</span>
            <span class="ux-utxo-owner">{{ u.owner }}</span>
            <span class="ux-utxo-amt">{{ u.amount }}</span>
          </div>
          <div v-for="id in cur.spent" :key="id" class="ux-utxo spent">
            <span class="ux-utxo-id">{{ id }}</span>
            <span class="ux-utxo-owner">消費</span>
            <span class="ux-utxo-amt">×</span>
          </div>
        </div>
      </div>

      <div class="ux-panel">
        <div class="ux-panel-head">残高（UTXO を数え上げた結果）</div>
        <div class="ux-body">
          <div class="ux-bal a">
            <span class="ux-bal-name">Alice</span>
            <span class="ux-bal-bar"><span class="ux-bal-fill" :style="{ width: balA * 1.6 + 'px' }" /></span>
            <span class="ux-bal-num">{{ balA }}</span>
          </div>
          <div class="ux-bal b">
            <span class="ux-bal-name">Bob</span>
            <span class="ux-bal-bar"><span class="ux-bal-fill" :style="{ width: balB * 1.6 + 'px' }" /></span>
            <span class="ux-bal-num">{{ balB }}</span>
          </div>
          <p class="ux-bal-note">残高はどこにも保存されず、自分あて UTXO の合計として毎回導かれる</p>
        </div>
      </div>
    </div>

    <div class="ux-action" :class="acc">
      <span class="ux-actor" v-if="cur.actor">{{ cur.actor }}</span>
      <span class="ux-act">{{ cur.action }}</span>
      <span v-if="cur.fee !== null" class="ux-fee">手数料 {{ cur.fee }}</span>
      <span v-if="acc === 'ok'" class="ux-res ok">受理</span>
      <span v-else-if="acc === 'rejected'" class="ux-res ng">拒否（使用済み）</span>
    </div>
    <p class="ux-detail">{{ cur.detail }}</p>
    <p class="ux-note">{{ cur.note }}</p>

    <div class="ux-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="ux-count">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="ux-legend">
      送金は残高を書き換える操作ではなく、<strong>過去の出力を消費して新しい出力を生む</strong>こと。
      残高は集合を数えた結果にすぎず、二重支払いは<strong>一度消費された出力が集合から消える</strong>ことで防がれる。
    </p>
  </DemoShell>
</template>

<style scoped>
.ux-hint {
  font-size: 12px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
.ux-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 16px;
}
@media (max-width: 560px) {
  .ux-grid {
    grid-template-columns: 1fr;
  }
}
.ux-panel {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  overflow: hidden;
}
.ux-panel-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.ux-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 132px;
}
.ux-empty {
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.ux-utxo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  font-size: 13px;
}
.ux-utxo.born {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.ux-utxo.spent {
  opacity: 0.5;
  text-decoration: line-through;
  border-style: dashed;
}
.ux-utxo-id {
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
  min-width: 44px;
}
.ux-utxo-owner {
  flex: 1;
  font-weight: 600;
}
.ux-utxo.a .ux-utxo-owner {
  color: var(--vp-c-brand-1);
}
.ux-utxo.b .ux-utxo-owner {
  color: var(--vp-c-purple-1, #a970ff);
}
.ux-utxo-amt {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.ux-bal {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
}
.ux-bal-name {
  min-width: 44px;
  font-weight: 600;
}
.ux-bal.a .ux-bal-name {
  color: var(--vp-c-brand-1);
}
.ux-bal.b .ux-bal-name {
  color: var(--vp-c-purple-1, #a970ff);
}
.ux-bal-bar {
  flex: 1;
  height: 12px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  overflow: hidden;
}
.ux-bal-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
  transition: width 0.25s;
}
.ux-bal.b .ux-bal-fill {
  background-color: var(--vp-c-purple-1, #a970ff);
}
.ux-bal-num {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  min-width: 28px;
  text-align: right;
}
.ux-bal-note {
  margin: 6px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
  line-height: 1.6;
}
.ux-action {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  font-size: 13px;
}
.ux-action.rejected {
  border-left-color: var(--vp-c-danger-1);
}
.ux-action.ok {
  border-left-color: var(--vp-c-green-1);
}
.ux-actor {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  color: var(--vp-c-text-1);
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  padding: 1px 8px;
}
.ux-act {
  flex: 1;
  color: var(--vp-c-text-1);
}
.ux-fee {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  padding: 2px 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
}
.ux-res {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
}
.ux-res.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.ux-res.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.ux-detail {
  margin: 10px 0 0;
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  line-height: 1.7;
}
.ux-note {
  margin: 8px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.ux-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
}
.ux-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.ux-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
</style>
