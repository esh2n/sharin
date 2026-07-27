<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/typeinfer(Go)を移植。例の式から型(または型エラー)を実際に推論する。

type Ty = { k: "con"; name: string } | { k: "var"; name: string } | { k: "fun"; from: Ty; to: Ty };
type Subst = Record<string, Ty>;
const tInt: Ty = { k: "con", name: "Int" };
const tBool: Ty = { k: "con", name: "Bool" };

function tstr(t: Ty): string {
  if (t.k === "con" || t.k === "var") return t.name;
  return `(${tstr(t.from)} → ${tstr(t.to)})`;
}
function applyT(s: Subst, t: Ty): Ty {
  if (t.k === "var") return s[t.name] ? applyT(s, s[t.name]) : t;
  if (t.k === "fun") return { k: "fun", from: applyT(s, t.from), to: applyT(s, t.to) };
  return t;
}
function compose(s1: Subst, s2: Subst): Subst {
  const out: Subst = {};
  for (const k in s2) out[k] = applyT(s1, s2[k]);
  for (const k in s1) out[k] = s1[k];
  return out;
}
function occurs(n: string, t: Ty): boolean {
  if (t.k === "var") return t.name === n;
  if (t.k === "fun") return occurs(n, t.from) || occurs(n, t.to);
  return false;
}
function bind(n: string, t: Ty): Subst {
  if (t.k === "var" && t.name === n) return {};
  if (occurs(n, t)) throw new Error(`occurs check: ${n} が ${tstr(t)} に現れる(無限型)`);
  return { [n]: t };
}
function unify(a: Ty, b: Ty): Subst {
  if (a.k === "var") return bind(a.name, b);
  if (b.k === "var") return bind(b.name, a);
  if (a.k === "con" && b.k === "con") {
    if (a.name === b.name) return {};
    throw new Error(`型不一致: ${a.name} と ${b.name}`);
  }
  if (a.k === "fun" && b.k === "fun") {
    const s1 = unify(a.from, b.from);
    const s2 = unify(applyT(s1, a.to), applyT(s1, b.to));
    return compose(s2, s1);
  }
  throw new Error(`型不一致: ${tstr(a)} と ${tstr(b)}`);
}

// Expr
type Expr =
  | { k: "int" }
  | { k: "bool" }
  | { k: "var"; name: string }
  | { k: "lam"; param: string; body: Expr }
  | { k: "app"; fn: Expr; arg: Expr }
  | { k: "let"; name: string; value: Expr; body: Expr }
  | { k: "if"; cond: Expr; then: Expr; else: Expr };
type Scheme = { vars: string[]; type: Ty };
type Env = Record<string, Scheme>;

let counter = 0;
function fresh(): Ty {
  return { k: "var", name: `t${++counter}` };
}
function freeVars(t: Ty, out: Set<string>) {
  if (t.k === "var") out.add(t.name);
  else if (t.k === "fun") {
    freeVars(t.from, out);
    freeVars(t.to, out);
  }
}
function instantiate(sc: Scheme): Ty {
  const sub: Subst = {};
  for (const v of sc.vars) sub[v] = fresh();
  return applyT(sub, sc.type);
}
function generalize(env: Env, t: Ty): Scheme {
  const tvars = new Set<string>();
  freeVars(t, tvars);
  const envVars = new Set<string>();
  for (const k in env) {
    const ft = new Set<string>();
    freeVars(env[k].type, ft);
    for (const v of ft) if (!env[k].vars.includes(v)) envVars.add(v);
  }
  return { vars: [...tvars].filter((v) => !envVars.has(v)), type: t };
}
function applyEnv(s: Subst, env: Env): Env {
  const out: Env = {};
  for (const k in env) out[k] = { vars: env[k].vars, type: applyT(s, env[k].type) };
  return out;
}
function infer(env: Env, e: Expr): [Subst, Ty] {
  switch (e.k) {
    case "int":
      return [{}, tInt];
    case "bool":
      return [{}, tBool];
    case "var": {
      const sc = env[e.name];
      if (!sc) throw new Error(`未束縛の変数: ${e.name}`);
      return [{}, instantiate(sc)];
    }
    case "lam": {
      const tv = fresh();
      const [s1, tbody] = infer({ ...env, [e.param]: { vars: [], type: tv } }, e.body);
      return [s1, { k: "fun", from: applyT(s1, tv), to: tbody }];
    }
    case "app": {
      const [s1, tfn] = infer(env, e.fn);
      const [s2, targ] = infer(applyEnv(s1, env), e.arg);
      const tv = fresh();
      const s3 = unify(applyT(s2, tfn), { k: "fun", from: targ, to: tv });
      return [compose(s3, compose(s2, s1)), applyT(s3, tv)];
    }
    case "let": {
      const [s1, tval] = infer(env, e.value);
      const env1 = applyEnv(s1, env);
      const [s2, tbody] = infer({ ...env1, [e.name]: generalize(env1, tval) }, e.body);
      return [compose(s2, s1), tbody];
    }
    case "if": {
      const [sc, tc] = infer(env, e.cond);
      const s1 = unify(tc, tBool);
      let s = compose(s1, sc);
      const [st, tt] = infer(applyEnv(s, env), e.then);
      s = compose(st, s);
      const [se, te] = infer(applyEnv(s, env), e.else);
      s = compose(se, s);
      const s2 = unify(applyT(s, tt), applyT(s, te));
      return [compose(s2, s), applyT(s2, applyT(s, tt))];
    }
  }
}

// --- 例 ---
const L = (param: string, body: Expr): Expr => ({ k: "lam", param, body });
const V = (name: string): Expr => ({ k: "var", name });
const A = (fn: Expr, arg: Expr): Expr => ({ k: "app", fn, arg });
const I = (): Expr => ({ k: "int" });
const B = (): Expr => ({ k: "bool" });
const IF = (cond: Expr, then: Expr, els: Expr): Expr => ({ k: "if", cond, then, else: els });
const LET = (name: string, value: Expr, body: Expr): Expr => ({ k: "let", name, value, body });

const examples = [
  { label: "λx.x", src: "λx.x", expr: L("x", V("x")), note: "恒等関数。x の型は不明なので型変数 t1 を置き、そのまま返す。どんな型にも使える多相関数" },
  { label: "(λx.x) 42", src: "(λx.x) 42", expr: A(L("x", V("x")), I()), note: "適用で 42:Int が引数になり、t1=Int と単一化。結果は Int" },
  {
    label: "let 多相",
    src: "let id = λx.x in if (id true) then (id 1) else 2",
    expr: LET("id", L("x", V("x")), IF(A(V("id"), B()), A(V("id"), I()), I())),
    note: "id を一般化し、id true では Bool→Bool、id 1 では Int→Int と別々に具体化。互いに干渉せず Int になる",
  },
  { label: "λx.x x(エラー)", src: "λx.x x", expr: L("x", A(V("x"), V("x"))), note: "x が x を受ける関数 = 無限型を要求。occurs check で弾かれる" },
  { label: "if 1 …(エラー)", src: "if 1 then 2 else 3", expr: IF(I(), I(), I()), note: "条件が Int。Bool と単一化できず型エラー" },
] as const;

const pick = ref(0);
const result = computed(() => {
  counter = 0;
  const ex = examples[pick.value];
  try {
    const [s, t] = infer({}, ex.expr);
    return { ok: true, type: tstr(applyT(s, t)), error: "" };
  } catch (err) {
    return { ok: false, type: "", error: err instanceof Error ? err.message : String(err) };
  }
});

const badge = computed(() => (result.value.ok ? `型: ${result.value.type}` : "型エラー"));
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (result.value.ok ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="型推論(Hindley–Milner)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="(ex, i) in examples"
          :key="i"
          class="sd-seg-opt"
          :class="{ on: pick === i }"
          @click="pick = i"
          >{{ ex.label }}</span
        >
      </span>
    </div>

    <div class="ti-panel">
      <div class="ti-row">
        <span class="ti-k mono">式</span>
        <span class="ti-src mono">{{ examples[pick].src }}</span>
      </div>
      <div class="ti-arrow">型推論 ↓</div>
      <div class="ti-row">
        <span class="ti-k mono">推論結果</span>
        <span v-if="result.ok" class="ti-type mono">{{ result.type }}</span>
        <span v-else class="ti-err mono">型エラー</span>
      </div>
      <div class="ti-detail" :class="result.ok ? 'ok' : 'bad'">
        <template v-if="result.ok">{{ examples[pick].note }}</template>
        <template v-else>{{ result.error }} — {{ examples[pick].note }}</template>
      </div>
    </div>

    <p class="ti-legend">
      Go 実装をそのまま移植して推論している。注釈は一切書いていない。未知の型を型変数 t1, t2… で置き、
      適用や条件から集めた「この型とこの型は等しい」を単一化(unify)で解いて型変数を埋める。let では残った
      型変数を一般化して、使うたび新しい変数へ具体化する(let 多相)。矛盾する制約(無限型・型不一致)は
      その場で型エラーになる。式の構造だけから、最も一般的な型が一意に定まる。
    </p>
  </DemoShell>
</template>

<style scoped>
.ti-panel {
  margin-top: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.ti-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
  padding: 4px 0;
}
.ti-k {
  width: 72px;
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  text-align: right;
}
.ti-src {
  font-size: 14px;
  color: var(--vp-c-text-1);
}
.ti-type {
  font-size: 16px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
}
.ti-err {
  font-size: 16px;
  font-weight: 700;
  color: var(--vp-c-danger-1);
}
.ti-arrow {
  margin: 8px 0;
  padding-left: 84px;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.ti-detail {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid;
  border-radius: 0;
  font-size: 12.5px;
  line-height: 1.6;
}
.ti-detail.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-text-2);
  background-color: var(--vp-c-bg);
}
.ti-detail.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ti-legend {
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
