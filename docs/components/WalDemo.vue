<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 db/wal の送金シーケンスをブラウザで再現するシミュレータ。
type CrashPoint = "none" | "before-commit" | "after-commit" | "mid-apply";

interface WalRec {
  text: string;
  commit?: boolean;
}

const POINTS: Array<{ value: CrashPoint; label: string }> = [
  { value: "none", label: "なし" },
  { value: "before-commit", label: "commit 前" },
  { value: "after-commit", label: "commit 直後" },
  { value: "mid-apply", label: "適用途中" },
];

const balances = ref({ a: 1000, b: 1000 });
const wal = ref<WalRec[]>([]);
const crashed = ref(false);
const story = ref<Array<{ text: string; crash?: boolean }>>([]);
const point = ref<CrashPoint>("after-commit");

const AMOUNT = 100;

const consistent = computed(() => (balances.value.a + balances.value.b) % 2000 === 0);

function run() {
  if (crashed.value) return;
  story.value = [];
  const newA = balances.value.a - AMOUNT;
  const newB = balances.value.b + AMOUNT;

  wal.value = [{ text: `set A=${newA}` }, { text: `set B=${newB}` }];
  story.value.push({ text: `WAL に set A=${newA} と set B=${newB} を追記` });
  if (point.value === "before-commit") {
    crashed.value = true;
    story.value.push({ text: "クラッシュ。commit レコードは書かれていない", crash: true });
    return;
  }

  wal.value = [...wal.value, { text: "commit", commit: true }];
  story.value.push({ text: "commit を追記して fsync。「送金はやる」とここで確定" });
  if (point.value === "after-commit") {
    crashed.value = true;
    story.value.push({ text: "クラッシュ。ページは1枚も書き換えていない", crash: true });
    return;
  }

  balances.value = { ...balances.value, a: newA };
  story.value.push({ text: `ページ適用: 口座A を ${newA} に書き換え` });
  if (point.value === "mid-apply") {
    crashed.value = true;
    story.value.push({ text: `クラッシュ。口座B は古いまま。データファイルは不整合`, crash: true });
    return;
  }
  balances.value = { ...balances.value, b: newB };
  story.value.push({ text: `ページ適用: 口座B を ${newB} に書き換え` });

  wal.value = [];
  story.value.push({ text: "checkpoint: 適用済みなので WAL を空に" });
}

function recoverDb() {
  if (!crashed.value) return;
  story.value = [{ text: "再起動。リカバリが WAL を先頭から読む" }];
  const hasCommit = wal.value.some((r) => r.commit);
  if (hasCommit) {
    for (const r of wal.value.filter((r) => !r.commit)) {
      const m = r.text.match(/set (\w)=(\d+)/);
      if (m) balances.value = { ...balances.value, [m[1].toLowerCase()]: Number(m[2]) };
    }
    story.value.push({ text: "commit を発見。set を全部やり直す(redo)。冪等なので適用済み分も壊れない" });
    story.value.push({ text: `復元完了: A=${balances.value.a}, B=${balances.value.b}。送金は完遂` });
  } else if (wal.value.length) {
    story.value.push({ text: "commit が無いので書きかけバッチを捨てる。送金は「無かったこと」に" });
  }
  wal.value = [];
  crashed.value = false;
}

function reset() {
  balances.value = { a: 1000, b: 1000 };
  wal.value = [];
  crashed.value = false;
  story.value = [];
}
</script>

<template>
  <DemoShell
    title="送金シミュレータ"
    :badge="crashed ? 'クラッシュ中' : consistent ? '整合' : '不整合'"
    :badge-tone="crashed ? 'ng' : consistent ? 'ok' : 'ng'"
  >
    <div class="sd-controls">
      <span class="wd-caption">クラッシュ地点</span>
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
      <button v-if="!crashed" class="sd-btn sd-btn--primary" type="button" @click="run">
        A→B へ 100 送金
      </button>
      <button v-else class="sd-btn sd-btn--primary" type="button" @click="recoverDb">
        再起動してリカバリ
      </button>
      <button class="sd-btn" type="button" @click="reset">リセット</button>
    </div>

    <div class="wd-files">
      <div class="wd-file">
        <div class="wd-file-head">
          <span class="wd-file-name">data.db</span>
          <span class="wd-file-desc">口座ページ</span>
        </div>
        <div class="wd-file-body">
          <div class="wd-account">
            <span>口座A</span>
            <strong>{{ balances.a }}</strong>
          </div>
          <div class="wd-account">
            <span>口座B</span>
            <strong>{{ balances.b }}</strong>
          </div>
          <div class="wd-sum" :class="consistent ? 'ok' : 'ng'">
            合計 {{ balances.a + balances.b }}
            {{ consistent ? "" : " — 100 が消えている" }}
          </div>
        </div>
      </div>
      <div class="wd-file">
        <div class="wd-file-head">
          <span class="wd-file-name">wal.log</span>
          <span class="wd-file-desc">先行書き込みログ</span>
        </div>
        <div class="wd-file-body">
          <template v-if="wal.length">
            <div v-for="(r, i) in wal" :key="i" class="wd-rec" :class="{ commit: r.commit }">
              {{ r.text }}
            </div>
          </template>
          <div v-else class="wd-rec empty">(空)</div>
        </div>
      </div>
    </div>

    <ol v-if="story.length" class="wd-story">
      <li v-for="(s, i) in story" :key="i" :class="{ crash: s.crash }">{{ s.text }}</li>
    </ol>
  </DemoShell>
</template>

<style scoped>
.wd-caption {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.wd-files {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 14px;
}
@media (max-width: 560px) {
  .wd-files {
    grid-template-columns: 1fr;
  }
}
.wd-file {
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
  overflow: hidden;
}
.wd-file-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-default-soft);
}
.wd-file-name {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  font-weight: 600;
}
.wd-file-desc {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.wd-file-body {
  padding: 10px 12px;
  min-height: 108px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.wd-account {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding: 4px 8px;
  border-radius: 6px;
  background-color: var(--vp-c-default-soft);
  font-size: 13px;
}
.wd-account strong {
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
}
.wd-sum {
  margin-top: auto;
  font-size: 12px;
  font-weight: 600;
  text-align: right;
}
.wd-sum.ok {
  color: var(--vp-c-green-1);
}
.wd-sum.ng {
  color: var(--vp-c-danger-1);
}
.wd-rec {
  padding: 3px 8px;
  border-left: 3px solid var(--vp-c-divider);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
}
.wd-rec.commit {
  border-left-color: var(--vp-c-brand-1);
  font-weight: 700;
}
.wd-rec.empty {
  border-left: none;
  color: var(--vp-c-text-3);
}
.wd-story {
  margin: 14px 0 0;
  padding-left: 22px;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.wd-story li {
  margin: 3px 0;
  padding-left: 4px;
}
.wd-story li.crash {
  color: var(--vp-c-danger-1);
  font-weight: 600;
}
</style>
