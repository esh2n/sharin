import { type VNode, isEventProp } from "../vdom/vdom";

// SSR(Server-Side Rendering): 同じ仮想DOM木を、実DOMでなく HTML文字列に変換する。
// サーバで文字列を組み立てて返せば、初回表示が速く、クローラも中身を読める。
// クライアントに実DOMは要らない——文字列を組むだけなので Node でも動く。

// 閉じタグを持たない void 要素。
const VOID = new Set(["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr"]);

// #region render{ts}
export function renderToString(vnode: VNode): string {
  // テキストは中身をエスケープするだけ（タグ注入を防ぐ最初の一歩）
  if (vnode.kind === "text") return escapeHtml(vnode.text);

  const attrs = Object.entries(vnode.props)
    .filter(([key, value]) => !isEventProp(key) && value !== false && value !== null && value !== undefined)
    .map(([key, value]) => ` ${key}="${escapeAttr(String(value))}"`)
    .join("");

  // イベント(on*)は関数なので文字列化できない → 出力しない。
  // ハイドレーション(hydrate.ts)でクライアント側から付け直す。
  if (VOID.has(vnode.type)) return `<${vnode.type}${attrs}>`;

  const inner = vnode.children.map(renderToString).join("");
  return `<${vnode.type}${attrs}>${inner}</${vnode.type}>`;
}
// #endregion render{ts}

// #region escape{ts}
// テキスト中の < > & を実体参照に。これを怠ると文字列がタグとして解釈される(XSS)。
function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}
// 属性値は " で囲むので、" と & を実体参照に。
function escapeAttr(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/"/g, "&quot;");
}
// #endregion escape{ts}
