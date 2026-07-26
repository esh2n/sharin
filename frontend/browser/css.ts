// CSSパーサ: スタイルシート文字列を規則(セレクタ + 宣言)の配列にする。
// セレクタは tag / .class / #id の単純セレクタのみ。組み合わせ(子孫結合子など)は扱わない。

// #region types{ts}
export interface Selector {
  tag?: string;
  id?: string;
  classes: string[];
}
export interface Declaration {
  name: string;
  value: string;
}
export interface Rule {
  selectors: Selector[];
  declarations: Declaration[];
}
export interface Stylesheet {
  rules: Rule[];
}

// 詳細度(specificity): id, class, tag の個数のタプル。大きいほど優先。
export type Specificity = [number, number, number];
export function specificity(sel: Selector): Specificity {
  return [sel.id ? 1 : 0, sel.classes.length, sel.tag ? 1 : 0];
}
// #endregion types{ts}

// #region parse{ts}
export function parseCSS(input: string): Stylesheet {
  const rules: Rule[] = [];
  // 規則を "}" で分割する素朴な方式（ネストや @media は無い前提）。
  for (const chunk of input.split("}")) {
    const [head, body] = chunk.split("{");
    if (head === undefined || body === undefined) continue; // 末尾の空白など
    const selectors = parseSelectors(head);
    const declarations = parseDeclarations(body);
    if (selectors.length > 0) rules.push({ selectors, declarations });
  }
  return { rules };
}

// "h1, .note, #main" → セレクタ配列。詳細度の高い順に並べておく。
function parseSelectors(input: string): Selector[] {
  const selectors = input
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
    .map(parseSimpleSelector);
  selectors.sort((a, b) => cmpSpecificity(specificity(b), specificity(a)));
  return selectors;
}

// "div.note#main" のような単純セレクタを分解する。
function parseSimpleSelector(input: string): Selector {
  const sel: Selector = { classes: [] };
  // .class / #id / tag を先頭から順に取り出す
  const tokens = input.match(/[.#]?[a-zA-Z0-9-]+/g) ?? [];
  for (const tok of tokens) {
    if (tok.startsWith(".")) sel.classes.push(tok.slice(1));
    else if (tok.startsWith("#")) sel.id = tok.slice(1);
    else sel.tag = tok;
  }
  return sel;
}

function parseDeclarations(input: string): Declaration[] {
  return input
    .split(";")
    .map((d) => d.trim())
    .filter(Boolean)
    .map((d) => {
      const i = d.indexOf(":");
      return { name: d.slice(0, i).trim(), value: d.slice(i + 1).trim() };
    });
}
// #endregion parse{ts}

export function cmpSpecificity(a: Specificity, b: Specificity): number {
  return a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
}
