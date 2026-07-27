<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// db/mvcc(Go)の考え方をブラウザに移植。
// snapshot: 版の積み上がりとスナップショット読み + first-committer-wins。
// skew: 当直医の write skew を SI / Serializable で対比する。

interface Ver {
  value: string;
  ts: number;
  fresh?: boolean;
}
interface KeyRow {
  key: string;
  versions: Ver[];
}
interface TxnView {
  name: string;
  snap: number;
  note: string;
  state: "run" | "committed" | "aborted" | "idle";
}
interface Frame {
  action: string;
  result: "ok" | "bad" | null;
  note: string;
  rows: KeyRow[];
  txns: TxnView[];
}

function snapshotFrames(): Frame[] {
  const v = (value: string, ts: number, fresh = false): Ver => ({ value, ts, fresh });
  return [
    {
      action: "初期状態",
      result: null,
      note: "balance は版の連なりで持つ(上書きしない)。今は TS=0 の版が 1 つ",
      rows: [{ key: "balance", versions: [v("100", 0)] }],
      txns: [{ name: "T1", snap: 0, note: "未開始", state: "idle" }, { name: "T2", snap: 0, note: "未開始", state: "idle" }],
    },
    {
      action: "T1 begin(snap=1) / T2 begin(snap=2)",
      result: null,
      note: "どちらも balance=100 の世界を見る。読みにロックは要らない",
      rows: [{ key: "balance", versions: [v("100", 0)] }],
      txns: [
        { name: "T1", snap: 1, note: "balance=100 が見える", state: "run" },
        { name: "T2", snap: 2, note: "balance=100 が見える", state: "run" },
      ],
    },
    {
      action: "T1: +50 のつもりで balance=150 を書く(バッファ)",
      result: null,
      note: "書き込みはコミットまでバッファに留まり、ストアにも T2 にも見えない",
      rows: [{ key: "balance", versions: [v("100", 0)] }],
      txns: [
        { name: "T1", snap: 1, note: "write buffer: balance=150", state: "run" },
        { name: "T2", snap: 2, note: "まだ balance=100 が見える", state: "run" },
      ],
    },
    {
      action: "T1 commit → TS=3 の版が積まれる",
      result: "ok",
      note: "上書きせず、新しい版として追加される。古い版は残る(T2 のような古いスナップショットのため)",
      rows: [{ key: "balance", versions: [v("100", 0), v("150", 3, true)] }],
      txns: [
        { name: "T1", snap: 1, note: "commit 成功(TS=3)", state: "committed" },
        { name: "T2", snap: 2, note: "snap=2 なので今も balance=100 を見る", state: "run" },
      ],
    },
    {
      action: "T2: -30 のつもりで balance=70 を書き、commit",
      result: "bad",
      note: "first-committer-wins: T2 が書こうとした balance には、T2 の snapshot(2)より後のコミット(TS=3)が既にある。ここで通すと T1 の +50 が消える(lost update)。だから T2 は敗北し、やり直しになる",
      rows: [{ key: "balance", versions: [v("100", 0), v("150", 3)] }],
      txns: [
        { name: "T1", snap: 1, note: "commit 済み", state: "committed" },
        { name: "T2", snap: 2, note: "ErrWriteConflict で中止", state: "aborted" },
      ],
    },
  ];
}

function skewFrames(serializable: boolean): Frame[] {
  const v = (value: string, ts: number, fresh = false): Ver => ({ value, ts, fresh });
  const base: Frame[] = [
    {
      action: "初期状態: alice と bob が当直(規則: 最低 1 人)",
      result: null,
      note: "oncall:alice=yes, oncall:bob=yes。2 人とも「相手が残るなら抜けたい」と考えている",
      rows: [
        { key: "oncall:alice", versions: [v("yes", 0)] },
        { key: "oncall:bob", versions: [v("yes", 0)] },
      ],
      txns: [{ name: "T1(alice)", snap: 0, note: "未開始", state: "idle" }, { name: "T2(bob)", snap: 0, note: "未開始", state: "idle" }],
    },
    {
      action: "両方 begin し、両方のキーを読む",
      result: null,
      note: "T1 も T2 も「当直は 2 人」を確認。どちらの判断も、その時点では正しい",
      rows: [
        { key: "oncall:alice", versions: [v("yes", 0)] },
        { key: "oncall:bob", versions: [v("yes", 0)] },
      ],
      txns: [
        { name: "T1(alice)", snap: 1, note: "読み: alice=yes, bob=yes → 抜けてよし", state: "run" },
        { name: "T2(bob)", snap: 2, note: "読み: alice=yes, bob=yes → 抜けてよし", state: "run" },
      ],
    },
    {
      action: "T1 は oncall:alice=no、T2 は oncall:bob=no を書く",
      result: null,
      note: "書くキーが別々なので、書き込み競合(first-committer-wins)は起きない。ここが write skew の抜け道になる",
      rows: [
        { key: "oncall:alice", versions: [v("yes", 0)] },
        { key: "oncall:bob", versions: [v("yes", 0)] },
      ],
      txns: [
        { name: "T1(alice)", snap: 1, note: "write buffer: alice=no", state: "run" },
        { name: "T2(bob)", snap: 2, note: "write buffer: bob=no", state: "run" },
      ],
    },
    {
      action: "T1 commit → 成功(TS=3)",
      result: "ok",
      note: "alice が抜けた。この時点では bob が残っているので規則は守られている",
      rows: [
        { key: "oncall:alice", versions: [v("yes", 0), v("no", 3, true)] },
        { key: "oncall:bob", versions: [v("yes", 0)] },
      ],
      txns: [
        { name: "T1(alice)", snap: 1, note: "commit 成功", state: "committed" },
        { name: "T2(bob)", snap: 2, note: "コミット待ち", state: "run" },
      ],
    },
  ];
  if (serializable) {
    base.push({
      action: "T2 commit → 読み集合の検証で中止",
      result: "bad",
      note: "T2 が読んだ oncall:alice は、T2 のスナップショット後に書き換えられた(TS=3)。T2 の「2 人いる」という判断は古い世界のもの。Serializable はここで止める。当直は 1 人残り、規則は守られる",
      rows: [
        { key: "oncall:alice", versions: [v("yes", 0), v("no", 3)] },
        { key: "oncall:bob", versions: [v("yes", 0)] },
      ],
      txns: [
        { name: "T1(alice)", snap: 1, note: "commit 済み", state: "committed" },
        { name: "T2(bob)", snap: 2, note: "ErrRWConflict で中止 → やり直すと「1 人しかいない」と分かる", state: "aborted" },
      ],
    });
  } else {
    base.push({
      action: "T2 commit → SI では通ってしまう",
      result: "bad",
      note: "T2 が書いた oncall:bob には後発コミットが無いので、first-committer-wins は素通し。両方コミットされ、当直はゼロ。それぞれは正しい判断なのに、直列実行では決して起きない状態になった。これが write skew",
      rows: [
        { key: "oncall:alice", versions: [v("yes", 0), v("no", 3)] },
        { key: "oncall:bob", versions: [v("yes", 0), v("no", 4, true)] },
      ],
      txns: [
        { name: "T1(alice)", snap: 1, note: "commit 済み", state: "committed" },
        { name: "T2(bob)", snap: 2, note: "commit 成功 → 当直ゼロ(異常)", state: "committed" },
      ],
    });
  }
  return base;
}

const mode = ref<"snapshot" | "skew">("snapshot");
const level = ref<"si" | "ssi">("si");
const frames = computed(() => (mode.value === "snapshot" ? snapshotFrames() : skewFrames(level.value === "ssi")));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);

function setMode(m: "snapshot" | "skew") {
  mode.value = m;
  at.value = 0;
}
function setLevel(l: "si" | "ssi") {
  level.value = l;
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
  if (mode.value === "snapshot") return "lost update を防いだ";
  return level.value === "ssi" ? "write skew を止めた" : "write skew が通った";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => {
  if (!done.value) return "neutral";
  if (mode.value === "skew" && level.value === "si") return "ng";
  return "ok";
});
</script>

<template>
  <DemoShell title="mvcc(多版と分離レベル)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === 'snapshot' }" @click="setMode('snapshot')">スナップショット + 先勝ち</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'skew' }" @click="setMode('skew')">write skew</span>
      </span>
      <span class="spacer" />
      <span v-if="mode === 'skew'" class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: level === 'si' }" @click="setLevel('si')">SI</span>
        <span class="sd-seg-opt" :class="{ on: level === 'ssi' }" @click="setLevel('ssi')">Serializable</span>
      </span>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
    </div>

    <div class="mv-panel">
      <div class="mv-panel-head">ストア(キーごとの版の連なり)</div>
      <div class="mv-body">
        <div v-for="row in cur.rows" :key="row.key" class="mv-row">
          <span class="mv-key mono">{{ row.key }}</span>
          <span class="mv-arrow">→</span>
          <span v-for="ver in row.versions" :key="ver.ts" class="mv-ver mono" :class="{ fresh: ver.fresh }">
            {{ ver.value }}<span class="mv-ts">@{{ ver.ts }}</span>
          </span>
        </div>
      </div>
    </div>

    <div class="mv-txns">
      <div v-for="tx in cur.txns" :key="tx.name" class="mv-txn" :class="tx.state">
        <div class="mv-txn-head">
          <span class="mv-txn-name mono">{{ tx.name }}</span>
          <span v-if="tx.state !== 'idle'" class="mv-txn-snap mono">snap={{ tx.snap }}</span>
          <span class="mv-txn-st" :class="tx.state">{{ tx.state === "run" ? "実行中" : tx.state === "committed" ? "commit" : tx.state === "aborted" ? "中止" : "-" }}</span>
        </div>
        <p class="mv-txn-note">{{ tx.note }}</p>
      </div>
    </div>

    <div class="mv-action" :class="cur.result">
      <span class="mv-act mono">{{ cur.action }}</span>
    </div>
    <p class="mv-note">{{ cur.note }}</p>

    <div class="mv-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="mv-count mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="mv-legend">
      MVCC は上書きせず版を積み、各トランザクションは開始時点で見える版だけを読む。読み手はロックを
      取らない。同じキーへの並行書き込みは先勝ちで lost update を防ぐが、別々のキーに書く write skew は
      すり抜ける。Serializable は「読んだ値が後から変わっていないか」まで検証して、それも止める。
    </p>
  </DemoShell>
</template>

<style scoped>
.mv-panel {
  margin-top: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.mv-panel-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.mv-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.mv-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 13px;
}
.mv-key {
  font-weight: 700;
  color: var(--vp-c-brand-1);
  min-width: 100px;
}
.mv-arrow {
  color: var(--vp-c-text-3);
}
.mv-ver {
  font-size: 12px;
  padding: 2px 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
}
.mv-ver.fresh {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.mv-ts {
  color: var(--vp-c-text-3);
  font-size: 10px;
  margin-left: 3px;
}
.mv-txns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 12px;
}
@media (max-width: 560px) {
  .mv-txns {
    grid-template-columns: 1fr;
  }
}
.mv-txn {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg);
}
.mv-txn.committed {
  border-color: var(--vp-c-green-1);
}
.mv-txn.aborted {
  border-color: var(--vp-c-danger-1);
}
.mv-txn-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.mv-txn-name {
  font-weight: 700;
  font-size: 13px;
}
.mv-txn-snap {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.mv-txn-st {
  margin-left: auto;
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.mv-txn-st.committed {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.mv-txn-st.aborted {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.mv-txn-note {
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--vp-c-text-2);
}
.mv-action {
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
.mv-action.ok {
  border-left-color: var(--vp-c-green-1);
}
.mv-action.bad {
  border-left-color: var(--vp-c-danger-1);
}
.mv-act {
  flex: 1;
  color: var(--vp-c-text-1);
}
.mv-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 60px;
}
.mv-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}
.mv-count {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.mv-legend {
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
