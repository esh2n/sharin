import { parseHTML } from "./html";
import { parseCSS } from "./css";
import { styleTree } from "./style";
import { layout, type LayoutBox } from "./layout";
import { paint, type DisplayCommand } from "./paint";

// レンダリングパイプライン全体: HTML + CSS + 幅 → 描画コマンド一覧。
// parse(HTML) → parse(CSS) → style(合体) → layout(位置決め) → paint(コマンド化)。

// #region render{ts}
export interface RenderResult {
  layout: LayoutBox;
  display: DisplayCommand[];
}

export function render(html: string, css: string, width: number): RenderResult {
  const dom = parseHTML(html); // ① HTML文字列 → DOM木
  const sheet = parseCSS(css); // ② CSS文字列 → 規則
  const styled = styleTree(dom, sheet); // ③ DOM × CSS → スタイル木
  const box = layout(styled, width); // ④ スタイル木 → 位置と大きさ
  const display = paint(box); // ⑤ レイアウト木 → 描画コマンド
  return { layout: box, display };
}
// #endregion render{ts}
