<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/scaletrigger(Go)を移植。実時間も乱数も使わない。
type View = "shape" | "inflight" | "partition";
const view = ref<View>("inflight");

// ---- ① 取れる情報の違い ----
const TOTAL = 200;
const done = ref(50);
const logLag = computed(() => TOTAL - done.value);
const queueVisible = computed(() => TOTAL - done.value);

// ---- ② 見えている数 vs 終わっていない数 ----
const PER_REPLICA = 10;
const inFlight = ref(200);
const visible = computed(() => TOTAL - inFlight.value);
const outstanding = computed(() => visible.value + inFlight.value);
const ceilDiv = (a: number, b: number) => (a <= 0 || b <= 0 ? 0 : Math.ceil(a / b));
const byVisible = computed(() => ceilDiv(visible.value, PER_REPLICA));
const byOutstanding = computed(() => ceilDiv(outstanding.value, PER_REPLICA));

// ---- ③ 分割の上限 ----
const partitions = ref(10);
const replicas = ref(20);
const effective = computed(() => Math.min(replicas.value, partitions.value));
const idle = computed(() => replicas.value - effective.value);
const throughput = computed(() => effective.value * PER_REPLICA);
const BACKLOG = 2000;
const drain = computed(() => (throughput.value <= 0 ? -1 : Math.ceil(BACKLOG / throughput.value)));

const badge = computed(() => {
  if (view.value === "shape") return `${TOTAL} 件中 ${done.value} 件を処理した状態`;
  if (view.value === "inflight") return `処理中 ${inFlight.value} 件`;
  return `分割 ${partitions.value} / レプリカ ${replicas.value}`;
});
const badgeTone = computed(() => {
  if (view.value === "inflight" && byVisible.value < byOutstanding.value) return "ng";
  if (view.value === "partition" && idle.value > 0) return "ng";
  return undefined;
});
</script>

<template>
  <DemoShell title="滞留から何が読めるか" :badge="badge" :badge-tone="badgeTone">
    <div class="st-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: view === 'shape' }" @click="view = 'shape'">取れる情報</span>
        <span class="sd-seg-opt" :class="{ on: view === 'inflight' }" @click="view = 'inflight'">処理中の扱い</span>
        <span class="sd-seg-opt" :class="{ on: view === 'partition' }" @click="view = 'partition'">分割の上限</span>
      </span>
    </div>

    <!-- ① 取れる情報 -->
    <template v-if="view === 'shape'">
      <div class="st-actions st-row2">
        <span class="st-label mono">処理した件数 {{ done }}</span>
        <input v-model.number="done" type="range" min="0" :max="TOTAL" step="10" class="st-range" />
      </div>

      <div class="st-rows">
        <div class="st-item">
          <span class="st-name">ログ型</span>
          <span class="st-facts mono">
            書いた <b>{{ TOTAL }}</b> ・ 読んだ <b>{{ done }}</b> ・ ラグ <b>{{ logLag }}</b>
          </span>
        </div>
        <div class="st-item">
          <span class="st-name">キュー型</span>
          <span class="st-facts mono">
            書いた <b class="na">取れない</b> ・ 読んだ <b class="na">取れない</b> ・ 残り <b>{{ queueVisible }}</b>
          </span>
        </div>
      </div>

      <div class="st-verdict">
        残りはどちらも {{ logLag }} で一致する。違うのは、そこに至る位置を持っているかどうか。
        読んだら消える形では「これまで何件流れたか」が残らないので、引き算そのものが定義できない
      </div>
    </template>

    <!-- ② 処理中の扱い -->
    <template v-else-if="view === 'inflight'">
      <div class="st-actions st-row2">
        <span class="st-label mono">処理中 {{ inFlight }} 件</span>
        <input v-model.number="inFlight" type="range" min="0" :max="TOTAL" step="20" class="st-range" />
      </div>

      <div class="st-bar">
        <span class="st-seg vis" :style="{ width: (visible / TOTAL) * 100 + '%' }"></span>
        <span class="st-seg inf" :style="{ width: (inFlight / TOTAL) * 100 + '%' }"></span>
      </div>
      <div class="st-key mono">
        <span><i class="st-dot vis"></i>見えている {{ visible }}</span>
        <span><i class="st-dot inf"></i>処理中 {{ inFlight }}</span>
      </div>

      <div class="st-rows">
        <div class="st-item">
          <span class="st-name">見えている数で決める</span>
          <span class="st-num mono" :class="{ ng: byVisible < byOutstanding }"><b>{{ byVisible }}</b> 個</span>
        </div>
        <div class="st-item">
          <span class="st-name">終わっていない数で決める</span>
          <span class="st-num mono"><b>{{ byOutstanding }}</b> 個</span>
        </div>
      </div>

      <div class="st-verdict">
        <template v-if="visible === 0 && inFlight > 0">
          全部が処理中なので、見えている数は 0。ここで縮めると、まだ {{ outstanding }} 件の仕事が残っているのに
          レプリカが消える。しかも処理に失敗すれば、この {{ inFlight }} 件は見えている側へ戻ってくる
        </template>
        <template v-else-if="byVisible < byOutstanding">
          見えている数だけだと {{ byVisible }} 個。処理中の {{ inFlight }} 件を数に入れると {{ byOutstanding }} 個になる
        </template>
        <template v-else>
          処理中が 0 なので、どちらで見ても同じ答えになる。処理が一瞬で終わる仕事ではこの穴が開かない
        </template>
      </div>
    </template>

    <!-- ③ 分割の上限 -->
    <template v-else>
      <div class="st-actions st-row2">
        <span class="st-label mono">分割 {{ partitions }}</span>
        <input v-model.number="partitions" type="range" min="1" max="50" step="1" class="st-range" />
      </div>
      <div class="st-actions st-row2">
        <span class="st-label mono">レプリカ {{ replicas }}</span>
        <input v-model.number="replicas" type="range" min="1" max="50" step="1" class="st-range" />
      </div>

      <div class="st-bar">
        <span class="st-seg vis" :style="{ width: (effective / Math.max(replicas, 1)) * 100 + '%' }"></span>
        <span class="st-seg idle" :style="{ width: (idle / Math.max(replicas, 1)) * 100 + '%' }"></span>
      </div>
      <div class="st-key mono">
        <span><i class="st-dot vis"></i>働く {{ effective }}</span>
        <span><i class="st-dot idle"></i>遊ぶ {{ idle }}</span>
        <span>捌ける {{ throughput }} 件/単位時間</span>
      </div>

      <div class="st-verdict">
        <template v-if="idle > 0">
          分割が {{ partitions }} なので、働くのは {{ effective }} 個だけ。{{ idle }} 個は割り当てが無く遊ぶ。
          滞留 {{ BACKLOG }} 件を捌くのに {{ drain }} 単位時間かかり、これは {{ partitions }} 個に減らしても変わらない
        </template>
        <template v-else>
          全部のレプリカに割り当てがある。滞留 {{ BACKLOG }} 件を {{ drain }} 単位時間で捌く。
          ここから速くしたければ、レプリカでなく分割を増やす
        </template>
      </div>
    </template>

    <p class="st-note">
      式は現在のレプリカ数に依存しないので、滞留が青天井なら要求も青天井になる。だが実際に働ける数には
      物理的な上限があり、指標の側には現れない。上限の無い指標を使うときほど、外から蓋をする必要がある。
    </p>
  </DemoShell>
</template>

<style scoped>
.st-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.st-row2 {
  margin-top: 12px;
}
.st-label {
  width: 140px;
  flex: none;
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.st-range {
  flex: 1 1 auto;
  max-width: 260px;
  accent-color: var(--vp-c-brand-1);
}
.st-bar {
  display: flex;
  height: 14px;
  background-color: var(--vp-c-default-soft);
  margin-top: 14px;
}
.st-seg {
  display: block;
  height: 100%;
}
.st-seg.vis {
  background-color: var(--vp-c-brand-1);
}
.st-seg.inf {
  background-color: var(--vp-c-text-3);
}
.st-seg.idle {
  background-color: var(--vp-c-danger-1, #d64545);
}
.st-key {
  display: flex;
  gap: 16px;
  margin-top: 8px;
  font-size: 10px;
  color: var(--vp-c-text-3);
  flex-wrap: wrap;
}
.st-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 5px;
}
.st-dot.vis {
  background-color: var(--vp-c-brand-1);
}
.st-dot.inf {
  background-color: var(--vp-c-text-3);
}
.st-dot.idle {
  background-color: var(--vp-c-danger-1, #d64545);
}
.st-rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 14px;
}
.st-item {
  display: flex;
  align-items: center;
  gap: 10px;
}
.st-name {
  width: 176px;
  flex: none;
  font-size: 12px;
  color: var(--vp-c-text-1);
}
.st-facts {
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.st-facts b {
  color: var(--vp-c-text-1);
  font-size: 12.5px;
}
.st-facts b.na {
  color: var(--vp-c-text-3);
  font-weight: 400;
}
.st-num {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.st-num b {
  font-size: 13px;
  color: var(--vp-c-text-1);
}
.st-num.ng b {
  color: var(--vp-c-danger-1, #d64545);
}
.st-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--vp-c-text-1);
}
.st-note {
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
