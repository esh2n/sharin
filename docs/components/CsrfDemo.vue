<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// crypto/csrf(Go)を移植。合言葉は整数 LCG から決定的に作るので、毎回同じ値になる。

const MUL = 6364136223846793005n;
const ADD = 1442695040888963407n;
const MASK = (1n << 64n) - 1n;

function issue(session: string): string {
  let s = 1469598103934665603n;
  for (const c of new TextEncoder().encode(session)) s = (s * MUL + ADD + BigInt(c)) & MASK;
  return (s >> 16n).toString(36);
}

type Site = "None" | "Lax" | "Strict";
interface Nav { crossSite: boolean; topLevel: boolean; method: string }
const SAFE = new Set(["GET", "HEAD", "OPTIONS", "TRACE"]);
const safeMethod = (m: string) => SAFE.has(m.toUpperCase());

function sends(s: Site, n: Nav) {
  if (!n.crossSite) return true;
  if (s === "Strict") return false;
  if (s === "Lax") return n.topLevel && safeMethod(n.method);
  return true;
}

interface Defense { token?: boolean; sameSite: Site; origin?: boolean }
interface Attempt { name: string; nav: Nav; knowsToken?: boolean; attack: boolean }

const SESSION = "sess-42";

function check(d: Defense, a: Attempt) {
  if (!sends(d.sameSite, a.nav))
    return { cookie: false, accepted: false, reason: `SameSite=${d.sameSite} でクッキーが付かない` };
  if (d.origin && a.nav.crossSite)
    return { cookie: true, accepted: false, reason: "Origin が自分のものでない" };
  if (d.token && !safeMethod(a.nav.method)) {
    const tok = a.knowsToken ? issue(SESSION) : "";
    if (!tok || tok !== issue(SESSION))
      return { cookie: true, accepted: false, reason: "合言葉が無いか、合っていない" };
  }
  return { cookie: true, accepted: true, reason: "通った" };
}

const ATTEMPTS: Attempt[] = [
  { name: "正規の画面から POST", nav: { crossSite: false, topLevel: true, method: "POST" }, knowsToken: true, attack: false },
  { name: "他サイトからリンクで流入", nav: { crossSite: true, topLevel: true, method: "GET" }, attack: false },
  { name: "攻撃: 隠しフォームで POST", nav: { crossSite: true, topLevel: true, method: "POST" }, attack: true },
  { name: "攻撃: img で GET", nav: { crossSite: true, topLevel: false, method: "GET" }, attack: true },
  { name: "攻撃: XSS で合言葉を読んで POST", nav: { crossSite: false, topLevel: true, method: "POST" }, knowsToken: true, attack: true },
];

const DEFENSES: [string, Defense][] = [
  ["何もしない", { sameSite: "None" }],
  ["合言葉だけ", { token: true, sameSite: "None" }],
  ["SameSite=Lax だけ", { sameSite: "Lax" }],
  ["SameSite=Strict だけ", { sameSite: "Strict" }],
  ["合言葉 + Lax", { token: true, sameSite: "Lax" }],
  ["Origin 検査 + Lax", { origin: true, sameSite: "Lax" }],
];

const pick = ref(4);
const d = computed(() => DEFENSES[pick.value][1]);
const rows = computed(() => ATTEMPTS.map((a) => ({ a, r: check(d.value, a) })));

const blocked = computed(() => rows.value.filter((x) => x.a.attack && !x.r.accepted).length);
const attacks = ATTEMPTS.filter((a) => a.attack).length;
const passed = computed(() => rows.value.filter((x) => !x.a.attack && x.r.accepted).length);
const legit = ATTEMPTS.length - attacks;

const badge = computed(() => `攻撃 ${blocked.value}/${attacks} 阻止 ・ 正規 ${passed.value}/${legit} 通過`);
const tone = computed(() =>
  blocked.value === attacks && passed.value === legit ? "ok" : blocked.value === 0 ? "ng" : "neutral",
);

const verdict = computed(() => {
  const name = DEFENSES[pick.value][0];
  const leaks = rows.value.filter((x) => x.a.attack && x.r.accepted).map((x) => x.a.name.replace("攻撃: ", ""));
  const lost = rows.value.filter((x) => !x.a.attack && !x.r.accepted).map((x) => x.a.name);
  const parts: string[] = [];
  if (leaks.length) parts.push(`止められない攻撃が ${leaks.length} 件(${leaks.join(" / ")})`);
  else parts.push("攻撃はすべて止まる");
  if (lost.length) parts.push(`正規を ${lost.length} 件落としている(${lost.join(" / ")})`);
  else parts.push("正規はすべて通る");
  return `${name}: ${parts.join("。")}`;
});
</script>

<template>
  <DemoShell title="どの守りが何を止めて、何を止めないか" :badge="badge" :badge-tone="tone">
    <div class="cs-actions">
      <span class="sd-seg">
        <span v-for="([name], i) in DEFENSES" :key="name" class="sd-seg-opt"
              :class="{ on: pick === i }" @click="pick = i">{{ name }}</span>
      </span>
    </div>

    <p class="cs-note-top mono">合言葉 = <b>{{ issue(SESSION) }}</b>(セッション {{ SESSION }} から決定的に導出)</p>

    <div class="cs-rows">
      <div v-for="{ a, r } in rows" :key="a.name" class="cs-row" :class="a.attack ? 'atk' : 'ok'">
        <span class="cs-kind">{{ a.attack ? "攻撃" : "正規" }}</span>
        <span class="cs-name">{{ a.name.replace("攻撃: ", "") }}</span>
        <span class="cs-cookie mono">{{ r.cookie ? "クッキー付く" : "クッキー無し" }}</span>
        <span class="cs-verd" :class="a.attack ? (r.accepted ? 'bad' : 'good') : (r.accepted ? 'good' : 'bad')">
          {{ r.accepted ? "通った" : "止まった" }}
        </span>
        <span class="cs-why">{{ r.reason }}</span>
      </div>
    </div>

    <div class="cs-verdict">{{ verdict }}</div>

    <p class="cs-note">
      守りは攻撃を止めるだけでなく、正規を通す必要がある。Strict と Origin 検査は他サイトからの流入を切るので、
      止めた数だけを見ると強く見えるが、リンクで来た読者がログアウト状態に見える。
      そして最後の1件は、どの守りでも止まらない。自分のページで動くスクリプトは合言葉を読めるからだ。
    </p>
  </DemoShell>
</template>

<style scoped>
.cs-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.cs-note-top { margin: 12px 0 0; font-size: 11px; color: var(--vp-c-text-3); }
.cs-note-top b { color: var(--vp-c-text-1); }
.cs-rows { display: flex; flex-direction: column; gap: 1px; margin-top: 12px; background-color: var(--vp-c-divider); border: 1px solid var(--vp-c-divider); }
.cs-row { background-color: var(--vp-c-bg); display: grid; grid-template-columns: 42px 1fr 88px 62px 1.2fr; gap: 8px; align-items: center; padding: 7px 10px; font-size: 11.5px; }
@media (max-width: 640px) { .cs-row { grid-template-columns: 42px 1fr; row-gap: 3px; } }
.cs-kind { font-size: 10px; text-align: center; padding: 1px 0; background-color: var(--vp-c-default-soft); color: var(--vp-c-text-3); }
.cs-row.atk .cs-kind { background-color: var(--vp-c-danger-soft); color: var(--vp-c-danger-1); }
.cs-name { color: var(--vp-c-text-1); }
.cs-cookie { font-size: 10px; color: var(--vp-c-text-3); }
.cs-verd { font-size: 11px; font-weight: 700; text-align: center; }
.cs-verd.good { color: var(--vp-c-green-1); }
.cs-verd.bad { color: var(--vp-c-danger-1); }
.cs-why { font-size: 10.5px; color: var(--vp-c-text-2); }
.cs-verdict {
  margin-top: 14px; padding: 8px 12px; background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1); font-size: 12.5px; line-height: 1.6; color: var(--vp-c-text-1);
}
.cs-note {
  margin: 14px 0 0; padding-top: 12px; border-top: 1px solid var(--vp-c-divider);
  font-size: 12px; line-height: 1.7; color: var(--vp-c-text-2);
}
.mono { font-family: var(--vp-font-family-mono); }
</style>
