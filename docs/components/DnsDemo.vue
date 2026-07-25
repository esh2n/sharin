<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 dns の BuildQuery を JS で再現し、バイト列を色分けして見せる。
const name = ref("example.com");
const id = 0x1234;

interface Seg {
  label: string;
  bytes: number[];
  cls: "header" | "name" | "type";
  note: string;
}

const segments = computed<Seg[]>(() => {
  const header = [
    (id >> 8) & 0xff,
    id & 0xff, // ID
    0x01,
    0x00, // フラグ RD=1
    0x00,
    0x01, // QDCOUNT=1
    0x00,
    0x00,
    0x00,
    0x00,
    0x00,
    0x00, // AN/NS/AR = 0
  ];
  const nameBytes: number[] = [];
  for (const label of name.value.split(".")) {
    nameBytes.push(label.length);
    for (const ch of label) nameBytes.push(ch.charCodeAt(0));
  }
  nameBytes.push(0);
  return [
    { label: "ヘッダ (12B)", bytes: header, cls: "header", note: "ID, フラグ, 各セクションの件数" },
    { label: "名前", bytes: nameBytes, cls: "name", note: "[長さ][ラベル]... を 0 で終端" },
    { label: "QTYPE/QCLASS", bytes: [0, 1, 0, 1], cls: "type", note: "A レコード / IN(インターネット)" },
  ];
});

function toHex(b: number) {
  return b.toString(16).padStart(2, "0");
}
function toChar(b: number) {
  return b >= 33 && b <= 126 ? String.fromCharCode(b) : "·";
}

const totalBytes = computed(() => segments.value.reduce((a, s) => a + s.bytes.length, 0));
</script>

<template>
  <DemoShell title="DNS 問い合わせメッセージ" :badge="`${totalBytes} バイト`" badge-tone="neutral">
    <div class="sd-controls">
      <span class="dn-caption">ドメイン名:</span>
      <input v-model="name" class="dn-input" spellcheck="false" />
      <span class="spacer"></span>
      <span class="dn-hint">これを UDP で 8.8.8.8:53 に送る</span>
    </div>

    <div class="dn-segments">
      <div v-for="seg in segments" :key="seg.label" class="dn-seg">
        <div class="dn-seg-head">
          <span class="dn-seg-name" :class="seg.cls">{{ seg.label }}</span>
          <span class="dn-seg-note">{{ seg.note }}</span>
        </div>
        <div class="dn-bytes">
          <span v-for="(b, i) in seg.bytes" :key="i" class="dn-byte" :class="seg.cls">
            <span class="dn-hex">{{ toHex(b) }}</span>
            <span class="dn-ascii">{{ toChar(b) }}</span>
          </span>
        </div>
      </div>
    </div>
  </DemoShell>
</template>

<style scoped>
.dn-caption {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.dn-input {
  flex: 0 1 200px;
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.dn-hint {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.dn-segments {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 14px;
}
.dn-seg-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 5px;
}
.dn-seg-name {
  padding: 1px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 700;
}
.dn-seg-name.header {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
}
.dn-seg-name.name {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.dn-seg-name.type {
  background-color: var(--vp-c-yellow-soft);
  color: var(--vp-c-yellow-1);
}
.dn-seg-note {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.dn-bytes {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
}
.dn-byte {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 24px;
  padding: 2px 0;
  border-radius: 4px;
  font-family: var(--vp-font-family-mono);
}
.dn-byte.header {
  background-color: color-mix(in srgb, var(--vp-c-brand-1) 12%, transparent);
}
.dn-byte.name {
  background-color: color-mix(in srgb, var(--vp-c-green-1) 14%, transparent);
}
.dn-byte.type {
  background-color: color-mix(in srgb, var(--vp-c-yellow-1) 16%, transparent);
}
.dn-hex {
  font-size: 12px;
  color: var(--vp-c-text-1);
}
.dn-ascii {
  font-size: 9px;
  color: var(--vp-c-text-3);
}
</style>
