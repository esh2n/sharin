<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// db/sqlinject(Go)を移植。値が構文に何トークン寄与したかで注入を見る。

type Kind = "word" | "num" | "str" | "op" | "comment";
interface Tok { kind: Kind; text: string; from: number }

const KEYWORDS = new Set([
  "select", "from", "where", "or", "and", "insert", "into", "values",
  "drop", "table", "order", "by", "union", "delete", "update",
]);
const isWordRune = (c: string) => /[A-Za-z0-9_*]/.test(c);

function lex(q: string): Tok[] {
  const out: Tok[] = [];
  const r = [...q];
  let i = 0;
  while (i < r.length) {
    const c = r[i];
    if (c === " " || c === "\t" || c === "\n") { i++; continue; }
    if (c === "-" && r[i + 1] === "-") {
      let j = i; while (j < r.length && r[j] !== "\n") j++;
      out.push({ kind: "comment", text: r.slice(i, j).join(""), from: i }); i = j; continue;
    }
    if (c === "'") {
      let j = i + 1;
      while (j < r.length) {
        if (r[j] === "'" && r[j + 1] === "'") { j += 2; continue; }
        if (r[j] === "'") { j++; break; }
        j++;
      }
      out.push({ kind: "str", text: r.slice(i, j).join(""), from: i }); i = j; continue;
    }
    if (c >= "0" && c <= "9") {
      let j = i; while (j < r.length && r[j] >= "0" && r[j] <= "9") j++;
      out.push({ kind: "num", text: r.slice(i, j).join(""), from: i }); i = j; continue;
    }
    if (isWordRune(c)) {
      let j = i; while (j < r.length && isWordRune(r[j])) j++;
      out.push({ kind: "word", text: r.slice(i, j).join(""), from: i }); i = j; continue;
    }
    out.push({ kind: "op", text: c, from: i }); i++;
  }
  return out;
}
const isKeyword = (t: Tok) => t.kind === "word" && KEYWORDS.has(t.text.toLowerCase());

type Mode = "concat" | "escape" | "bind";
type Slot = "quoted" | "bare" | "ident";
const MODES: [Mode, string][] = [
  ["concat", "文字列連結"], ["escape", "引用符を二重にする"], ["bind", "プレースホルダ"],
];
const SLOTS: [Slot, string][] = [
  ["quoted", "引用符ありの値"], ["bare", "引用符なしの値"], ["ident", "識別子(列名など)"],
];

function frame(s: Slot): [string, string] {
  if (s === "bare") return ["SELECT * FROM users WHERE id = ", ""];
  if (s === "ident") return ["SELECT * FROM users ORDER BY ", ""];
  return ["SELECT * FROM users WHERE name = '", "'"];
}
const bindFrame = (s: Slot) =>
  s === "bare" ? "SELECT * FROM users WHERE id = ?" : "SELECT * FROM users WHERE name = ?";

interface Q { text: string; params: string[]; span: [number, number] | null }
function build(m: Mode, s: Slot, v: string): Q {
  if (m === "bind" && s !== "ident") return { text: bindFrame(s), params: [v], span: null };
  const [head, tail] = frame(s);
  const body = m === "escape" ? v.replaceAll("'", "''") : v;
  return { text: head + body + tail, params: [], span: [[...head].length, [...head].length + [...body].length] };
}
function fromValue(q: Q): Tok[] {
  if (!q.span) return [];
  return lex(q.text).filter((t) => t.from < q.span![1] && t.from + [...t.text].length > q.span![0]);
}
const injected = (q: Q) => {
  const ts = fromValue(q);
  return ts.length > 1 || ts.some((t) => isKeyword(t) || t.kind === "comment");
};

const VALUES: [string, Slot[]][] = [
  ["esh2n", ["quoted", "ident"]],
  ["' OR '1'='1", ["quoted"]],
  ["'; DROP TABLE users;--", ["quoted"]],
  ["1 OR 1=1", ["bare"]],
  ["name; DROP TABLE users;--", ["ident"]],
];

const mode = ref<Mode>("concat");
const slot = ref<Slot>("quoted");
const vi = ref(1);
const values = computed(() => VALUES.filter(([, s]) => s.includes(slot.value)));
const value = computed(() => (values.value[vi.value] ?? values.value[0])[0]);
const usable = computed(() => !(mode.value === "bind" && slot.value === "ident"));
const q = computed(() => build(mode.value, slot.value, value.value));
const toks = computed(() => fromValue(q.value));
const bad = computed(() => usable.value && injected(q.value));

const badge = computed(() =>
  !usable.value ? "この場所では使えない" : bad.value ? "注入された" : "値のまま",
);
const tone = computed(() => (!usable.value ? "neutral" : bad.value ? "ng" : "ok"));
const verdict = computed(() => {
  if (!usable.value)
    return "列名は値ではなく構文の一部なので、実行時に差し込めない。ここは許可制にするしかない";
  if (mode.value === "bind")
    return `値は問い合わせに入らない(別経路で ${JSON.stringify(q.value.params[0])})。入力が何であっても、問い合わせの形は1文字も変わらない`;
  if (bad.value)
    return `値が ${toks.value.length} 個のトークンに割れた。1つに収まっていないので、値の一部が構文として読まれている`;
  return `値は ${toks.value.length} 個のトークンに収まっている。構文には寄与していない`;
});
</script>

<template>
  <DemoShell title="値が構文に何トークン寄与したか" :badge="badge" :badge-tone="tone">
    <div class="sq-actions">
      <span class="sd-seg">
        <span v-for="[s, label] in SLOTS" :key="s" class="sd-seg-opt"
              :class="{ on: slot === s }" @click="slot = s; vi = 0">{{ label }}</span>
      </span>
    </div>
    <div class="sq-actions sq-row2">
      <span class="sd-seg">
        <span v-for="[m, label] in MODES" :key="m" class="sd-seg-opt"
              :class="{ on: mode === m }" @click="mode = m">{{ label }}</span>
      </span>
    </div>
    <div class="sq-actions sq-row2">
      <span class="sd-seg">
        <span v-for="([v], i) in values" :key="v" class="sd-seg-opt mono"
              :class="{ on: vi === i }" @click="vi = i">{{ v || "(空)" }}</span>
      </span>
    </div>

    <div class="sq-q mono">{{ q.text }}</div>
    <div v-if="q.params.length" class="sq-param mono">別経路で渡す値: {{ JSON.stringify(q.params[0]) }}</div>

    <div class="sq-toks">
      <span class="sq-label">値の寄与</span>
      <template v-if="toks.length">
        <span v-for="(t, i) in toks" :key="i" class="sq-tok mono"
              :class="{ kw: isKeyword(t) || t.kind === 'comment' }">{{ t.text }}</span>
      </template>
      <span v-else class="sq-none">0 トークン(問い合わせに入っていない)</span>
    </div>

    <div class="sq-verdict" :class="bad ? 'bad' : ''">{{ verdict }}</div>

    <p class="sq-note">
      注入かどうかを「危ない文字が入っているか」で見ると、必ず漏れる。
      値が構文に何トークン寄与したかで見れば、攻撃の形を知らなくても判定できる。
      プレースホルダが効くのは、値を安全な文字に置き換えているからではなく、
      値を問い合わせに入れていないからになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.sq-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.sq-row2 { margin-top: 8px; }
.sq-q {
  margin-top: 14px; padding: 8px 10px; background-color: var(--vp-c-bg-soft);
  font-size: 11.5px; color: var(--vp-c-text-1); overflow-x: auto; white-space: pre;
}
.sq-param { margin-top: 6px; font-size: 10.5px; color: var(--vp-c-text-3); }
.sq-toks { display: flex; align-items: center; gap: 5px; flex-wrap: wrap; margin-top: 12px; }
.sq-label { font-size: 10px; color: var(--vp-c-text-3); margin-right: 4px; }
.sq-tok { font-size: 11px; padding: 2px 6px; background-color: var(--vp-c-default-soft); color: var(--vp-c-text-1); }
.sq-tok.kw { background-color: var(--vp-c-danger-soft); color: var(--vp-c-danger-1); font-weight: 700; }
.sq-none { font-size: 11px; color: var(--vp-c-green-1); font-weight: 600; }
.sq-verdict {
  margin-top: 14px; padding: 8px 12px; background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1); font-size: 12.5px; line-height: 1.6; color: var(--vp-c-text-1);
}
.sq-verdict.bad { border-left-color: var(--vp-c-danger-1); }
.sq-note {
  margin: 14px 0 0; padding-top: 12px; border-top: 1px solid var(--vp-c-divider);
  font-size: 12px; line-height: 1.7; color: var(--vp-c-text-2);
}
.mono { font-family: var(--vp-font-family-mono); }
</style>
