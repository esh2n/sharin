<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// 仮想DOMの diff/patch を可視化する。frontend/vdom の考え方を、操作を記録できるよう
// 計測版にして持ち込む。肝は「宣言し直した木は丸ごとでも、実DOM操作は差分だけ」。

type VNode = VEl | VText;
interface VEl {
  kind: "element";
  type: string;
  attrs: Record<string, string>;
  children: VNode[];
}
interface VText {
  kind: "text";
  text: string;
}
const el = (type: string, attrs: Record<string, string>, ...children: VNode[]): VEl => ({
  kind: "element",
  type,
  attrs,
  children,
});
const tx = (text: string): VText => ({ kind: "text", text });

type OpKind = "update" | "add" | "remove" | "attr";
interface Op {
  kind: OpKind;
  label: string;
}

// アプリ状態 → あるべき仮想DOM木。ボタンを押すたびに、この木を毎回まるごと作り直す。
function build(s: { title: string; count: number; items: string[] }): VEl {
  return el(
    "div",
    { class: "card" },
    el("div", { class: "head" }, el("span", { class: "title" }, tx(s.title)), el("span", { class: "badge" }, tx("×" + s.count))),
    el("ul", { class: "list" }, ...s.items.map((t) => el("li", {}, tx(t)))),
  );
}

function countNodes(v: VNode): number {
  if (v.kind === "text") return 1;
  return 1 + v.children.reduce((n, c) => n + countNodes(c), 0);
}

// 計測付き diff: 実DOMへ当てるはずの最小操作を Op として記録する（実DOMは触らない）。
function diff(oldV: VNode, newV: VNode, path: string, ops: Op[]): void {
  if (oldV.kind === "text" && newV.kind === "text") {
    if (oldV.text !== newV.text) ops.push({ kind: "update", label: `テキスト更新 "${oldV.text}" → "${newV.text}"` });
    return;
  }
  if (oldV.kind !== newV.kind || (oldV.kind === "element" && newV.kind === "element" && oldV.type !== newV.type)) {
    ops.push({ kind: "add", label: `<${nodeName(newV)}> に差し替え @${path}` });
    return;
  }
  if (oldV.kind === "element" && newV.kind === "element") {
    // 属性の差分
    for (const k of Object.keys(oldV.attrs)) if (!(k in newV.attrs)) ops.push({ kind: "attr", label: `属性削除 ${k} @${path}` });
    for (const k of Object.keys(newV.attrs)) if (oldV.attrs[k] !== newV.attrs[k]) ops.push({ kind: "attr", label: `属性設定 ${k}="${newV.attrs[k]}" @${path}` });
    // 子の差分（index 対応・共通/追加/削除）
    const common = Math.min(oldV.children.length, newV.children.length);
    for (let i = 0; i < common; i++) diff(oldV.children[i], newV.children[i], `${path}/${i}`, ops);
    for (let i = common; i < newV.children.length; i++) ops.push({ kind: "add", label: `子を追加 <${nodeName(newV.children[i])}> @${path}/${i}` });
    for (let i = common; i < oldV.children.length; i++) ops.push({ kind: "remove", label: `子を削除 <${nodeName(oldV.children[i])}> @${path}/${i}` });
  }
}
function nodeName(v: VNode): string {
  return v.kind === "text" ? `"${v.text}"` : v.type;
}

const state = reactive({
  title: "Todos",
  count: 2,
  items: ["牛乳を買う", "本を返す"],
  ops: [] as Op[],
  declared: 0,
  note: "同じ木を毎回まるごと宣言しても、diff が最小の実DOM操作だけを取り出す。ボタンで試す",
});

// 直前の木を保持して次回の diff の相手にする。
let prev: VEl = build(state);

function apply(next: { title: string; count: number; items: string[] }, note: string) {
  const nextTree = build(next);
  const ops: Op[] = [];
  diff(prev, nextTree, "root", ops);
  state.title = next.title;
  state.count = next.count;
  state.items = next.items;
  state.ops = ops;
  state.declared = countNodes(nextTree);
  state.note = note;
  prev = nextTree;
}

function incr() {
  apply({ ...snapshot(), count: state.count + 1 }, `カウントを ${state.count + 1} に。テキスト1つだけが更新対象`);
}
function addItem() {
  const labels = ["メールを書く", "薬を飲む", "部屋を片付ける", "電話をかける"];
  const next = state.items.concat(labels[state.items.length % labels.length]);
  apply({ ...snapshot(), count: next.length, items: next }, "項目を追加。増えた <li> の append と、件数テキストの更新だけ");
}
function removeItem() {
  if (state.items.length === 0) return;
  const next = state.items.slice(0, -1);
  apply({ ...snapshot(), count: next.length, items: next }, "末尾の項目を削除。末尾 <li> の remove と件数更新だけ");
}
function rename() {
  const titles = ["Todos", "やること", "Tasks", "買い物"];
  const nextTitle = titles[(titles.indexOf(state.title) + 1) % titles.length];
  apply({ ...snapshot(), title: nextTitle }, "タイトルだけ変更。差分はテキスト1つ");
}
function snapshot() {
  return { title: state.title, count: state.count, items: [...state.items] };
}
function reset() {
  state.title = "Todos";
  state.count = 2;
  state.items = ["牛乳を買う", "本を返す"];
  prev = build(state);
  // 初回描画は「全ノード生成」に相当。基準表示として mount 相当を示す。
  state.declared = countNodes(prev);
  state.ops = [{ kind: "add", label: `初回 mount: ${state.declared} ノードを生成` }];
  state.note = "同じ木を毎回まるごと宣言しても、diff が最小の実DOM操作だけを取り出す。ボタンで試す";
}
reset();

const domOps = computed(() => (state.ops.length === 1 && state.ops[0].kind === "add" && state.ops[0].label.startsWith("初回") ? state.declared : state.ops.length));
const badge = computed(() => `宣言 ${state.declared}ノード / 実DOM操作 ${domOps.value}`);
const railColor = (k: OpKind) =>
  k === "update" ? "var(--vp-c-brand-2)" : k === "add" ? "var(--vp-c-green-2)" : k === "remove" ? "var(--vp-c-red-2)" : "var(--vp-c-yellow-2)";
const opTag = (k: OpKind) => (k === "update" ? "更新" : k === "add" ? "追加" : k === "remove" ? "削除" : "属性");
</script>

<template>
  <DemoShell title="仮想DOM(diff / patch)" :badge="badge" badge-tone="neutral">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" @click="incr">カウント+1</button>
      <button class="sd-btn" @click="addItem">項目を追加</button>
      <button class="sd-btn" @click="removeItem">項目を削除</button>
      <button class="sd-btn" @click="rename">タイトル変更</button>
      <span class="spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="vd-grid">
      <div class="vd-col">
        <div class="vd-label">実DOMプレビュー(宣言した木の描画結果)</div>
        <div class="vd-preview">
          <div class="vd-card">
            <div class="vd-card-head">
              <span class="vd-card-title">{{ state.title }}</span>
              <span class="vd-card-badge">×{{ state.count }}</span>
            </div>
            <ul class="vd-card-list">
              <li v-for="(it, i) in state.items" :key="i">{{ it }}</li>
              <li v-if="state.items.length === 0" class="vd-empty">(空)</li>
            </ul>
          </div>
        </div>
      </div>

      <div class="vd-col">
        <div class="vd-label">この更新で実際に走った最小DOM操作</div>
        <div class="vd-ops">
          <div v-for="(op, i) in state.ops" :key="i" class="vd-op" :style="{ borderLeftColor: railColor(op.kind) }">
            <span class="vd-op-tag" :style="{ color: railColor(op.kind) }">{{ opTag(op.kind) }}</span>
            <span class="vd-op-label">{{ op.label }}</span>
          </div>
          <div v-if="state.ops.length === 0" class="vd-op vd-op--none">差分なし(実DOMは一切変更されない)</div>
        </div>
      </div>
    </div>

    <p class="sd-msg">{{ state.note }}</p>
    <div class="vd-legend">
      <span>宣言し直した木は毎回 {{ state.declared }} ノード。だが diff が取り出す実DOM操作は上のぶんだけ</span>
      <span>「毎回まるごと宣言 → 差分だけ適用」が仮想DOMの働き。手で最小のDOM操作を書くより速くはならない</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.vd-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-top: 14px;
}
@media (max-width: 640px) {
  .vd-grid {
    grid-template-columns: 1fr;
  }
}
.vd-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin-bottom: 6px;
}
.vd-preview {
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  padding: 12px;
}
.vd-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.vd-card-title {
  font-size: 14px;
  font-weight: 600;
}
.vd-card-badge {
  margin-left: auto;
  font-size: 11px;
  font-weight: 700;
  padding: 1px 8px;
  border-radius: 8px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.vd-card-list {
  margin: 0;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.vd-card-list li {
  font-size: 13px;
  color: var(--vp-c-text-1);
}
.vd-empty {
  list-style: none;
  color: var(--vp-c-text-3);
  font-size: 12px;
}
.vd-ops {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.vd-op {
  display: flex;
  align-items: baseline;
  gap: 8px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  padding: 6px 10px;
}
.vd-op-tag {
  flex: none;
  font-size: 10px;
  font-weight: 700;
}
.vd-op-label {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  word-break: break-all;
}
.vd-op--none {
  color: var(--vp-c-text-3);
  font-size: 12px;
  border-left-color: var(--vp-c-divider);
}
.vd-legend {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 12px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
