import { type Router } from "./router";
import { renderToString } from "./ssr";
import { hydrate } from "./hydrate";

// SSR → hydrate の一連の流れをまとめる。
// サーバ: renderRoute で HTML文字列を作って返す。
// クライアント: hydrateRoute で、その HTML の上にイベントを付けて動かす。

// #region app{ts}
// サーバ側: パスに対応するページを HTML文字列にして返す。
// 未一致なら空文字（404 をどう出すかは呼び出し側=フレームワーク利用者の責務）。
export function renderRoute(router: Router, url: string): string {
  const matched = router.resolve(url);
  if (!matched) return "";
  const tree = matched.page(matched.params);
  return renderToString(tree);
}

// クライアント側: 同じパスから同じ木を作り、サーバ描画済みの container にハイドレート。
// tree を作り直しても mount はしない——既存DOMにイベントを付けるだけ。
export function hydrateRoute(router: Router, url: string, container: HTMLElement): void {
  const matched = router.resolve(url);
  if (!matched) return;
  const tree = matched.page(matched.params);
  const root = container.firstChild;
  if (root) hydrate(tree, root);
}
// #endregion app{ts}
