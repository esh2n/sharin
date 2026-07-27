<script setup lang="ts">
import { reactive, ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/virtmem(Go)を移植。2 プロセスのアドレス変換・ページフォルト・
// TLB・プロセス隔離を可視化する。

const PAGE_SIZE = 256;
const PAGE_BITS = 8;
const MAX_FRAMES = 8;

interface MMUState {
  valid: Set<number>;
  frame: Map<number, number>; // vpage -> frame
  tlb: Set<number>; // vpage が TLB に載っているか
  faults: number;
  hits: number;
  misses: number;
}
function newMMU(): MMUState {
  return { valid: new Set([0, 1, 2, 3]), frame: new Map(), tlb: new Set(), faults: 0, hits: 0, misses: 0 };
}

const shared = reactive({ nextFrame: 0 });
const procs = reactive<{ A: MMUState; B: MMUState }>({ A: newMMU(), B: newMMU() });

const lastResult = ref<{
  proc: string;
  vaddr: number;
  vpage: number;
  offset: number;
  paddr: number | null;
  frame: number | null;
  status: "hit" | "fault" | "resident" | "segfault" | "oom";
} | null>(null);

function translate(procName: "A" | "B", vaddr: number) {
  const m = procs[procName];
  const vpage = vaddr >> PAGE_BITS;
  const offset = vaddr & (PAGE_SIZE - 1);
  let status: "hit" | "fault" | "resident" | "segfault" | "oom";
  let frame: number | null = null;

  if (m.tlb.has(vpage)) {
    m.hits++;
    frame = m.frame.get(vpage)!;
    status = "hit";
  } else {
    m.misses++;
    if (!m.valid.has(vpage)) {
      lastResult.value = { proc: procName, vaddr, vpage, offset, paddr: null, frame: null, status: "segfault" };
      return;
    }
    if (m.frame.has(vpage)) {
      frame = m.frame.get(vpage)!;
      status = "resident";
    } else {
      if (shared.nextFrame >= MAX_FRAMES) {
        lastResult.value = { proc: procName, vaddr, vpage, offset, paddr: null, frame: null, status: "oom" };
        return;
      }
      m.faults++;
      frame = shared.nextFrame++;
      m.frame.set(vpage, frame);
      status = "fault";
    }
    m.tlb.add(vpage);
  }
  const paddr = (frame << PAGE_BITS) | offset;
  lastResult.value = { proc: procName, vaddr, vpage, offset, paddr, frame, status };
}

function flush(procName: "A" | "B") {
  procs[procName].tlb.clear();
  lastResult.value = null;
}
function reset() {
  shared.nextFrame = 0;
  Object.assign(procs, { A: newMMU(), B: newMMU() });
  lastResult.value = null;
}

const hex = (n: number, w = 4) => "0x" + n.toString(16).padStart(w, "0");
const statusLabel: Record<string, string> = {
  hit: "TLBヒット(一発)",
  fault: "ページフォルト → フレーム割当",
  resident: "常駐(TLBミス→テーブル)",
  segfault: "SEGFAULT(未マップ)",
  oom: "OOM(物理フレーム枯渇)",
};
const statusTone = (s: string) => (s === "segfault" || s === "oom" ? "bad" : s === "hit" ? "ok" : "neutral");

const accesses = [
  { proc: "A" as const, vaddr: 0x0040, label: "A: 0x0040" },
  { proc: "A" as const, vaddr: 0x0150, label: "A: 0x0150" },
  { proc: "B" as const, vaddr: 0x0040, label: "B: 0x0040" },
  { proc: "A" as const, vaddr: 0x0500, label: "A: 0x0500" },
];

const badge = computed(() => `物理フレーム ${shared.nextFrame}/${MAX_FRAMES} 使用`);

function tableRows(m: MMUState) {
  return [0, 1, 2, 3].map((vp) => ({
    vpage: vp,
    frame: m.frame.get(vp),
    present: m.frame.has(vp),
    tlb: m.tlb.has(vp),
  }));
}
</script>

<template>
  <DemoShell title="仮想メモリ(アドレス変換)" badge-tone="neutral" :badge="badge">
    <div class="vm-actions">
      <button v-for="a in accesses" :key="a.label" class="sd-btn" @click="translate(a.proc, a.vaddr)">
        {{ a.label }}
      </button>
      <span class="vm-spacer" />
      <button class="sd-btn" @click="flush('A')">A: TLB flush</button>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <!-- 変換結果 -->
    <div v-if="lastResult" class="vm-result" :class="statusTone(lastResult.status)">
      <div class="vm-decode mono">
        <span class="vm-proc">プロセス{{ lastResult.proc }}</span>
        仮想 {{ hex(lastResult.vaddr) }} = ページ{{ lastResult.vpage }} · オフセット {{ hex(lastResult.offset, 2) }}
      </div>
      <div class="vm-trans mono">
        <template v-if="lastResult.paddr !== null">
          → 物理 <b>{{ hex(lastResult.paddr) }}</b>(フレーム{{ lastResult.frame }} · オフセット {{ hex(lastResult.offset, 2) }})
        </template>
        <template v-else>→ 変換できず</template>
      </div>
      <div class="vm-status" :class="statusTone(lastResult.status)">{{ statusLabel[lastResult.status] }}</div>
    </div>
    <p v-else class="vm-hint">アクセスボタンを押すと、仮想アドレスが物理アドレスに変換される様子が出る</p>

    <!-- 2 プロセスのページテーブル -->
    <div class="vm-tables">
      <div v-for="p in (['A', 'B'] as const)" :key="p" class="vm-table">
        <div class="vm-table-h">プロセス{{ p }} のページテーブル<span class="vm-stat mono">fault {{ procs[p].faults }} / hit {{ procs[p].hits }}</span></div>
        <div v-for="r in tableRows(procs[p])" :key="r.vpage" class="vm-pte" :class="{ present: r.present }">
          <span class="vm-pte-v mono">ページ{{ r.vpage }}</span>
          <span class="vm-pte-arrow mono">→</span>
          <span class="vm-pte-f mono">{{ r.present ? `フレーム${r.frame}` : "(未割当)" }}</span>
          <span v-if="r.tlb" class="vm-pte-tlb mono">TLB</span>
        </div>
      </div>
    </div>

    <p class="vm-legend">
      仮想アドレスを上位のページ番号と下位のオフセットに分け、プロセスごとのページテーブルでフレームを引く。
      未割当ページへの初回アクセスはページフォルトで、そこで初めて物理フレームを渡す(デマンドページング)。
      同じページの再アクセスは TLB ヒットで一発。注目は隔離で、プロセスAとBが同じ仮想 0x0040 に触っても、
      別々のテーブルなので別のフレーム(別の物理)を指す。だから互いのメモリを壊さない。
    </p>
  </DemoShell>
</template>

<style scoped>
.vm-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.vm-spacer {
  flex: 1;
}
.vm-result {
  margin-top: 14px;
  padding: 10px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.vm-result.ok {
  border-left-color: var(--vp-c-green-1);
}
.vm-result.bad {
  border-left-color: var(--vp-c-danger-1);
}
.vm-decode {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.vm-proc {
  font-weight: 700;
  color: var(--vp-c-brand-1);
  margin-right: 6px;
}
.vm-trans {
  font-size: 13px;
  color: var(--vp-c-text-1);
  margin-top: 4px;
}
.vm-status {
  font-size: 11.5px;
  font-weight: 700;
  margin-top: 4px;
  color: var(--vp-c-text-2);
}
.vm-status.ok {
  color: var(--vp-c-green-1);
}
.vm-status.bad {
  color: var(--vp-c-danger-1);
}
.vm-hint {
  margin-top: 14px;
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.vm-tables {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-top: 14px;
}
.vm-table {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px;
  background-color: var(--vp-c-bg-soft);
}
.vm-table-h {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.vm-stat {
  font-size: 10px;
  font-weight: 400;
  color: var(--vp-c-text-3);
}
.vm-pte {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  margin-bottom: 3px;
  opacity: 0.5;
}
.vm-pte.present {
  opacity: 1;
  border-left-color: var(--vp-c-brand-1);
}
.vm-pte-v {
  font-size: 11px;
  color: var(--vp-c-text-2);
  width: 52px;
}
.vm-pte-arrow {
  color: var(--vp-c-text-3);
}
.vm-pte-f {
  font-size: 11px;
  color: var(--vp-c-text-1);
  flex: 1;
}
.vm-pte-tlb {
  font-size: 9.5px;
  font-weight: 700;
  color: var(--vp-c-green-1);
  padding: 1px 5px;
  background-color: var(--vp-c-green-soft);
}
.vm-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.mono {
  font-family: var(--vp-font-family-mono);
}
</style>
