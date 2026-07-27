<script setup lang="ts">
import { ref, reactive } from "vue";
import DemoShell from "./DemoShell.vue";

// frontend/store(TS)を移植。action を dispatch して単方向フローで状態が変わり、
// middleware が dispatch を包んでログを取る様子を見せる。

interface Action {
  type: string;
  [k: string]: unknown;
}
interface State {
  count: number;
  todos: string[];
  version: number; // 状態オブジェクトの世代(参照が変わるたび +1)
}

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "inc":
      return { ...state, count: state.count + 1, version: state.version + 1 };
    case "add":
      return { ...state, count: state.count + (action.by as number), version: state.version + 1 };
    case "addTodo":
      return { ...state, todos: [...state.todos, action.text as string], version: state.version + 1 };
    default:
      return state; // 未知の action は同じ参照
  }
}

// 表示用のリアクティブなミラー。
const view = reactive<State>({ count: 0, todos: [], version: 0 });
const log = ref<string[]>([]);
const notifyCount = ref(0);
const lastSameRef = ref(false);

// --- 移植した store ---
let state: State = { count: 0, todos: [], version: 0 };
const listeners = new Set<() => void>();
function baseDispatch(action: Action): Action {
  const before = state;
  state = reducer(state, action);
  lastSameRef.value = state === before; // 未知 action なら同じ参照
  for (const l of [...listeners]) l();
  return action;
}
// logger middleware。
function dispatch(action: Action): Action {
  log.value = [...log.value, `before: ${action.type}`];
  const r = baseDispatch(action);
  log.value = [...log.value, `after:  ${action.type}`];
  if (log.value.length > 8) log.value = log.value.slice(-8);
  return r;
}
// 購読: state が変わったら view に反映し、通知回数を数える。
listeners.add(() => {
  notifyCount.value++;
  view.count = state.count;
  view.todos = state.todos;
  view.version = state.version;
});

let todoN = 0;
function reset() {
  state = { count: 0, todos: [], version: 0 };
  view.count = 0;
  view.todos = [];
  view.version = 0;
  log.value = [];
  notifyCount.value = 0;
  lastSameRef.value = false;
  todoN = 0;
}
</script>

<template>
  <DemoShell title="状態管理(store)" badge-tone="neutral" :badge="`状態オブジェクト 第${view.version}世代`">
    <div class="st-actions">
      <button class="sd-btn" @click="dispatch({ type: 'inc' })">dispatch inc</button>
      <button class="sd-btn" @click="dispatch({ type: 'add', by: 5 })">dispatch add 5</button>
      <button class="sd-btn" @click="dispatch({ type: 'addTodo', text: `todo${++todoN}` })">dispatch addTodo</button>
      <button class="sd-btn" @click="dispatch({ type: 'unknown' })">dispatch unknown</button>
      <span class="st-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="st-cols">
      <div class="st-panel">
        <div class="st-h">state(読み取り専用)</div>
        <div class="st-state mono">
          <div>count: <span class="st-v">{{ view.count }}</span></div>
          <div>todos: <span class="st-v">[{{ view.todos.join(", ") }}]</span></div>
          <div class="st-meta">オブジェクト参照: 第{{ view.version }}世代</div>
        </div>
        <div class="st-stats">
          <span class="st-stat mono">購読者への通知 {{ notifyCount }}回</span>
          <span v-if="lastSameRef" class="st-same mono">直前の dispatch は無変化 → 同じ参照(再描画不要)</span>
        </div>
      </div>

      <div class="st-panel">
        <div class="st-h">middleware のログ(dispatch を包む)</div>
        <div class="st-log">
          <div v-for="(l, i) in log" :key="i" class="st-log-line mono">{{ l }}</div>
          <div v-if="log.length === 0" class="st-empty">(まだ dispatch していない)</div>
        </div>
      </div>
    </div>

    <p class="st-legend">
      状態は直接触らず、action を dispatch する。reducer が古い状態を書き換えず新しい状態を返すので、
      変更のたびに state オブジェクトの世代が上がる(イミュータブル)。未知の action は同じ参照を返し、
      再描画が要らないと分かる。middleware は dispatch を包んで前後にログを挟む。すべての変更がこの
      一本道を通るので、いつ何が状態を変えたかが必ず記録に残る。
    </p>
  </DemoShell>
</template>

<style scoped>
.st-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.st-spacer {
  flex: 1;
}
.st-cols {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-top: 14px;
}
.st-panel {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px;
  background-color: var(--vp-c-bg-soft);
}
.st-h {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.st-state {
  font-size: 13px;
  line-height: 1.9;
  color: var(--vp-c-text-1);
}
.st-v {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.st-meta {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
  margin-top: 4px;
}
.st-stats {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--vp-c-divider);
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.st-stat {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.st-same {
  font-size: 10.5px;
  color: var(--vp-c-warning-1);
}
.st-log {
  min-height: 120px;
}
.st-log-line {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.st-empty {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.st-legend {
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
