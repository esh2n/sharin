<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 id-generation と同じロジックの JS ミラー。
const props = defineProps<{ kind: "uuidv4" | "uuidv7" | "ulid" | "snowflake" }>();

const KIND_LABEL: Record<string, string> = {
  uuidv4: "UUIDv4",
  uuidv7: "UUIDv7",
  ulid: "ULID",
  snowflake: "Snowflake",
};

interface Part {
  text: string;
  cls: "time" | "node" | "seq" | "rand";
}

const rows = ref<Part[][]>([]);

const CROCKFORD = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
const EPOCH = 1_600_000_000_000;
let lastMs = -1;
let seq = 0;

function randBytes(n: number): number[] {
  const b = new Uint8Array(n);
  crypto.getRandomValues(b);
  return [...b];
}

const hex = (bytes: number[]) => bytes.map((b) => b.toString(16).padStart(2, "0")).join("");

function generate(): Part[] {
  const ms = Date.now();
  if (props.kind === "uuidv4") {
    const b = randBytes(16);
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    const h = hex(b);
    return [{ text: `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`, cls: "rand" }];
  }
  if (props.kind === "uuidv7") {
    const t = ms.toString(16).padStart(12, "0");
    const b = randBytes(10);
    b[0] = (b[0] & 0x0f) | 0x70;
    b[2] = (b[2] & 0x3f) | 0x80;
    const h = hex(b);
    return [
      { text: `${t.slice(0, 8)}-${t.slice(8, 12)}-`, cls: "time" },
      { text: `${h.slice(0, 4)}-${h.slice(4, 8)}-${h.slice(8)}`, cls: "rand" },
    ];
  }
  if (props.kind === "ulid") {
    let t = ms;
    let timePart = "";
    for (let i = 0; i < 10; i++) {
      timePart = CROCKFORD[t % 32] + timePart;
      t = Math.floor(t / 32);
    }
    const rand = randBytes(10);
    let randPart = "";
    for (let i = 0; i < 16; i++) {
      const off = i * 5;
      const idx = Math.floor(off / 8);
      const v = ((rand[idx] << 8) | (rand[idx + 1] ?? 0)) >> (11 - (off % 8));
      randPart += CROCKFORD[v & 31];
    }
    return [
      { text: timePart, cls: "time" },
      { text: randPart, cls: "rand" },
    ];
  }
  // snowflake
  const rel = ms - EPOCH;
  if (rel === lastMs) {
    seq++;
  } else {
    lastMs = rel;
    seq = 0;
  }
  const id = (BigInt(rel) << 22n) | (42n << 12n) | BigInt(seq);
  return [
    { text: `${id} = `, cls: "rand" },
    { text: `${rel}`, cls: "time" },
    { text: " | node 42", cls: "node" },
    { text: ` | 連番 ${seq}`, cls: "seq" },
  ];
}

function fire() {
  rows.value = [...rows.value, generate()].slice(-6);
}

// 生成順に並べたとき、辞書順(文字列比較)でも昇順になっているか。
const sorted = computed(() => {
  const texts = rows.value.map((r) => r.map((p) => p.text).join(""));
  return texts.every((t, i) => i === 0 || texts[i - 1] <= t);
});
</script>

<template>
  <DemoShell
    :title="KIND_LABEL[kind] ?? kind"
    :badge="rows.length >= 2 ? (sorted ? '生成順=辞書順' : '順序不一致') : undefined"
    :badge-tone="rows.length >= 2 ? (sorted ? 'ok' : 'ng') : 'neutral'"
  >
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" type="button" @click="fire">生成する</button>
    </div>
    <ul v-if="rows.length" class="ig-list">
      <li v-for="(row, i) in rows" :key="i">
        <span v-for="(p, j) in row" :key="j" :class="`ig-${p.cls}`">{{ p.text }}</span>
      </li>
    </ul>
    <p v-if="rows.length" class="ig-legend">
      <span class="ig-time">時刻</span> / <span class="ig-node">ノードID</span> /
      <span class="ig-seq">連番</span> / <span class="ig-rand">乱数・その他</span>
    </p>
  </DemoShell>
</template>

<style scoped>
.ig-list {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  overflow-x: auto;
}
.ig-list li {
  margin: 2px 0;
  white-space: nowrap;
}
.ig-time {
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.ig-node {
  color: var(--vp-c-green-1);
  font-weight: 600;
}
.ig-seq {
  color: var(--vp-c-yellow-1);
  font-weight: 600;
}
.ig-rand {
  color: var(--vp-c-text-2);
}
.ig-legend {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-3);
}
</style>
