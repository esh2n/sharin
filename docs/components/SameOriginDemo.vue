<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// crypto/sameorigin(Go)を移植。判定だけなので、実時間も乱数も出てこない。

const DEFAULT_PORT: Record<string, number> = { http: 80, https: 443 };

interface Origin { scheme: string; host: string; port: number }
function parse(raw: string): Origin {
  const u = new URL(raw);
  const scheme = u.protocol.replace(":", "").toLowerCase();
  return {
    scheme,
    host: u.hostname.toLowerCase(),
    port: u.port ? Number(u.port) : (DEFAULT_PORT[scheme] ?? 0),
  };
}
const same = (a: Origin, b: Origin) => a.scheme === b.scheme && a.host === b.host && a.port === b.port;
const show = (o: Origin) =>
  o.port === DEFAULT_PORT[o.scheme] ? `${o.scheme}://${o.host}` : `${o.scheme}://${o.host}:${o.port}`;

// ① 生成元の比較
const BASE = "https://example.com/login";
const OTHERS = [
  ["https://example.com/admin/secret", "パスは生成元に入らない"],
  ["https://example.com:443/", "既定ポートは省いても同じ"],
  ["https://EXAMPLE.COM/", "大文字小文字は畳まれる"],
  ["http://example.com/", "スキームが違う"],
  ["https://example.com:8443/", "ポートが違う"],
  ["https://api.example.com/", "サブドメインは別のホスト"],
  ["https://example.com.evil.test/", "前方一致は別物"],
];

// ② やり方ごとに何が通るか
interface Access { send: boolean; read: boolean; cookie: boolean }
const KINDS: [string, string, Access][] = [
  ["form", "<form> の送信", { send: true, read: false, cookie: true }],
  ["img", "<img src>", { send: true, read: false, cookie: true }],
  ["script", "<script src>", { send: true, read: false, cookie: true }],
  ["link", "リンクを踏む", { send: true, read: false, cookie: true }],
  ["fetch", "fetch()", { send: true, read: false, cookie: false }],
  ["fetch-credentials", 'fetch(…, {credentials:"include"})', { send: true, read: false, cookie: true }],
];

// ③ プリフライト
const SIMPLE_METHOD = new Set(["GET", "HEAD", "POST"]);
const SIMPLE_TYPE = new Set(["application/x-www-form-urlencoded", "multipart/form-data", "text/plain"]);
const SIMPLE_HEADER = new Set(["accept", "accept-language", "content-language", "content-type"]);
function needsPreflight(method: string, ct: string, headers: string[]) {
  if (!SIMPLE_METHOD.has(method.toUpperCase())) return true;
  const t = ct.split(";")[0].trim().toLowerCase();
  if (t && !SIMPLE_TYPE.has(t)) return true;
  return headers.some((h) => !SIMPLE_HEADER.has(h.toLowerCase()));
}
const REQS: [string, string, string, string[]][] = [
  ["GET", "GET", "", []],
  ["POST フォーム形式", "POST", "application/x-www-form-urlencoded", []],
  ["POST multipart", "POST", "multipart/form-data; boundary=x", []],
  ["POST text/plain", "POST", "text/plain", []],
  ["POST JSON", "POST", "application/json", []],
  ["PUT", "PUT", "", []],
  ["DELETE", "DELETE", "", []],
  ["GET + 独自ヘッダ", "GET", "", ["X-Token"]],
];

// ④ CORS の設定
interface Policy { allow: string[]; wildcard?: boolean; credentials?: boolean; reflect?: boolean }
const ME = "https://app.example.com";
const ATTACKER = "https://evil.test";
function decide(p: Policy, origin: string, withCookie: boolean) {
  if (p.wildcard && withCookie && p.credentials)
    return { header: "*", readable: false, reason: "* とクッキー付きは同時に使えない" };
  if (p.wildcard && withCookie)
    return { header: "*", readable: false, reason: "クッキー付きなのに許可が * になっている" };
  if (p.wildcard) return { header: "*", readable: true, reason: "どこからでも読める" };
  if (p.reflect)
    return { header: origin, readable: true, reason: "Origin をそのまま返している(実質すべて許可)" };
  if (p.allow.includes(origin)) {
    if (withCookie && !p.credentials)
      return { header: origin, readable: false, reason: "許可した相手だが、クッキー付きは許していない" };
    return { header: origin, readable: true, reason: "名指しで許可されている" };
  }
  return { header: "(返さない)", readable: false, reason: "許可した一覧に無い" };
}
const POLICIES: [string, Policy][] = [
  ["名指し", { allow: [ME], credentials: true }],
  ["*", { allow: [], wildcard: true }],
  ["* + クッキー許可", { allow: [], wildcard: true, credentials: true }],
  ["Origin をそのまま返す", { allow: [], reflect: true }],
];

const view = ref<"origin" | "access" | "preflight" | "cors">("access");
const VIEWS = [
  ["access", "送れる / 読める"],
  ["origin", "生成元の比較"],
  ["preflight", "先に問い合わせるか"],
  ["cors", "CORS の設定"],
] as const;

const sendable = KINDS.filter(([, , a]) => a.send).length;
const readable = KINDS.filter(([, , a]) => a.read).length;

const badge = computed(() =>
  view.value === "access" ? `送れる ${sendable} / 読める ${readable}`
  : view.value === "origin" ? "基準 https://example.com/login"
  : view.value === "preflight" ? "単純要求かどうか"
  : "攻撃者からクッキー付きで");

const verdict = computed(() => {
  if (view.value === "access")
    return `${KINDS.length} 通りのどれでも送れて、どれも読めない。クッキーが自動で付くのは ${KINDS.filter(([, , a]) => a.cookie).length} 通り。攻撃者は応答を読めないが、送るだけで足りる用件なら読む必要が無い`;
  if (view.value === "origin")
    return "パス・クエリ・断片は生成元に入らない。だから同じホストに置いたものは互いに全部読める。分けたいならホストかポートを変えることになる";
  if (view.value === "preflight")
    return "問い合わせなしで通るのは、だいたい <form> で送れたものになる。Content-Type を JSON にした瞬間に OPTIONS が飛ぶのは、フォームでは送れなかった形だからだ";
  return "危ないのは Origin をそのまま返す設定だけ。* には「クッキー付きでは使えない」という安全弁があるが、反射には無い。しかも許可した一覧は空なので、設定を読んでも気づけない";
});
</script>

<template>
  <DemoShell title="同一生成元ポリシーと CORS" :badge="badge">
    <div class="so-actions">
      <span class="sd-seg">
        <span v-for="[k, label] in VIEWS" :key="k" class="sd-seg-opt"
              :class="{ on: view === k }" @click="view = k">{{ label }}</span>
      </span>
    </div>

    <div class="so-scroll">
      <table v-if="view === 'access'" class="so-t">
        <thead><tr><th>やり方</th><th>送れる</th><th>読める</th><th>クッキー</th></tr></thead>
        <tbody>
          <tr v-for="[k, label, a] in KINDS" :key="k">
            <td class="mono">{{ label }}</td>
            <td :class="a.send ? 'yes' : 'no'">{{ a.send ? "通る" : "止まる" }}</td>
            <td :class="a.read ? 'yes' : 'no'">{{ a.read ? "読める" : "読めない" }}</td>
            <td :class="a.cookie ? 'warn' : 'no'">{{ a.cookie ? "付く" : "付かない" }}</td>
          </tr>
        </tbody>
      </table>

      <table v-else-if="view === 'origin'" class="so-t">
        <thead><tr><th>相手</th><th>判定</th><th>なぜ</th></tr></thead>
        <tbody>
          <tr v-for="[u, why] in OTHERS" :key="u">
            <td class="mono">{{ u }}</td>
            <td :class="same(parse(BASE), parse(u)) ? 'yes' : 'no'">
              {{ same(parse(BASE), parse(u)) ? "同じ" : "違う" }}
            </td>
            <td class="so-why">{{ why }}</td>
          </tr>
        </tbody>
      </table>

      <table v-else-if="view === 'preflight'" class="so-t">
        <thead><tr><th>要求</th><th>Content-Type</th><th>先に問い合わせる</th></tr></thead>
        <tbody>
          <tr v-for="[name, m, ct, hs] in REQS" :key="name">
            <td class="mono">{{ name }}</td>
            <td class="mono so-why">{{ ct || "—" }}</td>
            <td :class="needsPreflight(m, ct, hs) ? 'warn' : 'yes'">
              {{ needsPreflight(m, ct, hs) ? "要る" : "要らない" }}
            </td>
          </tr>
        </tbody>
      </table>

      <table v-else class="so-t">
        <thead><tr><th>設定</th><th>攻撃者に返す値</th><th>読めるか</th><th>理由</th></tr></thead>
        <tbody>
          <tr v-for="[name, p] in POLICIES" :key="name">
            <td class="mono">{{ name }}</td>
            <td class="mono so-why">{{ decide(p, ATTACKER, true).header }}</td>
            <td :class="decide(p, ATTACKER, true).readable ? 'bad' : 'yes'">
              {{ decide(p, ATTACKER, true).readable ? "読める" : "読めない" }}
            </td>
            <td class="so-why">{{ decide(p, ATTACKER, true).reason }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="so-verdict">{{ verdict }}</div>

    <p class="so-note">
      同一生成元ポリシーは「別のサイトへ行かせない」仕組みではない。行けるし、届くし、クッキーも付く。
      応答をスクリプトから読ませないだけだ。CORS はその読み取りを開ける仕組みで、送信のほうは別に止める必要がある。
    </p>
  </DemoShell>
</template>

<style scoped>
.so-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.so-scroll { overflow-x: auto; margin-top: 14px; border: 1px solid var(--vp-c-divider); }
.so-t { border-collapse: collapse; width: 100%; font-size: 11.5px; }
.so-t th, .so-t td { padding: 6px 10px; text-align: left; border-bottom: 1px solid var(--vp-c-divider); white-space: nowrap; }
.so-t thead th { background-color: var(--vp-c-bg-soft); font-size: 10px; color: var(--vp-c-text-3); font-weight: 600; }
.so-t tbody tr:last-child td { border-bottom: 0; }
.so-t td.yes { color: var(--vp-c-green-1); font-weight: 600; }
.so-t td.no { color: var(--vp-c-text-3); }
.so-t td.warn { color: var(--vp-c-yellow-1); font-weight: 600; }
.so-t td.bad { color: var(--vp-c-danger-1); font-weight: 700; }
.so-why { color: var(--vp-c-text-2); font-weight: 400; white-space: normal; }
.so-verdict {
  margin-top: 14px; padding: 8px 12px; background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1); font-size: 12.5px; line-height: 1.6; color: var(--vp-c-text-1);
}
.so-note {
  margin: 14px 0 0; padding-top: 12px; border-top: 1px solid var(--vp-c-divider);
  font-size: 12px; line-height: 1.7; color: var(--vp-c-text-2);
}
.mono { font-family: var(--vp-font-family-mono); }
</style>
