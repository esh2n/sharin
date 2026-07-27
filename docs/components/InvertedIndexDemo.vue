<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// search/invertedindex(Go)の転置インデックス + ブール検索 + BM25 をブラウザに移植。
// 固定コーパスに対しクエリを選び、(1) 各語のポスティングリスト、(2) AND/OR/BM25 の
// 結果がどう出るかを見せる。検索が文書本文を読み直さないことが要。

const corpus = [
  "the quick brown fox jumps over the lazy dog",
  "a quick brown dog runs in the park",
  "the lazy cat sleeps all day",
  "fox and cat play in the park",
  "dog dog dog barks at the fox",
];

// --- Go 実装の移植 ---
function tokenize(text: string): string[] {
  return text.toLowerCase().match(/[a-z0-9]+/g) ?? [];
}
interface Posting {
  doc: number;
  tf: number;
}
const postings = new Map<string, Posting[]>();
const docLen: number[] = [];
corpus.forEach((doc, id) => {
  const tokens = tokenize(doc);
  docLen.push(tokens.length);
  const tf = new Map<string, number>();
  for (const t of tokens) tf.set(t, (tf.get(t) ?? 0) + 1);
  for (const [t, n] of [...tf.entries()].sort()) {
    if (!postings.has(t)) postings.set(t, []);
    postings.get(t)!.push({ doc: id, tf: n });
  }
});
const N = corpus.length;
const avgLen = docLen.reduce((a, b) => a + b, 0) / N;
const K1 = 1.2, B = 0.75;

function searchAND(terms: string[]): number[] {
  if (!terms.length) return [];
  let result = (postings.get(terms[0]) ?? []).map((p) => p.doc);
  for (const t of terms.slice(1)) {
    const ids = new Set((postings.get(t) ?? []).map((p) => p.doc));
    result = result.filter((d) => ids.has(d));
  }
  return result;
}
function searchOR(terms: string[]): number[] {
  const s = new Set<number>();
  for (const t of terms) for (const p of postings.get(t) ?? []) s.add(p.doc);
  return [...s].sort((a, b) => a - b);
}
function searchBM25(terms: string[]): { doc: number; score: number }[] {
  const scores = new Map<number, number>();
  for (const t of terms) {
    const ps = postings.get(t) ?? [];
    const df = ps.length;
    if (!df) continue;
    const idf = Math.log(1 + (N - df + 0.5) / (df + 0.5));
    for (const p of ps) {
      const norm = 1 - B + (B * docLen[p.doc]) / avgLen;
      const s = (idf * (p.tf * (K1 + 1))) / (p.tf + K1 * norm);
      scores.set(p.doc, (scores.get(p.doc) ?? 0) + s);
    }
  }
  return [...scores.entries()]
    .map(([doc, score]) => ({ doc, score }))
    .sort((a, b) => b.score - a.score || a.doc - b.doc);
}

// --- UI ---
const queries = [
  { label: "quick dog", terms: ["quick", "dog"] },
  { label: "fox cat", terms: ["fox", "cat"] },
  { label: "dog", terms: ["dog"] },
  { label: "fox park", terms: ["fox", "park"] },
];
const qi = ref(0);
const mode = ref<"and" | "or" | "bm25">("and");
const terms = computed(() => queries[qi.value].terms);

const andResult = computed(() => searchAND(terms.value));
const orResult = computed(() => searchOR(terms.value));
const bm25Result = computed(() => searchBM25(terms.value));

const resultDocs = computed<{ doc: number; score?: number }[]>(() => {
  if (mode.value === "and") return andResult.value.map((doc) => ({ doc }));
  if (mode.value === "or") return orResult.value.map((doc) => ({ doc }));
  return bm25Result.value;
});
const badge = computed(() => `${resultDocs.value.length} 件`);

const modeNote = computed(() => {
  if (mode.value === "and")
    return "AND: 全語のポスティングリストの積。どちらのリストも DocID 昇順なので、マージ走査で線形に交差できる。文書本文は読んでいない";
  if (mode.value === "or") return "OR: いずれかの語を含む文書の和。ヒットは広がるが、どれが「より関連するか」は分からない";
  return "BM25: 語の珍しさ(IDF)× 文書内頻度(TF、ただし飽和つき)を語ごとに足し込む。長い文書は割り引かれる。スコア順に並ぶ";
});

function fmtScore(s?: number): string {
  return s === undefined ? "" : s.toFixed(3);
}
function highlight(doc: string): string {
  const set = new Set(terms.value);
  return tokenize(doc)
    .map((t) => (set.has(t) ? `[${t}]` : t))
    .join(" ");
}
</script>

<template>
  <DemoShell title="inverted-index(転置インデックス)" :badge="badge" badge-tone="neutral">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="(q, i) in queries" :key="q.label" class="sd-seg-opt" :class="{ on: qi === i }" @click="qi = i">{{ q.label }}</span>
      </span>
      <span class="spacer" />
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === 'and' }" @click="mode = 'and'">AND</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'or' }" @click="mode = 'or'">OR</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'bm25' }" @click="mode = 'bm25'">BM25</span>
      </span>
    </div>

    <div class="ii-panel">
      <div class="ii-panel-head">転置インデックス(クエリ語のポスティングリスト)</div>
      <div class="ii-body">
        <div v-for="t in terms" :key="t" class="ii-post">
          <span class="ii-term mono">{{ t }}</span>
          <span class="ii-arrow">→</span>
          <span v-if="!(postings.get(t) ?? []).length" class="ii-none">(なし)</span>
          <span v-for="p in postings.get(t) ?? []" :key="p.doc" class="ii-cell mono">doc{{ p.doc }}<template v-if="p.tf > 1">×{{ p.tf }}</template></span>
        </div>
      </div>
    </div>

    <div class="ii-panel">
      <div class="ii-panel-head">検索結果({{ mode.toUpperCase() }})</div>
      <div class="ii-body">
        <div v-if="resultDocs.length === 0" class="ii-none">該当なし</div>
        <div v-for="(r, rank) in resultDocs" :key="r.doc" class="ii-hit">
          <span v-if="mode === 'bm25'" class="ii-rank mono">#{{ rank + 1 }}</span>
          <span class="ii-doc mono">doc{{ r.doc }}</span>
          <span class="ii-text mono">{{ highlight(corpus[r.doc]) }}</span>
          <span v-if="r.score !== undefined" class="ii-score mono">{{ fmtScore(r.score) }}</span>
        </div>
      </div>
    </div>

    <p class="ii-note">{{ modeNote }}</p>

    <p class="ii-legend">
      索引を先に作っておくので、検索は「語で表を引いてリストを突き合わせる」だけで済む。
      文書が何万件あっても、走査するのはクエリ語のポスティングリストだけだ。順位づけは
      TF(その文書に何回出るか)と IDF(その語がどれだけ珍しいか)の掛け算が土台で、BM25 は
      TF に飽和を入れ、文書の長さで割り引いた実戦版になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.ii-panel {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.ii-panel-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.ii-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.ii-post {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 13px;
}
.ii-term {
  font-weight: 700;
  color: var(--vp-c-brand-1);
  min-width: 52px;
}
.ii-arrow {
  color: var(--vp-c-text-3);
}
.ii-cell {
  font-size: 12px;
  padding: 2px 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.ii-none {
  color: var(--vp-c-text-3);
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
}
.ii-hit {
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-size: 12.5px;
  padding: 6px 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  overflow-x: auto;
}
.ii-rank {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.ii-doc {
  color: var(--vp-c-text-3);
  white-space: nowrap;
}
.ii-text {
  flex: 1;
  color: var(--vp-c-text-1);
  white-space: nowrap;
}
.ii-score {
  color: var(--vp-c-green-1);
  font-weight: 600;
  white-space: nowrap;
}
.ii-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.ii-legend {
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
