<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/lang(Go) の考え方をブラウザで動かすため、字句→Pratt→評価をJSに移植した
// コンパクト版。整数・真偽・算術・比較・let・if・fn・呼び出し・クロージャ・エラーに対応。

// ---- 字句解析 ----
interface Tok { type: string; lit: string }
const KW: Record<string, string> = { fn: "FN", let: "LET", true: "TRUE", false: "FALSE", if: "IF", else: "ELSE", return: "RETURN" };
function lex(src: string): Tok[] {
  const toks: Tok[] = [];
  let i = 0;
  const isL = (c: string) => /[a-zA-Z_]/.test(c);
  const isD = (c: string) => /[0-9]/.test(c);
  while (i < src.length) {
    const c = src[i];
    if (/\s/.test(c)) { i++; continue; }
    if (c === "=" && src[i + 1] === "=") { toks.push({ type: "EQ", lit: "==" }); i += 2; continue; }
    if (c === "!" && src[i + 1] === "=") { toks.push({ type: "NEQ", lit: "!=" }); i += 2; continue; }
    const single: Record<string, string> = { "=": "ASSIGN", "+": "+", "-": "-", "*": "*", "/": "/", "<": "<", ">": ">", "!": "!", ",": ",", ";": ";", "(": "(", ")": ")", "{": "{", "}": "}" };
    if (single[c]) { toks.push({ type: single[c], lit: c }); i++; continue; }
    if (isL(c)) { let s = ""; while (i < src.length && (isL(src[i]) || isD(src[i]))) s += src[i++]; toks.push({ type: KW[s] ?? "IDENT", lit: s }); continue; }
    if (isD(c)) { let s = ""; while (i < src.length && isD(src[i])) s += src[i++]; toks.push({ type: "INT", lit: s }); continue; }
    toks.push({ type: "ILLEGAL", lit: c }); i++;
  }
  toks.push({ type: "EOF", lit: "" });
  return toks;
}

// ---- Pratt 構文解析 ----
type Node = any;
const PREC: Record<string, number> = { EQ: 2, NEQ: 2, "<": 3, ">": 3, "+": 4, "-": 4, "*": 5, "/": 5, "(": 7 };
class Parser {
  p = 0;
  errs: string[] = [];
  constructor(private t: Tok[]) {}
  cur() { return this.t[this.p]; }
  peek() { return this.t[this.p + 1]; }
  next() { this.p++; }
  expect(ty: string): boolean { if (this.peek().type === ty) { this.next(); return true; } this.errs.push(`${ty} を期待`); return false; }
  program(): Node { const s: Node[] = []; while (this.cur().type !== "EOF") { const st = this.stmt(); if (st) s.push(st); this.next(); } return { k: "program", s }; }
  stmt(): Node {
    if (this.cur().type === "LET") return this.letStmt();
    if (this.cur().type === "RETURN") return this.retStmt();
    return this.exprStmt();
  }
  letStmt(): Node { if (!this.expect("IDENT")) return null; const name = this.cur().lit; if (!this.expect("ASSIGN")) return null; this.next(); const v = this.expr(1); if (this.peek().type === ";") this.next(); return { k: "let", name, v }; }
  retStmt(): Node { this.next(); const v = this.expr(1); if (this.peek().type === ";") this.next(); return { k: "return", v }; }
  exprStmt(): Node { const e = this.expr(1); if (this.peek().type === ";") this.next(); return { k: "expr", e }; }
  block(): Node { const s: Node[] = []; this.next(); while (this.cur().type !== "}" && this.cur().type !== "EOF") { const st = this.stmt(); if (st) s.push(st); this.next(); } return { k: "block", s }; }
  expr(prec: number): Node {
    let left = this.prefix();
    while (this.peek().type !== ";" && prec < (PREC[this.peek().type] ?? 1)) {
      const op = this.peek().type;
      this.next();
      left = op === "(" ? this.call(left) : this.infix(left);
    }
    return left;
  }
  prefix(): Node {
    const c = this.cur();
    switch (c.type) {
      case "INT": return { k: "int", v: parseInt(c.lit, 10) };
      case "IDENT": return { k: "ident", v: c.lit };
      case "TRUE": return { k: "bool", v: true };
      case "FALSE": return { k: "bool", v: false };
      case "!": case "-": { this.next(); return { k: "prefix", op: c.type, r: this.expr(6) }; }
      case "(": { this.next(); const e = this.expr(1); this.expect(")"); return e; }
      case "IF": return this.ifExpr();
      case "FN": return this.fnLit();
      default: this.errs.push(`前置解析できない: ${c.type}`); return null;
    }
  }
  infix(left: Node): Node { const op = this.cur().type; const p = PREC[op] ?? 1; this.next(); return { k: "infix", op, l: left, r: this.expr(p) }; }
  ifExpr(): Node { if (!this.expect("(")) return null; this.next(); const cond = this.expr(1); if (!this.expect(")")) return null; if (!this.expect("{")) return null; const then = this.block(); let els = null; if (this.peek().type === "ELSE") { this.next(); if (!this.expect("{")) return null; els = this.block(); } return { k: "if", cond, then, els }; }
  fnLit(): Node { if (!this.expect("(")) return null; const ps: string[] = []; if (this.peek().type !== ")") { this.next(); ps.push(this.cur().lit); while (this.peek().type === ",") { this.next(); this.next(); ps.push(this.cur().lit); } } this.expect(")"); if (!this.expect("{")) return null; const body = this.block(); return { k: "fn", ps, body }; }
  call(fn: Node): Node { const args: Node[] = []; if (this.peek().type !== ")") { this.next(); args.push(this.expr(1)); while (this.peek().type === ",") { this.next(); this.next(); args.push(this.expr(1)); } } this.expect(")"); return { k: "call", fn, args }; }
}

// ---- 評価 ----
class LangError { constructor(public msg: string) {} }
type Env = { store: Record<string, any>; outer: Env | null };
const newEnv = (outer: Env | null = null): Env => ({ store: {}, outer });
function envGet(e: Env, n: string): any { if (n in e.store) return e.store[n]; if (e.outer) return envGet(e.outer, n); return new LangError(`未定義の変数: ${n}`); }
const truthy = (v: any) => v !== false && v !== null && !(v instanceof LangError);

function evalNode(n: Node, env: Env): any {
  if (!n) return new LangError("構文エラー");
  switch (n.k) {
    case "program": { let r: any = null; for (const s of n.s) { r = evalNode(s, env); if (r instanceof LangError) return r; if (r && r.__ret) return r.v; } return r; }
    case "block": { let r: any = null; for (const s of n.s) { r = evalNode(s, env); if (r instanceof LangError || (r && r.__ret)) return r; } return r; }
    case "expr": return evalNode(n.e, env);
    case "let": { const v = evalNode(n.v, env); if (v instanceof LangError) return v; env.store[n.name] = v; return v; }
    case "return": { const v = evalNode(n.v, env); if (v instanceof LangError) return v; return { __ret: true, v }; }
    case "int": return n.v;
    case "bool": return n.v;
    case "ident": return envGet(env, n.v);
    case "prefix": { const r = evalNode(n.r, env); if (r instanceof LangError) return r; if (n.op === "!") return !truthy(r); if (typeof r !== "number") return new LangError("- は整数のみ"); return -r; }
    case "infix": return evalInfix(n, env);
    case "if": { const c = evalNode(n.cond, env); if (c instanceof LangError) return c; if (truthy(c)) return evalNode(n.then, env); if (n.els) return evalNode(n.els, env); return null; }
    case "fn": return { __fn: true, ps: n.ps, body: n.body, env };
    case "call": return evalCall(n, env);
  }
  return new LangError("未知のノード");
}
function evalInfix(n: Node, env: Env): any {
  const l = evalNode(n.l, env); if (l instanceof LangError) return l;
  const r = evalNode(n.r, env); if (r instanceof LangError) return r;
  if (typeof l === "number" && typeof r === "number") {
    switch (n.op) {
      case "+": return l + r; case "-": return l - r; case "*": return l * r;
      case "/": return r === 0 ? new LangError("ゼロ除算") : Math.trunc(l / r);
      case "<": return l < r; case ">": return l > r; case "EQ": return l === r; case "NEQ": return l !== r;
    }
  }
  if (n.op === "EQ") return l === r;
  if (n.op === "NEQ") return l !== r;
  if (typeof l !== typeof r) return new LangError(`型が違う: ${typeName(l)} ${n.op} ${typeName(r)}`);
  return new LangError(`未対応の演算: ${n.op}`);
}
function evalCall(n: Node, env: Env): any {
  const fn = evalNode(n.fn, env); if (fn instanceof LangError) return fn;
  const args = n.args.map((a: Node) => evalNode(a, env));
  for (const a of args) if (a instanceof LangError) return a;
  if (!fn || !fn.__fn) return new LangError("関数ではない");
  if (args.length !== fn.ps.length) return new LangError(`引数の数が違う: 期待 ${fn.ps.length}, 実際 ${args.length}`);
  const inner = newEnv(fn.env);
  fn.ps.forEach((p: string, i: number) => (inner.store[p] = args[i]));
  const res = evalNode(fn.body, inner);
  return res && res.__ret ? res.v : res;
}
function typeName(v: any): string { return typeof v === "number" ? "INTEGER" : typeof v === "boolean" ? "BOOLEAN" : v && v.__fn ? "FUNCTION" : "NULL"; }
function show(v: any): string {
  if (v instanceof LangError) return "ERROR: " + v.msg;
  if (v === null || v === undefined) return "null";
  if (v && v.__fn) return `fn(${v.ps.join(", ")}) { ... }`;
  if (typeof v === "boolean") return v ? "true" : "false";
  return String(v);
}

function run(src: string): { toks: Tok[]; result: string; isErr: boolean } {
  const toks = lex(src).filter((t) => t.type !== "EOF");
  const p = new Parser(lex(src));
  const prog = p.program();
  if (p.errs.length > 0) return { toks, result: "構文エラー: " + p.errs[0], isErr: true };
  const v = evalNode(prog, newEnv());
  return { toks, result: show(v), isErr: v instanceof LangError };
}

const PRESETS = [
  { name: "算術", src: "2 + 3 * 4" },
  { name: "let & if", src: "let x = 10;\nif (x > 5) { x * 2 } else { 0 }" },
  { name: "クロージャ", src: "let adder = fn(x) {\n  fn(y) { x + y }\n};\nlet add5 = adder(5);\nadd5(3)" },
];

const state = reactive({ idx: 0, src: PRESETS[0].src });
function selectPreset(i: number) { state.idx = i; state.src = PRESETS[i].src; }

const out = computed(() => run(state.src));
const badge = computed(() => (out.value.isErr ? "エラー" : "= " + out.value.result));
const badgeTone = computed<"ok" | "ng">(() => (out.value.isErr ? "ng" : "ok"));
function tokClass(ty: string): string {
  if (ty === "INT") return "int";
  if (ty === "IDENT") return "ident";
  if (["LET", "FN", "IF", "ELSE", "RETURN", "TRUE", "FALSE"].includes(ty)) return "kw";
  return "sym";
}
</script>

<template>
  <DemoShell title="lang(字句 → 構文 → 評価)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <div class="lg-seg" role="group" aria-label="例">
        <button v-for="(p, i) in PRESETS" :key="i" class="lg-seg-btn" :class="{ on: state.idx === i }" @click="selectPreset(i)">{{ p.name }}</button>
      </div>
    </div>

    <div class="lg-label">ソース(編集できます)</div>
    <textarea v-model="state.src" class="lg-src" spellcheck="false" rows="4"></textarea>

    <div class="lg-label">① 字句解析: トークン列</div>
    <div class="lg-toks">
      <span v-for="(t, i) in out.toks" :key="i" class="lg-tok" :class="tokClass(t.type)">{{ t.lit }}</span>
    </div>

    <div class="lg-label">③ 評価: 結果</div>
    <div class="lg-result" :class="{ err: out.isErr }">{{ out.result }}</div>

    <div class="lg-legend">
      <span>ソース文字列 → トークン列(字句) → AST(構文) → 値(評価)。3段の一方通行</span>
      <span>クロージャ例: 返された関数が外側の x を覚えている(定義時の環境を抱える)</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.lg-seg {
  display: inline-flex;
  border: 1px solid var(--vp-c-divider);
  overflow: hidden;
}
.lg-seg-btn {
  padding: 4px 12px;
  font-size: 12px;
  color: var(--vp-c-text-2);
  background-color: var(--vp-c-bg);
  border-right: 1px solid var(--vp-c-divider);
}
.lg-seg-btn:last-child {
  border-right: none;
}
.lg-seg-btn.on {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.lg-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin: 14px 0 5px;
}
.lg-src {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-alt);
  color: var(--vp-c-text-1);
  padding: 10px 12px;
  font-size: 13px;
  font-family: var(--vp-font-family-mono);
  line-height: 1.6;
  resize: vertical;
}
.lg-toks {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.lg-tok {
  padding: 2px 8px;
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-2);
}
.lg-tok.int {
  color: var(--vp-c-green-1);
  border-color: var(--vp-c-green-2);
}
.lg-tok.kw {
  color: var(--vp-c-purple-1);
  border-color: var(--vp-c-purple-2);
  font-weight: 600;
}
.lg-tok.ident {
  color: var(--vp-c-brand-1);
  border-color: var(--vp-c-brand-2);
}
.lg-result {
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-green-2);
  background-color: var(--vp-c-bg);
  padding: 10px 14px;
  font-size: 16px;
  font-weight: 600;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-green-1);
}
.lg-result.err {
  border-left-color: var(--vp-c-red-2);
  color: var(--vp-c-red-1);
  font-size: 13px;
}
.lg-legend {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 14px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
