<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/throttle(Go)を移植。100 tick ごとに 40 tick ぶんだけ CPU を使える。
const QUOTA = 40;
const PERIOD = 100;
const SHOW = 300; // 描く範囲

const ncpu = ref(8);
const carry = ref(false);

interface Tick {
  t: number;
  running: number; // この tick に走った本数
  stalled: boolean; // 走りたいのに枠が無かった
}

interface Run {
  ticks: Tick[];
  done: number;
  throttled: number;
  exhausted: number; // 最初の期間で枠を使い切った時刻
}

function run(n: number, burstCap: number): Run {
  const NEED = 10;
  const left = Array(8).fill(NEED);
  const ticks: Tick[] = [];
  let remaining = 0;
  let exhausted = -1;
  let throttled = 0;
  let done = 0;

  for (let t = 0; t < SHOW; t++) {
    if (t % PERIOD === 0) {
      const c = t === 0 ? 0 : Math.min(remaining, burstCap);
      remaining = QUOTA + c;
    }
    let ran = 0;
    let stalled = false;
    for (let i = 0; i < left.length; i++) {
      if (left[i] === 0) continue;
      if (ran >= n) break;
      if (remaining === 0) {
        throttled++;
        stalled = true;
        continue;
      }
      remaining--;
      if (remaining === 0 && exhausted < 0) exhausted = t + 1;
      ran++;
      left[i]--;
      if (left[i] === 0) done = t + 1;
    }
    ticks.push({ t, running: ran, stalled });
    if (left.every((x) => x === 0)) break;
  }
  return { ticks, done, throttled, exhausted };
}

const result = computed(() => run(ncpu.value, carry.value ? 120 : 0));

// 期間ごとに帯を作る。仕事が終わって tick が無い区間も「何もしていない」枠として
// 残さないと、最後の期間だけ印が太くなって走りっぱなしに見える。
type Cell = "run" | "stall" | "idle";
const periods = computed(() => {
  const out: { start: number; used: number; stalled: number; cells: Cell[] }[] = [];
  for (const tk of result.value.ticks) {
    const i = Math.floor(tk.t / PERIOD);
    if (!out[i]) out[i] = { start: i * PERIOD, used: 0, stalled: 0, cells: Array(PERIOD).fill("idle") };
    out[i].cells[tk.t % PERIOD] = tk.running > 0 ? "run" : tk.stalled ? "stall" : "idle";
    out[i].used += tk.running > 0 ? 1 : 0;
    out[i].stalled += tk.stalled ? 1 : 0;
  }
  return out.filter(Boolean);
});

const badge = computed(() => `${result.value.done} tick で完了`);
const verdict = computed(() => {
  const r = result.value;
  if (r.exhausted < 0) return "枠を使い切らずに終わった。止められていない";
  const idle = PERIOD - r.exhausted;
  return `最初の期間は ${r.exhausted} tick 目で枠を使い切り、残りの ${idle} tick は走りたくても走れない。同時に走らせる本数を増やすほど、使い切るのが早くなる`;
});
</script>

<template>
  <DemoShell title="CPU スロットリング(cpu.max)" :badge="badge" badge-tone="neutral">
    <div class="th-actions">
      <span class="th-label">同時に走らせる本数</span>
      <span class="sd-seg">
        <span v-for="n in [1, 2, 4, 8]" :key="n" class="sd-seg-opt" :class="{ on: ncpu === n }" @click="ncpu = n">
          {{ n }}
        </span>
      </span>
      <span class="spacer"></span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !carry }" @click="carry = false">使い残しは消える</span>
        <span class="sd-seg-opt" :class="{ on: carry }" @click="carry = true">使い残しを繰り越す</span>
      </span>
    </div>

    <p class="th-setting mono">
      cpu.max = {{ QUOTA }} / {{ PERIOD }} ・ 合計 80 tick ぶんの仕事を 8 本に分けて同時に出す
    </p>

    <div class="th-track">
      <div v-for="p in periods" :key="p.start" class="th-period">
        <div class="th-bar">
          <span v-for="(c, i) in p.cells" :key="i" class="th-tick" :class="c"></span>
        </div>
        <div class="th-legend mono">
          {{ p.start }}〜 走った {{ p.used }} / 止められた {{ p.stalled }}
        </div>
      </div>
    </div>

    <div class="th-verdict">{{ verdict }}</div>

    <p class="th-note">
      濃い印が走った tick、薄い印が「走りたいのに枠が無い」tick。メモリの上限なら断って終わりだが、
      CPU は断れないので、次の期間が来るまで止めるしかない。使い残しを繰り越せるようにすると、
      暇だったあとの一時的な集中を吸える。
    </p>
  </DemoShell>
</template>

<style scoped>
.th-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.th-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.th-setting {
  margin: 12px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.th-track {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 10px;
}
.th-bar {
  display: flex;
  gap: 1px;
  height: 20px;
}
.th-tick {
  flex: 1 1 0;
  min-width: 0;
}
.th-tick.run {
  background-color: var(--vp-c-brand-1);
}
.th-tick.stall {
  background-color: var(--vp-c-default-soft);
}
.th-tick.idle {
  background-color: var(--vp-c-bg-soft);
}
.th-legend {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.th-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--vp-c-text-1);
}
.th-note {
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
