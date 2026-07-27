<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/txn(Go)の 2PC と Saga をブラウザに移植。
// 2pc: prepare→decide の 2 相と、調整役故障によるブロッキング。
// saga: 逆順補償。途中状態が見えることまで含めて対比する。

interface Svc {
  name: string;
  detail: string;
  state: "idle" | "prepared" | "committed" | "aborted" | "done" | "compensated" | "failed" | "blocked";
}
interface Frame {
  action: string;
  result: "ok" | "bad" | "warn" | null;
  note: string;
  svcs: Svc[];
}

function twopcFrames(crash: boolean): Frame[] {
  const base: Frame[] = [
    {
      action: "初期状態",
      result: null,
      note: "在庫と決済、別々のサービスの更新を 1 つのトランザクションに揃えたい。単一の commit は存在しない",
      svcs: [
        { name: "inventory", detail: "残高 100", state: "idle" },
        { name: "payment", detail: "残高 100", state: "idle" },
      ],
    },
    {
      action: "phase 1: prepare(30 を引けるか?)",
      result: null,
      note: "調整役が全員に聞く。各参加者は可能なら 30 をロックして Yes と答える。Yes と答えたら、commit が来たとき必ず遂行できる状態を保ち続ける義務を負う",
      svcs: [
        { name: "inventory", detail: "残高 70 + ロック 30 → Yes", state: "prepared" },
        { name: "payment", detail: "残高 70 + ロック 30 → Yes", state: "prepared" },
      ],
    },
  ];
  if (crash) {
    base.push(
      {
        action: "調整役がクラッシュ(決定を配る前)",
        result: "bad",
        note: "参加者は prepared のまま。自分では commit も abort も決められない——commit を選んで実は abort だったら原子性が壊れるからだ。両者はロックを抱えて調整役の復旧を待つしかない",
        svcs: [
          { name: "inventory", detail: "ロック 30 を抱えて待機", state: "blocked" },
          { name: "payment", detail: "ロック 30 を抱えて待機", state: "blocked" },
        ],
      },
      {
        action: "…時間だけが過ぎる",
        result: "bad",
        note: "ロックされた在庫と与信枠は他の取引に使えない。これが 2PC のブロッキング問題で、調整役が単一障害点になる。実務では調整役のログ永続化 + 復旧、またはタイムアウトと運用介入で解く",
        svcs: [
          { name: "inventory", detail: "ロック 30(塞がったまま)", state: "blocked" },
          { name: "payment", detail: "ロック 30(塞がったまま)", state: "blocked" },
        ],
      },
    );
  } else {
    base.push(
      {
        action: "phase 2: 全員 Yes → commit を配る",
        result: "ok",
        note: "全員 Yes のときだけ commit。両者ともロック分を確定する。「一部だけ引き落とされた」状態は決して確定しない",
        svcs: [
          { name: "inventory", detail: "残高 70(確定)", state: "committed" },
          { name: "payment", detail: "残高 70(確定)", state: "committed" },
        ],
      },
      {
        action: "別の回: payment の残高が 10 しかない",
        result: "warn",
        note: "payment は No と答える。1 人でも No なら決定は abort になり、Yes と答えてロックしていた inventory も全額戻る。原子性は守られた",
        svcs: [
          { name: "inventory", detail: "残高 100(ロック解放)", state: "aborted" },
          { name: "payment", detail: "残高 10 → No", state: "aborted" },
        ],
      },
    );
  }
  return base;
}

function sagaFrames(): Frame[] {
  return [
    {
      action: "旅行予約 Saga: flight → hotel → car",
      result: null,
      note: "各ステップは Do(本処理)と Compensate(補償)の対で定義する。ロックは持たず、各ステップはローカルに即コミットする",
      svcs: [
        { name: "flight", detail: "未実行", state: "idle" },
        { name: "hotel", detail: "未実行", state: "idle" },
        { name: "car", detail: "未実行", state: "idle" },
      ],
    },
    {
      action: "flight.Do() → 予約確定",
      result: "ok",
      note: "航空券は即確定する。この瞬間、外部からは「航空券だけ取れている」状態が見える。2PC ならロックの内側に隠れていた途中状態が、Saga では露出する",
      svcs: [
        { name: "flight", detail: "予約済み(確定)", state: "done" },
        { name: "hotel", detail: "未実行", state: "idle" },
        { name: "car", detail: "未実行", state: "idle" },
      ],
    },
    {
      action: "hotel.Do() → 予約確定",
      result: "ok",
      note: "ホテルも即確定。ここまで順調。ロックが無いので、他の客の予約を待たせていない",
      svcs: [
        { name: "flight", detail: "予約済み(確定)", state: "done" },
        { name: "hotel", detail: "予約済み(確定)", state: "done" },
        { name: "car", detail: "未実行", state: "idle" },
      ],
    },
    {
      action: "car.Do() → 失敗(在庫なし)",
      result: "bad",
      note: "前進をやめ、補償に切り替える。失敗したステップ自身は補償しない(何も確定していないから)",
      svcs: [
        { name: "flight", detail: "予約済み(確定)", state: "done" },
        { name: "hotel", detail: "予約済み(確定)", state: "done" },
        { name: "car", detail: "在庫なしで失敗", state: "failed" },
      ],
    },
    {
      action: "補償を逆順に: hotel をキャンセル → flight をキャンセル",
      result: "warn",
      note: "補償はロールバックではなく「キャンセル」という新しい操作だ。予約はいったん確定していたので、取り消しではなく打ち消しを実行する。逆順なのは、後のステップが前の結果に依存しているから。最終的に辻褄が合う(結果整合)",
      svcs: [
        { name: "flight", detail: "キャンセル済み", state: "compensated" },
        { name: "hotel", detail: "キャンセル済み", state: "compensated" },
        { name: "car", detail: "在庫なしで失敗", state: "failed" },
      ],
    },
  ];
}

const mode = ref<"2pc" | "saga">("2pc");
const crash = ref(false);
const frames = computed(() => (mode.value === "2pc" ? twopcFrames(crash.value) : sagaFrames()));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);

function setMode(m: "2pc" | "saga") {
  mode.value = m;
  at.value = 0;
}
function setCrash(v: boolean) {
  crash.value = v;
  at.value = 0;
}
const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.value.length - 1);
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
  at.value = frames.value.length - 1;
}

const done = computed(() => at.value === frames.value.length - 1);
const badge = computed(() => {
  if (!done.value) return `step ${at.value + 1}`;
  if (mode.value === "saga") return "補償で辻褄が合った";
  return crash.value ? "ロックを抱えて停止" : "原子性を守った";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => {
  if (!done.value) return "neutral";
  return mode.value === "2pc" && crash.value ? "ng" : "ok";
});

function stLabel(s: Svc["state"]): string {
  switch (s) {
    case "prepared":
      return "prepared";
    case "committed":
      return "commit";
    case "aborted":
      return "abort";
    case "done":
      return "確定";
    case "compensated":
      return "補償済";
    case "failed":
      return "失敗";
    case "blocked":
      return "待機中";
    default:
      return "-";
  }
}
</script>

<template>
  <DemoShell title="distributed-txn(2PC / Saga)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === '2pc' }" @click="setMode('2pc')">2PC</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'saga' }" @click="setMode('saga')">Saga</span>
      </span>
      <span class="spacer" />
      <span v-if="mode === '2pc'" class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !crash }" @click="setCrash(false)">正常</span>
        <span class="sd-seg-opt" :class="{ on: crash }" @click="setCrash(true)">調整役が落ちる</span>
      </span>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
    </div>

    <div class="dt-svcs" :class="{ three: cur.svcs.length === 3 }">
      <div v-for="s in cur.svcs" :key="s.name" class="dt-svc" :class="s.state">
        <div class="dt-svc-head">
          <span class="dt-svc-name mono">{{ s.name }}</span>
          <span class="dt-svc-st" :class="s.state">{{ stLabel(s.state) }}</span>
        </div>
        <p class="dt-svc-detail">{{ s.detail }}</p>
      </div>
    </div>

    <div class="dt-action" :class="cur.result">
      <span class="dt-act mono">{{ cur.action }}</span>
    </div>
    <p class="dt-note">{{ cur.note }}</p>

    <div class="dt-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="dt-count mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="dt-legend">
      2PC は prepare で全員の合意を取ってから commit を配る。原子性は強いが、Yes と答えた参加者は
      調整役の決定が届くまでロックを抱え、調整役が落ちると動けない。Saga はロックを持たず各ステップを
      即コミットし、失敗したら補償を逆順に実行する。止まらない代わりに、途中状態が外から見える。
    </p>
  </DemoShell>
</template>

<style scoped>
.dt-svcs {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 16px;
}
.dt-svcs.three {
  grid-template-columns: 1fr 1fr 1fr;
}
@media (max-width: 560px) {
  .dt-svcs,
  .dt-svcs.three {
    grid-template-columns: 1fr;
  }
}
.dt-svc {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg);
  min-height: 74px;
}
.dt-svc.committed,
.dt-svc.done {
  border-color: var(--vp-c-green-1);
}
.dt-svc.failed,
.dt-svc.blocked {
  border-color: var(--vp-c-danger-1);
}
.dt-svc.prepared,
.dt-svc.compensated,
.dt-svc.aborted {
  border-color: var(--vp-c-yellow-1, #d0a215);
}
.dt-svc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.dt-svc-name {
  font-weight: 700;
  font-size: 13px;
}
.dt-svc-st {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.dt-svc-st.committed,
.dt-svc-st.done {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.dt-svc-st.failed,
.dt-svc-st.blocked {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.dt-svc-st.prepared,
.dt-svc-st.compensated,
.dt-svc-st.aborted {
  background-color: var(--vp-c-yellow-soft, #f5e8c0);
  color: var(--vp-c-yellow-1, #a37e00);
}
.dt-svc-detail {
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--vp-c-text-2);
}
.dt-action {
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
.dt-action.ok {
  border-left-color: var(--vp-c-green-1);
}
.dt-action.bad {
  border-left-color: var(--vp-c-danger-1);
}
.dt-action.warn {
  border-left-color: var(--vp-c-yellow-1, #d0a215);
}
.dt-act {
  flex: 1;
  color: var(--vp-c-text-1);
}
.dt-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 60px;
}
.dt-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}
.dt-count {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.dt-legend {
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
