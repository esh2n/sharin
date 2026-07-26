// NOTE: デモは VitePress のテーマ変数(--vp-c-*)前提で色を塗る。素の DOM に mount すると
//       これらが無く色が出ないので、VitePress デフォルトテーマの CSS をここで読み込ませる。
//       fonts.css は Web フォント取得(ネットワーク)を伴うので入れない — 同一環境なら
//       システムフォントで描画が決定的になる。
import "vitepress/dist/client/theme-default/styles/vars.css";
import "vitepress/dist/client/theme-default/styles/base.css";

// NOTE: 点滅カーソルやアニメーション途中を撮らないための決定化。撮影側でも
//       animations: disabled を指定するが、CSS でも二重に止める。
const style = document.createElement("style");
style.textContent =
  "*, *::before, *::after { animation: none !important; transition: none !important; caret-color: transparent !important; }";
document.head.appendChild(style);
