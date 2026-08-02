<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// crypto/xss(Go)を移植。判定は「外へ出るための文字が残っているか」で見る模型。

type Place = "text" | "quoted" | "bare" | "js" | "url";
type Esc = "none" | "html" | "js" | "url" | "scheme";

const PLACES: [Place, string][] = [
  ["text", "本文"],
  ["quoted", "属性値(引用符あり)"],
  ["bare", "属性値(引用符なし)"],
  ["js", "スクリプトの中の文字列"],
  ["url", "リンク先"],
];
const ESCAPES: [Esc, string][] = [
  ["none", "何もしない"],
  ["html", "HTML エスケープ"],
  ["js", "JS 文字列エスケープ"],
  ["url", "URL エンコード"],
  ["scheme", "スキーム検査"],
];

function render(p: Place, v: string) {
  if (p === "quoted") return `<img alt="${v}">`;
  if (p === "bare") return `<img alt=${v}>`;
  if (p === "js") return `<script>var s = "${v}";</` + `script>`;
  if (p === "url") return `<a href="${v}">link</a>`;
  return `<div>${v}</div>`;
}

const OK_SCHEME = new Set(["http", "https", "mailto"]);
function safeURL(v: string) {
  const s = v.trim();
  const i = s.indexOf(":");
  if (i < 0) return true;
  const j = s.search(/[/?#]/);
  if (j >= 0 && j < i) return true;
  return OK_SCHEME.has(s.slice(0, i).toLowerCase());
}

function apply(e: Esc, v: string) {
  if (e === "html")
    return v.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  if (e === "js")
    return v.replace(/\\/g, "\\\\").replace(/"/g, '\\"').replace(/'/g, "\\'")
      .replace(/\n/g, "\\n").replace(/\r/g, "\\r")
      .replace(/</g, "\\u003C").replace(/>/g, "\\u003E");
  if (e === "url") return encodeURIComponent(v);
  if (e === "scheme") return safeURL(v) ? v : "#";
  return v;
}

function hasBareQuote(v: string) {
  let esc = false;
  for (const c of v) {
    if (esc) { esc = false; continue; }
    if (c === "\\") { esc = true; continue; }
    if (c === '"') return true;
  }
  return false;
}

const right: Record<Place, Esc> = { text: "html", quoted: "html", bare: "html", js: "js", url: "scheme" };

interface Attack { name: string; payload: string; targets: Place[] }
const ATTACKS: Attack[] = [
  { name: "タグを開く", payload: `<script>alert(1)</` + `script>`, targets: ["text"] },
  { name: "引用符を閉じて属性を足す", payload: `" onerror="alert(1)`, targets: ["quoted"] },
  { name: "空白で属性を足す", payload: `x onerror=alert(1)`, targets: ["bare"] },
  { name: "文字列を閉じる", payload: `";alert(1);//`, targets: ["js"] },
  { name: "スクリプトを閉じる", payload: `</` + `script><script>alert(1)</` + `script>`, targets: ["js"] },
  { name: "実行するスキーム", payload: `javascript:alert(1)`, targets: ["url"] },
];

type Outcome = "動く" | "無害" | "壊れる";
function check(p: Place, e: Esc, a: Attack): Outcome {
  const v = apply(e, a.payload);
  if (!a.targets.includes(p)) return "無害";
  if (p === "text" && v.includes("<")) return "動く";
  if (p === "quoted" && v.includes('"')) return "動く";
  if (p === "bare" && /[ \t\n]/.test(v)) return "動く";
  if (p === "js" && (hasBareQuote(v) || v.includes("<"))) return "動く";
  if (p === "url" && !safeURL(v)) return "動く";
  if (v !== a.payload && e !== right[p]) return "壊れる";
  return "無害";
}

const place = ref<Place>("bare");
const esc = ref<Esc>("html");
const attacks = computed(() => ATTACKS.filter((a) => a.targets.includes(place.value)));
const attack = computed(() => attacks.value[0]);
const out = computed(() => apply(esc.value, attack.value.payload));
const outcome = computed(() => check(place.value, esc.value, attack.value));

const grid = computed(() =>
  PLACES.map(([p, label]) => {
    const rows = ATTACKS.filter((a) => a.targets.includes(p));
    const res = rows.map((a) => check(p, esc.value, a));
    return {
      p, label,
      stopped: res.filter((r) => r !== "動く").length,
      intact: res.filter((r) => r === "無害").length,
      total: rows.length,
    };
  }),
);

const badge = computed(() => outcome.value);
const tone = computed(() => (outcome.value === "動く" ? "ng" : outcome.value === "無害" ? "ok" : "neutral"));
const verdict = computed(() => {
  if (outcome.value === "動く")
    return `この場所では、この変換は足りない。変換後にも「外へ出るための文字」が残っている`;
  if (outcome.value === "壊れる")
    return `攻撃は止まったが、その場所のための変換ではないので、出したい値のほうが壊れる`;
  return `この場所のための変換になっている。攻撃も止まり、値も無事`;
});
</script>

<template>
  <DemoShell title="出す場所ごとに、正しい変換が違う" :badge="badge" :badge-tone="tone">
    <div class="xs-actions">
      <span class="sd-seg">
        <span v-for="[p, label] in PLACES" :key="p" class="sd-seg-opt"
              :class="{ on: place === p }" @click="place = p">{{ label }}</span>
      </span>
    </div>
    <div class="xs-actions xs-row2">
      <span class="sd-seg">
        <span v-for="[e, label] in ESCAPES" :key="e" class="sd-seg-opt"
              :class="{ on: esc === e }" @click="esc = e">{{ label }}</span>
      </span>
    </div>

    <div class="xs-flow">
      <div class="xs-step"><i>攻撃の文字列</i><code>{{ attack.payload }}</code></div>
      <div class="xs-step"><i>変換したあと</i><code>{{ out }}</code></div>
      <div class="xs-step"><i>ページに出た形</i><code>{{ render(place, out) }}</code></div>
    </div>

    <div class="xs-verdict" :class="outcome === '動く' ? 'bad' : outcome === '壊れる' ? 'warn' : ''">{{ verdict }}</div>

    <div class="xs-scroll">
      <table class="xs-t">
        <thead><tr><th>出す場所</th><th>止めた</th><th>値も無事</th></tr></thead>
        <tbody>
          <tr v-for="g in grid" :key="g.p" :class="{ cur: g.p === place }">
            <td>{{ g.label }}</td>
            <td :class="g.stopped < g.total ? 'bad' : 'ok'">{{ g.stopped }} / {{ g.total }}</td>
            <td :class="g.intact < g.total ? 'warn' : 'ok'">{{ g.intact }} / {{ g.total }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <p class="xs-note">
      表は、選んだ変換を 5 か所すべてに当てたときの成績。止めたかと、値が無事かを分けて数えている。
      全部潰せば止まるが、それは対策ではなく破壊になる。引用符なしの属性値だけは、
      止めて値も無事になる変換が1つも無い ── そこは引用符を書くところになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.xs-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.xs-row2 { margin-top: 8px; }
.xs-flow { display: flex; flex-direction: column; gap: 6px; margin-top: 14px; }
.xs-step { display: flex; flex-direction: column; gap: 3px; }
.xs-step i { font-style: normal; font-size: 10px; color: var(--vp-c-text-3); }
.xs-step code {
  font-family: var(--vp-font-family-mono); font-size: 11px; padding: 5px 8px;
  background-color: var(--vp-c-bg-soft); color: var(--vp-c-text-1); overflow-x: auto; white-space: pre;
}
.xs-verdict {
  margin-top: 14px; padding: 8px 12px; background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1); font-size: 12.5px; line-height: 1.6; color: var(--vp-c-text-1);
}
.xs-verdict.bad { border-left-color: var(--vp-c-danger-1); }
.xs-verdict.warn { border-left-color: var(--vp-c-yellow-1); }
.xs-scroll { overflow-x: auto; margin-top: 14px; border: 1px solid var(--vp-c-divider); }
.xs-t { border-collapse: collapse; width: 100%; font-size: 11.5px; }
.xs-t th, .xs-t td { padding: 6px 10px; text-align: left; border-bottom: 1px solid var(--vp-c-divider); white-space: nowrap; }
.xs-t thead th { background-color: var(--vp-c-bg-soft); font-size: 10px; color: var(--vp-c-text-3); font-weight: 600; }
.xs-t tbody tr:last-child td { border-bottom: 0; }
.xs-t tbody tr.cur { background-color: var(--vp-c-default-soft); }
.xs-t td.ok { color: var(--vp-c-green-1); font-weight: 600; }
.xs-t td.bad { color: var(--vp-c-danger-1); font-weight: 700; }
.xs-t td.warn { color: var(--vp-c-yellow-1); font-weight: 600; }
.xs-note {
  margin: 14px 0 0; padding-top: 12px; border-top: 1px solid var(--vp-c-divider);
  font-size: 12px; line-height: 1.7; color: var(--vp-c-text-2);
}
</style>
