<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 data-structures/lru の JS ミラー。容量3、recency 順のリストを可視化する。
const CAPACITY = 3;

const order = ref<string[]>([]); // 先頭 = 最近使った
const message = ref("");

const badge = computed(() =>
  order.value.length >= CAPACITY ? "満杯" : `${order.value.length}/${CAPACITY}`,
);

function touch(key: string) {
  const cur = order.value.filter((k) => k !== key);
  const hit = cur.length !== order.value.length;
  cur.unshift(key);
  let evicted: string | undefined;
  if (cur.length > CAPACITY) {
    evicted = cur.pop();
  }
  order.value = cur;
  message.value = hit
    ? `Get(${key}): ヒット。リストの先頭へ移動`
    : `Put(${key}): 先頭に追加${evicted ? `。溢れたので末尾の ${evicted} を追い出し` : ""}`;
}

function reset() {
  order.value = [];
  message.value = "";
}
</script>

<template>
  <DemoShell title="LRUキャッシュ(容量3)" :badge="badge" :badge-tone="order.length >= CAPACITY ? 'ng' : 'neutral'">
    <div class="sd-controls">
      <span class="ld-caption">キーを使う:</span>
      <button v-for="k in ['A', 'B', 'C', 'D', 'E']" :key="k" class="sd-btn sd-btn--primary" type="button" @click="touch(k)">
        {{ k }}
      </button>
      <span class="spacer"></span>
      <button class="sd-btn" type="button" @click="reset">リセット</button>
    </div>

    <div class="ld-list">
      <span class="ld-end">最近使った側</span>
      <div class="ld-track">
        <template v-for="(k, i) in order" :key="k">
          <span v-if="i > 0" class="ld-arrow"></span>
          <span class="ld-node" :class="{ tail: i === order.length - 1 && order.length === CAPACITY }">
            {{ k }}
          </span>
        </template>
        <span v-if="!order.length" class="ld-empty">まだ空です</span>
      </div>
      <span class="ld-end">追い出される側</span>
    </div>

    <p v-if="message" class="sd-msg">{{ message }}</p>
  </DemoShell>
</template>

<style scoped>
.ld-caption {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.ld-list {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 14px;
}
.ld-track {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 46px;
  padding: 8px 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
}
.ld-node {
  min-width: 34px;
  padding: 5px 0;
  border-radius: 6px;
  background-color: var(--vp-c-default-soft);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  font-weight: 600;
  text-align: center;
}
.ld-node.tail {
  box-shadow: 0 0 0 1px var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
}
.ld-arrow {
  width: 14px;
  height: 1px;
  background-color: var(--vp-c-text-3);
}
.ld-end {
  font-size: 11px;
  color: var(--vp-c-text-3);
  white-space: nowrap;
}
.ld-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
</style>
