<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 db/minisql の lexer→parser→engine を JS で再現。
// 実際のストレージ(B-Tree)は Map で代用し、SQL の3段パイプラインを見せることに絞る。

type Tok = { kind: string; text: string };

const KEYWORDS = new Set(["INSERT", "INTO", "VALUES", "SELECT", "FROM", "WHERE"]);

function lex(s: string): Tok[] {
  const toks: Tok[] = [];
  let i = 0;
  while (i < s.length) {
    const c = s[i];
    if (/\s/.test(c)) i++;
    else if ("*,()=".includes(c)) {
      toks.push({ kind: c, text: c });
      i++;
    } else if (/[0-9]/.test(c)) {
      let j = i;
      while (j < s.length && /[0-9]/.test(s[j])) j++;
      toks.push({ kind: "number", text: s.slice(i, j) });
      i = j;
    } else if (/[A-Za-z_]/.test(c)) {
      let j = i;
      while (j < s.length && /[A-Za-z0-9_]/.test(s[j])) j++;
      const w = s.slice(i, j);
      const up = w.toUpperCase();
      toks.push(KEYWORDS.has(up) ? { kind: "keyword", text: up } : { kind: "ident", text: w });
      i = j;
    } else throw new Error(`予期しない文字 "${c}"`);
  }
  return toks;
}

interface Ast {
  type: "insert" | "select";
  table: string;
  key?: number;
  value?: number;
  where?: number;
}

function parse(toks: Tok[]): Ast {
  let p = 0;
  const peek = () => toks[p] ?? { kind: "eof", text: "" };
  const eat = (kind: string, what: string) => {
    if (peek().kind !== kind) throw new Error(`${what} が必要です(got "${peek().text || "文末"}")`);
    return toks[p++];
  };
  const kw = (k: string) => {
    if (peek().kind !== "keyword" || peek().text !== k) throw new Error(`${k} が必要です`);
    p++;
  };
  const num = () => Number(eat("number", "数値").text);

  const head = peek();
  if (head.kind === "keyword" && head.text === "INSERT") {
    p++;
    kw("INTO");
    const table = eat("ident", "テーブル名").text;
    kw("VALUES");
    eat("(", "'('");
    const key = num();
    eat(",", "','");
    const value = num();
    eat(")", "')'");
    return { type: "insert", table, key, value };
  }
  if (head.kind === "keyword" && head.text === "SELECT") {
    p++;
    eat("*", "'*'");
    kw("FROM");
    const table = eat("ident", "テーブル名").text;
    const ast: Ast = { type: "select", table };
    if (peek().kind === "keyword" && peek().text === "WHERE") {
      p++;
      const col = eat("ident", "列名").text;
      if (col !== "id") throw new Error("WHERE は id のみ対応");
      eat("=", "'='");
      ast.where = num();
    }
    return ast;
  }
  throw new Error(`未対応の文 "${head.text}"`);
}

// 最初から2行入れておく。空だと engine の段に何も出ず、道の選び方が見えない。
const seed = (): Map<number, number> => new Map([[1, 100], [2, 200]]);

const store = ref<Map<number, number>>(seed());
const input = ref("SELECT * FROM users WHERE id = 1");
const toks = ref<Tok[]>([]);
const ast = ref<Ast | null>(null);
const result = ref<{
  ok: boolean;
  text: string;
  rows?: [number, number][];
  // access は engine が選んだ道。Go 版 Plan.Access と同じ判定。
  access?: "index" | "scan";
} | null>(null);

const PRESETS = [
  "INSERT INTO users VALUES (1, 100)",
  "INSERT INTO users VALUES (2, 200)",
  "SELECT * FROM users WHERE id = 1",
  "SELECT * FROM users",
];

function run() {
  try {
    toks.value = lex(input.value);
    ast.value = parse(toks.value);
    const a = ast.value;
    if (a.type === "insert") {
      const m = new Map(store.value);
      m.set(a.key!, a.value!);
      store.value = m;
      result.value = { ok: true, text: `1行 INSERT した (${a.key} → ${a.value})` };
    } else if (a.where !== undefined) {
      const v = store.value.get(a.where);
      result.value = v === undefined
        ? { ok: true, text: "0行(該当なし)", rows: [], access: "index" }
        : { ok: true, text: "1行", rows: [[a.where, v]], access: "index" };
    } else {
      const rows = [...store.value.entries()].sort((x, y) => x[0] - y[0]) as [number, number][];
      result.value = { ok: true, text: `${rows.length}行(昇順)`, rows, access: "scan" };
    }
  } catch (e) {
    toks.value = [];
    ast.value = null;
    result.value = { ok: false, text: (e as Error).message };
  }
}

function usePreset(s: string) {
  input.value = s;
  run();
}

function reset() {
  store.value = new Map();
  toks.value = [];
  ast.value = null;
  result.value = null;
}

run();

const astText = computed(() => {
  const a = ast.value;
  if (!a) return "";
  if (a.type === "insert") return `Insert{ table: "${a.table}", key: ${a.key}, value: ${a.value} }`;
  return `Select{ table: "${a.table}", where: ${a.where === undefined ? "なし(全件)" : `id=${a.where}`} }`;
});
</script>

<template>
  <DemoShell title="SQL の3段パイプライン" :badge="`${store.size} 行`" badge-tone="neutral">
    <div class="ms-presets">
      <button v-for="s in PRESETS" :key="s" class="sd-btn ms-preset" type="button" @click="usePreset(s)">
        {{ s }}
      </button>
    </div>

    <div class="sd-controls ms-input-row">
      <input v-model="input" class="ms-input" spellcheck="false" @keyup.enter="run" />
      <button class="sd-btn sd-btn--primary" type="button" @click="run">実行</button>
      <button class="sd-btn" type="button" @click="reset">テーブルを空に</button>
    </div>

    <div class="ms-stages">
      <div class="ms-stage">
        <p class="ms-stage-head">1. lexer → トークン</p>
        <div v-if="toks.length" class="ms-toks">
          <span v-for="(t, i) in toks" :key="i" class="ms-tok" :class="t.kind">{{ t.text }}</span>
        </div>
        <p v-else class="ms-none">—</p>
      </div>
      <div class="ms-stage">
        <p class="ms-stage-head">2. parser → AST</p>
        <code v-if="astText" class="ms-ast">{{ astText }}</code>
        <p v-else class="ms-none">—</p>
      </div>
      <div class="ms-stage">
        <p class="ms-stage-head">3. engine → 実行</p>
        <p v-if="result" class="ms-result" :class="{ err: !result.ok }">{{ result.text }}</p>
        <p v-if="result?.access" class="ms-access">
          <span class="ms-access-tag" :class="result.access">{{ result.access }}</span>
          {{ result.access === "index" ? "WHERE があるので B-Tree を1点引き。読むのは根から葉までの高さぶん"
                                       : "WHERE が無いので全走査。読むページは行数とともに増える" }}
        </p>
        <table v-if="result?.rows?.length" class="ms-rows">
          <thead><tr><th>id</th><th>value</th></tr></thead>
          <tbody>
            <tr v-for="r in result.rows" :key="r[0]"><td>{{ r[0] }}</td><td>{{ r[1] }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </DemoShell>
</template>

<style scoped>
.ms-presets {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.ms-preset {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
}
.ms-input-row {
  gap: 8px;
}
.ms-input {
  flex: 1;
  min-width: 200px;
  padding: 7px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.ms-stages {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin-top: 14px;
}
@media (max-width: 700px) {
  .ms-stages {
    grid-template-columns: 1fr;
  }
}
.ms-stage {
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
  padding: 10px 12px;
  min-height: 96px;
}
.ms-stage-head {
  margin: 0 0 8px;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.ms-toks {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.ms-tok {
  padding: 2px 7px;
  border-radius: 5px;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  background-color: var(--vp-c-default-soft);
}
.ms-tok.keyword {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.ms-tok.number {
  color: var(--vp-c-green-1);
}
.ms-tok.ident {
  color: var(--vp-c-text-1);
}
.ms-ast {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  color: var(--vp-c-text-1);
  white-space: pre-wrap;
  word-break: break-word;
}
.ms-none {
  margin: 0;
  color: var(--vp-c-text-3);
}
.ms-result {
  margin: 0 0 8px;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.ms-result.err {
  color: var(--vp-c-danger-1);
  font-weight: 600;
}
.ms-access {
  margin: 6px 0 0;
  font-size: 11px;
  line-height: 1.6;
  color: var(--vp-c-text-3);
}
.ms-access-tag {
  display: inline-block;
  margin-right: 6px;
  padding: 0 6px;
  font-family: var(--vp-font-family-mono);
  font-size: 10.5px;
  font-weight: 700;
}
.ms-access-tag.index {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
}
.ms-access-tag.scan {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.ms-rows {
  font-size: 13px;
  margin: 0;
}
.ms-rows th,
.ms-rows td {
  padding: 2px 12px 2px 0;
  text-align: left;
}
.ms-rows th {
  color: var(--vp-c-text-2);
  border-bottom: 1px solid var(--vp-c-divider);
}
</style>
