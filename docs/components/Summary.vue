<script setup lang="ts">
// 章冒頭の導入文。装飾はつけない。本文と地続きの、少しだけ大きめのリード段落。
</script>

<template>
  <p class="lead"><slot /></p>
</template>

<style scoped>
.lead {
  margin: 4px 0 28px;
  font-size: 1.05rem;
  line-height: 1.9;
  color: var(--vp-c-text-1);
}
</style>
