<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/reconcile(Go)を移植。desired と実際の Pod を見比べ、
// 調整ループが差を埋める(作成・スケール・自己修復)様子を見せる。

type Phase = "Pending" | "Running" | "Failed";
interface Pod {
  name: string;
  phase: Phase;
}

const desired = ref(3);
const pods = ref<Pod[]>([]);
const seq = ref(0);
const log = ref<string[]>([]);

function reconcile() {
  const acts: string[] = [];
  // 1. Pending → Running(前回作った Pod が起動)。
  for (const p of pods.value) if (p.phase === "Pending") p.phase = "Running";
  // 2. Failed を掃除。
  const failed = pods.value.filter((p) => p.phase === "Failed");
  for (const f of failed) acts.push(`delete ${f.name}(Failed を回収)`);
  pods.value = pods.value.filter((p) => p.phase !== "Failed");
  // 3. 生きている数を desired に合わせる。
  const alive = pods.value.length;
  if (alive < desired.value) {
    for (let i = 0; i < desired.value - alive; i++) {
      seq.value++;
      const name = `pod-${seq.value}`;
      pods.value = [...pods.value, { name, phase: "Pending" }];
      acts.push(`create ${name}`);
    }
  } else if (alive > desired.value) {
    const excess = pods.value.slice(desired.value);
    for (const e of excess) acts.push(`delete ${e.name}(余剰)`);
    pods.value = pods.value.slice(0, desired.value);
  }
  log.value = acts.length ? acts : ["差なし → 何もしない(冪等)"];
}

function failOne() {
  const running = pods.value.find((p) => p.phase === "Running");
  if (!running) {
    log.value = ["落とせる Running Pod がない"];
    return;
  }
  running.phase = "Failed";
  pods.value = [...pods.value];
  log.value = [`${running.name} が落ちた(まだ調整していない)`];
}
function scale(d: number) {
  desired.value = Math.max(0, desired.value + d);
  log.value = [`desired を ${desired.value} に宣言(まだ調整していない)`];
}
function reset() {
  desired.value = 3;
  pods.value = [];
  seq.value = 0;
  log.value = [];
}

const running = computed(() => pods.value.filter((p) => p.phase === "Running").length);
const failed = computed(() => pods.value.filter((p) => p.phase === "Failed").length);
const pending = computed(() => pods.value.filter((p) => p.phase === "Pending").length);
const converged = computed(() => running.value === desired.value && failed.value === 0 && pending.value === 0);

const badge = computed(() => `desired ${desired.value} / running ${running.value}`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  converged.value ? "ok" : failed.value > 0 ? "ng" : "neutral",
);
</script>

<template>
  <DemoShell title="調整ループ(reconciliation)" :badge="badge" :badge-tone="badgeTone">
    <div class="rc-actions">
      <button class="sd-btn sd-btn--primary" @click="reconcile">調整(reconcile)</button>
      <button class="sd-btn" @click="failOne">Podを1つ落とす</button>
      <button class="sd-btn" @click="scale(1)">desired +1</button>
      <button class="sd-btn" @click="scale(-1)">desired −1</button>
      <span class="rc-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="rc-state">
      <div class="rc-side">
        <div class="rc-side-h">desired state(宣言)</div>
        <div class="rc-desired mono">{{ desired }} レプリカ</div>
      </div>
      <div class="rc-vs mono">vs</div>
      <div class="rc-side rc-observed">
        <div class="rc-side-h">observed state(現状) — {{ pods.length }} Pod</div>
        <div class="rc-pods">
          <span v-for="p in pods" :key="p.name" class="rc-pod mono" :class="p.phase.toLowerCase()">
            {{ p.name }}<small>{{ p.phase === "Pending" ? "起動待" : p.phase === "Running" ? "稼働" : "障害" }}</small>
          </span>
          <span v-if="pods.length === 0" class="rc-empty">(Pod なし)</span>
        </div>
      </div>
    </div>

    <div class="rc-verdict" :class="converged ? 'ok' : failed > 0 ? 'bad' : 'neutral'">
      {{ converged ? "収束: desired と observed が一致し全 Pod 稼働中" : failed > 0 ? "差あり: 障害を検知。次の調整で作り直される" : "差あり: 調整が必要" }}
    </div>

    <div class="rc-log">
      <div class="rc-log-h">直前の調整の操作</div>
      <div v-for="(l, i) in log" :key="i" class="rc-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="rc-empty">(まだ調整していない)</div>
    </div>

    <p class="rc-legend">
      人は「desired = 3」という状態だけを宣言する。調整ループは desired と現状を毎回見比べ、足りなければ作り、
      多ければ消し、同じなら何もしない(冪等)。Pod を落としてから調整すると、同じループが作り直す(自己修復)。
      複数まとめて落としてから 1 回調整しても、現状を数え直すので全部復旧する(level-triggered)。作成・スケール・
      障害回復が、すべてこの 1 つの「差を埋める」処理に集約されている。
    </p>
  </DemoShell>
</template>

<style scoped>
.rc-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.rc-spacer {
  flex: 1;
}
.rc-state {
  display: flex;
  align-items: stretch;
  gap: 12px;
  margin-top: 16px;
}
.rc-side {
  flex: 1;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px;
  background-color: var(--vp-c-bg-soft);
}
.rc-observed {
  flex: 2;
}
.rc-side-h {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.rc-desired {
  font-size: 22px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
}
.rc-vs {
  display: flex;
  align-items: center;
  color: var(--vp-c-text-3);
  font-size: 13px;
}
.rc-pods {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.rc-pod {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 5px 8px;
  border-radius: 0;
  font-size: 11px;
  border: 1px solid var(--vp-c-divider);
}
.rc-pod small {
  font-size: 9px;
  opacity: 0.85;
}
.rc-pod.pending {
  background-color: var(--vp-c-warning-soft);
  color: var(--vp-c-warning-1);
  border-color: var(--vp-c-warning-1);
}
.rc-pod.running {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
  border-color: var(--vp-c-green-1);
}
.rc-pod.failed {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
  border-color: var(--vp-c-danger-1);
}
.rc-empty {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.rc-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.rc-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.rc-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.rc-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 56px;
}
.rc-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.rc-log-line {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.rc-legend {
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
