<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// chain/rollup(Go)の「実行は L2、L1 は root を記録」「不正をどう暴くか」を対比する。
// Optimistic: 楽観受理 → fraud proof で巻き戻し + 保証金没収。
// ZK: validity proof を commit 時に検証 → 不正はそもそも入らない。

type St = "pending" | "final" | "reverted";
interface Rec {
  idx: number;
  title: string;
  status: St;
  fraud: boolean;
}
interface Frame {
  records: Rec[];
  canonical: string;
  slashed: number;
  action: string;
  result: "ok" | "bad" | "revert" | null;
  note: string;
}

function buildOptimistic(): Frame[] {
  return [
    {
      records: [],
      canonical: "g0",
      slashed: 0,
      action: "初期状態",
      result: null,
      note: "L1 は genesis root(g0)だけを持つ。実行は L2 に逃がし、L1 は state root の列を記録する",
    },
    {
      records: [{ idx: 0, title: "alice→bob 30（正直）", status: "pending", fraud: false }],
      canonical: "s1",
      slashed: 0,
      action: "batch0 を commit",
      result: null,
      note: "Optimistic は再実行も検証もせず受理する(だから安い)。状態は Pending——challenge 期間中はまだ覆りうる",
    },
    {
      records: [
        { idx: 0, title: "alice→bob 30（正直）", status: "pending", fraud: false },
        { idx: 1, title: "attacker 残高を水増し（嘘）", status: "pending", fraud: true },
      ],
      canonical: "s2*",
      slashed: 0,
      action: "batch1（不正）を commit",
      result: "bad",
      note: "不正バッチも検証されないので通ってしまう。ここが optimistic の危うさ——「1 人でも正直な監視者がいる」ことに安全性が乗っている",
    },
    {
      records: [
        { idx: 0, title: "alice→bob 30（正直）", status: "pending", fraud: false },
        { idx: 1, title: "attacker 残高を水増し（嘘）", status: "reverted", fraud: true },
      ],
      canonical: "s1",
      slashed: 50,
      action: "batch1 を challenge（fraud proof）",
      result: "revert",
      note: "監視者が witness(開始状態)を提示 → L1 が再実行して主張と食い違うことを確認 → 不正確定。batch1 は巻き戻り、canonical は s1 へ。保証金 50 を没収(slashing)",
    },
    {
      records: [
        { idx: 0, title: "alice→bob 30（正直）", status: "final", fraud: false },
        { idx: 1, title: "attacker 残高を水増し（嘘）", status: "reverted", fraud: true },
      ],
      canonical: "s1",
      slashed: 50,
      action: "challenge 期間経過 → Finalize",
      result: "ok",
      note: "期間を過ぎた正直なバッチ batch0 が Final に。一度確定するともう覆せない。確定まで待つのが optimistic の代償(実際は約 7 日)",
    },
  ];
}

function buildZK(): Frame[] {
  return [
    {
      records: [],
      canonical: "g0",
      slashed: 0,
      action: "初期状態",
      result: null,
      note: "L1 は genesis root(g0)だけ。ZK では各バッチに「計算が正しい証明(validity proof)」が付く",
    },
    {
      records: [{ idx: 0, title: "alice→bob 40（正直・proof 有効）", status: "final", fraud: false }],
      canonical: "s1",
      slashed: 0,
      action: "batch0 を commit",
      result: "ok",
      note: "L1 は proof を検証。有効なので即 Final——challenge 期間は要らない。監視者もいらない。ファイナリティが速い",
    },
    {
      records: [{ idx: 0, title: "alice→bob 40（正直・proof 有効）", status: "final", fraud: false }],
      canonical: "s1",
      slashed: 0,
      action: "batch1（不正）を commit → 拒否",
      result: "bad",
      note: "嘘の PostRoot には有効な proof が作れない。L1 は commit の瞬間に proof 無効を検出して拒否——不正はそもそも記録に入らない。optimistic のように「入れてから暴く」必要がない",
    },
  ];
}

const mode = ref<"optimistic" | "zk">("optimistic");
const frames = computed(() => (mode.value === "optimistic" ? buildOptimistic() : buildZK()));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);

function setMode(m: "optimistic" | "zk") {
  mode.value = m;
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
  return mode.value === "optimistic" ? "不正は巻き戻された" : "不正は commit で拒否";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (done.value ? "ok" : "neutral"));

function statusLabel(s: St): string {
  return s === "final" ? "確定" : s === "reverted" ? "巻き戻し" : "保留中";
}
</script>

<template>
  <DemoShell title="rollup(Optimistic vs ZK)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === 'optimistic' }" @click="setMode('optimistic')">Optimistic</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'zk' }" @click="setMode('zk')">ZK</span>
      </span>
      <span class="spacer" />
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
    </div>

    <div class="rl-grid">
      <div class="rl-panel">
        <div class="rl-panel-head">L1 に記録されたバッチ</div>
        <div class="rl-body">
          <div v-if="cur.records.length === 0" class="rl-empty">(まだバッチなし)</div>
          <div v-for="rec in cur.records" :key="rec.idx" class="rl-batch" :class="[rec.status, { fraud: rec.fraud }]">
            <span class="rl-batch-idx">#{{ rec.idx }}</span>
            <span class="rl-batch-title">{{ rec.title }}</span>
            <span class="rl-batch-st" :class="rec.status">{{ statusLabel(rec.status) }}</span>
          </div>
        </div>
      </div>

      <div class="rl-panel">
        <div class="rl-panel-head">L1 の状態</div>
        <div class="rl-body">
          <div class="rl-kv">
            <span class="rl-k">canonical root</span>
            <span class="rl-v mono">{{ cur.canonical }}</span>
          </div>
          <div class="rl-kv">
            <span class="rl-k">没収した保証金</span>
            <span class="rl-v mono" :class="{ warn: cur.slashed > 0 }">{{ cur.slashed }}</span>
          </div>
          <p class="rl-mode-note">
            <template v-if="mode === 'optimistic'">Optimistic: 検証せず受理し、challenge 期間内の fraud proof で覆す。安いが遅く、正直な監視者が前提</template>
            <template v-else>ZK: validity proof を commit 時に検証して即確定。証明生成は重いが速く、監視者不要</template>
          </p>
        </div>
      </div>
    </div>

    <div class="rl-action" :class="cur.result">
      <span class="rl-act">{{ cur.action }}</span>
      <span v-if="cur.result === 'ok'" class="rl-res ok">確定</span>
      <span v-else-if="cur.result === 'bad'" class="rl-res ng">{{ mode === 'zk' ? '拒否' : '不正が混入' }}</span>
      <span v-else-if="cur.result === 'revert'" class="rl-res warn">巻き戻し + 没収</span>
    </div>
    <p class="rl-note">{{ cur.note }}</p>

    <div class="rl-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="rl-count">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="rl-legend">
      L1 は取引を<strong>再実行しない</strong>——root の列を記録するだけ。正しさの担保が、
      Optimistic では<strong>事後の fraud proof</strong>(＋保証金没収)、ZK では
      <strong>事前の validity proof</strong>に分かれる。「暴くか、証明するか」が Layer2 設計の中心。
    </p>
  </DemoShell>
</template>

<style scoped>
.rl-grid {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 12px;
  margin-top: 16px;
}
@media (max-width: 560px) {
  .rl-grid {
    grid-template-columns: 1fr;
  }
}
.rl-panel {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  overflow: hidden;
}
.rl-panel-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.rl-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 124px;
}
.rl-empty {
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.rl-batch {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  font-size: 13px;
}
.rl-batch.fraud {
  border-color: var(--vp-c-danger-1);
}
.rl-batch.reverted {
  opacity: 0.55;
  text-decoration: line-through;
  border-style: dashed;
}
.rl-batch.final {
  border-color: var(--vp-c-green-1);
}
.rl-batch-idx {
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
}
.rl-batch-title {
  flex: 1;
}
.rl-batch-st {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.rl-batch-st.final {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.rl-batch-st.reverted {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.rl-kv {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  font-size: 13px;
}
.rl-k {
  color: var(--vp-c-text-3);
}
.rl-v {
  color: var(--vp-c-text-1);
  font-weight: 600;
}
.rl-v.mono {
  font-family: var(--vp-font-family-mono);
}
.rl-v.warn {
  color: var(--vp-c-danger-1);
}
.rl-mode-note {
  margin: 6px 0 0;
  font-size: 11px;
  line-height: 1.6;
  color: var(--vp-c-text-3);
}
.rl-action {
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
.rl-action.ok {
  border-left-color: var(--vp-c-green-1);
}
.rl-action.bad {
  border-left-color: var(--vp-c-danger-1);
}
.rl-action.revert {
  border-left-color: var(--vp-c-yellow-1, #d0a215);
}
.rl-act {
  flex: 1;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-1);
}
.rl-res {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
}
.rl-res.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.rl-res.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.rl-res.warn {
  background-color: var(--vp-c-yellow-soft, #f5e8c0);
  color: var(--vp-c-yellow-1, #a37e00);
}
.rl-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.rl-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
}
.rl-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.rl-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
</style>
