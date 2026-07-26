import { type LayoutBox } from "./layout";

// ペイント: レイアウト木を「描画コマンドの一覧(ディスプレイリスト)」に変換する。
// 実際のブラウザはこのリストを GPU に渡す。ここでは背景色の矩形とテキストだけを出す。

// #region types{ts}
export type DisplayCommand =
  | { type: "rect"; x: number; y: number; width: number; height: number; color: string }
  | { type: "text"; x: number; y: number; text: string; color: string };
// #endregion types{ts}

// #region paint{ts}
export function paint(root: LayoutBox): DisplayCommand[] {
  const list: DisplayCommand[] = [];
  paintBox(root, list);
  return list;
}

function paintBox(box: LayoutBox, list: DisplayCommand[]): void {
  // 1. 背景: background 指定があれば矩形を積む(親→子の順=子が上に来る)
  const bg = box.styled.specified.background ?? box.styled.specified["background-color"];
  if (bg) {
    const { x, y, width, height } = box.rect;
    list.push({ type: "rect", x, y, width, height, color: bg });
  }
  // 2. テキスト: テキストノードなら文字を積む
  const node = box.styled.node;
  if (node.kind === "text" && node.text.length > 0) {
    list.push({ type: "text", x: box.rect.x, y: box.rect.y, text: node.text, color: box.styled.specified.color ?? "#000" });
  }
  // 3. 子を再帰的に描く(後に積んだものが手前)
  for (const child of box.children) paintBox(child, list);
}
// #endregion paint{ts}
