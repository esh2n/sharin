<script setup lang="ts">
export interface ViewNode {
  id: number;
  keys: number[];
  children: ViewNode[];
}

defineProps<{ node: ViewNode; touched: number[] }>();
</script>

<template>
  <div class="bt-sub">
    <div class="bt-node" :class="{ hot: touched.includes(node.id) }">
      <span v-for="k in node.keys" :key="k" class="bt-key">{{ k }}</span>
      <span v-if="!node.keys.length" class="bt-key bt-empty">空</span>
    </div>
    <div v-if="node.children.length" class="bt-children">
      <BTreeNodeView v-for="c in node.children" :key="c.id" :node="c" :touched="touched" />
    </div>
  </div>
</template>

<style scoped>
.bt-sub {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
}
.bt-node {
  display: flex;
  gap: 3px;
  padding: 4px 6px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  transition: border-color 0.2s, box-shadow 0.2s;
}
.bt-node.hot {
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 0 0 1px var(--vp-c-brand-1);
}
.bt-key {
  min-width: 26px;
  padding: 1px 4px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  text-align: center;
}
.bt-empty {
  color: var(--vp-c-text-3);
}
.bt-children {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
</style>
