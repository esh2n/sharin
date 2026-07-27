<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// frontend/bundler(TS)を移植。依存グラフから到達可能・トポロジカル順序・
// tree-shaking、そして循環依存の検出を可視化する。

interface Import {
  from: string;
  names: string[];
}
interface Module {
  id: string;
  imports: Import[];
  exports: string[];
}
type Registry = Record<string, Module>;

function collectReachable(entry: string, reg: Registry): Set<string> {
  const r = new Set<string>();
  const stack = [entry];
  while (stack.length) {
    const id = stack.pop()!;
    if (r.has(id)) continue;
    r.add(id);
    for (const imp of reg[id].imports) stack.push(imp.from);
  }
  return r;
}
function topoOrder(entry: string, reg: Registry): { order: string[]; cycle: string[] | null } {
  const order: string[] = [];
  const state = new Map<string, "gray" | "black">();
  const path: string[] = [];
  let cycle: string[] | null = null;
  function visit(id: string): boolean {
    const s = state.get(id);
    if (s === "black") return true;
    if (s === "gray") {
      cycle = [...path.slice(path.indexOf(id)), id];
      return false;
    }
    state.set(id, "gray");
    path.push(id);
    for (const imp of reg[id].imports) if (!visit(imp.from)) return false;
    path.pop();
    state.set(id, "black");
    order.push(id);
    return true;
  }
  visit(entry);
  return { order, cycle };
}
function treeShake(entry: string, reg: Registry): Map<string, string[]> {
  const reach = collectReachable(entry, reg);
  const used = new Map<string, Set<string>>();
  for (const id of reach)
    for (const imp of reg[id].imports) {
      if (!reach.has(imp.from)) continue;
      const set = used.get(imp.from) ?? new Set<string>();
      for (const n of imp.names) set.add(n);
      used.set(imp.from, set);
    }
  const kept = new Map<string, string[]>();
  for (const id of reach) {
    if (id === entry) {
      kept.set(id, [...reg[id].exports]);
      continue;
    }
    const u = used.get(id) ?? new Set<string>();
    kept.set(id, reg[id].exports.filter((e) => u.has(e)));
  }
  return kept;
}

const normalReg: Registry = {
  entry: {
    id: "entry",
    imports: [
      { from: "math", names: ["add"] },
      { from: "util", names: ["log"] },
    ],
    exports: [],
  },
  math: { id: "math", imports: [], exports: ["add", "sub", "mul"] },
  util: { id: "util", imports: [{ from: "math", names: ["sub"] }], exports: ["log"] },
  orphan: { id: "orphan", imports: [], exports: ["x"] },
};
const cyclicReg: Registry = {
  a: { id: "a", imports: [{ from: "b", names: [] }], exports: [] },
  b: { id: "b", imports: [{ from: "c", names: [] }], exports: [] },
  c: { id: "c", imports: [{ from: "a", names: [] }], exports: [] },
};

const modes = [
  { key: "normal", label: "正常な依存グラフ" },
  { key: "cycle", label: "循環依存" },
] as const;
const mode = ref<"normal" | "cycle">("normal");

const reachable = computed(() => (mode.value === "normal" ? collectReachable("entry", normalReg) : new Set<string>()));
const topo = computed(() => (mode.value === "normal" ? topoOrder("entry", normalReg) : topoOrder("a", cyclicReg)));
const kept = computed(() => (mode.value === "normal" ? treeShake("entry", normalReg) : new Map<string, string[]>()));

const allModules = computed(() => Object.values(mode.value === "normal" ? normalReg : cyclicReg));

function isReachable(id: string): boolean {
  return reachable.value.has(id);
}
function keptExports(id: string): string[] {
  return kept.value.get(id) ?? [];
}
function droppedExports(m: Module): string[] {
  const k = keptExports(m.id);
  return m.exports.filter((e) => !k.includes(e));
}
// import 行のラベル（テンプレート内に波括弧を書くと Vue パーサが誤解するので script 側で組む）。
function importLabel(m: Module): string {
  return "import " + m.imports.map((i) => `[${i.names.join(",")}] from ${i.from}`).join("  ");
}

const badge = computed(() =>
  mode.value === "cycle" ? (topo.value.cycle ? "循環を検出" : "循環なし") : "依存解決",
);
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (mode.value === "cycle" ? "ng" : "neutral"));
</script>

<template>
  <DemoShell title="バンドラ(依存解決)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="m in modes"
          :key="m.key"
          class="sd-seg-opt"
          :class="{ on: mode === m.key }"
          @click="mode = m.key"
          >{{ m.label }}</span
        >
      </span>
    </div>

    <!-- 正常 -->
    <div v-if="mode === 'normal'" class="bd-panel">
      <div class="bd-h">モジュール(到達できないものは灰色)</div>
      <div class="bd-mods">
        <div v-for="m in allModules" :key="m.id" class="bd-mod" :class="{ excluded: !isReachable(m.id) }">
          <div class="bd-mod-id mono">{{ m.id }}</div>
          <div class="bd-mod-imp mono" v-if="m.imports.length">{{ importLabel(m) }}</div>
          <div class="bd-mod-exp mono" v-if="m.exports.length">
            export
            <span v-for="e in keptExports(m.id)" :key="e" class="bd-exp keep">{{ e }}</span>
            <span v-for="e in droppedExports(m)" :key="e" class="bd-exp drop">{{ e }}</span>
          </div>
          <div v-if="!isReachable(m.id)" class="bd-tag mono">到達不能 → 除外</div>
        </div>
      </div>
      <div class="bd-order">
        <span class="bd-order-label mono">連結順(依存が先):</span>
        <span class="bd-order-seq mono">{{ topo.order.join(" → ") }}</span>
      </div>
    </div>

    <!-- 循環 -->
    <div v-else class="bd-panel">
      <div class="bd-h">循環したグラフ(a → b → c → a)</div>
      <div class="bd-cycle-graph mono">
        <span v-for="(id, i) in topo.cycle" :key="i" class="bd-cycle-node" :class="{ head: i === 0 || i === (topo.cycle?.length ?? 0) - 1 }">
          {{ id }}<span v-if="i < (topo.cycle?.length ?? 0) - 1" class="bd-cycle-arrow"> → </span>
        </span>
      </div>
      <div class="bd-cycle-msg" v-if="topo.cycle">
        DFS が訪問中(gray)のノード <b>{{ topo.cycle[0] }}</b> に戻った → 循環依存。初期化順序が定まらず実行時エラーの温床になるので、バンドル時に検出して警告する
      </div>
    </div>

    <p class="bd-legend">
      entry から import を辿って到達できるモジュールだけを集める(孤立した orphan は除外)。依存を先に、
      使う側を後に並べる(トポロジカル順序)。どこからも import されない export は死んだコードとして落とす
      (tree-shaking。math の mul が消える)。import が輪を作れば循環依存として検出する。すべて静的な
      import/export が読めることが前提で、これが ES Modules の静的構文が重要な理由だ。
    </p>
  </DemoShell>
</template>

<style scoped>
.bd-panel {
  margin-top: 16px;
}
.bd-h {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.bd-mods {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.bd-mod {
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  padding: 8px 10px;
  background-color: var(--vp-c-bg-soft);
}
.bd-mod.excluded {
  border-left-color: var(--vp-c-divider);
  opacity: 0.5;
}
.bd-mod-id {
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.bd-mod-imp,
.bd-mod-exp {
  font-size: 11px;
  color: var(--vp-c-text-2);
  margin-top: 3px;
}
.bd-exp {
  display: inline-block;
  padding: 1px 6px;
  margin-left: 4px;
  border-radius: 0;
  font-size: 11px;
}
.bd-exp.keep {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.bd-exp.drop {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
  text-decoration: line-through;
}
.bd-tag {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
  margin-top: 3px;
}
.bd-order {
  margin-top: 12px;
  padding: 8px 10px;
  border-left: 3px solid var(--vp-c-warning-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.bd-order-label {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.bd-order-seq {
  font-size: 13px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  margin-left: 6px;
}
.bd-cycle-graph {
  font-size: 16px;
  padding: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  text-align: center;
}
.bd-cycle-node {
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.bd-cycle-node.head {
  color: var(--vp-c-danger-1);
}
.bd-cycle-arrow {
  color: var(--vp-c-text-3);
}
.bd-cycle-msg {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-danger-1);
  border-radius: 0;
  background-color: var(--vp-c-danger-soft);
  font-size: 12.5px;
  color: var(--vp-c-danger-1);
  line-height: 1.6;
}
.bd-legend {
  margin: 16px 0 0;
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
