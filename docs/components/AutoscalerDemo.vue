<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/autoscaler(Go)を移植。HPA の式に、許容誤差と安定化ウィンドウを
// 付けた場合と外した場合を切り替えて、揺れへの応答の違いを見せる。

const TARGET = 50;
const MIN = 1;
const MAX = 10;
const TOLERANCE = 10; // 目標の ±10%
const WINDOW = 3; // 縮小に必要な観測回数

// 負荷の揺らぎ。乱数でなく固定列を回すので、結果は毎回同じになる。
const NOISE = [0, 8, -6, 5, -8, 3, 7, -4, -7, 6];
const LOADS = { normal: 100, burst: 400, night: 20 } as const;
type LoadKey = keyof typeof LOADS;

interface Tick {
  t: number;
  util: number;
  replicas: number;
  raw: number;
}

const loadKey = ref<LoadKey>("normal");
const guarded = ref(true); // 許容誤差と安定化ウィンドウを効かせるか
const replicas = ref(2);
const tick = ref(0);
const history = ref<number[]>([]); // 直近の提案
const ticks = ref<Tick[]>([]);
const reason = ref("まだ判断していない");

function ceilDiv(a: number, b: number) {
  return Math.ceil(a / b);
}

// その周期の実負荷。基準値に固定の揺らぎを乗せる。
function loadAt(t: number) {
  return Math.max(0, LOADS[loadKey.value] + NOISE[t % NOISE.length]);
}

function step() {
  const load = loadAt(tick.value);
  const util = Math.floor(load / replicas.value); // 1 レプリカあたりの使用率
  const raw = ceilDiv(replicas.value * util, TARGET); // HPA の式

  history.value = [...history.value, raw].slice(-WINDOW);

  let next = replicas.value;
  const clamped = Math.min(MAX, Math.max(MIN, raw));

  if (guarded.value && Math.abs(util - TARGET) * 100 <= TARGET * TOLERANCE) {
    reason.value = `使用率 ${util}% は目標 ${TARGET}% の許容誤差の内側。動かさない`;
  } else if (clamped > replicas.value) {
    next = clamped;
    reason.value = `使用率 ${util}% が目標を超えた。${replicas.value} → ${next} へすぐ拡大`;
  } else if (clamped < replicas.value) {
    if (guarded.value) {
      // ウィンドウが埋まるまでは現在の数も候補に含める(起動直後に縮めない)。
      let m = history.value.length < WINDOW ? replicas.value : 0;
      for (const v of history.value) if (v > m) m = v;
      const stable = Math.min(MAX, Math.max(MIN, m));
      if (stable >= replicas.value) {
        reason.value = `使用率 ${util}% は低いが、縮小の提案がまだ安定していない。据え置く`;
      } else {
        next = stable;
        reason.value = `低い使用率が ${WINDOW} 周期続いた。${replicas.value} → ${next} へ縮小`;
      }
    } else {
      next = clamped;
      reason.value = `使用率 ${util}% が目標を下回った。${replicas.value} → ${next} へ即縮小`;
    }
  } else {
    reason.value = `式の結果が現在と同じ ${replicas.value} 個。動かさない`;
  }

  ticks.value = [...ticks.value, { t: tick.value, util, replicas: replicas.value, raw }].slice(-24);
  replicas.value = next;
  tick.value++;
}

function step10() {
  for (let i = 0; i < 10; i++) step();
}
function setLoad(k: LoadKey) {
  loadKey.value = k;
}
function reset() {
  replicas.value = 2;
  tick.value = 0;
  history.value = [];
  ticks.value = [];
  reason.value = "まだ判断していない";
}

// 直近 6 周期でレプリカ数が何回変わったか。変動の多さを見る指標。
const flaps = computed(() => {
  const recent = ticks.value.slice(-7).map((x) => x.replicas);
  let n = 0;
  for (let i = 1; i < recent.length; i++) if (recent[i] !== recent[i - 1]) n++;
  return n;
});
const lastUtil = computed(() => (ticks.value.length ? ticks.value[ticks.value.length - 1].util : 0));
const settled = computed(() => ticks.value.length >= 4 && flaps.value === 0);
const badge = computed(() => `${replicas.value} レプリカ / 使用率 ${lastUtil.value}%`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  flaps.value >= 3 ? "ng" : settled.value ? "ok" : "neutral",
);
const barH = (util: number) => Math.min(100, Math.floor((util / 200) * 100));
</script>

<template>
  <DemoShell title="水平オートスケール(HPA)" :badge="badge" :badge-tone="badgeTone">
    <div class="as-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: loadKey === 'night' }" @click="setLoad('night')">深夜(負荷小)</span>
        <span class="sd-seg-opt" :class="{ on: loadKey === 'normal' }" @click="setLoad('normal')">平常</span>
        <span class="sd-seg-opt" :class="{ on: loadKey === 'burst' }" @click="setLoad('burst')">バースト</span>
      </span>
      <span class="as-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="as-actions as-second">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: guarded }" @click="guarded = true">許容誤差と安定化あり</span>
        <span class="sd-seg-opt" :class="{ on: !guarded }" @click="guarded = false">式そのまま</span>
      </span>
      <button class="sd-btn sd-btn--primary" @click="step">1周期進める</button>
      <button class="sd-btn" @click="step10">10周期進める</button>
    </div>

    <div class="as-chart">
      <div v-if="ticks.length" class="as-target" :style="{ bottom: barH(TARGET) + '%' }">
        <span class="as-target-l mono">目標 {{ TARGET }}%</span>
      </div>
      <div v-for="x in ticks" :key="x.t" class="as-col">
        <div class="as-bar" :class="x.util > TARGET ? 'hot' : 'cool'" :style="{ height: barH(x.util) + '%' }" />
        <span class="as-rep mono">{{ x.replicas }}</span>
      </div>
      <div v-if="ticks.length === 0" class="as-empty">周期を進めると、使用率(棒)とレプリカ数(下の数字)が並ぶ</div>
    </div>

    <div class="as-verdict" :class="flaps >= 3 ? 'bad' : settled ? 'ok' : 'neutral'">
      <template v-if="flaps >= 3">
        直近 6 周期で {{ flaps }} 回もレプリカ数が変わっている(flapping)。揺れに素直に従いすぎている
      </template>
      <template v-else-if="settled">落ち着いている: {{ replicas }} レプリカで目標付近を保っている</template>
      <template v-else-if="ticks.length">直近 6 周期の変更は {{ flaps }} 回</template>
      <template v-else>周期を進めると、判断とその理由が出る</template>
    </div>

    <div v-if="ticks.length" class="as-log mono">直前の判断: {{ reason }}</div>

    <p class="as-legend">
      負荷には固定の揺らぎが乗っている。「式そのまま」に切り替えて平常負荷で 10 周期進めると、
      わずかな揺れにレプリカ数が反応して増減を繰り返す(flapping)。「許容誤差と安定化あり」に戻すと、
      目標 ±{{ TOLERANCE }}% の揺れを無視するので動かない。バーストに切り替えると 1 周期で必要数まで跳び、
      深夜に戻しても {{ WINDOW }} 周期待ってから縮む。上がるのは速く、下がるのは遅い。
    </p>
  </DemoShell>
</template>

<style scoped>
.as-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.as-second {
  margin-top: 8px;
}
.as-spacer {
  flex: 1;
}
.as-chart {
  position: relative;
  display: flex;
  align-items: flex-end;
  gap: 3px;
  height: 150px;
  margin-top: 18px;
  padding: 0 6px 18px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.as-target {
  position: absolute;
  left: 0;
  right: 0;
  border-top: 1px dashed var(--vp-c-text-3);
  margin-bottom: 18px;
}
.as-target-l {
  position: absolute;
  right: 4px;
  top: -13px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.as-col {
  position: relative;
  flex: 1;
  min-width: 8px;
  height: 100%;
  display: flex;
  align-items: flex-end;
}
.as-bar {
  width: 100%;
  min-height: 2px;
}
.as-bar.cool {
  background-color: var(--vp-c-brand-1);
}
.as-bar.hot {
  background-color: var(--vp-c-danger-1);
}
.as-rep {
  position: absolute;
  bottom: -16px;
  left: 0;
  right: 0;
  text-align: center;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.as-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.as-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.as-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.as-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.as-log {
  margin-top: 8px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.as-legend {
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
