<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/harness(Go)を移植。同じ仕事をループとグラフで進める。

interface Step {
  tool: string;
  obs: string;
  byModel: boolean;
}
interface Run {
  steps: Step[];
  modelCalls: number;
  ok: boolean;
  reason: string;
}

const failFirst = ref(true);

function makeTools(failFirst: boolean) {
  let edits = 0;
  const need = failFirst ? 2 : 1;
  return {
    search: () => "見つけた: calc.go:42",
    read: () => "return a - b",
    edit: () => {
      edits++;
      return "書き換えた";
    },
    test: () => (edits >= need ? "PASS" : "FAIL"),
  } as Record<string, () => string>;
}

// ループ: 毎手モデルに訊く。失敗したら最初からやり直す台本。
function runLoop(ff: boolean): Run {
  const tools = makeTools(ff);
  const plan = ["search", "read", "edit", "test"];
  const steps: Step[] = [];
  let modelCalls = 0;
  for (let round = 0; round < 3; round++) {
    for (const t of plan) {
      modelCalls++;
      const obs = tools[t]();
      steps.push({ tool: t, obs, byModel: true });
      if (steps.length >= 20) return { steps, modelCalls, ok: false, reason: "上限に達した" };
    }
    if (steps[steps.length - 1].obs === "PASS") {
      modelCalls++; // 最後の「終わり」
      return { steps, modelCalls, ok: true, reason: "モデルが終わりだと言った" };
    }
  }
  return { steps, modelCalls, ok: false, reason: "上限に達した" };
}

// グラフ: 経路は固定。edit の節だけモデルに訊く。落ちたら edit へ戻る。
function runGraph(ff: boolean): Run {
  const tools = makeTools(ff);
  const steps: Step[] = [];
  let modelCalls = 0;
  let visits = 0;
  let node = "search";
  while (node) {
    if (node === "edit") {
      visits++;
      if (visits > 4) return { steps, modelCalls, ok: false, reason: "同じ節を回りすぎた: edit" };
      modelCalls++;
    }
    const obs = tools[node]();
    steps.push({ tool: node, obs, byModel: node === "edit" });
    if (node === "test") {
      if (obs !== "PASS") {
        node = "edit";
        continue;
      }
      return { steps, modelCalls, ok: true, reason: "最後の節まで来た" };
    }
    node = { search: "read", read: "edit", edit: "test" }[node] ?? "";
  }
  return { steps, modelCalls, ok: true, reason: "最後の節まで来た" };
}

const loop = computed(() => runLoop(failFirst.value));
const graph = computed(() => runGraph(failFirst.value));
const rows = computed(() => [
  { name: "ループ", run: loop.value },
  { name: "グラフ", run: graph.value },
]);
const badge = computed(() =>
  failFirst.value ? "1回目の直しが外れる" : "一発で直る",
);
const verdict = computed(() => {
  const l = loop.value;
  const g = graph.value;
  if (!failFirst.value)
    return `道具を使う回数は同じ ${l.steps.length} 手なのに、モデルに訊く回数は ${l.modelCalls} 回と ${g.modelCalls} 回で違う。順番が変わらない区間は、毎回選ばせなくてよい`;
  return `ループは戻る先を知らないので最初からやり直して ${l.steps.length} 手。グラフは edit へ戻ると先に書いてあるので ${g.steps.length} 手。search と read はやり直していない`;
});
</script>

<template>
  <DemoShell title="ループとグラフ" :badge="badge" :badge-tone="failFirst ? 'neutral' : 'ok'">
    <div class="hn-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !failFirst }" @click="failFirst = false">一発で直る</span>
        <span class="sd-seg-opt" :class="{ on: failFirst }" @click="failFirst = true">1回目の直しが外れる</span>
      </span>
    </div>

    <div class="hn-grid">
      <div v-for="r in rows" :key="r.name" class="hn-col">
        <div class="hn-head">
          <span class="hn-name">{{ r.name }}</span>
          <span class="hn-counts mono">
            道具 {{ r.run.steps.length }} 手 ・ モデル <b>{{ r.run.modelCalls }}</b> 回
          </span>
        </div>
        <ol class="hn-steps">
          <li v-for="(s, i) in r.run.steps" :key="i" class="hn-step" :class="s.byModel ? 'asked' : ''">
            <span class="hn-tool mono">{{ s.tool }}</span>
            <span class="hn-obs mono" :class="s.obs === 'FAIL' ? 'bad' : s.obs === 'PASS' ? 'ok' : ''">{{ s.obs }}</span>
          </li>
        </ol>
        <div class="hn-reason mono">{{ r.run.reason }}</div>
      </div>
    </div>

    <div class="hn-verdict">{{ verdict }}</div>

    <p class="hn-note">
      色の付いた手が「モデルに訊いて決めた手」。ループは毎手訊くので、道具の手数とほぼ同じ回数だけ
      モデルを呼ぶ。グラフは search・read・test の順を先に書いてあるので、訊くのは edit の節だけになる。
      失敗したときも、ループは戻る先を知らないぶん最初からやり直すことになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.hn-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.hn-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 14px;
}
@media (max-width: 560px) {
  .hn-grid {
    grid-template-columns: 1fr;
  }
}
.hn-col {
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  padding: 10px 12px;
}
.hn-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--vp-c-divider);
}
.hn-name {
  font-size: 13px;
  font-weight: 600;
}
.hn-counts {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.hn-counts b {
  color: var(--vp-c-brand-1);
}
.hn-steps {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.hn-step {
  display: flex;
  gap: 8px;
  align-items: baseline;
  font-size: 11px;
  padding: 2px 6px;
  border-left: 2px solid transparent;
}
.hn-step.asked {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-default-soft);
}
.hn-tool {
  width: 52px;
  flex: none;
  color: var(--vp-c-text-1);
}
.hn-obs {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.hn-obs.bad {
  color: var(--vp-c-danger-1);
  font-weight: 600;
}
.hn-obs.ok {
  color: var(--vp-c-green-1);
  font-weight: 600;
}
.hn-reason {
  margin-top: 8px;
  padding-top: 6px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.hn-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
}
.hn-note {
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
