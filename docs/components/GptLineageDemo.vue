<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// GPT 系譜を 1 世代ずつ進める年表デモ。規模(対数バー)と賭け・分かったことを並べる。

interface Gen {
  name: string;
  year: string;
  params: number | null; // null = 非公開
  bet: string;
  learned: string;
}

const gens: Gen[] = [
  {
    name: "GPT-1",
    year: "2018",
    params: 117e6,
    bet: "ラベルなしテキストの事前学習が、タスク別モデルの初期値として効くか",
    learned: "効いた。「まず言語モデルとして育て、タスクは微調整で後付け」という型が確立した",
  },
  {
    name: "GPT-2",
    year: "2019",
    params: 1.5e9,
    bet: "規模を 13 倍にすれば、微調整なしでもタスクがこなせるようになるか",
    learned: "ある程度こなせた(zero-shot)。ウェブテキストの続きを書く能力は、要約や翻訳を暗黙に含んでいた",
  },
  {
    name: "GPT-3",
    year: "2020",
    params: 175e9,
    bet: "さらに 100 倍にしたら何が起きるか",
    learned: "文脈内学習が現れた。プロンプトに数例書くだけで、勾配更新なしに新タスクへ適応する。重みは非公開になり API 提供へ",
  },
  {
    name: "InstructGPT / ChatGPT",
    year: "2022",
    params: 175e9,
    bet: "規模ではなく後段学習(SFT + RLHF)に賭けたら、使い勝手はどう変わるか",
    learned: "人間評価で 1.3B 版が素の 175B に勝った。知識は規模、答え方は後段学習という分業が見えた。チャット UI で一般に普及",
  },
  {
    name: "GPT-4",
    year: "2023",
    params: null,
    bet: "マルチモーダル入力と、さらなる規模・データ(詳細非公開)",
    learned: "画像入力と専門試験の人間水準を達成。一方で構成・規模は非公開になり、研究の系譜としてはここで閉じた",
  },
  {
    name: "GPT-4o",
    year: "2024",
    params: null,
    bet: "音声・画像・テキストを別モデルの連鎖でなく単一モデルに畳めるか",
    learned: "畳めた(omni)。変換の継ぎ目が消え、リアルタイム音声対話が実用になった",
  },
];

// 対数バー: 1e8 〜 1e12 を 0〜100% に
const barPct = (p: number | null) => (p === null ? 100 : ((Math.log10(p) - 8) / (12 - 8)) * 100);
const fmtParams = (p: number | null) => (p === null ? "非公開" : p >= 1e9 ? (p / 1e9).toFixed(1).replace(/\.0$/, "") + "B" : (p / 1e6).toFixed(0) + "M");

const at = ref(0);
const cur = computed(() => gens[at.value]);

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < gens.length - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = gens.length - 1; }

const badge = computed(() => `${cur.value.name} (${cur.value.year})`);
</script>

<template>
  <DemoShell title="GPT系譜(賭けと結果)" badge-tone="neutral" :badge="badge">
    <div class="gl-timeline">
      <div v-for="(g, i) in gens" :key="g.name" class="gl-row" :class="{ on: i === at, future: i > at }">
        <span class="gl-name">{{ g.name }}</span>
        <span class="gl-year mono">{{ g.year }}</span>
        <span class="gl-track">
          <span class="gl-fill" :class="{ unknown: g.params === null }" :style="{ width: barPct(g.params) + '%' }"></span>
        </span>
        <span class="gl-params mono">{{ fmtParams(g.params) }}</span>
      </div>
      <p class="gl-scale">規模バーは対数(10⁸〜10¹²)。非公開の世代は点線で示している</p>
    </div>

    <div class="gl-detail">
      <div class="gl-block">
        <span class="gl-block-label">賭けたこと</span>
        <p class="gl-block-text">{{ cur.bet }}</p>
      </div>
      <div class="gl-block learned">
        <span class="gl-block-label">分かったこと</span>
        <p class="gl-block-text">{{ cur.learned }}</p>
      </div>
    </div>

    <div class="gl-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="gl-nav mono">{{ at + 1 }} / {{ gens.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">次の世代へ</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="gl-legend">
      規模が 3 桁上がる間に、能力の言葉は「微調整すれば使える」→「例を見せれば使える」→
      「頼めば使える」へ変わった。設計して入れた能力ではなく、賭けの結果として確認された能力の
      系譜として読むのがこの年表の読み方になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.gl-timeline {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px 6px;
}
.gl-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-left: 3px solid transparent;
  border-radius: 0;
}
.gl-row.on {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg-soft);
}
.gl-row.future {
  opacity: 0.45;
}
.gl-name {
  width: 150px;
  font-size: 12px;
  font-weight: 700;
}
.gl-year {
  width: 38px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.gl-track {
  flex: 1;
  height: 10px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.gl-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.gl-fill.unknown {
  background: repeating-linear-gradient(
    -45deg,
    var(--vp-c-brand-soft),
    var(--vp-c-brand-soft) 4px,
    transparent 4px,
    transparent 8px
  );
}
.gl-params {
  width: 52px;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.gl-scale {
  margin: 6px 0 2px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.gl-detail {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.gl-block {
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 8px 14px;
}
.gl-block.learned {
  border-left-color: var(--vp-c-brand-1);
}
.gl-block-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-3);
}
.gl-block-text {
  margin: 2px 0 0;
  font-size: 13px;
  line-height: 1.7;
}
.gl-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.gl-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.gl-legend {
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
