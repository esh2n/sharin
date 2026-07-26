import { type VNode } from "../vdom/vdom";

// ルーティング: URL のパスをページコンポーネントに対応づける。
// Next.js の pages/posts/[id].tsx が /posts/:id に対応するのと同じ発想を、
// ファイルの代わりに明示的な route 配列で表す。

export type Params = Record<string, string>;
export type PageComponent = (params: Params) => VNode;

export interface Route {
  path: string; // "/posts/:id" のように :name で動的セグメントを表す
  page: PageComponent;
}

export interface Matched {
  page: PageComponent;
  params: Params;
}

export interface Router {
  resolve(url: string): Matched | null;
}

// #region router{ts}
export function createRouter(routes: Route[]): Router {
  // 各 route のパスをセグメント配列に前処理しておく。
  const compiled = routes.map((r) => ({ segments: split(r.path), page: r.page }));

  return {
    resolve(url) {
      const target = split(stripQuery(url));
      // 登録順に先頭一致で採用する（静的 route を動的より先に置けば優先できる）
      for (const route of compiled) {
        const params = matchSegments(route.segments, target);
        if (params) return { page: route.page, params };
      }
      return null;
    },
  };
}

// パスのセグメントどうしを突き合わせる。:name は任意の1セグメントを捕まえて params に入れる。
// 一致しなければ null。
function matchSegments(pattern: string[], target: string[]): Params | null {
  if (pattern.length !== target.length) return null;
  const params: Params = {};
  for (let i = 0; i < pattern.length; i++) {
    const p = pattern[i] as string;
    const t = target[i] as string;
    if (p.startsWith(":")) {
      params[p.slice(1)] = decodeURIComponent(t); // 動的: 捕獲
    } else if (p !== t) {
      return null; // 静的: 不一致
    }
  }
  return params;
}
// #endregion router{ts}

const stripQuery = (url: string): string => {
  const q = url.indexOf("?");
  return q === -1 ? url : url.slice(0, q);
};

// "/posts/42" → ["posts","42"]。前後のスラッシュと空セグメントを落とす。
const split = (path: string): string[] => path.split("/").filter((s) => s.length > 0);
