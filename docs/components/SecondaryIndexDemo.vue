<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// db/secondary(Go)を移植。1万行、1ページ 100 行、索引は 1ページ 500 件。
const N = 10000;
const ROWS_PER_PAGE = 100;
const ENTRIES_PER_PAGE = 500;
const FULL = N / ROWS_PER_PAGE;

// 当たる件数の選択肢(表に占める割合)。
const HITS = [1, 10, 50, 100, 200, 500, 1000];
const hits = ref(100);

function height(n: number): number {
  let h = 1;
  let m = n;
  while (m > ENTRIES_PER_PAGE) {
    m = Math.ceil(m / ENTRIES_PER_PAGE);
    h++;
  }
  return h;
}
const H = height(N);

interface Plan {
  name: string;
  index: number;
  table: number;
  note: string;
}

const plans = computed<Plan[]>(() => {
  const m = hits.value;
  const idx = H + Math.floor((m - 1) / ENTRIES_PER_PAGE);
  return [
    { name: "全走査", index: 0, table: FULL, note: "当たる件数に関係なく、いつも表の全ページ" },
    {
      name: "索引 + 本体",
      index: idx,
      table: m,
      note: "索引で主キーを拾い、1件ずつ本体へ戻る。行が散っているので1件1ページ",
    },
    {
      name: "並びをそろえた索引",
      index: idx,
      table: Math.max(1, Math.ceil(m / ROWS_PER_PAGE)),
      note: "本体が索引と同じ順。同じページの行はまとめて読める",
    },
    { name: "本体に戻らない索引", index: idx, table: 0, note: "欲しい列が索引に揃っているので戻らない" },
  ];
});

const maxTotal = computed(() => Math.max(...plans.value.map((p) => p.index + p.table)));
const ratio = computed(() => ((hits.value / N) * 100).toFixed(1));
const sec = computed(() => plans.value[1]);
const badge = computed(() => `当たる ${hits.value} 件(${ratio.value}%)`);
const verdict = computed(() => {
  const t = sec.value.index + sec.value.table;
  if (t > FULL)
    return `索引を使うほうが ${t} ページ、全走査は ${FULL} ページ。当たる件数が表の 1% を超えたあたりで、索引経由のほうが読むページが多くなる`;
  return `索引を使うと ${t} ページ、全走査は ${FULL} ページ。当たる件数が少ないうちは索引が圧倒的に有利`;
});
</script>

<template>
  <DemoShell title="索引を使うか、全部読むか" :badge="badge" :badge-tone="sec.index + sec.table > FULL ? 'ng' : 'ok'">
    <div class="si-actions">
      <span class="si-label">当たる件数</span>
      <span class="sd-seg">
        <span v-for="h in HITS" :key="h" class="sd-seg-opt" :class="{ on: hits === h }" @click="hits = h">
          {{ h }}
        </span>
      </span>
    </div>

    <p class="si-setting mono">
      {{ N }} 行 = {{ FULL }} ページ ・ 索引の高さ {{ H }} ・ 1ページに本体 {{ ROWS_PER_PAGE }} 行 / 索引 {{ ENTRIES_PER_PAGE }} 件
    </p>

    <div class="si-rows">
      <div v-for="p in plans" :key="p.name" class="si-row">
        <span class="si-name">{{ p.name }}</span>
        <span class="si-bar">
          <span class="si-seg idx" :style="{ width: (p.index / maxTotal) * 100 + '%' }"></span>
          <span class="si-seg tbl" :style="{ width: (p.table / maxTotal) * 100 + '%' }"></span>
        </span>
        <span class="si-num mono"><b>{{ p.index + p.table }}</b> ページ</span>
      </div>
    </div>

    <div class="si-key mono">
      <span><i class="si-dot idx"></i>索引を読んだぶん</span>
      <span><i class="si-dot tbl"></i>本体を読んだぶん</span>
    </div>

    <div class="si-verdict">{{ verdict }}</div>

    <p class="si-note">
      索引を引いても行そのものは手に入らないので、主キーを持って本体へ戻ることになる。戻る回数は
      当たった件数ぶん積み上がるので、当たる件数が増えるほど不利になる。本体を索引と同じ順に並べるか、
      欲しい列を索引に持たせるかすれば、戻る回数を減らせる。
    </p>
  </DemoShell>
</template>

<style scoped>
.si-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.si-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.si-setting {
  margin: 12px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.si-rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 12px;
}
.si-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.si-name {
  width: 132px;
  flex: none;
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.si-bar {
  flex: 1 1 auto;
  display: flex;
  height: 12px;
  background-color: var(--vp-c-default-soft);
}
.si-seg {
  display: block;
  height: 100%;
}
.si-seg.idx {
  background-color: var(--vp-c-brand-1);
}
.si-seg.tbl {
  background-color: var(--vp-c-text-3);
}
.si-num {
  width: 82px;
  flex: none;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.si-num b {
  font-size: 12.5px;
  color: var(--vp-c-text-1);
}
.si-key {
  display: flex;
  gap: 16px;
  margin-top: 8px;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.si-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 5px;
}
.si-dot.idx {
  background-color: var(--vp-c-brand-1);
}
.si-dot.tbl {
  background-color: var(--vp-c-text-3);
}
.si-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--vp-c-text-1);
}
.si-note {
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
