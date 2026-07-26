<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// mini-next の SSR → ハイドレーションの流れを可視化する。
// URL でページを選び、サーバが HTML文字列を返し、ブラウザが静的描画。
// ハイドレートするまでボタンは「繋がっていない」——押しても動かないのを体感する。

interface Page {
  path: string;
  title: string;
  body: string;
  action?: string; // ページ内のボタン文言（あれば）
}

// ルート表（Next.js の pages/ に相当する対応表）
const ROUTES: { pattern: string; build: (params: Record<string, string>) => Page }[] = [
  { pattern: "/", build: () => ({ path: "/", title: "トップ", body: "ようこそ sharin へ" }) },
  { pattern: "/about", build: () => ({ path: "/about", title: "このサイトについて", body: "車輪の再発明の記録" }) },
  {
    pattern: "/posts/:id",
    build: (p) => ({ path: `/posts/${p.id}`, title: `記事 #${p.id}`, body: "本文のプレビュー…", action: "いいね" }),
  },
];

function resolve(url: string): { page: Page } | null {
  const target = url.split("/").filter(Boolean);
  for (const r of ROUTES) {
    const pat = r.pattern.split("/").filter(Boolean);
    if (pat.length !== target.length) continue;
    const params: Record<string, string> = {};
    let ok = true;
    for (let i = 0; i < pat.length; i++) {
      if (pat[i].startsWith(":")) params[pat[i].slice(1)] = target[i];
      else if (pat[i] !== target[i]) { ok = false; break; }
    }
    if (ok) return { page: r.build(params) };
  }
  return null;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// SSR: ページを HTML文字列にする。イベント(onClick)は文字列化できないので載らない。
function renderToString(page: Page): string {
  const btn = page.action ? `<button>${escapeHtml(page.action)}</button>` : "";
  return `<main><h1>${escapeHtml(page.title)}</h1><p>${escapeHtml(page.body)}</p>${btn}</main>`;
}

const URLS = ["/", "/about", "/posts/42"];

const state = reactive({
  url: "/posts/42",
  hydrated: false,
  likes: 0,
  note: "サーバが返した静的HTMLをブラウザが描画済み。だが未ハイドレート＝ボタンはまだ繋がっていない",
});

const matched = computed(() => resolve(state.url));
const page = computed(() => matched.value?.page ?? null);
const html = computed(() => (page.value ? renderToString(page.value) : ""));

function go(url: string) {
  state.url = url;
  state.hydrated = false; // 遷移＝サーバ描画やり直し。再びハイドレート前に戻る
  state.likes = 0;
  state.note = matched.value
    ? `ルータが "${url}" を解決 → SSRでHTML生成 → 静的描画。まだ未ハイドレート`
    : `"${url}" に一致するルートなし（呼び出し側が404を出す）`;
}
function hydrateNow() {
  if (!page.value) return;
  state.hydrated = true;
  state.note = "ハイドレート完了。既存DOMは作り直さず、イベントだけ後付けした。ボタンが動く";
}
function clickAction() {
  if (!page.value?.action) return;
  if (!state.hydrated) {
    state.note = "未ハイドレートなのでイベント未接続。押しても何も起きない（SSRにイベントは載らない）";
    return;
  }
  state.likes += 1;
  state.note = `ボタンが反応した（いいね ${state.likes}）。ハイドレーションがイベントを繋いだ証拠`;
}
function reset() {
  state.url = "/posts/42";
  state.hydrated = false;
  state.likes = 0;
  state.note = "サーバが返した静的HTMLをブラウザが描画済み。だが未ハイドレート＝ボタンはまだ繋がっていない";
}
reset();

const badge = computed(() => (state.hydrated ? "対話可能(hydrated)" : "静的(未hydrate)"));
const badgeTone = computed<"ok" | "neutral">(() => (state.hydrated ? "ok" : "neutral"));
</script>

<template>
  <DemoShell title="mini-next(SSR → ハイドレーション)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <div class="mn-seg" role="group" aria-label="URL">
        <button v-for="u in URLS" :key="u" class="mn-seg-btn" :class="{ on: state.url === u }" @click="go(u)">{{ u }}</button>
      </div>
      <span class="spacer" />
      <button class="sd-btn sd-btn--primary" :disabled="state.hydrated || !page" @click="hydrateNow">ハイドレート</button>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="mn-flow">
      <div class="mn-step">
        <div class="mn-step-label">1. ルータの解決</div>
        <div class="mn-box">
          <template v-if="page">
            <div><span class="mn-k">URL</span> {{ state.url }}</div>
            <div><span class="mn-k">page</span> {{ page.title }}</div>
          </template>
          <div v-else class="mn-miss">一致なし → 404</div>
        </div>
      </div>

      <div class="mn-step">
        <div class="mn-step-label">2. SSR 出力(サーバが返すHTML・イベントは載らない)</div>
        <pre class="mn-html"><code>{{ html || "(なし)" }}</code></pre>
      </div>

      <div class="mn-step">
        <div class="mn-step-label">3. ブラウザ(静的描画 → ハイドレートで対話可能)</div>
        <div class="mn-browser" :class="{ live: state.hydrated }">
          <template v-if="page">
            <div class="mn-page-title">{{ page.title }}</div>
            <div class="mn-page-body">{{ page.body }}</div>
            <div v-if="page.action" class="mn-page-actions">
              <button class="mn-like" @click="clickAction">{{ page.action }}<span v-if="state.likes"> · {{ state.likes }}</span></button>
              <span class="mn-wire" :class="state.hydrated ? 'ok' : 'off'">{{ state.hydrated ? "イベント接続済み" : "イベント未接続" }}</span>
            </div>
          </template>
          <div v-else class="mn-miss">404 Not Found</div>
        </div>
      </div>
    </div>

    <p class="sd-msg">{{ state.note }}</p>
    <div class="mn-legend">
      <span>サーバは HTML文字列を返すだけ（実DOM不要＝Nodeで動く）。イベントは文字列に載らない</span>
      <span>ハイドレーション＝既存DOMを作り直さず、欠けていたイベントだけを後付けする</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.mn-seg {
  display: inline-flex;
  border: 1px solid var(--vp-c-divider);
  overflow: hidden;
}
.mn-seg-btn {
  padding: 4px 12px;
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  background-color: var(--vp-c-bg);
  border-right: 1px solid var(--vp-c-divider);
}
.mn-seg-btn:last-child {
  border-right: none;
}
.mn-seg-btn.on {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.mn-flow {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 14px;
}
.mn-step-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin-bottom: 6px;
}
.mn-box {
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  padding: 10px 12px;
  font-size: 13px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.mn-k {
  display: inline-block;
  min-width: 44px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
  font-size: 11px;
}
.mn-html {
  margin: 0;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-alt);
  padding: 10px 12px;
  font-size: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
.mn-browser {
  border: 1px solid var(--vp-c-divider);
  border-top: 3px solid var(--vp-c-text-3);
  background-color: var(--vp-c-bg);
  padding: 14px;
}
.mn-browser.live {
  border-top-color: var(--vp-c-green-2);
}
.mn-page-title {
  font-size: 15px;
  font-weight: 600;
}
.mn-page-body {
  font-size: 13px;
  color: var(--vp-c-text-2);
  margin: 4px 0 10px;
}
.mn-page-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.mn-like {
  padding: 4px 14px;
  font-size: 13px;
  border: 1px solid var(--vp-c-brand-2);
  color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.mn-wire {
  font-size: 11px;
  font-weight: 600;
}
.mn-wire.ok {
  color: var(--vp-c-green-1);
}
.mn-wire.off {
  color: var(--vp-c-text-3);
}
.mn-miss {
  color: var(--vp-c-text-3);
  font-size: 13px;
}
.mn-legend {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 12px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
