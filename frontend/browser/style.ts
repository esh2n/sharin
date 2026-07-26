import { type DomNode, type Element } from "./html";
import { type Stylesheet, type Selector, type Declaration, specificity, cmpSpecificity } from "./css";

// スタイル計算: DOM木の各要素に、マッチした CSS 宣言を貼り付けて「スタイル木」を作る。
// 同じプロパティが複数当たったら、詳細度の高い規則が勝つ(カスケード)。

// #region types{ts}
export interface StyledNode {
  node: DomNode;
  specified: Record<string, string>; // 確定したプロパティ
  children: StyledNode[];
}
// #endregion types{ts}

// #region match{ts}
// セレクタが要素にマッチするか。tag/id/class がすべて合致すれば真。
function matches(el: Element, sel: Selector): boolean {
  if (sel.tag && sel.tag !== el.tag) return false;
  if (sel.id && sel.id !== el.attrs.id) return false;
  const classList = (el.attrs.class ?? "").split(/\s+/).filter(Boolean);
  return sel.classes.every((c) => classList.includes(c));
}

// 要素に当たる宣言をすべて集め、詳細度順に適用して確定値を作る。
function specifiedValues(el: Element, sheet: Stylesheet): Record<string, string> {
  const matched: { spec: ReturnType<typeof specificity>; decls: Declaration[] }[] = [];
  for (const rule of sheet.rules) {
    // 規則内で最も詳細度の高いマッチセレクタを、その規則の詳細度とする
    const best = rule.selectors.filter((s) => matches(el, s)).sort((a, b) => cmpSpecificity(specificity(b), specificity(a)))[0];
    if (best) matched.push({ spec: specificity(best), decls: rule.declarations });
  }
  // 詳細度の低い順に適用 → 高い規則が後勝ちで上書きする(カスケード)
  matched.sort((a, b) => cmpSpecificity(a.spec, b.spec));
  const values: Record<string, string> = {};
  for (const m of matched) for (const d of m.decls) values[d.name] = d.value;
  return values;
}
// #endregion match{ts}

// #region tree{ts}
// 子へ受け継ぐプロパティ(継承)。color や font 系はテキストにも伝わる(box系は伝わらない)。
const INHERITED = ["color", "font-size", "font-family"];

export function styleTree(root: DomNode, sheet: Stylesheet, inherited: Record<string, string> = {}): StyledNode {
  const own = root.kind === "element" ? specifiedValues(root, sheet) : {};
  // 継承値を土台に、自分の指定で上書きする(自分の指定が勝つ)
  const specified = { ...inherited, ...own };
  // 子に渡す継承値: 継承対象プロパティだけを抜き出す
  const forChildren: Record<string, string> = {};
  for (const p of INHERITED) if (specified[p] !== undefined) forChildren[p] = specified[p] as string;
  const children = root.kind === "element" ? root.children.map((c) => styleTree(c, sheet, forChildren)) : [];
  return { node: root, specified, children };
}

// display の既定はテキストなら inline、要素なら block(簡略)。none は描かない。
export function display(styled: StyledNode): "block" | "inline" | "none" {
  const d = styled.specified.display;
  if (d === "none") return "none";
  if (d === "inline") return "inline";
  return styled.node.kind === "text" ? "inline" : "block";
}
// #endregion tree{ts}
