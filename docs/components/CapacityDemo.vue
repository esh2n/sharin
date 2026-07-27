<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// キャパシティ見積もりを 1 段ずつ進める電卓。
// DAU を切り替えると全段の数字が連動して再計算され、
// どの規模で構成の選択(単一DB→シャード、CDN必須)が変わるかを見る。

const dauOptions = [
  { label: "100万 DAU", dau: 1e6 },
  { label: "1000万 DAU", dau: 1e7 },
  { label: "1億 DAU", dau: 1e8 },
];
const pick = ref(1);
const at = ref(0);

const POSTS = 2;
const VIEWS = 100;
const ORIG = 2 * 1024 * 1024; // 2MB
const SERV = 200 * 1024; // 200KB
const META = 1024; // 1KB
const REPL = 3;
const PEAK = 3;

function fmtBytes(b: number): string {
  const units = ["B", "KB", "MB", "GB", "TB", "PB", "EB"];
  let i = 0;
  while (b >= 1024 && i < units.length - 1) {
    b /= 1024;
    i++;
  }
  return `${b >= 100 ? b.toFixed(0) : b.toFixed(1)} ${units[i]}`;
}
const fmtNum = (n: number) => (n >= 1e8 ? (n / 1e8).toFixed(1) + "億" : n >= 1e4 ? (n / 1e4).toFixed(0) + "万" : n.toFixed(0));

const calc = computed(() => {
  const dau = dauOptions[pick.value].dau;
  const uploads = dau * POSTS;
  const storageDay = uploads * (ORIG + SERV);
  const storageYear = storageDay * 365;
  const storageReal = storageYear * REPL;
  const writeQps = uploads / 86400;
  const views = dau * VIEWS;
  const readQps = views / 86400;
  const readPeak = readQps * PEAK;
  const bandwidth = readPeak * SERV;
  const hot = uploads * 3 * 0.2;
  const cache = hot * SERV;
  const metaYear = uploads * META * 365;
  const metaRowsYear = uploads * 365;
  return { dau, uploads, storageDay, storageYear, storageReal, writeQps, views, readQps, readPeak, bandwidth, hot, cache, metaYear, metaRowsYear };
});

interface Step {
  title: string;
  formula: (c: any) => string;
  value: (c: any) => string;
  verdict: (c: any) => string;
}

const steps: Step[] = [
  {
    title: "投稿数 / 日",
    formula: (c) => `${fmtNum(c.dau)}人 × ${POSTS}枚`,
    value: (c) => `${fmtNum(c.uploads)}枚/日`,
    verdict: () => "すべての計算の起点。ここから保存・QPS・帯域が芋づる式に決まる",
  },
  {
    title: "ストレージ / 日",
    formula: (c) => `${fmtNum(c.uploads)}枚 × (2MB + 200KB)`,
    value: (c) => `${fmtBytes(c.storageDay)}/日`,
    verdict: (c) =>
      c.storageDay > 10 * 1024 ** 4
        ? "日次で数十TB。RDB の BLOB は既に論外の桁"
        : "日次数TB。この時点でオブジェクトストレージが妥当",
  },
  {
    title: "ストレージ / 年(レプリカ込み)",
    formula: (c) => `${fmtBytes(c.storageDay)} × 365 × ${REPL}本`,
    value: (c) => `${fmtBytes(c.storageReal)}/年`,
    verdict: (c) =>
      c.storageReal > 1024 ** 5
        ? "PB 級。水平に伸びるオブジェクトストレージ(S3系)一択"
        : "数百TB級。それでもオブジェクトストレージが安全",
  },
  {
    title: "書き込み QPS",
    formula: (c) => `${fmtNum(c.uploads)}枚 ÷ 86,400秒 (ピーク×${PEAK})`,
    value: (c) => `${c.writeQps.toFixed(0)} → ピーク ${(c.writeQps * PEAK).toFixed(0)} QPS`,
    verdict: () => "書き込みは意外と小さい。キュー1本とアプリサーバ数台で足りる",
  },
  {
    title: "読み取り QPS",
    formula: (c) => `${fmtNum(c.dau)}人 × ${VIEWS}枚 ÷ 86,400秒 (ピーク×${PEAK})`,
    value: (c) => `${fmtNum(c.readQps)} → ピーク ${fmtNum(c.readPeak)} QPS`,
    verdict: (c) => `読み書き比 ${(c.readQps / c.writeQps).toFixed(0)}:1。読み側だけ別の仕組みで受ける非対称構成が確定`,
  },
  {
    title: "配信帯域",
    formula: (c) => `${fmtNum(c.readPeak)} QPS × 200KB`,
    value: (c) => `${fmtBytes(c.bandwidth)}/s ≈ ${((c.bandwidth * 8) / 1e9).toFixed(0)} Gbps`,
    verdict: (c) =>
      (c.bandwidth * 8) / 1e9 > 10
        ? "自前で出せる帯域ではない。CDN 必須。原本でなく 200KB 配信版を作る意味もここ"
        : "この規模ならオリジン直配も辛うじて可能。だが CDN の方が安い",
  },
  {
    title: "キャッシュサイズ",
    formula: (c) => `直近3日 ${fmtNum(c.uploads * 3)}枚 × 上位20% × 200KB`,
    value: (c) => `${fmtBytes(c.cache)}`,
    verdict: (c) =>
      c.cache > 1024 ** 4
        ? `メモリ128GBのサーバ ${Math.ceil(c.cache / (128 * 1024 ** 3))} 台。LRU の追い出しがそのまま動く規模`
        : "キャッシュサーバ数台に収まる",
  },
  {
    title: "メタデータ DB",
    formula: (c) => `${fmtNum(c.uploads)}行/日 × 1KB × 365`,
    value: (c) => `${fmtBytes(c.metaYear)}/年・${fmtNum(c.metaRowsYear)}行/年`,
    verdict: (c) =>
      c.metaRowsYear > 3e9
        ? "数十億行。単一DBの索引が苦しい。Snowflake ID + ユーザー単位シャーディングへ"
        : "単一DB + リードレプリカでまだ持つ。ただし成長率次第で境界を跨ぐ",
  },
];

function setPick(i: number) {
  pick.value = i;
}

const cur = computed(() => steps[at.value]);

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < steps.length - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = steps.length - 1; }

const badge = computed(() => `${at.value + 1}. ${cur.value.title}`);
</script>

<template>
  <DemoShell title="キャパシティ見積もり電卓" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="(d, i) in dauOptions" :key="d.dau" class="sd-seg-opt" :class="{ on: pick === i }" @click="setPick(i)">{{ d.label }}</span>
      </span>
    </div>

    <div class="cp-steps">
      <div v-for="(s, i) in steps" :key="s.title" class="cp-step" :class="{ on: i === at, done: i < at }">
        <span class="cp-step-title">{{ i + 1 }}. {{ s.title }}</span>
        <span v-if="i <= at" class="cp-step-val mono">{{ s.value(calc) }}</span>
        <span v-else class="cp-step-val mono dim">…</span>
      </div>
    </div>

    <div class="cp-detail">
      <div class="cp-formula mono">{{ cur.formula(calc) }} = {{ cur.value(calc) }}</div>
      <p class="cp-verdict">{{ cur.verdict(calc) }}</p>
    </div>

    <div class="cp-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="cp-nav mono">{{ at + 1 }} / {{ steps.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">次の見積もりへ</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="cp-legend">
      前提: 1人2枚/日投稿・100枚/日閲覧・原本2MB/配信200KB/メタデータ1KB・ピーク3倍・レプリカ3本。
      DAU を切り替えると全段が再計算され、「単一DBで持つ/シャーディング必須」「CDN があった方が安い/無いと不可能」の
      境界がどの規模で跨がれるかが分かる。
    </p>
  </DemoShell>
</template>

<style scoped>
.cp-steps {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 6px 0;
}
.cp-step {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 4px 12px;
  border-left: 3px solid transparent;
  border-radius: 0;
  font-size: 12.5px;
  color: var(--vp-c-text-3);
}
.cp-step.done {
  color: var(--vp-c-text-2);
}
.cp-step.on {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
  font-weight: 700;
}
.cp-step-val {
  font-size: 12px;
  white-space: nowrap;
}
.cp-step-val.dim {
  color: var(--vp-c-text-3);
}
.cp-detail {
  margin-top: 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 10px 14px;
}
.cp-formula {
  font-size: 13px;
  font-weight: 700;
}
.cp-verdict {
  margin: 6px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.cp-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.cp-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.cp-legend {
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
