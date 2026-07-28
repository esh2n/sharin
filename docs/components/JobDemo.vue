<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/job(Go)を移植。Job の量と速さと諦める点、そして
// 周期実行で前の実行が残っているときの3つの方針を見せる。

type Mode = "job" | "cron";
const mode = ref<Mode>("job");

// --- Job ---
const completions = ref(6);
const parallelism = ref(2);
const backoff = ref(2);
const failFirst = ref(3);

interface JobState {
  succeeded: number;
  failed: number;
  attempts: number;
  active: number;
  phase: "Running" | "Complete" | "Failed";
  steps: number;
}

function runJob(comp: number, par: number, back: number, fail: number, maxSteps = 60): JobState {
  const s: JobState = { succeeded: 0, failed: 0, attempts: 0, active: 0, phase: "Running", steps: 0 };
  for (let i = 0; i < maxSteps && s.phase === "Running"; i++) {
    s.steps++;
    while (s.active > 0) {
      s.active--;
      s.attempts++;
      if (s.attempts <= fail) s.failed++;
      else s.succeeded++;
    }
    if (s.failed > back) {
      s.phase = "Failed";
      break;
    }
    if (s.succeeded >= comp) {
      s.phase = "Complete";
      break;
    }
    s.active = Math.min(par, comp - s.succeeded);
  }
  return s;
}

const job = computed(() => runJob(completions.value, parallelism.value, backoff.value, failFirst.value));

// --- CronJob ---
type Policy = "Allow" | "Forbid" | "Replace";
const policy = ref<Policy>("Forbid");
const EVERY = 2;
const CRON_JOB = { comp: 6, par: 1, back: 0, fail: 0 };
const TICKS = 12;

interface Run {
  id: number;
  start: number;
  end: number | null;
  succeeded: number;
  phase: "Running" | "Complete" | "Failed";
  active: number;
  attempts: number;
}

const cron = computed(() => {
  const runs: Run[] = [];
  let started = 0,
    skipped = 0,
    replaced = 0;
  const log: string[] = [];
  for (let t = 1; t <= TICKS; t++) {
    for (const r of runs) {
      if (r.phase !== "Running") continue;
      while (r.active > 0) {
        r.active--;
        r.attempts++;
        r.succeeded++;
      }
      if (r.succeeded >= CRON_JOB.comp) {
        r.phase = "Complete";
        r.end = t;
        continue;
      }
      r.active = Math.min(CRON_JOB.par, CRON_JOB.comp - r.succeeded);
    }
    if (t % EVERY !== 0) continue;

    const active = runs.filter((r) => r.phase === "Running");
    if (active.length > 0) {
      if (policy.value === "Forbid") {
        skipped++;
        log.push(`t=${t} 前の実行が終わっていないので、この回は飛ばす`);
        continue;
      }
      if (policy.value === "Replace") {
        for (const r of active) {
          r.phase = "Failed";
          r.end = t;
        }
        replaced++;
        log.push(`t=${t} 前の実行を止めて置き換える`);
      }
    }
    started++;
    runs.push({ id: started, start: t, end: null, succeeded: 0, phase: "Running", active: 0, attempts: 0 });
    log.push(`t=${t} 実行を起動(${started} 回目)`);
  }
  return { runs, started, skipped, replaced, log, completed: runs.filter((r) => r.phase === "Complete").length };
});

const badge = computed(() =>
  mode.value === "job"
    ? `${job.value.phase} ・ 成功 ${job.value.succeeded} / 失敗 ${job.value.failed} ・ ${job.value.steps} 周期`
    : `起動 ${cron.value.started} / 完了 ${cron.value.completed} / 飛ばし ${cron.value.skipped} / 置換 ${cron.value.replaced}`,
);
const badgeTone = computed<"ok" | "ng" | "neutral">(() => {
  if (mode.value === "job") return job.value.phase === "Complete" ? "ok" : "ng";
  return cron.value.completed > 0 ? "ok" : "ng";
});
</script>

<template>
  <DemoShell title="JobとCronJob" :badge="badge" :badge-tone="badgeTone">
    <div class="jb-row">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === 'job' }" @click="mode = 'job'">Job(1回の仕事)</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'cron' }" @click="mode = 'cron'">CronJob(周期実行)</span>
      </span>
    </div>

    <template v-if="mode === 'job'">
      <div class="jb-row">
        <span class="jb-label">completions(仕事の量)</span>
        <span class="sd-seg">
          <span v-for="n in [3, 6, 9]" :key="n" class="sd-seg-opt" :class="{ on: completions === n }" @click="completions = n">{{ n }}</span>
        </span>
      </div>
      <div class="jb-row">
        <span class="jb-label">parallelism(同時に走らせる数)</span>
        <span class="sd-seg">
          <span v-for="n in [1, 2, 3]" :key="n" class="sd-seg-opt" :class="{ on: parallelism === n }" @click="parallelism = n">{{ n }}</span>
        </span>
      </div>
      <div class="jb-row">
        <span class="jb-label">backoffLimit(諦めるまでの失敗数)</span>
        <span class="sd-seg">
          <span v-for="n in [0, 2, 5]" :key="n" class="sd-seg-opt" :class="{ on: backoff === n }" @click="backoff = n">{{ n }}</span>
        </span>
      </div>
      <div class="jb-row">
        <span class="jb-label">最初の何回が失敗するか</span>
        <span class="sd-seg">
          <span v-for="n in [0, 3, 8]" :key="n" class="sd-seg-opt" :class="{ on: failFirst === n }" @click="failFirst = n">{{ n }}</span>
        </span>
      </div>

      <div class="jb-bars">
        <div class="jb-bar-row">
          <span class="jb-bar-l mono">成功</span>
          <span class="jb-track">
            <span v-for="n in completions" :key="n" class="jb-slot" :class="n <= job.succeeded ? 'ok' : ''" />
          </span>
          <span class="jb-bar-v mono">{{ job.succeeded }} / {{ completions }}</span>
        </div>
        <div class="jb-bar-row">
          <span class="jb-bar-l mono">失敗</span>
          <span class="jb-track">
            <span v-for="n in Math.max(backoff + 1, job.failed)" :key="n" class="jb-slot" :class="n <= job.failed ? 'ng' : ''" />
          </span>
          <span class="jb-bar-v mono">{{ job.failed }}(上限 {{ backoff }})</span>
        </div>
      </div>

      <div class="jb-verdict" :class="job.phase === 'Complete' ? 'ok' : 'bad'">
        <template v-if="job.phase === 'Complete'">
          {{ job.steps }} 周期で完了。{{ completions }} 個の仕事を同時 {{ parallelism }} で処理し、{{ job.failed }} 回の失敗を再試行で吸収した
        </template>
        <template v-else>
          失敗が上限 {{ backoff }} を超えたので Job ごと失敗にした。上限を決めていなければ、ここで永久に再試行し続けていた
        </template>
      </div>

      <p class="jb-legend">
        completions は仕事の量、parallelism は速さで、別の軸になっている。並列を上げると同じ量が少ない周期で終わる。
        backoffLimit は諦める点で、これを 0 にすると1回の失敗で Job ごと失敗になる。逆に決めておかないと、
        直らない失敗を永久に試し続けることになる。「最初の何回が失敗するか」を 8 にすると、どの上限でも諦める側に倒れる。
      </p>
    </template>

    <template v-else>
      <div class="jb-row">
        <span class="jb-label">前の実行が終わっていないとき</span>
        <span class="sd-seg">
          <span v-for="p in (['Allow', 'Forbid', 'Replace'] as Policy[])" :key="p" class="sd-seg-opt" :class="{ on: policy === p }" @click="policy = p">{{ p }}</span>
        </span>
      </div>
      <p class="jb-fixed mono">
        {{ EVERY }} 周期ごとに起動 / 1回の仕事は {{ CRON_JOB.comp }} 個・同時 {{ CRON_JOB.par }} なので、必ず次の時刻に食い込む
      </p>

      <div class="jb-timeline">
        <div v-for="r in cron.runs" :key="r.id" class="jb-run">
          <span class="jb-run-n mono">#{{ r.id }}</span>
          <span class="jb-run-track">
            <span
              v-for="t in TICKS"
              :key="t"
              class="jb-tick"
              :class="t >= r.start && (r.end === null ? t <= TICKS : t <= r.end)
                ? (r.phase === 'Complete' ? 'done' : r.phase === 'Failed' ? 'killed' : 'run')
                : ''"
            />
          </span>
          <span class="jb-run-s mono">{{ r.phase === "Complete" ? "完了" : r.phase === "Failed" ? "置き換えられた" : "実行中" }}</span>
        </div>
        <div v-if="cron.runs.length === 0" class="jb-empty">(まだ起動していない)</div>
      </div>

      <div class="jb-verdict" :class="cron.completed > 0 ? 'ok' : 'bad'">
        <template v-if="policy === 'Allow'">
          Allow: 前が終わっていなくても重ねて起動する。{{ cron.started }} 回起動し、同時に複数が走る。処理が重複してよい仕事向け
        </template>
        <template v-else-if="policy === 'Forbid'">
          Forbid: 前が残っていれば飛ばす。{{ cron.skipped }} 回ぶんの実行そのものが起きていない。毎回必ず走ることは保証されない
        </template>
        <template v-else>
          Replace: 前を止めて置き換える。{{ cron.replaced }} 回ぶんが途中で捨てられた。最新だけが要る仕事向け
        </template>
      </div>

      <div class="jb-log">
        <div class="jb-log-h">起きたこと</div>
        <div v-for="(l, i) in cron.log.slice(0, 8)" :key="i" class="jb-log-line mono">{{ l }}</div>
        <div v-if="cron.log.length > 8" class="jb-log-line mono">…ほか {{ cron.log.length - 8 }} 行</div>
      </div>

      <p class="jb-legend">
        1回の仕事が起動の周期より長いので、次の時刻に必ず食い込む。Allow は重ねるので同時に何本も走る。
        Forbid はその回を飛ばすので、実行そのものが起きない回ができる。Replace は前を止めるので、
        途中まで進んだ処理が捨てられる。どれを選んでも何かを失うので、仕事の性質で決めることになる。
      </p>
    </template>
  </DemoShell>
</template>

<style scoped>
.jb-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.jb-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  min-width: 210px;
}
.jb-fixed {
  margin: 8px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.jb-bars {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.jb-bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.jb-bar-l {
  font-size: 11px;
  min-width: 30px;
  color: var(--vp-c-text-2);
}
.jb-track {
  display: flex;
  gap: 3px;
  flex: 1;
}
.jb-slot {
  flex: 1;
  max-width: 26px;
  height: 12px;
  border: 1px solid var(--vp-c-divider);
}
.jb-slot.ok {
  background-color: var(--vp-c-green-1);
  border-color: var(--vp-c-green-1);
}
.jb-slot.ng {
  background-color: var(--vp-c-danger-1);
  border-color: var(--vp-c-danger-1);
}
.jb-bar-v {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
  min-width: 96px;
  text-align: right;
}
.jb-timeline {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.jb-run {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.jb-run-n {
  font-size: 10.5px;
  min-width: 24px;
  color: var(--vp-c-text-3);
}
.jb-run-track {
  display: flex;
  gap: 2px;
  flex: 1;
}
.jb-tick {
  flex: 1;
  height: 12px;
  border: 1px solid var(--vp-c-divider);
}
.jb-tick.run {
  background-color: var(--vp-c-brand-1);
  border-color: var(--vp-c-brand-1);
}
.jb-tick.done {
  background-color: var(--vp-c-green-1);
  border-color: var(--vp-c-green-1);
}
.jb-tick.killed {
  background-color: var(--vp-c-warning-1);
  border-color: var(--vp-c-warning-1);
}
.jb-run-s {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  min-width: 84px;
}
.jb-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.jb-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.jb-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.jb-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.jb-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.jb-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.jb-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.jb-legend {
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
