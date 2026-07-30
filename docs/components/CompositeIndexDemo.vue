<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// db/composite(Go)を移植。1万行、a は 100 種類、b は 10 種類。
// 索引は 1ページ 500 件、本体は 1ページ 100 行。乱数も時刻も使わない。
const N = 10000;
const A_VALS = 100;
const B_VALS = 10;
const ENTRIES_PER_PAGE = 500;

type Op = "=" | ">=";
interface Cond {
  col: "a" | "b";
  op: Op;
  val: number;
}

// Go の NewRows と同じ並べ方: a = i % aVals, b = (i / aVals) % bVals
interface Entry {
  a: number;
  b: number;
  id: number;
}
const rows: Entry[] = Array.from({ length: N }, (_, i) => ({
  a: i % A_VALS,
  b: Math.floor(i / A_VALS) % B_VALS,
  id: i,
}));

// 列順ごとに一度だけ整列しておく(Go の Build 相当)。
function build(cols: ("a" | "b")[]): Entry[] {
  return [...rows].sort((x, y) => {
    for (const c of cols) {
      if (x[c] !== y[c]) return x[c] - y[c];
    }
    return x.id - y.id;
  });
}
const INDEXES: Record<string, Entry[]> = {
  ab: build(["a", "b"]),
  ba: build(["b", "a"]),
};

const height = (() => {
  let h = 1;
  let m = N;
  while (m > ENTRIES_PER_PAGE) {
    m = Math.ceil(m / ENTRIES_PER_PAGE);
    h++;
  }
  return h;
})();

// Go の Usable: 左端から連続する等値まで。範囲が1つ来たらそこで打ち切り。
function usable(cols: ("a" | "b")[], conds: Cond[]): number {
  let n = 0;
  for (const col of cols) {
    const c = conds.find((x) => x.col === col);
    if (!c) break;
    n++;
    if (c.op !== "=") break;
  }
  return n;
}

interface Plan {
  usable: number;
  scanned: number;
  indexReads: number;
  rows: number;
}

// Go の Lookup: 使えた列で範囲を決め、残りはその中をふるいにかける。
function lookup(order: "ab" | "ba", conds: Cond[]): Plan {
  const cols: ("a" | "b")[] = order === "ab" ? ["a", "b"] : ["b", "a"];
  const es = INDEXES[order];
  const u = usable(cols, conds);
  const ok = (e: Entry, upto: number) => {
    for (let i = 0; i < upto; i++) {
      const c = conds.find((x) => x.col === cols[i]);
      if (!c) return false;
      if (c.op === "=" && e[cols[i]] !== c.val) return false;
      if (c.op === ">=" && e[cols[i]] < c.val) return false;
    }
    return true;
  };

  let lo = 0;
  let hi = es.length;
  if (u > 0) {
    // 下限を二分探索してから、条件を外れるまで進む。
    let l = 0;
    let r = es.length;
    while (l < r) {
      const m = (l + r) >> 1;
      let below = false;
      for (let i = 0; i < u; i++) {
        const c = conds.find((x) => x.col === cols[i])!;
        if (es[m][cols[i]] !== c.val) {
          below = es[m][cols[i]] < c.val;
          break;
        }
      }
      if (below) l = m + 1;
      else r = m;
    }
    lo = l;
    hi = lo;
    while (hi < es.length && ok(es[hi], u)) hi++;
  }

  const scanned = hi - lo;
  let reads = height;
  if (scanned > 1) reads += Math.floor((scanned - 1) / ENTRIES_PER_PAGE);

  let matched = 0;
  for (let i = lo; i < hi; i++) {
    const e = es[i];
    let good = true;
    for (const c of conds) {
      if (c.op === "=" && e[c.col] !== c.val) good = false;
      if (c.op === ">=" && e[c.col] < c.val) good = false;
    }
    if (good) matched++;
  }
  return { usable: u, scanned, indexReads: reads, rows: matched };
}

const QUERIES: { label: string; conds: Cond[] }[] = [
  { label: "a = 5 AND b = 3", conds: [{ col: "a", op: "=", val: 5 }, { col: "b", op: "=", val: 3 }] },
  { label: "a = 5", conds: [{ col: "a", op: "=", val: 5 }] },
  { label: "b = 3", conds: [{ col: "b", op: "=", val: 3 }] },
  { label: "a = 5 AND b >= 8", conds: [{ col: "a", op: "=", val: 5 }, { col: "b", op: ">=", val: 8 }] },
  { label: "a >= 95 AND b = 3", conds: [{ col: "a", op: ">=", val: 95 }, { col: "b", op: "=", val: 3 }] },
];

const view = ref<"read" | "write">("read");
// 既定は b だけの条件。(a, b) では左端を欠いて索引が働かないことが一目で出る。
const qi = ref(2);

const query = computed(() => QUERIES[qi.value]);
const ab = computed(() => lookup("ab", query.value.conds));
const ba = computed(() => lookup("ba", query.value.conds));
const maxScanned = computed(() => Math.max(ab.value.scanned, ba.value.scanned, 1));

const plans = computed(() => [
  { order: "(a, b)", plan: ab.value },
  { order: "(b, a)", plan: ba.value },
]);

const verdict = computed(() => {
  const a = ab.value;
  const b = ba.value;
  const n = (v: number) => v.toLocaleString();
  if (a.scanned === b.scanned) {
    return `どちらの列順でも ${n(a.scanned)} 件。この条件では順を変えても差が出ない`;
  }
  const win = a.scanned < b.scanned ? "(a, b)" : "(b, a)";
  const lose = a.scanned < b.scanned ? "(b, a)" : "(a, b)";
  const hi = Math.max(a.scanned, b.scanned);
  const lo = Math.min(a.scanned, b.scanned);
  return `${win} なら ${n(lo)} 件で済むが、${lose} では ${n(hi)} 件なめる。当たりはどちらも ${n(a.rows)} 件で同じ`;
});

const INDEX_COUNTS = [0, 1, 2, 4];
const writeCost = (k: number) => 1 + k;
const maxWrite = writeCost(INDEX_COUNTS[INDEX_COUNTS.length - 1]);

const badge = computed(() =>
  view.value === "read" ? `${N.toLocaleString()} 行 / 索引の高さ ${height}` : `1万行を入れたときのページ数`,
);
</script>

<template>
  <DemoShell title="列の順で、なめる件数が変わる" :badge="badge">
    <div class="ci-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: view === 'read' }" @click="view = 'read'">引く</span>
        <span class="sd-seg-opt" :class="{ on: view === 'write' }" @click="view = 'write'">書き込み</span>
      </span>
    </div>

    <template v-if="view === 'read'">
      <div class="ci-actions ci-queries">
        <span class="ci-label">条件</span>
        <span class="sd-seg">
          <span
            v-for="(q, i) in QUERIES"
            :key="q.label"
            class="sd-seg-opt mono"
            :class="{ on: qi === i }"
            @click="qi = i"
          >
            {{ q.label }}
          </span>
        </span>
      </div>

      <p class="ci-setting mono">
        a は {{ A_VALS }} 種類 / b は {{ B_VALS }} 種類 ・ 索引は 1ページ {{ ENTRIES_PER_PAGE }} 件
      </p>

      <div class="ci-rows">
        <div v-for="p in plans" :key="p.order" class="ci-row">
          <span class="ci-name mono">{{ p.order }}</span>
          <span class="ci-use">
            位置決め
            <b>{{ p.plan.usable }}</b>
            列
          </span>
          <span class="ci-bar">
            <span class="ci-seg" :style="{ width: (p.plan.scanned / maxScanned) * 100 + '%' }">
              <span class="ci-hit" :style="{ width: (p.plan.rows / p.plan.scanned) * 100 + '%' }"></span>
            </span>
          </span>
          <span class="ci-num mono"><b>{{ p.plan.scanned.toLocaleString() }}</b> 件</span>
        </div>
      </div>

      <div class="ci-key mono">
        <span><i class="ci-dot"></i>索引の中でなめた件数</span>
        <span><i class="ci-dot hit"></i>そのうち当たり({{ ab.rows.toLocaleString() }} 件)</span>
        <span>読んだ索引ページ (a, b) {{ ab.indexReads }} ・ (b, a) {{ ba.indexReads }}</span>
      </div>

      <div class="ci-verdict">{{ verdict }}</div>

      <p class="ci-note">
        位置決めに使えるのは、列の並びの左端から連続して指定した分だけになる。途中が抜けると、
        そこから先は範囲を狭められない。等値は連ねられるが、範囲を1つ挟むとその後ろも使えなくなる。
        なめた件数と当たりが一致していれば、その索引には無駄が無い。
      </p>
    </template>

    <template v-else>
      <p class="ci-setting mono">本体に1ページ、索引1本につき葉が1ページ</p>

      <div class="ci-rows">
        <div v-for="k in INDEX_COUNTS" :key="k" class="ci-row">
          <span class="ci-name mono">索引 {{ k }} 本</span>
          <span class="ci-use"><b>{{ writeCost(k) }}</b> ページ / 行</span>
          <span class="ci-bar">
            <span class="ci-seg write" :style="{ width: (writeCost(k) / maxWrite) * 100 + '%' }"></span>
          </span>
          <span class="ci-num mono"><b>{{ (N * writeCost(k)).toLocaleString() }}</b> ページ</span>
        </div>
      </div>

      <div class="ci-verdict">
        索引を4本張ると、書き込みで触るページは索引なしの5倍になる。
        引くのを速くする代わりに、書くのが重くなる
      </div>

      <p class="ci-note">
        <span class="mono">(a, b)</span> の索引は <span class="mono">a</span> だけの索引を兼ねるので、
        2本張る必要がない。複合1本で複数の問い合わせを賄えれば、索引の本数が減り、そのぶん書き込みが軽くなる。
        逆に <span class="mono">b</span> 単独で引きたいなら、この索引は役に立たない。
      </p>
    </template>
  </DemoShell>
</template>

<style scoped>
.ci-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.ci-queries {
  margin-top: 12px;
}
.ci-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.ci-setting {
  margin: 12px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.ci-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}
.ci-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ci-name {
  width: 66px;
  flex: none;
  font-size: 12px;
  color: var(--vp-c-text-1);
}
.ci-use {
  width: 96px;
  flex: none;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.ci-use b {
  font-size: 12.5px;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
}
.ci-bar {
  flex: 1 1 auto;
  display: flex;
  height: 12px;
  background-color: var(--vp-c-default-soft);
}
.ci-seg {
  display: block;
  height: 100%;
  background-color: var(--vp-c-text-3);
  min-width: 2px;
}
.ci-hit {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
  min-width: 2px;
}
.ci-seg.write {
  background-color: var(--vp-c-brand-1);
}
.ci-num {
  width: 92px;
  flex: none;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.ci-num b {
  font-size: 12.5px;
  color: var(--vp-c-text-1);
}
.ci-key {
  display: flex;
  gap: 16px;
  margin-top: 8px;
  font-size: 10px;
  color: var(--vp-c-text-3);
  flex-wrap: wrap;
}
.ci-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 5px;
  background-color: var(--vp-c-text-3);
}
.ci-dot.hit {
  background-color: var(--vp-c-brand-1);
}
.ci-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--vp-c-text-1);
}
.ci-note {
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
