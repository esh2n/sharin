// HTMLパーサ: HTML文字列を DOM 木にする。SSR(mini-next)の renderToString の逆向き。
// 再帰下降パーサ——「今の位置」を持ち、先頭を覗いて要素かテキストかを決めて食べ進む。

// #region dom{ts}
export interface Element {
  kind: "element";
  tag: string;
  attrs: Record<string, string>;
  children: DomNode[];
}
export interface Text {
  kind: "text";
  text: string;
}
export type DomNode = Element | Text;

export const elem = (tag: string, attrs: Record<string, string>, children: DomNode[]): Element => ({ kind: "element", tag, attrs, children });
export const text = (t: string): Text => ({ kind: "text", text: t });
// #endregion dom{ts}

// #region parser{ts}
// 入力文字列と現在位置を持つだけの素朴なパーサ。
class Parser {
  private pos = 0;
  constructor(private readonly input: string) {}

  // 1ノードを読む: '<' で始まれば要素、それ以外はテキスト。
  parseNode(): DomNode {
    return this.peek() === "<" ? this.parseElement() : this.parseText();
  }

  private parseElement(): Element {
    this.expect("<");
    const tag = this.parseName();
    const attrs = this.parseAttributes();
    this.expect(">");
    const children = this.parseNodes(); // 閉じタグまで子を読む
    this.expect("</");
    this.parseName();
    this.expect(">");
    return elem(tag, attrs, children);
  }

  // 閉じタグ '</' か入力末尾までノードを読み続ける。
  parseNodes(): DomNode[] {
    const nodes: DomNode[] = [];
    while (true) {
      this.skipWhitespace();
      if (this.eof() || this.startsWith("</")) break;
      nodes.push(this.parseNode());
    }
    return nodes;
  }

  private parseText(): Text {
    let s = "";
    while (!this.eof() && this.peek() !== "<") s += this.consume();
    return text(s.trim());
  }

  private parseAttributes(): Record<string, string> {
    const attrs: Record<string, string> = {};
    while (true) {
      this.skipWhitespace();
      if (this.peek() === ">" || this.eof()) break;
      const name = this.parseName();
      this.expect("=");
      const quote = this.consume(); // ' か "
      let value = "";
      while (!this.eof() && this.peek() !== quote) value += this.consume();
      this.expect(quote);
      attrs[name] = value;
    }
    return attrs;
  }

  private parseName(): string {
    let s = "";
    while (!this.eof() && /[a-zA-Z0-9-]/.test(this.peek())) s += this.consume();
    return s;
  }
  // #endregion parser{ts}

  private peek(): string {
    return this.input[this.pos] ?? "";
  }
  private consume(): string {
    return this.input[this.pos++] ?? "";
  }
  private startsWith(s: string): boolean {
    return this.input.startsWith(s, this.pos);
  }
  private expect(s: string): void {
    if (!this.startsWith(s)) throw new Error(`HTML parse: 位置 ${this.pos} で "${s}" を期待したが "${this.input.slice(this.pos, this.pos + 6)}"`);
    this.pos += s.length;
  }
  private skipWhitespace(): void {
    while (!this.eof() && /\s/.test(this.peek())) this.pos++;
  }
  private eof(): boolean {
    return this.pos >= this.input.length;
  }
}

// 単一ルート要素を前提にパースする。複数ルートなら暗黙の <html> で包む。
export function parseHTML(input: string): DomNode {
  const parser = new Parser(input.trim());
  const nodes = parser.parseNodes();
  if (nodes.length === 1) return nodes[0] as DomNode;
  return elem("html", {}, nodes);
}
