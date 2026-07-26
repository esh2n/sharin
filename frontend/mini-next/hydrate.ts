import { type VNode, isEventProp, eventName } from "../vdom/vdom";

// ハイドレーション(hydration): サーバが吐いた静的HTMLを「作り直さず」に、
// イベントリスナだけを後付けして対話可能にする。
//
// なぜ mount で作り直さないのか: サーバ描画済みの実DOMが既に画面にある。
// それを捨てて mount し直すと、一瞬ちらつくし無駄。既存ノードを使い回し、
// 「文字列HTMLには乗らなかった部分（＝イベント）」だけを付けるのがハイドレーション。

// #region hydrate{ts}
// vnode 木と、サーバ描画済みの実ノードを平行にたどり、on* をリスナとして付ける。
export function hydrate(vnode: VNode, node: Node): void {
  if (vnode.kind === "text") return; // テキストは中身だけ。付けるものは無い

  const el = node as HTMLElement;
  for (const [key, value] of Object.entries(vnode.props)) {
    if (isEventProp(key)) {
      el.addEventListener(eventName(key), value as EventListener);
    }
    // 属性は既にサーバHTMLに乗っている。ここでは触らない（作り直さないのが肝）
  }

  // 子を同じ位置(index)どうしで対応づけて再帰。
  // サーバとクライアントで同じ木を描いていれば構造は一致している前提。
  vnode.children.forEach((child, i) => {
    const childNode = el.childNodes[i];
    if (childNode) hydrate(child, childNode);
  });
}
// #endregion hydrate{ts}
