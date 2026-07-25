<script setup lang="ts">
export interface BstViewNode {
  key: number;
  left?: BstViewNode;
  right?: BstViewNode;
  hot?: boolean;
  dim?: boolean;
}

defineProps<{ node: BstViewNode }>();
</script>

<template>
  <div class="bn-sub">
    <div class="bn-node" :class="{ hot: node.hot, dim: node.dim }">{{ node.key }}</div>
    <div v-if="node.left || node.right" class="bn-children">
      <div class="bn-slot">
        <BstNodeView v-if="node.left" :node="node.left" />
        <div v-else class="bn-nil"></div>
      </div>
      <div class="bn-slot">
        <BstNodeView v-if="node.right" :node="node.right" />
        <div v-else class="bn-nil"></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bn-sub {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.bn-node {
  min-width: 32px;
  padding: 3px 7px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 50px;
  background-color: var(--vp-c-bg);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  text-align: center;
  transition: border-color 0.2s, box-shadow 0.2s;
}
.bn-node.hot {
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 0 0 1px var(--vp-c-brand-1);
}
.bn-node.dim {
  opacity: 0.3;
}
.bn-children {
  display: flex;
  align-items: flex-start;
  gap: 6px;
}
.bn-slot {
  display: flex;
  justify-content: center;
  min-width: 16px;
}
.bn-nil {
  width: 8px;
}
</style>
