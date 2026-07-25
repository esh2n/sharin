<script setup lang="ts">
import { ref } from "vue";
import DemoShell from "./DemoShell.vue";

// 各社モデルが「どの部品」を採用しているかの対応表。
// オープンモデルは公開情報、非公開モデルは推定/非公開と明記する。
const COLS = ["トークナイザ", "位置", "attention", "正規化", "活性化"];

interface Model {
  name: string;
  year: string;
  open: boolean;
  cells: string[]; // COLS の順
}

const MODELS: Model[] = [
  { name: "GPT-2", year: "2019", open: true, cells: ["BPE", "絶対(学習)", "MHA", "LayerNorm", "GELU"] },
  { name: "Llama 3", year: "2024", open: true, cells: ["BPE (tiktoken系)", "RoPE", "GQA", "RMSNorm", "SwiGLU"] },
  { name: "Mistral", year: "2023", open: true, cells: ["BPE", "RoPE", "GQA + SWA", "RMSNorm", "SwiGLU"] },
  { name: "DeepSeek-V3", year: "2024", open: true, cells: ["BPE", "RoPE", "MLA + MoE", "RMSNorm", "SwiGLU"] },
  { name: "GPT-4", year: "2023", open: false, cells: ["BPE", "非公開", "MoE(噂)", "非公開", "非公開"] },
  { name: "Claude", year: "2023〜", open: false, cells: ["非公開", "非公開", "非公開", "非公開", "非公開"] },
  { name: "Gemini", year: "2023〜", open: false, cells: ["非公開", "非公開", "非公開(MoE)", "非公開", "非公開"] },
];

const sel = ref<{ r: number; c: number } | null>(null);
</script>

<template>
  <DemoShell title="モデル × コンポーネント対応表" badge="緑=公開 / 灰=非公開" badge-tone="neutral">
    <div class="lc-scroll">
      <table class="lc-table">
        <thead>
          <tr>
            <th class="lc-corner">モデル</th>
            <th v-for="(c, ci) in COLS" :key="ci" :class="{ hi: sel && sel.c === ci }">{{ c }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(m, ri) in MODELS" :key="ri" :class="{ closed: !m.open }">
            <th class="lc-model" :class="{ hi: sel && sel.r === ri }">
              <span class="lc-dot" :class="m.open ? 'open' : 'closed'"></span>
              {{ m.name }}<span class="lc-year">{{ m.year }}</span>
            </th>
            <td
              v-for="(v, ci) in m.cells"
              :key="ci"
              :class="{ hi: sel && (sel.r === ri || sel.c === ci), na: v.includes('非公開') }"
              @mouseenter="sel = { r: ri, c: ci }"
            >{{ v }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="lc-note">
      土台はどれも Transformer。違いは部品の選択。RoPE・GQA・RMSNorm・SwiGLU・MoE は、
      GPT-2 以降に「大きく・長く・速く」するために生まれた改良で、オープンモデルはほぼ共通して採用している。
      GPT-4/Claude/Gemini は中身が非公開(推定のみ)。
    </p>
  </DemoShell>
</template>

<style scoped>
.lc-scroll {
  overflow-x: auto;
}
.lc-table {
  border-collapse: collapse;
  font-size: 12px;
  min-width: 640px;
}
.lc-table th,
.lc-table td {
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
  text-align: left;
  white-space: nowrap;
}
.lc-corner,
.lc-table thead th {
  background-color: var(--vp-c-bg-soft);
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.lc-model {
  background-color: var(--vp-c-bg-soft);
  font-weight: 600;
}
.lc-year {
  margin-left: 6px;
  font-weight: 400;
  color: var(--vp-c-text-3);
  font-size: 11px;
}
.lc-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  margin-right: 5px;
}
.lc-dot.open {
  background-color: var(--vp-c-green-1);
}
.lc-dot.closed {
  background-color: var(--vp-c-text-3);
}
.lc-table td {
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-1);
}
.lc-table td.na {
  color: var(--vp-c-text-3);
}
.lc-table .hi {
  background-color: color-mix(in srgb, var(--vp-c-brand-1) 12%, transparent);
}
.lc-note {
  margin: 14px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
</style>
