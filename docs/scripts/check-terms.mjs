// 章が掲げた語を、本文で一度も扱っていないのを見つける。
//
// 由来: agent-harness が「ハーネス」を題名と層の表に出しながら、本文の散文で
// 一度も説明していなかった。出現5回はすべて表・一覧・出典で、読者から見ると
// 名前だけがあって中身が無い状態だった(docs/AUDIT.md の 2026-08-02 の指摘)。
//
// 見るのは 0 回かどうかだけにしてある。最初は「定義文があるか」まで見ようとしたが、
// この本は定義文でなく、作ることと喩えで説明する。`circuit-breaker` は
// 「家庭の電気ブレーカーと同じ発想で」と説明しているし、`hash-map` は
// 「順序を捨てる代わりに平均 O(1)」で説明している。どちらも「とは」「のことだ」を
// 使わないので、定義文を探す検査は 38 件鳴って全部が誤検出だった。
// 鳴りすぎる検査は無視されるので、確実に落ち度と言える 0 回だけを見る。
//
//   node scripts/check-terms.mjs

import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const PARTS = new URL("../parts/", import.meta.url).pathname;

// 章末の定型節は「掲げる場所」ではないので外す。
function head(md) {
  const i = md.search(/^## (設計の観点|対照と実例|簡略化したこと|参考資料)/m);
  return i < 0 ? md : md.slice(0, i);
}

// 散文だけ。表・コード・引用は「説明した」と数えない。
function prose(md) {
  const out = [];
  let code = false;
  for (const l of md.split("\n")) {
    if (l.startsWith("```")) { code = !code; continue; }
    if (code || l.startsWith("<<<") || l.trimStart().startsWith("|")) continue;
    out.push(l);
  }
  return out.join("\n");
}

// 章が「これを扱う」と宣言した場所。
function declared(md) {
  const h = head(md);
  const bits = [];
  const h1 = h.match(/^# (.+)$/m);
  if (h1) bits.push(h1[1]);
  const sm = h.match(/<Summary>\n([\s\S]*?)\n<\/Summary>/);
  if (sm) bits.push(sm[1]);
  for (const m of h.matchAll(/^#{2,3} (.+)$/gm)) bits.push(m[1]);
  // 層や方式を並べた表の第1列も宣言に含める。ハーネスはここにだけ居た。
  for (const l of h.split("\n")) {
    if (!l.trimStart().startsWith("|")) continue;
    const c = l.trim().replace(/^\||\|$/g, "").split("|")[0]?.trim() ?? "";
    if (c && !/^[-: ]+$/.test(c)) bits.push(c);
  }
  return bits.join("\n");
}

// 概念になりうる語だけ。数値・記号・コード識別子は落とす。
function terms(text) {
  const found = new Set();
  for (const m of text.matchAll(/[ァ-ヴ][ァ-ヴー]{2,11}/g)) found.add(m[0]);
  return [...found];
}

let bad = 0;
for (const f of readdirSync(PARTS).filter((x) => x.endsWith(".md")).sort()) {
  const md = readFileSync(join(PARTS, f), "utf8");
  const p = prose(md);
  const miss = terms(declared(md)).filter((t) => !p.includes(t));
  if (miss.length) {
    bad++;
    console.log(`${f.replace(/\.md$/, "").padEnd(22)} ${miss.join(", ")}`);
  }
}
console.log(bad ? `\n${bad} 章で、掲げた語が本文に無い` : "掲げた語はすべて本文で扱っている");
process.exit(bad ? 1 : 0);
