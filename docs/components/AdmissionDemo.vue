<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/admission(Go)を移植。書き換え→検証の順序と、
// 応答が無いときの2つの方針が何を失うかを見せる。

interface Obj {
  kind: string;
  name: string;
  labels: Record<string, string>;
}

const hasTeamLabel = ref(false);
const mutatorOn = ref(true);
const available = ref(true);
const failure = ref<"Fail" | "Ignore">("Fail");
const target = ref<"通常のPod" | "webhook自身のPod">("通常のPod");

interface Step {
  stage: "書き換え" | "検証";
  hook: string;
  outcome: "適用" | "通過" | "拒否" | "素通し";
  detail: string;
}

const run = computed(() => {
  const obj: Obj = {
    kind: "Pod",
    name: target.value === "通常のPod" ? "web-1" : "policy-webhook-pod",
    labels: hasTeamLabel.value ? { team: "core" } : {},
  };
  const steps: Step[] = [];
  let allowed = true;
  let reason = "すべての関門を通った";

  // ① 書き換えの段
  if (mutatorOn.value) {
    if (!available.value) {
      if (failure.value === "Fail") {
        steps.push({ stage: "書き換え", hook: "add-team-label", outcome: "拒否", detail: "応答せず、方針は Fail" });
        return { obj, steps, allowed: false, reason: "add-team-label が応答しない。方針が Fail なので拒否する" };
      }
      steps.push({ stage: "書き換え", hook: "add-team-label", outcome: "素通し", detail: "応答せず、方針は Ignore" });
    } else if (!obj.labels.team) {
      obj.labels.team = "unknown";
      steps.push({ stage: "書き換え", hook: "add-team-label", outcome: "適用", detail: "team=unknown を付けた" });
    } else {
      steps.push({ stage: "書き換え", hook: "add-team-label", outcome: "通過", detail: "すでに team がある" });
    }
  }

  // ② 検証の段
  if (!available.value) {
    if (failure.value === "Fail") {
      steps.push({ stage: "検証", hook: "require-team-label", outcome: "拒否", detail: "応答せず、方針は Fail" });
      allowed = false;
      reason = "require-team-label が応答しない。方針が Fail なので拒否する";
    } else {
      steps.push({ stage: "検証", hook: "require-team-label", outcome: "素通し", detail: "応答せず、方針は Ignore" });
    }
  } else if (!obj.labels.team) {
    steps.push({ stage: "検証", hook: "require-team-label", outcome: "拒否", detail: "team ラベルが無い" });
    allowed = false;
    reason = "require-team-label が拒否: team ラベルが無い";
  } else {
    steps.push({ stage: "検証", hook: "require-team-label", outcome: "通過", detail: "team=" + obj.labels.team });
  }

  return { obj, steps, allowed, reason };
});

const lockout = computed(
  () => target.value === "webhook自身のPod" && !available.value && failure.value === "Fail",
);
const badge = computed(() => (run.value.allowed ? "通過" : "拒否"));
const badgeTone = computed<"ok" | "ng">(() => (run.value.allowed ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="admission webhook" :badge="badge" :badge-tone="badgeTone">
    <div class="ad-row">
      <span class="ad-label">作ろうとしている Pod</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: target === '通常のPod' }" @click="target = '通常のPod'">通常のPod</span>
        <span class="sd-seg-opt" :class="{ on: target === 'webhook自身のPod' }" @click="target = 'webhook自身のPod'">webhook自身のPod</span>
      </span>
    </div>
    <div class="ad-row">
      <span class="ad-label">team ラベル</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !hasTeamLabel }" @click="hasTeamLabel = false">書いていない</span>
        <span class="sd-seg-opt" :class="{ on: hasTeamLabel }" @click="hasTeamLabel = true">書いてある</span>
      </span>
    </div>
    <div class="ad-row">
      <span class="ad-label">書き換えの関門</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mutatorOn }" @click="mutatorOn = true">ある</span>
        <span class="sd-seg-opt" :class="{ on: !mutatorOn }" @click="mutatorOn = false">ない</span>
      </span>
    </div>
    <div class="ad-row">
      <span class="ad-label">webhook の状態</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: available }" @click="available = true">応答する</span>
        <span class="sd-seg-opt" :class="{ on: !available }" @click="available = false">落ちている</span>
      </span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: failure === 'Fail' }" @click="failure = 'Fail'">Fail</span>
        <span class="sd-seg-opt" :class="{ on: failure === 'Ignore' }" @click="failure = 'Ignore'">Ignore</span>
      </span>
    </div>

    <div class="ad-flow">
      <div class="ad-in mono">
        Pod {{ run.obj.name }}<small>{{ hasTeamLabel ? "team=core" : "(ラベルなし)" }}</small>
      </div>
      <div v-for="(s, i) in run.steps" :key="i" class="ad-step" :class="s.outcome">
        <span class="ad-stage mono">{{ s.stage }}</span>
        <span class="ad-hook mono">{{ s.hook }}</span>
        <span class="ad-out">{{ s.outcome }}</span>
        <span class="ad-detail">{{ s.detail }}</span>
      </div>
      <div class="ad-out-box mono" :class="run.allowed ? 'ok' : 'ng'">
        {{ run.allowed ? `保存される: ${run.obj.name}(team=${run.obj.labels.team ?? "なし"})` : "保存されない" }}
      </div>
    </div>

    <div class="ad-verdict" :class="lockout ? 'bad' : run.allowed ? 'ok' : 'warn'">
      <template v-if="lockout">
        webhook 自身を動かす Pod の作成が、その webhook に止められている。直すための Pod を作れないので復旧できない
      </template>
      <template v-else-if="run.allowed && !available && failure === 'Ignore'">
        通ったが、検証はされていない。Ignore は作成を止めない代わりに、検証されていないものを通す
      </template>
      <template v-else-if="run.allowed">{{ run.reason }}</template>
      <template v-else>{{ run.reason }}</template>
    </div>

    <p class="ad-legend">
      書き換えの関門があると、ラベルを書いていなくても足されて検証を通る。関門を「ない」にすると、同じ Pod が
      検証で止まる。webhook が落ちているとき、Fail は合格するはずのものまで拒否し、Ignore は検証されていないものを
      通す。どちらも危険で、失うものが違う。そして「webhook自身のPod」を Fail で作ろうとすると、直すための Pod が
      作れなくなる。webhook もクラスタの中で動く Pod だという当たり前が、ここで効いてくる。
    </p>
  </DemoShell>
</template>

<style scoped>
.ad-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.ad-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  min-width: 150px;
}
.ad-flow {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.ad-in {
  font-size: 11.5px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
  padding-bottom: 6px;
  border-bottom: 1px solid var(--vp-c-divider);
  margin-bottom: 8px;
}
.ad-in small {
  font-weight: 400;
  color: var(--vp-c-text-3);
  margin-left: 8px;
}
.ad-step {
  display: grid;
  grid-template-columns: 54px 138px 48px 1fr;
  gap: 8px;
  align-items: baseline;
  font-size: 10.5px;
  padding: 4px 6px;
  margin-bottom: 3px;
  border-left: 3px solid var(--vp-c-divider);
  color: var(--vp-c-text-2);
}
.ad-step.適用 {
  border-left-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
}
.ad-step.通過 {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
}
.ad-step.拒否 {
  border-left-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
  font-weight: 700;
}
.ad-step.素通し {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
}
.ad-stage,
.ad-hook {
  font-size: 10px;
}
.ad-out {
  font-weight: 700;
}
.ad-out-box {
  margin-top: 8px;
  padding: 5px 8px;
  font-size: 11px;
  font-weight: 700;
}
.ad-out-box.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.ad-out-box.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.ad-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.ad-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ad-verdict.warn {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.ad-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ad-legend {
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
