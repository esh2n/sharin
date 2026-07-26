<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";
import { render } from "../../frontend/browser/pipeline";

// browser のレンダリングパイプラインを可視化する。frontend/browser の render を直接使い、
// HTML+CSS → 描画コマンドを計算し、最終段のコマンドを SVG で実描画する。

const WIDTH = 300;

interface Sample {
  name: string;
  html: string;
  css: string;
}
const SAMPLES: Sample[] = [
  {
    name: "カード",
    html: '<div class="card"><h1 class="t">記事タイトル</h1><p class="b">本文のプレビュー。箱を縦に積んで描く。</p></div>',
    css: ".card{background:#eef2ff;padding:12px} .t{background:#c7d2fe;height:28px;color:#3730a3} .b{height:40px;color:#475569}",
  },
  {
    name: "ネスト",
    html: '<div class="page"><div class="row">A</div><div class="row">B</div></div>',
    css: ".page{background:#ecfdf5;padding:10px} .row{background:#a7f3d0;height:30px;margin:6px;color:#065f46}",
  },
];

const state = reactive({ idx: 0 });
const sample = computed(() => SAMPLES[state.idx]);
const result = computed(() => render(sample.value.html, sample.value.css, WIDTH));
const cmds = computed(() => result.value.display);
const canvasHeight = computed(() => Math.max(60, result.value.layout.rect.height));

function label(kind: string): string {
  return kind === "rect" ? "矩形" : "文字";
}
</script>

<template>
  <DemoShell title="browser(レンダリングパイプライン)" :badge="`描画コマンド ${cmds.length}`" badge-tone="neutral">
    <div class="sd-controls">
      <div class="br-seg" role="group" aria-label="サンプル">
        <button v-for="(s, i) in SAMPLES" :key="i" class="br-seg-btn" :class="{ on: state.idx === i }" @click="state.idx = i">{{ s.name }}</button>
      </div>
    </div>

    <div class="br-grid">
      <div class="br-col">
        <div class="br-label">入力: HTML</div>
        <pre class="br-code"><code>{{ sample.html }}</code></pre>
        <div class="br-label">入力: CSS</div>
        <pre class="br-code"><code>{{ sample.css }}</code></pre>
        <div class="br-label">最終段: 描画コマンド(ディスプレイリスト)</div>
        <div class="br-cmds">
          <div v-for="(c, i) in cmds" :key="i" class="br-cmd" :style="{ borderLeftColor: c.type === 'rect' ? 'var(--vp-c-brand-2)' : 'var(--vp-c-green-2)' }">
            <span class="br-cmd-k">{{ label(c.type) }}</span>
            <span v-if="c.type === 'rect'" class="br-cmd-v">({{ c.x }},{{ c.y }}) {{ c.width }}×{{ c.height }} <em :style="{ color: c.color }">■</em>{{ c.color }}</span>
            <span v-else class="br-cmd-v">({{ c.x }},{{ c.y }}) "{{ c.text }}"</span>
          </div>
        </div>
      </div>

      <div class="br-col">
        <div class="br-label">描画結果(コマンドを実際に描く)</div>
        <svg class="br-canvas" :width="WIDTH" :height="canvasHeight" :viewBox="`0 0 ${WIDTH} ${canvasHeight}`" role="img" aria-label="描画結果">
          <template v-for="(c, i) in cmds" :key="i">
            <rect v-if="c.type === 'rect'" :x="c.x" :y="c.y" :width="c.width" :height="c.height" :fill="c.color" />
            <text v-else :x="c.x + 2" :y="c.y + 14" :fill="c.color" font-size="12" font-family="system-ui, sans-serif">{{ c.text }}</text>
          </template>
        </svg>
        <div class="br-note">幅 {{ WIDTH }}px・高さ {{ canvasHeight }}px。矩形は背景、文字はテキストノード。パイプラインが計算した座標そのまま</div>
      </div>
    </div>

    <div class="br-legend">
      <span>HTML→DOM木 / CSS→規則 → style(カスケード+継承) → layout(縦積み) → paint(コマンド)</span>
      <span>左のコマンド一覧を、右でそのまま矩形と文字として描いている</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.br-seg {
  display: inline-flex;
  border: 1px solid var(--vp-c-divider);
  overflow: hidden;
}
.br-seg-btn {
  padding: 4px 14px;
  font-size: 12px;
  color: var(--vp-c-text-2);
  background-color: var(--vp-c-bg);
  border-right: 1px solid var(--vp-c-divider);
}
.br-seg-btn:last-child {
  border-right: none;
}
.br-seg-btn.on {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.br-grid {
  display: grid;
  grid-template-columns: 1fr 320px;
  gap: 16px;
  margin-top: 14px;
}
@media (max-width: 680px) {
  .br-grid {
    grid-template-columns: 1fr;
  }
}
.br-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin: 10px 0 5px;
}
.br-label:first-child {
  margin-top: 0;
}
.br-code {
  margin: 0;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-alt);
  padding: 8px 10px;
  font-size: 11px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
.br-cmds {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.br-cmd {
  display: flex;
  gap: 8px;
  align-items: baseline;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  padding: 4px 8px;
}
.br-cmd-k {
  flex: none;
  font-size: 10px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.br-cmd-v {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  word-break: break-all;
}
.br-cmd-v em {
  font-style: normal;
}
.br-canvas {
  border: 1px solid var(--vp-c-divider);
  background-color: #ffffff;
  max-width: 100%;
  height: auto;
}
.br-note {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin-top: 6px;
}
.br-legend {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 14px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
