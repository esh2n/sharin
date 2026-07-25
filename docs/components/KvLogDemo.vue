<script setup lang="ts">
import { computed, ref } from "vue";

// Go 版 db/kvlog の動きをブラウザ内で再現するシミュレータ。
interface Rec {
  key: string;
  val: string;
  tomb: boolean;
}

const log = ref<Rec[]>([]);
const counters = ref<Record<string, number>>({});
const message = ref("");

// インデックスは「ログから導出できる」のがこの方式の要点なので、毎回導出する。
const index = computed(() => {
  const idx: Record<string, number> = {};
  log.value.forEach((r, i) => {
    if (r.tomb) delete idx[r.key];
    else idx[r.key] = i;
  });
  return idx;
});

function put(key: string) {
  const n = (counters.value[key] ?? 0) + 1;
  counters.value = { ...counters.value, [key]: n };
  log.value = [...log.value, { key, val: `v${n}`, tomb: false }];
  message.value = `Put(${key}, v${n}): レコード#${log.value.length - 1} を追記し、インデックスの ${key} を更新`;
}

function del(key: string) {
  if (!(key in index.value)) {
    message.value = `Delete(${key}): もう存在しない`;
    return;
  }
  log.value = [...log.value, { key, val: "", tomb: true }];
  message.value = `Delete(${key}): 墓石レコード#${log.value.length - 1} を追記し、インデックスから削除`;
}

function crash() {
  if (!log.value.length) {
    message.value = "ログが空なのでクラッシュしても何も起きない";
    return;
  }
  // 最後のレコードが「書きかけ」でクラッシュしたことにする。
  const dropped = log.value[log.value.length - 1];
  log.value = log.value.slice(0, -1);
  message.value = `クラッシュ: レコード#${log.value.length}(${dropped.key})は書きかけだったので再生時に切り捨て。残りを先頭から再生してインデックスを復元`;
}

function reset() {
  log.value = [];
  counters.value = {};
  message.value = "";
}
</script>

<template>
  <div class="kv-demo">
    <div class="kv-controls">
      <button v-for="k in ['a', 'b', 'c']" :key="k" class="kv-btn brand" type="button" @click="put(k)">
        {{ k }} に書く
      </button>
      <button class="kv-btn" type="button" @click="del('a')">a を消す</button>
      <button class="kv-btn danger" type="button" @click="crash">クラッシュして再起動</button>
      <button class="kv-btn" type="button" @click="reset">リセット</button>
    </div>

    <p v-if="message" class="kv-msg">{{ message }}</p>

    <p class="kv-label">ログファイル(追記のみ。何も上書きされない)</p>
    <div class="kv-log">
      <div
        v-for="(r, i) in log"
        :key="i"
        class="kv-cell"
        :class="{ hot: index[r.key] === i, tomb: r.tomb }"
      >
        <span class="kv-off">#{{ i }}</span>
        <span>{{ r.tomb ? `${r.key} 墓石` : `${r.key}=${r.val}` }}</span>
      </div>
      <p v-if="!log.length" class="kv-empty">まだ空です</p>
    </div>

    <p class="kv-label">インデックス(メモリ上。key がどのレコードにあるか)</p>
    <table v-if="Object.keys(index).length" class="kv-index">
      <thead>
        <tr><th>key</th><th>最新レコード</th></tr>
      </thead>
      <tbody>
        <tr v-for="(off, k) in index" :key="k">
          <td>{{ k }}</td>
          <td>#{{ off }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else class="kv-empty">空(キーなし)</p>
  </div>
</template>

<style scoped>
.kv-demo {
  margin: 16px 0 24px;
  padding: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg-soft);
}
.kv-controls {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.kv-btn {
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-default-soft);
}
.kv-btn.brand {
  font-weight: 600;
  color: var(--vp-button-brand-text);
  background-color: var(--vp-button-brand-bg);
}
.kv-btn.brand:hover {
  background-color: var(--vp-button-brand-hover-bg);
}
.kv-btn.danger {
  color: var(--vp-c-danger-1);
}
.kv-msg {
  margin: 10px 0 0;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.kv-label {
  margin: 14px 0 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.kv-log {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.kv-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
  min-width: 54px;
  padding: 6px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
}
.kv-cell.hot {
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 0 0 1px var(--vp-c-brand-1);
}
.kv-cell.tomb {
  opacity: 0.5;
  border-style: dashed;
}
.kv-cell:not(.hot):not(.tomb) {
  opacity: 0.45;
}
.kv-off {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.kv-index {
  margin: 0;
  font-size: 13px;
}
.kv-index th,
.kv-index td {
  padding: 3px 14px;
}
.kv-empty {
  margin: 0;
  font-size: 13px;
  color: var(--vp-c-text-3);
}
</style>
