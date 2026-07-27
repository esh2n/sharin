<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// 旧 LlmComponentDemo を移設・拡充。各社モデルの部品選択の対応表。
// 行を選ぶと、その独自の1手を解説する。オープン(公開)と非公開を色で分ける。

const COLS = ["トークナイザ", "位置", "attention", "正規化", "活性化", "FFN"];

interface Model {
  name: string;
  year: string;
  open: boolean;
  cells: string[];
  note: string;
}

const MODELS: Model[] = [
  {
    name: "GPT-2",
    year: "2019",
    open: true,
    cells: ["BPE", "絶対(学習)", "MHA", "LayerNorm", "GELU", "dense"],
    note: "mini-GPT で作った基準点。以後の改良はすべてこの構成からの差分として語られる",
  },
  {
    name: "Llama 3",
    year: "2024",
    open: true,
    cells: ["BPE", "RoPE", "GQA", "RMSNorm", "SwiGLU", "dense"],
    note: "独自技術ではなく、標準部品を丁寧に組んで大量データ(15兆トークン)で学習する路線。オープンの基準点でエコシステム最大",
  },
  {
    name: "Mistral 7B",
    year: "2023",
    open: true,
    cells: ["BPE", "RoPE", "GQA + SWA", "RMSNorm", "SwiGLU", "dense"],
    note: "独自の1手は SWA(sliding window attention)。各トークンが直近 W 個だけを見て計算を抑える。7B で当時の 13B 級に並んだ",
  },
  {
    name: "Mixtral 8x7B",
    year: "2023",
    open: true,
    cells: ["BPE", "RoPE", "GQA + SWA", "RMSNorm", "SwiGLU", "MoE (top-2)"],
    note: "MoE をオープンに持ち込んだ。8 expert・top-2 で総量47B・アクティブ13B。Mistral 系の FFN を専門家に分割した版",
  },
  {
    name: "DeepSeek-V3",
    year: "2024",
    open: true,
    cells: ["BPE", "RoPE", "MLA", "RMSNorm", "SwiGLU", "細粒度MoE+共有"],
    note: "独自の1手が2つ。MLA(KVを圧縮、GQAより効率的)と細粒度MoE+共有expert(総量671B・アクティブ37B)。学習効率でも先端",
  },
  {
    name: "Qwen2",
    year: "2024",
    open: true,
    cells: ["BPE", "RoPE", "GQA", "RMSNorm", "SwiGLU", "dense / MoE"],
    note: "0.5B〜72B の幅広い展開と多言語(中国語に強い)。標準部品を採り、サイズの選択肢とデータで勝負する",
  },
  {
    name: "GPT-4",
    year: "2023",
    open: false,
    cells: ["BPE", "非公開", "非公開", "非公開", "非公開", "MoE(噂)"],
    note: "構成は非公開。MoE という報告が広く信じられているが未確認。オープン系と違い外部から検証できない",
  },
  {
    name: "Claude",
    year: "2023〜",
    open: false,
    cells: ["非公開", "非公開", "非公開", "非公開", "非公開", "非公開"],
    note: "アーキテクチャ詳細は非公開。系譜の差は Constitutional AI など後段学習に出る(Claude の章を参照)",
  },
  {
    name: "Gemini",
    year: "2023〜",
    open: false,
    cells: ["非公開", "非公開", "非公開", "非公開", "非公開", "MoE(報告)"],
    note: "ネイティブマルチモーダルと長文脈が特徴。内部構成は非公開で、MoE 採用が報告されている程度(Gemini の章を参照)",
  },
];

const sel = ref(1); // 既定は Llama 3

const cur = computed(() => MODELS[sel.value]);
</script>

<template>
  <DemoShell title="モデル × 部品 対応表" badge-tone="neutral" :badge="cur.open ? '公開' : '非公開'">
    <div class="om-scroll">
      <table class="om-table">
        <thead>
          <tr>
            <th class="om-corner">モデル</th>
            <th v-for="c in COLS" :key="c">{{ c }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(m, ri) in MODELS" :key="m.name" :class="{ on: ri === sel, closed: !m.open }" @click="sel = ri">
            <th class="om-model">
              <span class="om-dot" :class="m.open ? 'open' : 'closed'"></span>
              {{ m.name }}<span class="om-year">{{ m.year }}</span>
            </th>
            <td v-for="(v, ci) in m.cells" :key="ci" :class="{ na: v.includes('非公開') }">{{ v }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="om-detail">
      <span class="om-detail-name">{{ cur.name }}<span class="om-badge" :class="cur.open ? 'open' : 'closed'">{{ cur.open ? '重み公開' : '非公開' }}</span></span>
      <p class="om-detail-note">{{ cur.note }}</p>
    </div>

    <p class="om-legend">
      緑=重み公開 / 灰=非公開。土台はどれも Transformer で、オープン系は
      RoPE + GQA + RMSNorm + SwiGLU にほぼ収束している。差は各社の独自の 1 手
      (Mistral の SWA、DeepSeek の MLA + 細粒度 MoE)に出る。非公開モデルの列は推定を含む。
      行をクリックすると各モデルの独自性が読める。
    </p>
  </DemoShell>
</template>

<style scoped>
.om-scroll {
  overflow-x: auto;
  margin-top: 14px;
}
.om-table {
  border-collapse: collapse;
  font-size: 11.5px;
  min-width: 700px;
  width: 100%;
}
.om-table th,
.om-table td {
  padding: 6px 9px;
  border: 1px solid var(--vp-c-divider);
  text-align: left;
  white-space: nowrap;
}
.om-corner,
.om-table thead th {
  background-color: var(--vp-c-bg-soft);
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.om-table tbody tr {
  cursor: pointer;
}
.om-table tbody tr.on td,
.om-table tbody tr.on .om-model {
  background-color: var(--vp-c-brand-soft);
}
.om-model {
  background-color: var(--vp-c-bg-soft);
  font-weight: 600;
}
.om-year {
  margin-left: 6px;
  font-weight: 400;
  color: var(--vp-c-text-3);
  font-size: 10.5px;
}
.om-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  margin-right: 5px;
}
.om-dot.open { background-color: var(--vp-c-green-1); }
.om-dot.closed { background-color: var(--vp-c-text-3); }
.om-table td {
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-1);
}
.om-table td.na {
  color: var(--vp-c-text-3);
}
.om-detail {
  margin-top: 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 10px 14px;
}
.om-detail-name {
  font-size: 14px;
  font-weight: 700;
  display: flex;
  align-items: center;
  gap: 8px;
}
.om-badge {
  font-size: 10.5px;
  font-weight: 700;
  padding: 1px 7px;
  border-radius: 3px;
}
.om-badge.open { color: var(--vp-c-green-1); background-color: var(--vp-c-green-soft); }
.om-badge.closed { color: var(--vp-c-text-3); background-color: var(--vp-c-bg); }
.om-detail-note {
  margin: 6px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.om-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
</style>
