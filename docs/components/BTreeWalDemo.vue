<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 db/btreewal のトランザクション+クラッシュ挙動を、ページの状態遷移で見せる。
// 木そのものは描かず、「txn / WAL / data(実ページ)」の3つがどう動くかに絞る。
type Crash = "none" | "before-commit" | "after-commit" | "mid-apply";

const POINTS: Array<{ value: Crash; label: string }> = [
  { value: "none", label: "なし" },
  { value: "before-commit", label: "commit 前" },
  { value: "after-commit", label: "commit 直後" },
  { value: "mid-apply", label: "適用途中" },
];

// 「10 を挿入したら split が起きて 3 ページ(親・左・右)が書き換わる」という筋書き。
const CHANGED = ["ページ3 (親)", "ページ5 (左)", "ページ8 (右・新規)"];

const point = ref<Crash>("after-commit");
const txn = ref<string[]>([]);
const wal = ref<string[]>([]);
const applied = ref<Set<string>>(new Set(CHANGED)); // data に反映済みのページ
const crashed = ref(false);
const story = ref<Array<{ text: string; bad?: boolean }>>([]);

// 「Insert 前の data」= 3ページが古い内容。ここでは true=新しい, false=古い で表す。
const dataFresh = ref<Record<string, boolean>>({});

function resetState() {
  txn.value = [];
  wal.value = [];
  crashed.value = false;
  story.value = [];
  dataFresh.value = Object.fromEntries(CHANGED.map((p) => [p, false]));
}
resetState();

const consistent = computed(() => {
  const vals = Object.values(dataFresh.value);
  return vals.every((v) => v) || vals.every((v) => !v);
});

function run() {
  if (crashed.value) return;
  resetState();

  txn.value = [...CHANGED];
  story.value.push({ text: "10 を挿入。split が起きて3ページが書き換わるが、まず txn にため込む(data はまだ触らない)" });
  if (point.value === "before-commit") {
    crashed.value = true;
    story.value.push({ text: "クラッシュ。WAL には何も書いていない", bad: true });
    return;
  }

  wal.value = [...CHANGED, "commit"];
  story.value.push({ text: "txn を全部 WAL に書いて fsync。ここで「この Insert はやる」と確定" });
  if (point.value === "after-commit") {
    crashed.value = true;
    story.value.push({ text: "クラッシュ。data のページはまだ1枚も書き換えていない", bad: true });
    return;
  }

  dataFresh.value = { ...dataFresh.value, [CHANGED[0]]: true };
  story.value.push({ text: `data に適用: ${CHANGED[0]} を新しい内容にした` });
  if (point.value === "mid-apply") {
    crashed.value = true;
    story.value.push({ text: "クラッシュ。親ページだけ新しく、左右は古いまま。木は不整合!", bad: true });
    return;
  }

  for (const p of CHANGED.slice(1)) dataFresh.value = { ...dataFresh.value, [p]: true };
  story.value.push({ text: "残りのページも適用。3ページすべて新しい内容に" });
  txn.value = [];
  wal.value = [];
  story.value.push({ text: "checkpoint: 適用済みなので WAL を空に" });
}

function recover() {
  if (!crashed.value) return;
  story.value = [{ text: "再起動。リカバリが WAL を先頭から読む" }];
  const committed = wal.value.includes("commit");
  if (committed) {
    for (const p of CHANGED) dataFresh.value = { ...dataFresh.value, [p]: true };
    story.value.push({ text: "commit を発見。3ページを全部 redo(冪等なので適用済みでも安全)" });
    story.value.push({ text: "復元完了。木は整合。Insert は完遂された" });
  } else if (wal.value.length || txn.value.length) {
    story.value.push({ text: "commit が無いので書きかけを捨てる。Insert は「無かったこと」に。木は元のまま整合" });
  }
  txn.value = [];
  wal.value = [];
  crashed.value = false;
}

function reset() {
  resetState();
}

const badge = computed(() => {
  if (crashed.value) return "クラッシュ中";
  return consistent.value ? "木は整合" : "木が不整合";
});
const badgeTone = computed(() => (crashed.value || !consistent.value ? "ng" : "ok"));
</script>

<template>
  <DemoShell title="Insert の原子性" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="bw-caption">クラッシュ地点</span>
      <div class="sd-seg">
        <span
          v-for="p in POINTS"
          :key="p.value"
          class="sd-seg-opt"
          :class="{ on: point === p.value, disabled: crashed }"
          @click="!crashed && (point = p.value)"
        >
          {{ p.label }}
        </span>
      </div>
      <span class="spacer"></span>
      <button v-if="!crashed" class="sd-btn sd-btn--primary" type="button" @click="run">10 を挿入</button>
      <button v-else class="sd-btn sd-btn--primary" type="button" @click="recover">再起動してリカバリ</button>
      <button class="sd-btn" type="button" @click="reset">リセット</button>
    </div>

    <div class="bw-lanes">
      <div class="bw-lane">
        <p class="bw-lane-head">txn <span>(未確定の変更)</span></p>
        <div class="bw-cells">
          <span v-for="p in txn" :key="p" class="bw-cell">{{ p }}</span>
          <span v-if="!txn.length" class="bw-empty">空</span>
        </div>
      </div>
      <div class="bw-lane">
        <p class="bw-lane-head">wal.log <span>(先行書き込み)</span></p>
        <div class="bw-cells">
          <span v-for="p in wal" :key="p" class="bw-cell" :class="{ commit: p === 'commit' }">{{ p }}</span>
          <span v-if="!wal.length" class="bw-empty">空</span>
        </div>
      </div>
      <div class="bw-lane">
        <p class="bw-lane-head">data.db <span>(実ページ)</span></p>
        <div class="bw-cells">
          <span
            v-for="p in CHANGED"
            :key="p"
            class="bw-cell"
            :class="dataFresh[p] ? 'fresh' : 'stale'"
          >
            {{ p }}{{ dataFresh[p] ? " ✓新" : " 旧" }}
          </span>
        </div>
      </div>
    </div>

    <ol v-if="story.length" class="bw-story">
      <li v-for="(s, i) in story" :key="i" :class="{ bad: s.bad }">{{ s.text }}</li>
    </ol>
  </DemoShell>
</template>

<style scoped>
.bw-caption {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.bw-lanes {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin-top: 14px;
}
@media (max-width: 640px) {
  .bw-lanes {
    grid-template-columns: 1fr;
  }
}
.bw-lane {
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
  padding: 10px 12px;
  min-height: 120px;
}
.bw-lane-head {
  margin: 0 0 8px;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  font-weight: 700;
}
.bw-lane-head span {
  font-family: var(--vp-font-family-base);
  font-weight: 400;
  color: var(--vp-c-text-3);
}
.bw-cells {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.bw-cell {
  padding: 3px 8px;
  border-radius: 5px;
  background-color: var(--vp-c-default-soft);
  font-size: 12px;
}
.bw-cell.commit {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.bw-cell.fresh {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.bw-cell.stale {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-3);
}
.bw-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.bw-story {
  margin: 14px 0 0;
  padding-left: 22px;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.bw-story li {
  margin: 3px 0;
  padding-left: 4px;
}
.bw-story li.bad {
  color: var(--vp-c-danger-1);
  font-weight: 600;
}
</style>
