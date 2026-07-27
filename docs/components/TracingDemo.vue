<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// observability/tracing(Go)をブラウザに移植。
// 1 リクエストのトレースを時間軸で描き、各 span を横棒で並べる。
// 遅いサービスを切り替えると、クリティカルパス(全体時間を決める鎖)が変わる。

// span の定義。start/end は ms。depth は木の深さ(字下げ用)。
type Span = { id: number; parent: number; name: string; start: number; end: number; depth: number };

// 遅い箇所ごとに、対応するトレースを組む。
// 木の形は同じで、どのサービスが重いかだけを変える。
const slowTargets = [
  { key: "billing", label: "課金が遅い" },
  { key: "auth", label: "認証が遅い" },
  { key: "inventory", label: "在庫が遅い" },
] as const;
const slowPick = ref(0);
const slowKey = computed(() => slowTargets[slowPick.value].key);

// 基準の所要時間。slow のサービスだけ大きくする。
function buildTrace(slow: string): Span[] {
  const dur = (name: string, base: number) => (name === slow ? base + 60 : base);
  const auth = dur("auth", 20);
  const inv = dur("inventory", 15);
  const bill = dur("billing", 30);
  // gateway ├ auth, handler(├ inventory, billing)。handler は子を順に呼ぶ。
  const authStart = 10;
  const authEnd = authStart + auth; // auth 終了後に handler 開始
  const handlerStart = authEnd;
  const invStart = handlerStart + 5;
  const invEnd = invStart + inv;
  const billStart = invEnd; // 在庫の後に課金(直列)
  const billEnd = billStart + bill;
  const handlerEnd = billEnd + 5;
  const gatewayEnd = handlerEnd + 5;
  return [
    { id: 1, parent: 0, name: "gateway", start: 0, end: gatewayEnd, depth: 0 },
    { id: 2, parent: 1, name: "auth", start: authStart, end: authEnd, depth: 1 },
    { id: 3, parent: 1, name: "handler", start: handlerStart, end: handlerEnd, depth: 1 },
    { id: 4, parent: 3, name: "inventory", start: invStart, end: invEnd, depth: 2 },
    { id: 5, parent: 3, name: "billing", start: billStart, end: billEnd, depth: 2 },
  ];
}

const spans = computed(() => buildTrace(slowKey.value));
const total = computed(() => Math.max(...spans.value.map((s) => s.end)));

// クリティカルパス:根から「最も遅く終わる子」を辿る(Go の CriticalPath と同じ)。
const criticalIds = computed(() => {
  const byParent = new Map<number, Span[]>();
  for (const s of spans.value) {
    if (!byParent.has(s.parent)) byParent.set(s.parent, []);
    byParent.get(s.parent)!.push(s);
  }
  const ids: number[] = [];
  let cur: Span | undefined = spans.value.find((s) => s.parent === 0);
  while (cur) {
    ids.push(cur.id);
    const children = byParent.get(cur.id) ?? [];
    cur = children.reduce<Span | undefined>((slow, c) => (!slow || c.end > slow.end ? c : slow), undefined);
  }
  return new Set(ids);
});

const badge = computed(() => `全体 ${total.value}ms`);

const critNames = computed(() =>
  spans.value.filter((s) => criticalIds.value.has(s.id)).map((s) => s.name).join(" → "),
);
const note = computed(() => {
  const t = slowTargets[slowPick.value].label;
  return `${t}とき、全体は ${total.value}ms。クリティカルパスは ${critNames.value}。この鎖の上を速くしない限り全体は縮まない。パス外の span をいくら速くしても効かない`;
});

function pct(x: number): number {
  return (x / total.value) * 100;
}
</script>

<template>
  <DemoShell title="分散トレーシング" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="(o, i) in slowTargets"
          :key="o.key"
          class="sd-seg-opt"
          :class="{ on: slowPick === i }"
          @click="slowPick = i"
          >{{ o.label }}</span
        >
      </span>
    </div>

    <div class="tr-trace">
      <div v-for="s in spans" :key="s.id" class="tr-row">
        <span class="tr-name mono" :style="{ paddingLeft: s.depth * 14 + 'px' }">
          <span v-if="s.depth > 0" class="tr-tree">└</span>{{ s.name }}
        </span>
        <span class="tr-track">
          <span
            class="tr-bar"
            :class="{ crit: criticalIds.has(s.id) }"
            :style="{ left: pct(s.start) + '%', width: pct(s.end - s.start) + '%' }"
          >
            <span class="tr-dur mono">{{ s.end - s.start }}</span>
          </span>
        </span>
      </div>
      <div class="tr-axis">
        <span>0</span>
        <span>{{ Math.round(total / 2) }}ms</span>
        <span>{{ total }}ms</span>
      </div>
    </div>

    <p class="tr-note">{{ note }}</p>

    <p class="tr-legend">
      1 リクエストが gateway → auth / handler → inventory / billing と渡り歩く様子。全 span が同じ
      trace_id を共有し、親子で繋がる。強調した棒がクリティカルパス(各段で最も遅く終わる子の連なり)で、
      全体の所要時間を決めている。遅いサービスを切り替えると、直すべき場所が変わるのが分かる。
    </p>
  </DemoShell>
</template>

<style scoped>
.tr-trace {
  margin-top: 16px;
}
.tr-row {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 26px;
}
.tr-name {
  width: 110px;
  font-size: 12px;
  color: var(--vp-c-text-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tr-tree {
  color: var(--vp-c-text-3);
  margin-right: 3px;
}
.tr-track {
  position: relative;
  flex: 1;
  height: 16px;
  background-color: var(--vp-c-bg-soft);
}
.tr-bar {
  position: absolute;
  top: 0;
  height: 16px;
  background-color: var(--vp-c-default-2, var(--vp-c-gray-2));
  border-radius: 0;
  display: flex;
  align-items: center;
  min-width: 2px;
}
.tr-bar.crit {
  background-color: var(--vp-c-brand-1);
}
.tr-dur {
  font-size: 9.5px;
  color: var(--vp-c-text-1);
  padding-left: 4px;
  white-space: nowrap;
}
.tr-bar.crit .tr-dur {
  color: #fff;
}
.tr-axis {
  display: flex;
  justify-content: space-between;
  margin-top: 6px;
  padding-left: 120px;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.tr-note {
  margin: 12px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.tr-legend {
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
