<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// resilience/circuitbreaker(Go)をブラウザに移植。
// 依存先の健康状態を切り替えながら、closed→open→half-open→closed/open を追う。

type State = "closed" | "open" | "half-open";

const FAIL_THRESHOLD = 3;
const SUCCESS_THRESHOLD = 2;
const OPEN_TIMEOUT = 10;

interface Snapshot {
  state: State;
  failures: number;
  successes: number;
  now: number;
  openedAt: number;
  depHealthy: boolean;
  log: string;
}

// 決定的なシナリオ: 一連の操作列。
type Op = { kind: "call"; healthy: boolean } | { kind: "advance"; d: number };

const scenario: Op[] = [
  { kind: "call", healthy: true }, // ok
  { kind: "call", healthy: false }, // fail 1
  { kind: "call", healthy: false }, // fail 2
  { kind: "call", healthy: false }, // fail 3 → open
  { kind: "call", healthy: false }, // fail fast
  { kind: "call", healthy: false }, // fail fast
  { kind: "advance", d: 10 }, // → half-open eligible
  { kind: "call", healthy: false }, // half-open probe fails → open
  { kind: "advance", d: 10 }, // → half-open eligible
  { kind: "call", healthy: true }, // probe ok (1 success)
  { kind: "call", healthy: true }, // 2 success → closed
  { kind: "call", healthy: true }, // ok, running normally
];

function simulate(): Snapshot[] {
  let state: State = "closed";
  let failures = 0, successes = 0, now = 0, openedAt = 0;
  const snaps: Snapshot[] = [
    { state, failures, successes, now, openedAt, depHealthy: true, log: "初期状態: closed。依存先は健康。呼び出しを通しながら失敗を数える" },
  ];

  const maybeHalfOpen = () => {
    if (state === "open" && now - openedAt >= OPEN_TIMEOUT) {
      state = "half-open";
      successes = 0;
    }
  };

  for (const op of scenario) {
    if (op.kind === "advance") {
      now += op.d;
      maybeHalfOpen();
      snaps.push({
        state, failures, successes, now, openedAt, depHealthy: true,
        log: state === "half-open"
          ? `時計を +${op.d}(now=${now})。タイムアウト経過 → half-open。次の1本で回復を試す`
          : `時計を +${op.d}(now=${now})`,
      });
      continue;
    }
    maybeHalfOpen();
    let log = "";
    const healthy = op.healthy;
    if (state === "open") {
      log = "open のため呼び出しを弾いた(fail fast)。依存先には届かず、待ちも発生しない";
    } else if (state === "half-open") {
      if (healthy) {
        successes++;
        if (successes >= SUCCESS_THRESHOLD) {
          state = "closed";
          failures = 0;
          successes = 0;
          log = `half-open の試行が成功(連続${SUCCESS_THRESHOLD})→ closed。通常運転に復帰`;
        } else {
          log = `half-open の試行が成功(${successes}/${SUCCESS_THRESHOLD})。まだ様子見`;
        }
      } else {
        state = "open";
        openedAt = now;
        failures = 0;
        successes = 0;
        log = "half-open の試行が失敗 → 即 open に戻る。また待つ";
      }
    } else {
      // closed
      if (healthy) {
        failures = 0;
        log = "呼び出し成功。連続失敗カウントをリセット";
      } else {
        failures++;
        if (failures >= FAIL_THRESHOLD) {
          state = "open";
          openedAt = now;
          failures = 0;
          log = `連続${FAIL_THRESHOLD}失敗 → open。以後は fail fast で資源を守る`;
        } else {
          log = `呼び出し失敗(連続${failures}/${FAIL_THRESHOLD})。まだ closed`;
        }
      }
    }
    snaps.push({ state, failures, successes, now, openedAt, depHealthy: healthy, log });
  }
  return snaps;
}

const snaps = simulate();
const at = ref(0);
const cur = computed(() => snaps[at.value]);

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < snaps.length - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = snaps.length - 1; }

const stateTone = (s: State) => (s === "closed" ? "ok" : s === "open" ? "ng" : "warn");
const badge = computed(() => cur.value.state);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  cur.value.state === "closed" ? "ok" : cur.value.state === "open" ? "ng" : "neutral",
);
const states: State[] = ["closed", "open", "half-open"];
</script>

<template>
  <DemoShell title="サーキットブレーカー" :badge="badge" :badge-tone="badgeTone">
    <div class="cb-states">
      <div v-for="s in states" :key="s" class="cb-state" :class="[stateTone(s), { on: cur.state === s }]">
        <span class="cb-state-name">{{ s }}</span>
        <span class="cb-state-desc">{{ s === 'closed' ? '通す・数える' : s === 'open' ? '即失敗' : '1本試す' }}</span>
      </div>
    </div>

    <div class="cb-panel">
      <div class="cb-row">
        <span class="cb-label">依存先</span>
        <span class="cb-dep mono" :class="cur.depHealthy ? 'up' : 'down'">{{ cur.depHealthy ? "健康" : "障害中" }}</span>
      </div>
      <div class="cb-row">
        <span class="cb-label">連続失敗</span>
        <span class="cb-dots">
          <span v-for="n in FAIL_THRESHOLD" :key="n" class="cb-dot" :class="{ lit: n <= cur.failures, danger: cur.state === 'closed' }"></span>
        </span>
        <span class="cb-count mono">{{ cur.failures }}/{{ FAIL_THRESHOLD }}</span>
      </div>
      <div class="cb-row" v-if="cur.state === 'half-open'">
        <span class="cb-label">試行成功</span>
        <span class="cb-dots">
          <span v-for="n in SUCCESS_THRESHOLD" :key="n" class="cb-dot" :class="{ lit: n <= cur.successes, good: true }"></span>
        </span>
        <span class="cb-count mono">{{ cur.successes }}/{{ SUCCESS_THRESHOLD }}</span>
      </div>
      <div class="cb-row">
        <span class="cb-label">論理時計</span>
        <span class="cb-count mono">now = {{ cur.now }}<template v-if="cur.state === 'open'"> / 開いた時刻 {{ cur.openedAt }}(あと {{ Math.max(OPEN_TIMEOUT - (cur.now - cur.openedAt), 0) }})</template></span>
      </div>
    </div>

    <p class="cb-note">{{ cur.log }}</p>

    <div class="cb-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="cb-nav mono">{{ at + 1 }} / {{ snaps.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="cb-legend">
      連続失敗が閾値に達すると open になり、以後の呼び出しは依存先に届く前に即失敗する(fail fast)。
      待ちが発生しないので呼び出し側の資源が守られ、依存先も追い打ちを受けずに回復できる。
      タイムアウト後の half-open で1本だけ試し、回復していれば closed に戻る。
    </p>
  </DemoShell>
</template>

<style scoped>
.cb-states {
  margin-top: 14px;
  display: flex;
  gap: 8px;
}
.cb-state {
  flex: 1;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 8px 12px;
  opacity: 0.5;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.cb-state.on {
  opacity: 1;
  background-color: var(--vp-c-bg-soft);
}
.cb-state.on.ok { border-left-color: var(--vp-c-green-1); }
.cb-state.on.ng { border-left-color: var(--vp-c-danger-1); }
.cb-state.on.warn { border-left-color: var(--vp-c-warning-1); }
.cb-state-name {
  font-size: 13px;
  font-weight: 700;
  font-family: var(--vp-font-family-mono);
}
.cb-state-desc {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cb-panel {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 14px;
  background-color: var(--vp-c-bg-soft);
}
.cb-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 0;
}
.cb-label {
  width: 72px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.cb-dep {
  font-size: 12px;
  font-weight: 700;
  padding: 1px 8px;
  border-radius: 3px;
}
.cb-dep.up { color: var(--vp-c-green-1); background-color: var(--vp-c-green-soft); }
.cb-dep.down { color: var(--vp-c-danger-1); background-color: var(--vp-c-danger-soft); }
.cb-dots {
  display: flex;
  gap: 4px;
}
.cb-dot {
  width: 12px;
  height: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 2px;
  background-color: var(--vp-c-bg);
}
.cb-dot.lit.danger { background-color: var(--vp-c-danger-1); border-color: var(--vp-c-danger-1); }
.cb-dot.lit.good { background-color: var(--vp-c-green-1); border-color: var(--vp-c-green-1); }
.cb-count {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.cb-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.cb-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.cb-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.cb-legend {
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
