<script setup lang="ts">
// 全デモ共通の外枠。タイトル + 状態バッジ + 本文。
// デモ内で使う共通のボタン/コントロール類のスタイルもここで(グローバルに)定義する。
defineProps<{ title: string; badge?: string; badgeTone?: "ok" | "ng" | "neutral" }>();
</script>

<template>
  <div class="sd">
    <div class="sd-head">
      <span class="sd-kicker">デモ</span>
      <span class="sd-title">{{ title }}</span>
      <span v-if="badge" class="sd-badge" :class="badgeTone ?? 'neutral'">{{ badge }}</span>
    </div>
    <div class="sd-body">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.sd {
  margin: 16px 0 24px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 10px;
  background-color: var(--vp-c-bg-soft);
  overflow: hidden;
}
.sd-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--vp-c-divider);
}
.sd-kicker {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  color: var(--vp-c-brand-1);
}
.sd-title {
  font-size: 13px;
  font-weight: 600;
}
.sd-badge {
  margin-left: auto;
  padding: 2px 10px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
}
.sd-badge.neutral {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.sd-badge.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.sd-badge.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.sd-body {
  padding: 16px;
}
</style>

<style>
/* デモ共通のコントロール類。DemoShell を import した時点で有効になる */
.sd-btn {
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-default-soft);
  transition: background-color 0.15s;
}
.sd-btn:hover {
  background-color: var(--vp-c-gray-soft);
}
.sd-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.sd-btn--primary {
  font-weight: 600;
  color: var(--vp-button-brand-text);
  background-color: var(--vp-button-brand-bg);
}
.sd-btn--primary:hover:not(:disabled) {
  background-color: var(--vp-button-brand-hover-bg);
}
.sd-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.sd-controls .spacer {
  flex: 1;
}
.sd-msg {
  margin: 12px 0 0;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.sd-seg {
  display: inline-flex;
  padding: 2px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
}
.sd-seg-opt {
  padding: 4px 12px;
  border-radius: 6px;
  font-size: 12px;
  color: var(--vp-c-text-2);
  cursor: pointer;
  white-space: nowrap;
}
.sd-seg-opt.on {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
  font-weight: 600;
}
.sd-seg-opt.disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
</style>
