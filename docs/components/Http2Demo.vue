<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// network/http2(Go)を移植。応答サイズの組を、HTTP/1.1(直列)と
// HTTP/2(多重化)で流したときの各応答の完了時刻を並べて比べる。

function ticksH1(sizes: number[]): number[] {
  const done: number[] = [];
  let t = 0;
  for (const s of sizes) {
    t += s;
    done.push(t);
  }
  return done;
}
function ticksH2(sizes: number[]): number[] {
  const rem = [...sizes];
  const done = new Array(sizes.length).fill(0);
  let tick = 0;
  for (;;) {
    let active = false;
    for (let i = 0; i < rem.length; i++) {
      if (rem[i] > 0) {
        active = true;
        tick++;
        rem[i]--;
        if (rem[i] === 0) done[i] = tick;
      }
    }
    if (!active) break;
  }
  return done;
}

const scenarios = [
  { key: "big2small", label: "大1 + 小2", sizes: [10, 1, 1] },
  { key: "big4small", label: "大1 + 小4", sizes: [12, 1, 1, 1, 1] },
  { key: "even", label: "均等 3本", sizes: [4, 4, 4] },
] as const;
const pick = ref(0);
const sizes = computed(() => [...scenarios[pick.value].sizes]);

const h1 = computed(() => ticksH1(sizes.value));
const h2 = computed(() => ticksH2(sizes.value));
const maxTick = computed(() => Math.max(...h1.value, ...h2.value, 1));

// 各ストリームの表示情報。大きい応答(サイズ最大)を除く小さい応答が主役。
const rows = computed(() =>
  sizes.value.map((s, i) => ({
    id: i + 1,
    size: s,
    big: s === Math.max(...sizes.value),
    h1: h1.value[i],
    h2: h2.value[i],
  })),
);

// 小さな応答が H2 でどれだけ早く終わるか(平均短縮)。
const speedup = computed(() => {
  const smalls = rows.value.filter((r) => !r.big);
  if (!smalls.length) return 0;
  const avgH1 = smalls.reduce((a, r) => a + r.h1, 0) / smalls.length;
  const avgH2 = smalls.reduce((a, r) => a + r.h2, 0) / smalls.length;
  return avgH1 / avgH2;
});

const badge = computed(() =>
  speedup.value > 1 ? `小応答が H2 で約 ${speedup.value.toFixed(1)}倍速く完了` : "均等",
);

const note = computed(() => {
  if (speedup.value > 1)
    return `HTTP/1.1 では小さな応答が大きな応答の後ろで待たされる。HTTP/2 は多重化で先に完了させ、小応答は約 ${speedup.value.toFixed(1)}倍速い。ただし大きな応答は少し遅れる(仕事量は同じ、体感を最適化)`;
  return "全応答が同じサイズなら、多重化しても順に終わるだけで差は出にくい。多重化が効くのは応答サイズにばらつきがあるとき";
});

function pctTick(t: number): number {
  return (t / maxTick.value) * 100;
}
</script>

<template>
  <DemoShell title="HTTP/2 多重化" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="(sc, i) in scenarios"
          :key="sc.key"
          class="sd-seg-opt"
          :class="{ on: pick === i }"
          @click="pick = i"
          >{{ sc.label }}</span
        >
      </span>
    </div>

    <div class="h2-grid">
      <div class="h2-head">
        <span class="h2-h-name">応答</span>
        <span class="h2-h-proto">HTTP/1.1(直列)</span>
        <span class="h2-h-proto">HTTP/2(多重化)</span>
      </div>
      <div v-for="r in rows" :key="r.id" class="h2-row">
        <span class="h2-name mono">S{{ r.id }} <small>({{ r.size }}F{{ r.big ? "・大" : "" }})</small></span>
        <span class="h2-track">
          <span class="h2-bar h1" :style="{ width: pctTick(r.h1) + '%' }"></span>
          <span class="h2-tick mono">{{ r.h1 }}</span>
        </span>
        <span class="h2-track">
          <span class="h2-bar h2" :class="{ small: !r.big }" :style="{ width: pctTick(r.h2) + '%' }"></span>
          <span class="h2-tick mono">{{ r.h2 }}</span>
        </span>
      </div>
      <div class="h2-axis">完了 tick(短いほど早く終わる)→</div>
    </div>

    <p class="h2-note">{{ note }}</p>

    <p class="h2-legend">
      各応答をフレーム数で表し、1 フレーム送信を 1 tick と数えている。HTTP/1.1 は応答を要求順に直列で返すので、
      小さな応答が大きな応答の後ろで待つ(ヘッドオブラインブロッキング)。HTTP/2 は 1 本の接続でフレームを
      交互に流すので、小さな応答が先に完了する。総 tick 数は変わらない。小さく重要なリソースを先に届けて、
      ページが早く使えるようにするのが多重化の狙いだ。
    </p>
  </DemoShell>
</template>

<style scoped>
.h2-grid {
  margin-top: 16px;
}
.h2-head,
.h2-row {
  display: grid;
  grid-template-columns: 90px 1fr 1fr;
  gap: 10px;
  align-items: center;
}
.h2-head {
  margin-bottom: 8px;
}
.h2-h-name,
.h2-h-proto {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.h2-row {
  height: 30px;
}
.h2-name {
  font-size: 12px;
  color: var(--vp-c-text-1);
}
.h2-name small {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.h2-track {
  position: relative;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 18px;
}
.h2-bar {
  height: 14px;
  border-radius: 0;
  min-width: 2px;
}
.h2-bar.h1 {
  background-color: var(--vp-c-text-3);
}
.h2-bar.h2 {
  background-color: var(--vp-c-brand-1);
}
.h2-bar.h2.small {
  background-color: var(--vp-c-green-1);
}
.h2-tick {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.h2-axis {
  margin-top: 8px;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
  text-align: right;
}
.h2-note {
  margin: 12px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.h2-legend {
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
