<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/harness のコンテキストまわり(Go)を移植。台本なので何回動かしても同じ手数になる。

const FILES = ["calc.go", "tax.go", "fee.go", "cart.go", "item.go", "user.go", "order.go", "view.go"];
const HIT = "tax.go";
const BUDGET = 200;
const MAX_CALLS = 40;

type Mode = "all" | "recent" | "fold";
interface Step { tool: string; arg: string; obs: string }
interface Win {
  memo: string[]; folded: string[]; steps: Step[];
  size: number; over: number; lostSteps: number; lostChars: number;
}

const chars = (s: string) => [...s].length;
const sizeOf = (s: Step) => chars(s.tool + s.arg + s.obs);
const foldKey = (s: { tool: string; arg: string }) => (s.arg ? `${s.tool} ${s.arg}` : s.tool);
const sum = (xs: number[]) => xs.reduce((a, b) => a + b, 0);

function obsOf(tool: string, arg: string): string {
  if (tool === "search") return "呼び出し元 8 件: calc.go ほか";
  if (tool === "read")
    return arg === HIT
      ? `${arg} 24 行目 rate + price。掛けるのが正しい。呼び出し 3 箇所`
      : `${arg} 異常なし。呼び出し 3 箇所 / 24 行 / 直近の変更なし`;
  if (tool === "edit") return `${arg} を書き換えた`;
  return "PASS";
}

// 窓を作る。all は何も落とさない、recent は古い側を消す、fold は道具名だけ残す。
function curate(all: Step[], memo: string[], b: number, mode: Mode): Win {
  const base = sum(memo.map(chars));
  if (mode === "all") {
    const size = base + sum(all.map(sizeOf));
    return { memo, folded: [], steps: all, size, over: size > b ? size - b : 0, lostSteps: 0, lostChars: 0 };
  }
  const fold = mode === "fold";
  let keep = 0;
  while (keep < all.length) {
    const cut = all.length - keep - 1;
    let sz = base + sum(all.slice(cut).map(sizeOf));
    if (fold) sz += sum(all.slice(0, cut).map((s) => chars(foldKey(s))));
    if (sz > b) break;
    keep++;
  }
  const dropped = all.slice(0, all.length - keep);
  const steps = all.slice(all.length - keep);
  let size = base + sum(steps.map(sizeOf));
  if (fold) size += sum(dropped.map((s) => chars(foldKey(s))));
  return {
    memo, steps, size,
    folded: fold ? dropped.map(foldKey) : [],
    over: size > b ? size - b : 0,
    lostSteps: dropped.length,
    lostChars: sum(dropped.map((s) => chars(s.obs))),
  };
}

const did = (w: Win, tool: string, arg: string) =>
  w.steps.some((s) => s.tool === tool && s.arg === arg) || w.folded.includes(foldKey({ tool, arg }));
const obsIn = (w: Win, tool: string, arg: string) =>
  w.steps.find((s) => s.tool === tool && s.arg === arg)?.obs;
const recall = (w: Win, word: string) => w.memo.some((m) => m.includes(word));

// 窓に見えているものだけで動く台本。決まりは「呼び出し元を全部見てから直す」。
function decide(w: Win, writeMemo: boolean) {
  if (!did(w, "search", "")) return { tool: "search", arg: "" };
  if (did(w, "edit", HIT)) {
    if (!did(w, "test", "")) return { tool: "test", arg: "" };
    return { answer: "直した" };
  }
  if (obsIn(w, "read", HIT) !== undefined && writeMemo && !recall(w, HIT))
    return { note: `${HIT} は 24 行目、足すのでなく掛ける` };
  for (const f of FILES) if (!did(w, "read", f)) return { tool: "read", arg: f };
  if (obsIn(w, "read", HIT) === undefined && !recall(w, HIT)) return { tool: "read", arg: HIT };
  return { tool: "edit", arg: HIT };
}

function run(mode: Mode, writeMemo: boolean) {
  const steps: Step[] = [];
  const memo: string[] = [];
  let calls = 0, input = 0, ok = false, reason = "上限に達した";
  for (;;) {
    if (calls >= MAX_CALLS) break;
    const w = curate(steps, memo, BUDGET, mode);
    const a = decide(w, writeMemo) as Record<string, string>;
    calls++;
    input += w.size;
    if (a.note) {
      if (!memo.includes(a.note)) memo.push(a.note);
      continue;
    }
    if (a.answer) { ok = true; reason = "モデルが終わりだと言った"; break; }
    steps.push({ tool: a.tool, arg: a.arg, obs: obsOf(a.tool, a.arg) });
  }
  return { steps, memo, calls, input, ok, reason, win: curate(steps, memo, BUDGET, mode) };
}

const OPTIONS = [
  { key: "all", label: "全部残す", mode: "all" as Mode, memo: false },
  { key: "recent", label: "直近だけ", mode: "recent" as Mode, memo: false },
  { key: "fold", label: "畳む", mode: "fold" as Mode, memo: false },
  { key: "memo", label: "畳む + 覚え書き", mode: "fold" as Mode, memo: true },
];

const pick = ref("recent");
const opt = computed(() => OPTIONS.find((o) => o.key === pick.value)!);
const r = computed(() => run(opt.value.mode, opt.value.memo));

// 同じ道具を同じ引数で二度以上使った手に印を付ける。
const label = (s: { tool: string; arg: string }) => (s.arg ? `${s.tool} ${s.arg}` : s.tool);
const marks = computed(() => {
  const seen = new Map<string, number>();
  return r.value.steps.map((s) => {
    const n = (seen.get(label(s)) ?? 0) + 1;
    seen.set(label(s), n);
    return n > 1;
  });
});
const repeats = computed(() => marks.value.filter(Boolean).length);
// いちばん多く読み直したファイルの回数。
const worst = computed(() => {
  const seen = new Map<string, number>();
  for (const s of r.value.steps) if (s.tool === "read") seen.set(s.arg, (seen.get(s.arg) ?? 0) + 1);
  return Math.max(0, ...seen.values());
});

const badge = computed(() => (r.value.ok ? "終わった" : "終わらない"));
const verdict = computed(() => {
  const w = r.value.win;
  if (opt.value.key === "all")
    return `落とさなければ ${r.value.steps.length} 手で終わる。ただし記録は ${w.size} 文字あって、窓 ${BUDGET} を ${w.over} 文字超えている。実物ならここで拒否されるか、黙って古い側が消える`;
  if (opt.value.key === "recent")
    return `古い側を消すと、読んだこと自体が窓から消える。同じファイルを最大 ${worst.value} 回読み直し、余分な手が ${repeats.value} 回積み上がって、上限 ${MAX_CALLS} 回まで回っても終わらない`;
  if (opt.value.key === "fold")
    return `畳むと「どのファイルを読んだか」は残るので、読み直しは当たりの1本だけになる。${r.value.steps.length} 手で終わる。ただし観測 ${w.lostChars} 文字は読めないまま`;
  return `見つけた時点で ${chars(r.value.memo[0] ?? "")} 文字を窓の外に書き出しておくと、観測 ${w.lostChars} 文字を捨てても読み直しが要らない。${r.value.steps.length} 手で終わる`;
});
</script>

<template>
  <DemoShell title="窓に何を残すかで、同じ台本が終わったり終わらなかったりする"
             :badge="badge" :badge-tone="r.ok ? 'ok' : 'ng'">
    <div class="cw-actions">
      <span class="sd-seg">
        <span v-for="o in OPTIONS" :key="o.key" class="sd-seg-opt"
              :class="{ on: pick === o.key }" @click="pick = o.key">{{ o.label }}</span>
      </span>
    </div>

    <p class="cw-task mono">
      呼び出し元 8 本を全部見てから直す ・ 当たりは {{ HIT }}(2 本目)・ 窓 {{ BUDGET }} 文字 ・ 上限 {{ MAX_CALLS }} 回
    </p>

    <div class="cw-stats">
      <span class="cw-stat"><i>手数</i><b class="mono">{{ r.steps.length }}</b></span>
      <span class="cw-stat"><i>モデル</i><b class="mono">{{ r.calls }}</b></span>
      <span class="cw-stat"><i>渡した文字</i><b class="mono">{{ r.input.toLocaleString() }}</b></span>
      <span class="cw-stat"><i>余分な手</i><b class="mono" :class="repeats ? 'bad' : ''">{{ repeats }}</b></span>
      <span class="cw-stat"><i>窓を超えた</i><b class="mono" :class="r.win.over ? 'bad' : ''">{{ r.win.over }}</b></span>
    </div>

    <div class="cw-steps">
      <span v-for="(s, i) in r.steps" :key="i" class="cw-chip mono" :class="{ dup: marks[i] }">{{ label(s) }}</span>
    </div>

    <div class="cw-win">
      <div class="cw-win-head mono">最後に窓へ入っていたもの ・ {{ r.win.size }} 文字</div>
      <div class="cw-win-row">
        <span class="cw-tag">原文</span>
        <span class="cw-list mono">
          <template v-if="r.win.steps.length">
            <span v-for="(s, i) in r.win.steps" :key="i" class="cw-item">{{ label(s) }}</span>
          </template>
          <span v-else class="cw-none">なし</span>
        </span>
      </div>
      <div class="cw-win-row">
        <span class="cw-tag">畳んだ</span>
        <span class="cw-list mono">
          <template v-if="r.win.folded.length">
            <span v-for="(f, i) in r.win.folded" :key="i" class="cw-item folded">{{ f }}</span>
          </template>
          <span v-else class="cw-none">なし(消えた {{ r.win.lostSteps }} 手 / 観測 {{ r.win.lostChars }} 文字)</span>
        </span>
      </div>
      <div class="cw-win-row">
        <span class="cw-tag">覚え書き</span>
        <span class="cw-list mono">
          <template v-if="r.win.memo.length">
            <span v-for="(m, i) in r.win.memo" :key="i" class="cw-item memo">{{ m }}</span>
          </template>
          <span v-else class="cw-none">なし</span>
        </span>
      </div>
    </div>

    <div class="cw-verdict">{{ verdict }}</div>

    <p class="cw-note">
      台本は毎回同じで、変えたのは窓に何を残すかだけだ。原文で残っていれば観測の中身まで読めるが、
      畳んだ行から読めるのは「どの道具をどの引数で使ったか」までで、そこで分かったことは読めない。
      覚え書きは窓の外に置くので、観測を捨てても残る。
    </p>
  </DemoShell>
</template>

<style scoped>
.cw-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.cw-task { margin: 12px 0 0; font-size: 11px; color: var(--vp-c-text-3); line-height: 1.6; }
.cw-stats { display: flex; gap: 18px; flex-wrap: wrap; margin-top: 12px; }
.cw-stat { display: flex; flex-direction: column; gap: 2px; }
.cw-stat i { font-style: normal; font-size: 10px; color: var(--vp-c-text-3); }
.cw-stat b { font-size: 15px; font-weight: 600; color: var(--vp-c-text-1); }
.cw-stat b.bad { color: var(--vp-c-danger-1); }
.cw-steps { display: flex; flex-wrap: wrap; gap: 3px; margin-top: 14px; }
.cw-chip {
  font-size: 10px; padding: 2px 6px; background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2); white-space: nowrap;
}
.cw-chip.dup { background-color: var(--vp-c-danger-soft); color: var(--vp-c-danger-1); font-weight: 600; }
.cw-win { margin-top: 14px; border: 1px solid var(--vp-c-divider); background-color: var(--vp-c-bg-soft); }
.cw-win-head {
  font-size: 10px; color: var(--vp-c-text-3); padding: 6px 10px;
  border-bottom: 1px solid var(--vp-c-divider);
}
.cw-win-row { display: flex; gap: 10px; padding: 6px 10px; align-items: baseline; }
.cw-win-row + .cw-win-row { border-top: 1px solid var(--vp-c-divider); }
.cw-tag { width: 60px; flex: none; font-size: 10.5px; color: var(--vp-c-text-3); }
.cw-list { display: flex; flex-wrap: wrap; gap: 3px; }
.cw-item { font-size: 10px; padding: 1px 5px; background-color: var(--vp-c-default-soft); color: var(--vp-c-text-1); }
.cw-item.folded { color: var(--vp-c-text-3); }
.cw-item.memo { color: var(--vp-c-brand-1); }
.cw-none { font-size: 10px; color: var(--vp-c-text-3); }
.cw-verdict {
  margin-top: 14px; padding: 8px 12px; background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1); font-size: 12.5px; line-height: 1.6; color: var(--vp-c-text-1);
}
.cw-note {
  margin: 14px 0 0; padding-top: 12px; border-top: 1px solid var(--vp-c-divider);
  font-size: 12px; line-height: 1.7; color: var(--vp-c-text-2);
}
.mono { font-family: var(--vp-font-family-mono); }
</style>
