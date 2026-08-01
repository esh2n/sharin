<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// observability/collect(Go)を移植。実時間も乱数も使わない。
type View = "budget" | "down" | "cardinality" | "placement";
const view = ref<View>("budget");

// ---- ① 1周の予算 ----
const INTERVAL = 60;
const SCRAPE_COST = 1;
const targetCounts = [10, 30, 60, 61, 120, 240];
const targets = ref(120);
const workers = ref(1);

const maxTargets = computed(() => Math.floor((INTERVAL * workers.value) / SCRAPE_COST));
const scraped = computed(() => Math.min(targets.value, maxTargets.value));
const dropped = computed(() => Math.max(0, targets.value - maxTargets.value));
const roundCost = computed(() => Math.floor((scraped.value * SCRAPE_COST) / workers.value));

// ---- ② 落ちた対象 ----
const N_DOWN = 10;
const downIdx = [3, 7];
const known = ref(true);
const pullDetected = downIdx.map((i) => `t${i}`);
const pushDetected = computed(() => (known.value ? downIdx.map((i) => `t${i}`) : []));

// ---- ③ カーディナリティ ----
const LABELS = [
  { name: "pod", values: 30 },
  { name: "endpoint", values: 20 },
  { name: "status", values: 5 },
  { name: "version", values: 4 },
  { name: "user_id", values: 10000 },
];
const useLabels = ref(3);
const series = computed(() =>
  LABELS.slice(0, useLabels.value).reduce((n, l) => n * l.values, 1),
);
const bytes = computed(() => series.value * 8);
const fmt = (n: number) => n.toLocaleString();
const fmtBytes = (n: number) =>
  n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(1)} MB` : `${(n / 1024).toFixed(0)} KB`;

// ---- ④ 配置 ----
const N_TARGETS = 100;
const PER_NODE = 10;
const SERIES_PER_TARGET = 50;
const TOTAL = N_TARGETS * SERIES_PER_TARGET;
const layouts = [
  { name: "中央に1つ", collectors: 1, lost: TOTAL },
  { name: "ノードごと", collectors: N_TARGETS / PER_NODE, lost: PER_NODE * SERIES_PER_TARGET },
  { name: "対象ごと", collectors: N_TARGETS, lost: SERIES_PER_TARGET },
];

const badge = computed(() => {
  if (view.value === "budget") return `間隔 ${INTERVAL} / 同時 ${workers.value} 本 → 上限 ${maxTargets.value}`;
  if (view.value === "down") return `${N_DOWN} 対象中 2 つが落ちている`;
  if (view.value === "cardinality") return `${fmt(series.value)} 系列`;
  return `${N_TARGETS} 対象 / ${fmt(TOTAL)} 系列`;
});
const badgeTone = computed(() =>
  view.value === "budget" && dropped.value > 0 ? "ng" : undefined,
);
</script>

<template>
  <DemoShell title="メトリクスをどう集めるか" :badge="badge" :badge-tone="badgeTone">
    <div class="cl-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: view === 'budget' }" @click="view = 'budget'">1周の予算</span>
        <span class="sd-seg-opt" :class="{ on: view === 'down' }" @click="view = 'down'">落ちた対象</span>
        <span class="sd-seg-opt" :class="{ on: view === 'cardinality' }" @click="view = 'cardinality'">系列数</span>
        <span class="sd-seg-opt" :class="{ on: view === 'placement' }" @click="view = 'placement'">置き方</span>
      </span>
    </div>

    <!-- ① 1周の予算 -->
    <template v-if="view === 'budget'">
      <div class="cl-actions cl-row2">
        <span class="cl-label">対象数</span>
        <span class="sd-seg">
          <span v-for="n in targetCounts" :key="n" class="sd-seg-opt mono"
                :class="{ on: targets === n }" @click="targets = n">{{ n }}</span>
        </span>
      </div>
      <div class="cl-actions cl-row2">
        <span class="cl-label">同時本数</span>
        <span class="sd-seg">
          <span v-for="w in [1, 2, 4]" :key="w" class="sd-seg-opt mono"
                :class="{ on: workers === w }" @click="workers = w">{{ w }}</span>
        </span>
      </div>

      <p class="cl-setting mono">間隔 {{ INTERVAL }} ・ 1 対象を叩くコスト {{ SCRAPE_COST }}</p>

      <div class="cl-bar">
        <span class="cl-seg ok" :style="{ width: (scraped / Math.max(targets, 1)) * 100 + '%' }"></span>
        <span class="cl-seg ng" :style="{ width: (dropped / Math.max(targets, 1)) * 100 + '%' }"></span>
      </div>
      <div class="cl-key mono">
        <span><i class="cl-dot ok"></i>読めた {{ scraped }}</span>
        <span><i class="cl-dot ng"></i>落とした {{ dropped }}</span>
        <span>1周の時間 {{ roundCost }} / 間隔 {{ INTERVAL }}</span>
      </div>

      <div class="cl-verdict">
        <template v-if="dropped > 0">
          上限は {{ maxTargets }} 対象。{{ targets }} 対象あるので {{ dropped }} 個がこの周で読めない。
          同時本数を増やすか、間隔を延ばすしかない
        </template>
        <template v-else>
          {{ targets }} 対象は上限 {{ maxTargets }} の内側なので全部読める。1 周に {{ roundCost }} 使う
        </template>
      </div>
    </template>

    <!-- ② 落ちた対象 -->
    <template v-else-if="view === 'down'">
      <div class="cl-actions cl-row2">
        <span class="cl-label">居るはずの一覧を持つ</span>
        <span class="sd-seg">
          <span class="sd-seg-opt" :class="{ on: known }" @click="known = true">持つ</span>
          <span class="sd-seg-opt" :class="{ on: !known }" @click="known = false">持たない</span>
        </span>
      </div>

      <div class="cl-rows">
        <div class="cl-plan">
          <span class="cl-name">pull</span>
          <span class="cl-detect ok mono">t3, t7 を名指しできる</span>
        </div>
        <div class="cl-plan">
          <span class="cl-name">push</span>
          <span class="cl-detect mono" :class="known ? 'ok' : 'ng'">
            {{ known ? "t3, t7 を名指しできる" : "無音が 2 件。理由は分からない" }}
          </span>
        </div>
      </div>

      <div class="cl-verdict">
        <template v-if="known">
          一覧があれば、push でも落ちた対象を名指しできる。pull と同じ結論に届く
        </template>
        <template v-else>
          一覧が無いと、届かない理由が「落ちた / 送っていない / 経路が切れた」のどれか分からない。
          pull が検出できるのは、叩くために一覧を持たざるを得ないからになる
        </template>
      </div>
    </template>

    <!-- ③ 系列数 -->
    <template v-else-if="view === 'cardinality'">
      <div class="cl-actions cl-row2">
        <span class="cl-label">ラベルの数</span>
        <span class="sd-seg">
          <span v-for="n in [1, 2, 3, 4, 5]" :key="n" class="sd-seg-opt mono"
                :class="{ on: useLabels === n }" @click="useLabels = n">{{ n }}</span>
        </span>
      </div>

      <div class="cl-rows">
        <div v-for="(l, i) in LABELS" :key="l.name" class="cl-lab" :class="{ off: i >= useLabels }">
          <span class="cl-name mono">{{ l.name }}</span>
          <span class="cl-vals mono">{{ fmt(l.values) }} 種</span>
          <span class="cl-cum mono">
            {{ i < useLabels ? fmt(LABELS.slice(0, i + 1).reduce((n, x) => n * x.values, 1)) : "—" }}
          </span>
        </div>
      </div>

      <div class="cl-verdict">
        {{ useLabels }} 個で <b class="mono">{{ fmt(series) }}</b> 系列。1 系列 8 バイトなら {{ fmtBytes(bytes) }}(1 時点ぶん)。
        <template v-if="useLabels >= 5">
          user_id を 1 つ足しただけで桁が変わる。値域の広いものはラベルにしない
        </template>
      </div>
    </template>

    <!-- ④ 置き方 -->
    <template v-else>
      <p class="cl-setting mono">
        {{ N_TARGETS }} 対象 / {{ N_TARGETS / PER_NODE }} ノード / 1 対象 {{ SERIES_PER_TARGET }} 系列 = 合計 {{ fmt(TOTAL) }} 系列
      </p>
      <div class="cl-rows">
        <div v-for="l in layouts" :key="l.name" class="cl-place">
          <span class="cl-name">{{ l.name }}</span>
          <span class="cl-col mono">収集器 <b>{{ l.collectors }}</b></span>
          <span class="cl-bar sm">
            <span class="cl-seg ng" :style="{ width: (l.lost / TOTAL) * 100 + '%' }"></span>
          </span>
          <span class="cl-lost mono">失う <b>{{ fmt(l.lost) }}</b></span>
        </div>
      </div>
      <div class="cl-verdict">
        収集器を増やすほど、1 つ落ちて失う範囲は小さくなる。運用の手間はその逆に増えるので、
        両方を小さくはできない
      </div>
    </template>

    <p class="cl-note">
      pull は収集側が叩きに行くので、間隔の中で全対象を叩き切る必要がある。push は対象が送るのでその予算が無いかわりに、
      届かないことから理由を言えない。差の出どころは矢印の向きではなく、居るはずの相手を知っているかどうかになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.cl-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.cl-row2 {
  margin-top: 10px;
}
.cl-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.cl-setting {
  margin: 12px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cl-bar {
  display: flex;
  height: 14px;
  background-color: var(--vp-c-default-soft);
  margin-top: 14px;
}
.cl-bar.sm {
  flex: 1 1 auto;
  height: 10px;
  margin-top: 0;
}
.cl-seg {
  display: block;
  height: 100%;
}
.cl-seg.ok {
  background-color: var(--vp-c-brand-1);
}
.cl-seg.ng {
  background-color: var(--vp-c-danger-1, #d64545);
}
.cl-key {
  display: flex;
  gap: 16px;
  margin-top: 8px;
  font-size: 10px;
  color: var(--vp-c-text-3);
  flex-wrap: wrap;
}
.cl-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 5px;
}
.cl-dot.ok {
  background-color: var(--vp-c-brand-1);
}
.cl-dot.ng {
  background-color: var(--vp-c-danger-1, #d64545);
}
.cl-rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 14px;
}
.cl-plan,
.cl-lab,
.cl-place {
  display: flex;
  align-items: center;
  gap: 10px;
}
.cl-name {
  width: 82px;
  flex: none;
  font-size: 12px;
  color: var(--vp-c-text-1);
}
.cl-detect {
  font-size: 12px;
}
.cl-detect.ok {
  color: var(--vp-c-text-1);
}
.cl-detect.ng {
  color: var(--vp-c-text-3);
}
.cl-vals {
  width: 74px;
  flex: none;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cl-cum {
  font-size: 12.5px;
  color: var(--vp-c-text-1);
}
.cl-lab.off .cl-name,
.cl-lab.off .cl-vals,
.cl-lab.off .cl-cum {
  color: var(--vp-c-text-3);
  opacity: 0.45;
}
.cl-col {
  width: 76px;
  flex: none;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cl-col b,
.cl-lost b {
  font-size: 12.5px;
  color: var(--vp-c-text-1);
}
.cl-lost {
  width: 92px;
  flex: none;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cl-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--vp-c-text-1);
}
.cl-note {
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
