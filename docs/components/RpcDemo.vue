<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// RPC の相関(correlation)とフレーミングを可視化(Go 実装 messaging/rpc の考え方を移植)。
// 1本の接続に複数の呼び出しを多重化し、応答を要求ID で突き合わせる。応答は送った順に
// 返るとは限らない(サーバは並行処理)——それでも ID で正しく相関することを見る。

type CallState = "waiting" | "done" | "timeout";
interface Call {
  id: number;
  method: string;
  arg: string;
  state: CallState;
  result: string;
}

const METHODS = ["echo", "add", "slow"];

const state = reactive({
  calls: [] as Call[],
  served: [] as number[], // サーバが応答を出した ID
  nextId: 1,
  note: "メソッドを呼ぶと接続に多重化される。『サーバが応答』は後着から返し、順不同を作る",
});

function frameLen(c: Call) {
  // 長さ前置きフレームの概算バイト数(id + method + arg + タグ)。フレーミングの雰囲気用。
  return 3 + c.method.length + c.arg.length;
}

function callMethod(method: string) {
  const id = state.nextId++;
  const arg = method === "add" ? `${id},${id * 2}` : method === "echo" ? `ping${id}` : `job${id}`;
  state.calls.push({ id, method, arg, state: "waiting", result: "" });
  state.note = `#${id} ${method}(${arg}) を送信。応答が返るまで待つ(オフセットではなく ID で待つ)`;
}

function resultOf(c: Call) {
  if (c.method === "echo") return c.arg;
  if (c.method === "add") {
    const [a, b] = c.arg.split(",").map(Number);
    return String(a + b);
  }
  return "done";
}

function serverReply() {
  // まだ応答していない呼び出しのうち、最も新しいものから返す(LIFO)= わざと順不同にする。
  const pending = state.calls.filter((c) => !state.served.includes(c.id));
  if (pending.length === 0) {
    state.note = "サーバ側に未処理の呼び出しがない";
    return;
  }
  const target = pending[pending.length - 1];
  state.served.push(target.id);
  // 応答が接続を戻ってくる。クライアントは ID で相関する。
  const waiter = state.calls.find((c) => c.id === target.id && c.state === "waiting");
  if (waiter) {
    waiter.state = "done";
    waiter.result = resultOf(waiter);
    state.note = `#${target.id} の応答が到着。ID で相関して結果を返した(送信順と違ってもよい)`;
  } else {
    state.note = `#${target.id} の応答が返ったが、相関先が無い(タイムアウト済み)ので破棄`;
  }
}

function timeout(id: number) {
  const c = state.calls.find((x) => x.id === id);
  if (!c || c.state !== "waiting") return;
  c.state = "timeout";
  state.note = `#${id} をタイムアウト。待つのをやめた。後で応答が来ても相関先が無く破棄される`;
}

function reset() {
  state.calls = [];
  state.served = [];
  state.nextId = 1;
  state.note = "メソッドを呼ぶと接続に多重化される。『サーバが応答』は後着から返し、順不同を作る";
}

const waiting = computed(() => state.calls.filter((c) => c.state === "waiting"));
const badge = computed(() => `呼び出し ${state.calls.length} / 待機中 ${waiting.value.length}`);
function tone(c: Call): "ok" | "ng" | "neutral" {
  return c.state === "done" ? "ok" : c.state === "timeout" ? "ng" : "neutral";
}
function stateLabel(c: Call) {
  return c.state === "done" ? "完了" : c.state === "timeout" ? "タイムアウト" : "待機中";
}
</script>

<template>
  <DemoShell title="RPC(多重化と相関)" :badge="badge" badge-tone="neutral">
    <div class="sd-controls">
      <button v-for="m in METHODS" :key="m" class="sd-btn sd-btn--primary" @click="callMethod(m)">
        {{ m }} を呼ぶ
      </button>
      <button class="sd-btn" @click="serverReply">サーバが応答(後着から)</button>
      <span class="spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="rc-wire-wrap">
      <div class="rc-label">接続を流れるフレーム(長さ前置きで1メッセージを切り出す)</div>
      <div class="rc-wire">
        <span v-for="c in waiting" :key="c.id" class="rc-frame" :title="`${c.method}(${c.arg})`">
          <span class="rc-len">len {{ frameLen(c) }}</span>
          <span class="rc-fid">#{{ c.id }}</span>
          <span class="rc-fm">{{ c.method }}</span>
        </span>
        <span v-if="waiting.length === 0" class="rc-empty">(在中の要求なし)</span>
      </div>
    </div>

    <div class="rc-calls-wrap">
      <div class="rc-label">呼び出しの一覧(ID で応答と突き合わせる)</div>
      <div class="rc-calls">
        <div v-for="c in state.calls" :key="c.id" class="rc-call" :class="c.state">
          <span class="rc-id">#{{ c.id }}</span>
          <span class="rc-method">{{ c.method }}({{ c.arg }})</span>
          <span class="rc-badge" :class="tone(c)">{{ stateLabel(c) }}</span>
          <span v-if="c.state === 'done'" class="rc-result">→ {{ c.result }}</span>
          <button v-if="c.state === 'waiting'" class="sd-btn sd-btn--sm" @click="timeout(c.id)">
            タイムアウト
          </button>
        </div>
        <div v-if="state.calls.length === 0" class="rc-empty">(まだ呼び出しなし)</div>
      </div>
    </div>

    <p class="sd-msg">{{ state.note }}</p>
    <div class="rc-legend">
      <span>応答は送信順に返るとは限らない(サーバは並行処理)。だから ID で相関する</span>
      <span>タイムアウトした呼び出しに遅れて応答が来ても、相関先が無く破棄される</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.rc-wire-wrap,
.rc-calls-wrap {
  margin-top: 14px;
}
.rc-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin-bottom: 6px;
}
.rc-wire {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.rc-frame {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 8px;
  border: 1px solid var(--vp-c-brand-2);
  background-color: var(--vp-c-bg);
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
}
.rc-len {
  color: var(--vp-c-text-3);
}
.rc-fid {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.rc-fm {
  color: var(--vp-c-text-1);
}
.rc-calls {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.rc-call {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 10px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  font-size: 12px;
}
.rc-call.done {
  border-left-color: var(--vp-c-green-2);
}
.rc-call.timeout {
  border-left-color: var(--vp-c-red-2);
}
.rc-id {
  font-weight: 700;
  color: var(--vp-c-brand-1);
  font-family: var(--vp-font-family-mono);
}
.rc-method {
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
}
.rc-badge {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 7px;
  border-radius: 8px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.rc-badge.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.rc-badge.ng {
  background-color: var(--vp-c-red-soft);
  color: var(--vp-c-red-1);
}
.rc-result {
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-green-1);
}
.rc-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.sd-btn--sm {
  padding: 2px 10px;
  font-size: 11px;
}
.rc-legend {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 12px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
