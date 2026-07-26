import { type StyledNode, display } from "./style";

// レイアウト: スタイル木から、各ボックスの位置(x,y)と大きさ(width,height)を計算する。
// ここではブロックレイアウトだけを扱う——子は上から下へ縦に積む。
// 簡易ボックスモデル: margin / padding は "10px" のような単一px値を四辺に適用する。

// #region types{ts}
export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}
export interface LayoutBox {
  rect: Rect; // border-box(padding込み・margin除く)
  styled: StyledNode;
  children: LayoutBox[];
}
// #endregion types{ts}

const LINE_HEIGHT = 18; // テキスト1行の高さ(簡略)

// "120px" / "120" → 120。数値でなければ undefined。
function px(value: string | undefined): number | undefined {
  if (value === undefined) return undefined;
  const n = parseFloat(value);
  return Number.isNaN(n) ? undefined : n;
}
const prop = (s: StyledNode, name: string): number => px(s.specified[name]) ?? 0;

// #region layout{ts}
// 入口: ルートを、幅 containerWidth・原点(0,0) の中にレイアウトする。
export function layout(root: StyledNode, containerWidth: number): LayoutBox {
  return layoutBlock(root, { x: 0, y: 0, width: containerWidth, height: 0 }, 0);
}

// content: 親のコンテンツ領域(padding の内側)。offsetY: その中での縦の開始位置。
function layoutBlock(styled: StyledNode, content: Rect, offsetY: number): LayoutBox {
  const margin = prop(styled, "margin");
  const padding = prop(styled, "padding");

  // 幅: 明示 width があればそれ、無ければ親コンテンツ幅から左右marginを引いて満たす
  const width = px(styled.specified.width) ?? content.width - 2 * margin;
  const x = content.x + margin;
  const y = content.y + offsetY + margin;

  // 自分のコンテンツ領域(padding の内側)。子はここに積む。
  const innerX = x + padding;
  const innerY = y + padding;
  const innerW = width - 2 * padding;

  const children: LayoutBox[] = [];
  let childOffset = 0;
  for (const child of styled.children) {
    if (display(child) === "none") continue; // display:none は箱を作らない
    const box = layoutBlock(child, { x: innerX, y: innerY, width: innerW, height: 0 }, childOffset);
    const childMargin = prop(child, "margin");
    childOffset += box.rect.height + 2 * childMargin; // 次の子は前の子の下へ
    children.push(box);
  }

  // 高さ: 明示 height、無ければ子の合計。子も無ければテキストなら1行、空なら0。
  const explicit = px(styled.specified.height);
  const contentHeight = children.length > 0 ? childOffset : leafHeight(styled);
  const height = (explicit ?? contentHeight) + 2 * padding;

  return { rect: { x, y, width, height }, styled, children };
}

// 葉ボックスの高さ: 中身のあるテキストなら1行分、それ以外は0。
function leafHeight(styled: StyledNode): number {
  const n = styled.node;
  return n.kind === "text" && n.text.length > 0 ? LINE_HEIGHT : 0;
}
// #endregion layout{ts}
