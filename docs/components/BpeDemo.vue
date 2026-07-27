<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/bpe(Go)の Train / Encode をブラウザに移植。
// 固定コーパスで BPE の併合を 1 手ずつ学習し、その時点の規則でサンプル文が
// どう切られるかをトークンチップで見せる。未知文字は <unk> になる。

const CORPUS = "low low low lower lower newest";
const NUM_MERGES = 8;

function chunks(text: string): string[] {
  const out: string[] = [];
  let cur = "";
  for (const r of text) {
    if (r === " " && cur.length > 0) {
      out.push(cur);
      cur = "";
    }
    cur += r;
  }
  if (cur.length > 0) out.push(cur);
  return out;
}

function applyMerge(s: string[], left: string, right: string): string[] {
  const out: string[] = [];
  for (let i = 0; i < s.length; ) {
    if (i + 1 < s.length && s[i] === left && s[i + 1] === right) {
      out.push(left + right);
      i += 2;
    } else {
      out.push(s[i]);
      i++;
    }
  }
  return out;
}

interface MergeStep {
  left: string;
  right: string;
  count: number;
}

function train(corpus: string, numMerges: number): { steps: MergeStep[]; base: Set<string> } {
  const freq = new Map<string, number>();
  const order: string[] = [];
  for (const c of chunks(corpus)) {
    if (!freq.has(c)) order.push(c);
    freq.set(c, (freq.get(c) ?? 0) + 1);
  }
  let seqs = order.map((c) => [...c]);
  const base = new Set<string>();
  for (const s of seqs) for (const sym of s) base.add(sym);

  const steps: MergeStep[] = [];
  while (steps.length < numMerges) {
    const count = new Map<string, number>();
    seqs.forEach((s, i) => {
      const w = freq.get(order[i])!;
      for (let j = 0; j + 1 < s.length; j++) {
        const key = JSON.stringify([s[j], s[j + 1]]);
        count.set(key, (count.get(key) ?? 0) + w);
      }
    });
    let bestKey = "";
    let bestN = 0;
    for (const [key, n] of count) {
      if (n > bestN || (n === bestN && key < bestKey)) {
        bestKey = key;
        bestN = n;
      }
    }
    if (bestN === 0) break;
    const [left, right] = JSON.parse(bestKey) as [string, string];
    steps.push({ left, right, count: bestN });
    seqs = seqs.map((s) => applyMerge(s, left, right));
  }
  return { steps, base };
}

const { steps: MERGES, base: BASE } = train(CORPUS, NUM_MERGES);

function tokenize(text: string, upTo: number): { tok: string; known: boolean }[] {
  const out: { tok: string; known: boolean }[] = [];
  for (const c of chunks(text)) {
    let s = [...c];
    for (let i = 0; i < upTo; i++) s = applyMerge(s, MERGES[i].left, MERGES[i].right);
    for (const tok of s) {
      // 併合で生まれたトークンは既知。単独 rune はコーパスに出た rune だけ既知。
      const known = tok.length > 1 || BASE.has(tok);
      out.push({ tok, known });
    }
  }
  return out;
}

const samples = [
  { label: "low lower", value: "low lower" },
  { label: "newest lowest", value: "newest lowest" },
  { label: "box lower(未知)", value: "box lower" },
];
const pick = ref(0);
const at = ref(0); // 0 = 併合なし、k = 併合 k 個適用

function setPick(i: number) {
  pick.value = i;
}

const tokens = computed(() => tokenize(samples[pick.value].value, at.value));
const note = computed(() => {
  if (at.value === 0) {
    return "併合ゼロ = rune 単位の切り方。ここから「最頻の隣接ペアを 1 つに併合する」を繰り返して語彙を育てる";
  }
  const m = MERGES[at.value - 1];
  const show = (s: string) => s.replaceAll(" ", "␣");
  return `併合${at.value}: 最頻ペア (${show(m.left)}, ${show(m.right)}) が ${m.count} 回出現 → 新トークン「${show(m.left + m.right)}」。頻出する並びから順に 1 トークンに育っていく`;
});

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < MERGES.length);
function first() {
  at.value = 0;
}
function prev() {
  if (canPrev.value) at.value--;
}
function next() {
  if (canNext.value) at.value++;
}
function last() {
  at.value = MERGES.length;
}

const badge = computed(() => (at.value === 0 ? "rune 単位" : `併合 ${at.value}/${MERGES.length}`));
const badgeTone = computed<"ok" | "neutral">(() => (at.value === MERGES.length ? "ok" : "neutral"));
const disp = (t: string) => t.replaceAll(" ", "␣");
</script>

<template>
  <DemoShell title="BPE(併合の学習)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="bp-corpus mono">コーパス: {{ CORPUS }}</span>
      <span class="spacer" />
      <span class="sd-seg">
        <span
          v-for="(s, i) in samples"
          :key="s.value"
          class="sd-seg-opt"
          :class="{ on: pick === i }"
          @click="setPick(i)"
          >{{ s.label }}</span
        >
      </span>
    </div>

    <div class="bp-rules">
      <div class="bp-rules-head">学習済みの併合規則(適用順)</div>
      <div class="bp-rules-list">
        <span v-if="at === 0" class="bp-none">(まだ無し)</span>
        <span v-for="(m, i) in MERGES.slice(0, at)" :key="i" class="bp-rule mono" :class="{ latest: i === at - 1 }">
          {{ disp(m.left) }}+{{ disp(m.right) }}
        </span>
      </div>
    </div>

    <div class="bp-panel">
      <div class="bp-panel-head">
        「{{ samples[pick].value }}」の切り方
        <span class="bp-count mono">{{ tokens.length }} トークン</span>
      </div>
      <div class="bp-tokens">
        <span v-for="(t, i) in tokens" :key="i" class="bp-tok mono" :class="{ unk: !t.known }">
          {{ t.known ? disp(t.tok) : "<unk>" }}
        </span>
      </div>
    </div>

    <p class="bp-note">{{ note }}</p>

    <div class="bp-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="bp-nav mono">{{ at + 1 }} / {{ MERGES.length + 1 }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1併合すすめる</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="bp-legend">
      ␣ は語の先頭に付いた空白。頻出語 low は 2 手で 1 トークンになり、文中形の「␣low」も
      1 トークンに育つ。コーパスに無い文字(b, x)は語彙に無いので &lt;unk&gt; に落ちる。
      実物はバイト基底にすることで未知そのものを無くしている。
    </p>
  </DemoShell>
</template>

<style scoped>
.bp-corpus {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.bp-rules {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.bp-rules-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 7px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.bp-rules-list {
  padding: 10px 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-height: 40px;
}
.bp-none {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.bp-rule {
  font-size: 12px;
  padding: 2px 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.bp-rule.latest {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.bp-panel {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.bp-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 7px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.bp-count {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.bp-tokens {
  padding: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-height: 48px;
}
.bp-tok {
  font-size: 13px;
  padding: 4px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
}
.bp-tok.unk {
  border-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.bp-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.bp-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.bp-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.bp-legend {
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
