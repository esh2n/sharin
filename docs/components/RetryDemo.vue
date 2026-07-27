<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// resilience/retry(Go)をブラウザに移植。
// 「バックオフ」: 1クライアントのリトライ、待ちが指数的に伸びる様子。
// 「リトライ嵐」: 多数クライアントの再送時刻分布を、ジッター有無で比較。

const BASE = 100;
const MAX = 1600;
const MULT = 2;

function rawDelay(attempt: number): number {
  let d = BASE;
  for (let i = 0; i < attempt; i++) {
    d *= MULT;
    if (d >= MAX) return MAX;
  }
  return Math.min(d, MAX);
}

// 決定的擬似乱数
function makeRand(seed: number) {
  let s = BigInt(seed) * 2862933555777941757n + 1n;
  return () => {
    s = (s * 6364136223846793005n + 1442695040888963407n) & 0xffffffffffffffffn;
    return Number((s >> 33n) & 0xffffffffn);
  };
}

// --- バックオフ ---
const SUCCEED_AT = 4; // 4回目で成功
const backoffFrames = computed(() => {
  const frames: { attempt: number; delay: number; cumulative: number; result: string }[] = [];
  let cum = 0;
  for (let a = 0; a < 5; a++) {
    const success = a + 1 === SUCCEED_AT;
    const delay = success || a + 1 > SUCCEED_AT ? 0 : rawDelay(a);
    frames.push({
      attempt: a + 1,
      delay: success ? 0 : rawDelay(a),
      cumulative: cum,
      result: success ? "成功" : "失敗",
    });
    if (success) break;
    cum += rawDelay(a);
  }
  return frames;
});
const at = ref(0);

// --- リトライ嵐 ---
const N_CLIENTS = 120;
const jitterModes = [
  { key: "none", label: "ジッターなし" },
  { key: "full", label: "full jitter" },
] as const;
const jitterPick = ref(1);
const BUCKETS = 16;

const stormHist = computed(() => {
  const raw = rawDelay(3); // 全員 attempt 3 = 800
  const hist = new Array(BUCKETS).fill(0);
  const rand = makeRand(7);
  for (let c = 0; c < N_CLIENTS; c++) {
    let delay: number;
    if (jitterMode.value === "none") {
      delay = raw;
    } else {
      delay = rand() % (raw + 1); // [0, raw]
    }
    const bucket = Math.min(Math.floor((delay / raw) * BUCKETS), BUCKETS - 1);
    hist[bucket]++;
  }
  return hist;
});
const jitterMode = computed(() => jitterModes[jitterPick.value].key);
const maxBar = computed(() => Math.max(...stormHist.value));

const modes = [
  { key: "backoff", label: "バックオフ" },
  { key: "storm", label: "リトライ嵐" },
] as const;
const mode = ref<"backoff" | "storm">("backoff");
function setMode(m: "backoff" | "storm") {
  mode.value = m;
  at.value = 0;
}

const cur = computed(() => backoffFrames.value[at.value]);
const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < backoffFrames.value.length - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = backoffFrames.value.length - 1; }

const note = computed(() => {
  if (mode.value === "backoff") {
    const f = cur.value;
    if (f.result === "成功") return `試行 ${f.attempt}: 成功。ここまでの待ち合計 ${f.cumulative}。一時的失敗が解消され、リトライが報われた`;
    return `試行 ${f.attempt}: 失敗。次の再送まで ${f.delay} 待つ(前回の ${MULT} 倍、上限 ${MAX})。間隔を空けて相手を追い打ちしない`;
  }
  if (jitterMode.value === "none") {
    return `ジッターなし: ${N_CLIENTS} クライアント全員が attempt 3 の待ち(800)ちょうどで再送する。全員が同時刻に集中 = 負荷スパイク(リトライ嵐)`;
  }
  return `full jitter: 各クライアントが [0, 800] の乱数で待つ。再送時刻が広く散り、負荷スパイクが消える。同時失敗しても衝突しない`;
});

const badge = computed(() =>
  mode.value === "backoff" ? `試行 ${cur.value.attempt}` : jitterModes[jitterPick.value].label,
);
</script>

<template>
  <DemoShell title="リトライとバックオフ" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
      <span v-if="mode === 'storm'" class="spacer" />
      <span v-if="mode === 'storm'" class="sd-seg">
        <span v-for="(j, i) in jitterModes" :key="j.key" class="sd-seg-opt" :class="{ on: jitterPick === i }" @click="jitterPick = i">{{ j.label }}</span>
      </span>
    </div>

    <!-- バックオフ -->
    <div v-if="mode === 'backoff'" class="rt-backoff">
      <div v-for="(f, i) in backoffFrames" :key="i" class="rt-attempt" :class="{ on: i === at, dim: i > at, ok: f.result === '成功' }">
        <span class="rt-attempt-n mono">試行{{ f.attempt }}</span>
        <span class="rt-attempt-res" :class="f.result === '成功' ? 'good' : 'bad'">{{ f.result }}</span>
        <span v-if="f.result !== '成功' && i < backoffFrames.length - 1" class="rt-wait">
          <span class="rt-wait-bar" :style="{ width: (f.delay / MAX) * 100 + '%' }"></span>
          <span class="rt-wait-label mono">wait {{ f.delay }}</span>
        </span>
      </div>
    </div>

    <!-- リトライ嵐 -->
    <div v-else class="rt-storm">
      <div class="rt-storm-head">再送時刻の分布({{ N_CLIENTS }} クライアント・全員 attempt 3)</div>
      <div class="rt-hist">
        <div v-for="(count, i) in stormHist" :key="i" class="rt-bar-col">
          <span class="rt-bar" :class="{ spike: jitterMode === 'none' && count > 0 }" :style="{ height: (count / maxBar) * 100 + '%' }"></span>
        </div>
      </div>
      <div class="rt-axis"><span>早い</span><span>再送時刻 →</span><span>遅い(800)</span></div>
    </div>

    <p class="rt-note">{{ note }}</p>

    <div class="rt-foot" v-if="mode === 'backoff'">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="rt-nav mono">{{ at + 1 }} / {{ backoffFrames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1回リトライ</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="rt-legend">
      指数バックオフは待ちを試行ごとに伸ばし、過負荷の相手を追い打ちしない。だが全クライアントが
      同じ間隔で再送すると同時刻に集中する(リトライ嵐)。ジッターで待ちに乱数を足すと再送時刻が散り、
      負荷スパイクが消える。恒久的失敗(4xx)は待っても直らないので再送しない。
    </p>
  </DemoShell>
</template>

<style scoped>
.rt-backoff {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.rt-attempt {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.rt-attempt.dim { opacity: 0.35; }
.rt-attempt.on { border-left-color: var(--vp-c-brand-1); }
.rt-attempt.ok.on { border-left-color: var(--vp-c-green-1); }
.rt-attempt-n {
  width: 52px;
  font-size: 12px;
  font-weight: 700;
}
.rt-attempt-res {
  width: 40px;
  font-size: 12px;
  font-weight: 700;
}
.rt-attempt-res.good { color: var(--vp-c-green-1); }
.rt-attempt-res.bad { color: var(--vp-c-danger-1); }
.rt-wait {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
}
.rt-wait-bar {
  height: 10px;
  background-color: var(--vp-c-warning-1);
  min-width: 2px;
  border-radius: 0;
}
.rt-wait-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.rt-storm {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px;
  background-color: var(--vp-c-bg-soft);
}
.rt-storm-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.rt-hist {
  display: flex;
  align-items: flex-end;
  gap: 2px;
  height: 100px;
}
.rt-bar-col {
  flex: 1;
  height: 100%;
  display: flex;
  align-items: flex-end;
}
.rt-bar {
  width: 100%;
  background-color: var(--vp-c-brand-1);
  border-radius: 0;
  min-height: 0;
}
.rt-bar.spike {
  background-color: var(--vp-c-danger-1);
}
.rt-axis {
  display: flex;
  justify-content: space-between;
  margin-top: 6px;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.rt-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.rt-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.rt-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.rt-legend {
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
