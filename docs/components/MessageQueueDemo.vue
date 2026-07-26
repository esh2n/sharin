<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// ログ型メッセージキュー(Go 実装 messaging/queue の考え方を移植)。
// 配送保証(at-most-once / at-least-once)と、冪等消費による「実質1回」を目で見る。

type Semantics = "at-most-once" | "at-least-once";
interface Msg {
  offset: number;
  key: string;
  body: string;
}
interface Applied {
  key: string;
  body: string;
  dup: boolean; // 冪等で重複として捨てられたか
}

const state = reactive({
  log: [] as Msg[],
  committed: 0, // 次に読む offset
  semantics: "at-least-once" as Semantics,
  idempotent: false,
  applied: [] as Applied[], // 処理の効果ログ(呼ばれた順)
  seen: new Set<string>(),
  handleCalls: 0, // handle が呼ばれた総回数
  seq: 0,
  note: "発行して消費してみる。『クラッシュ消費』で確定前に落ちると挙動が分かれる",
});

const SEMANTICS: { key: Semantics; label: string }[] = [
  { key: "at-most-once", label: "at-most-once" },
  { key: "at-least-once", label: "at-least-once" },
];

function handle(m: Msg) {
  state.handleCalls++;
  if (state.idempotent && state.seen.has(m.key)) {
    state.applied.push({ key: m.key, body: m.body, dup: true });
    return;
  }
  state.seen.add(m.key);
  state.applied.push({ key: m.key, body: m.body, dup: false });
}

function batch(max: number): Msg[] {
  return state.log.slice(state.committed, state.committed + max);
}

function publish() {
  state.seq += 1;
  state.log.push({ offset: state.log.length, key: "msg-" + state.seq, body: "w" + state.seq });
  state.note = `メッセージ msg-${state.seq} を発行。ブローカのログに積まれた(消費してもログは消えない)`;
}

function poll() {
  const b = batch(3);
  if (b.length === 0) {
    state.note = "未読なし。先に発行する";
    return;
  }
  if (state.semantics === "at-most-once") {
    state.committed += b.length; // 先に確定
    for (const m of b) handle(m);
  } else {
    for (const m of b) handle(m);
    state.committed += b.length; // 後で確定
  }
  state.note = `${b.length}件を消費(${state.semantics})。オフセットは ${state.committed} まで進んだ`;
}

function pollCrash() {
  const b = batch(3);
  if (b.length === 0) {
    state.note = "未読なし。先に発行する";
    return;
  }
  if (state.semantics === "at-most-once") {
    // 確定は済ませるが処理の前に落ちる → 取りこぼす
    state.committed += b.length;
    state.note = `クラッシュ: at-most-once は確定が先。${b.length}件は処理されず取りこぼした(再配送されない)`;
  } else {
    // 処理はするが確定の前に落ちる → 次の消費で再配送=重複
    for (const m of b) handle(m);
    state.note = `クラッシュ: at-least-once は処理が先。${b.length}件は処理したが未確定。次の消費で再配送される`;
  }
}

function toggleIdempotent() {
  state.idempotent = !state.idempotent;
  state.note = state.idempotent
    ? "冪等 ON: 同じ Key の再配送は捨てる(実質1回になる)"
    : "冪等 OFF: 再配送された分もそのまま副作用になる(重複しうる)";
}
function setSemantics(s: Semantics) {
  state.semantics = s;
  state.note = `配送保証を ${s} に。${s === "at-most-once" ? "確定が先=取りこぼしうる" : "処理が先=重複しうる"}`;
}
function reset() {
  state.log = [];
  state.committed = 0;
  state.applied = [];
  state.seen = new Set();
  state.handleCalls = 0;
  state.seq = 0;
  state.note = "発行して消費してみる。『クラッシュ消費』で確定前に落ちると挙動が分かれる";
}

const appliedCount = computed(() => state.applied.filter((a) => !a.dup).length);
const dupCount = computed(() => state.applied.filter((a) => a.dup).length);
const badge = computed(() => `確定 ${state.committed}/${state.log.length}`);
</script>

<template>
  <DemoShell title="メッセージキュー(ログ + オフセット)" :badge="badge" badge-tone="neutral">
    <div class="sd-controls">
      <div class="sd-seg">
        <span
          v-for="s in SEMANTICS"
          :key="s.key"
          class="sd-seg-opt"
          :class="{ on: state.semantics === s.key }"
          @click="setSemantics(s.key)"
        >
          {{ s.label }}
        </span>
      </div>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: state.idempotent }" @click="toggleIdempotent">冪等 {{ state.idempotent ? "ON" : "OFF" }}</span>
      </span>
      <span class="spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>
    <div class="sd-controls mq-actions">
      <button class="sd-btn sd-btn--primary" @click="publish">発行</button>
      <button class="sd-btn" @click="poll">消費</button>
      <button class="sd-btn" @click="pollCrash">クラッシュ消費</button>
    </div>

    <div class="mq-log-wrap">
      <div class="mq-label">ブローカのログ(追記され、消えない)</div>
      <div class="mq-log">
        <span
          v-for="m in state.log"
          :key="m.offset"
          class="mq-cell"
          :class="{ consumed: m.offset < state.committed }"
          :title="m.key + ' / ' + m.body"
        >
          {{ m.body }}
        </span>
        <span v-if="state.log.length === 0" class="mq-empty">(空)</span>
      </div>
      <div class="mq-cursor">
        消費者オフセット: <strong>{{ state.committed }}</strong>(ここから先が未読)
      </div>
    </div>

    <div class="mq-applied-wrap">
      <div class="mq-label">
        処理の効果 — handle 呼び出し {{ state.handleCalls }}回 / 実質適用 {{ appliedCount }}件
        <template v-if="dupCount > 0"> / 重複 {{ dupCount }}件</template>
      </div>
      <div class="mq-applied">
        <span
          v-for="(a, i) in state.applied"
          :key="i"
          class="mq-tag"
          :class="{ dup: a.dup }"
          :title="a.dup ? '冪等で捨てられた重複' : '副作用として適用'"
        >
          {{ a.key }}<span v-if="a.dup" class="mq-x"> ×重複</span>
        </span>
        <span v-if="state.applied.length === 0" class="mq-empty">(まだ処理なし)</span>
      </div>
    </div>

    <p class="sd-msg">{{ state.note }}</p>
    <div class="mq-legend">
      <span><i class="sw consumed" />確定済み(消費者が読み終えた)</span>
      <span>at-least-once + 冪等で「取りこぼさない・重複させない」= 実質1回</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.mq-actions {
  margin-top: 8px;
}
.mq-log-wrap,
.mq-applied-wrap {
  margin-top: 14px;
}
.mq-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin-bottom: 6px;
}
.mq-log,
.mq-applied {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  align-items: center;
}
.mq-cell {
  min-width: 30px;
  padding: 4px 8px;
  text-align: center;
  border: 1px solid var(--vp-c-brand-2);
  color: var(--vp-c-brand-1);
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  background-color: var(--vp-c-bg);
}
.mq-cell.consumed {
  border-color: var(--vp-c-green-2);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.mq-cursor {
  margin-top: 8px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.mq-tag {
  padding: 3px 8px;
  border: 1px solid var(--vp-c-divider);
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-default-soft);
}
.mq-tag.dup {
  color: var(--vp-c-text-3);
  text-decoration: line-through;
  border-style: dashed;
}
.mq-x {
  text-decoration: none;
  color: var(--vp-c-red-1);
  font-weight: 600;
}
.mq-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.mq-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin-top: 12px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.mq-legend .sw {
  display: inline-block;
  width: 12px;
  height: 12px;
  margin-right: 5px;
  vertical-align: -2px;
  border: 1px solid var(--vp-c-green-2);
  background-color: var(--vp-c-green-soft);
}
</style>
