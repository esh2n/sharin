<script setup lang="ts">
import { computed, ref } from "vue";

// Go 版 db/wal の送金シーケンスをブラウザで再現するシミュレータ。
type CrashPoint = "none" | "before-commit" | "after-commit" | "mid-apply";

interface WalRec {
  text: string;
  commit?: boolean;
}

const balances = ref({ a: 1000, b: 1000 });
const wal = ref<WalRec[]>([]);
const crashed = ref(false);
const story = ref<string[]>([]);
const point = ref<CrashPoint>("after-commit");

const AMOUNT = 100;

const consistent = computed(() => (balances.value.a + balances.value.b) % 2000 === 0);

function run() {
  if (crashed.value) return;
  story.value = [];
  const newA = balances.value.a - AMOUNT;
  const newB = balances.value.b + AMOUNT;

  // 1. 変更内容を WAL に追記
  wal.value = [{ text: `set A=${newA}` }, { text: `set B=${newB}` }];
  story.value.push(`1. WAL に set A=${newA}, set B=${newB} を追記した`);
  if (point.value === "before-commit") {
    crashed.value = true;
    story.value.push("ここでクラッシュ。commit レコードは書かれていない");
    return;
  }

  // 2. commit + fsync
  wal.value = [...wal.value, { text: "commit", commit: true }];
  story.value.push("2. commit を追記して fsync。ここで「送金はやる」と確定");
  if (point.value === "after-commit") {
    crashed.value = true;
    story.value.push("ここでクラッシュ。ページは1枚も書き換えていない");
    return;
  }

  // 3. ページ適用
  balances.value = { ...balances.value, a: newA };
  story.value.push(`3. ページ適用: A=${newA} を書いた`);
  if (point.value === "mid-apply") {
    crashed.value = true;
    story.value.push(`ここでクラッシュ。B は古いまま(${balances.value.b})。データファイルは不整合!`);
    return;
  }
  balances.value = { ...balances.value, b: newB };
  story.value.push(`3. ページ適用: B=${newB} を書いた`);

  // 4. checkpoint
  wal.value = [];
  story.value.push("4. checkpoint: 適用済みなので WAL を空にした");
}

function recoverDb() {
  if (!crashed.value) return;
  story.value = ["再起動。リカバリが WAL を先頭から読む…"];
  const hasCommit = wal.value.some((r) => r.commit);
  if (hasCommit) {
    const sets = wal.value.filter((r) => !r.commit);
    for (const s of sets) {
      const m = s.text.match(/set (\w)=(\d+)/);
      if (m) {
        balances.value = { ...balances.value, [m[1].toLowerCase()]: Number(m[2]) };
      }
    }
    story.value.push("commit を発見。set を全部やり直す(redo)。冪等なので適用済みでも壊れない");
    story.value.push(`復元完了: A=${balances.value.a}, B=${balances.value.b}。送金は完遂された`);
  } else if (wal.value.length) {
    story.value.push("commit が無い。書きかけのバッチなので捨てる。送金は「無かったこと」になる");
  } else {
    story.value.push("WAL は空。何もすることがない");
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
  <div class="wd-demo">
    <div class="wd-controls">
      <label class="wd-select">
        クラッシュ地点:
        <select v-model="point" :disabled="crashed">
          <option value="none">クラッシュしない</option>
          <option value="before-commit">commit を書く前</option>
          <option value="after-commit">commit 直後(ページ適用前)</option>
          <option value="mid-apply">ページ適用の途中(1枚だけ)</option>
        </select>
      </label>
      <button class="wd-btn brand" type="button" :disabled="crashed" @click="run">
        A から B へ 100 送金
      </button>
      <button class="wd-btn" type="button" :disabled="!crashed" @click="recoverDb">
        再起動してリカバリ
      </button>
      <button class="wd-btn" type="button" @click="reset">リセット</button>
    </div>

    <div class="wd-panels">
      <div class="wd-panel">
        <p class="wd-label">データファイル(ページ)</p>
        <div class="wd-slots">
          <div class="wd-slot">口座A<br /><strong>{{ balances.a }}</strong></div>
          <div class="wd-slot">口座B<br /><strong>{{ balances.b }}</strong></div>
        </div>
        <p class="wd-verdict" :class="consistent ? 'ok' : 'ng'">
          合計 {{ balances.a + balances.b }} — {{ consistent ? "整合" : "不整合(お金が消えている)" }}
        </p>
      </div>
      <div class="wd-panel">
        <p class="wd-label">WAL</p>
        <div v-if="wal.length" class="wd-wal">
          <div v-for="(r, i) in wal" :key="i" class="wd-rec" :class="{ commit: r.commit }">
            {{ r.text }}
          </div>
        </div>
        <p v-else class="wd-empty">空</p>
      </div>
    </div>

    <ol v-if="story.length" class="wd-story">
      <li v-for="(s, i) in story" :key="i">{{ s }}</li>
    </ol>
  </div>
</template>

<style scoped>
.wd-demo {
  margin: 16px 0 24px;
  padding: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg-soft);
}
.wd-controls {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  align-items: center;
}
.wd-select {
  font-size: 13px;
}
.wd-select select {
  margin-left: 4px;
  padding: 4px 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-size: 13px;
}
.wd-btn {
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-default-soft);
}
.wd-btn.brand {
  font-weight: 600;
  color: var(--vp-button-brand-text);
  background-color: var(--vp-button-brand-bg);
}
.wd-btn.brand:hover {
  background-color: var(--vp-button-brand-hover-bg);
}
.wd-btn:disabled {
  opacity: 0.5;
}
.wd-panels {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  margin-top: 12px;
}
.wd-panel {
  flex: 1;
  min-width: 220px;
}
.wd-label {
  margin: 0 0 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.wd-slots {
  display: flex;
  gap: 8px;
}
.wd-slot {
  flex: 1;
  padding: 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  text-align: center;
  font-size: 13px;
}
.wd-verdict {
  margin: 8px 0 0;
  font-size: 13px;
  font-weight: 600;
}
.wd-verdict.ok {
  color: var(--vp-c-green-1);
}
.wd-verdict.ng {
  color: var(--vp-c-danger-1);
}
.wd-wal {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.wd-rec {
  padding: 4px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
}
.wd-rec.commit {
  border-color: var(--vp-c-brand-1);
  font-weight: 600;
}
.wd-empty {
  margin: 0;
  font-size: 13px;
  color: var(--vp-c-text-3);
}
.wd-story {
  margin: 14px 0 0;
  padding-left: 20px;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.wd-story li {
  margin: 2px 0;
}
</style>
