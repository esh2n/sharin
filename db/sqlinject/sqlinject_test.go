package sqlinject

import "testing"

// この章の中心その1。注入とは、値が構文に何トークンも寄与してしまうことだ。
func TestInjectionIsValueBecomingSyntax(t *testing.T) {
	safe := Build(Concat, Quoted, "esh2n")
	bad := Build(Concat, Quoted, `' OR '1'='1`)

	t.Logf("普通の値  %s", safe.Text)
	for _, tk := range safe.FromValue() {
		t.Logf("    値の寄与  %q", tk.Text)
	}
	t.Logf("攻撃の値  %s", bad.Text)
	for _, tk := range bad.FromValue() {
		t.Logf("    値の寄与  %q", tk.Text)
	}
	t.Logf("値が構文に寄与したトークン数  普通 %d / 攻撃 %d",
		len(safe.FromValue()), len(bad.FromValue()))

	// 普通の値は、文字列トークン1つの中に収まる。
	if n := len(safe.FromValue()); n != 1 {
		t.Fatalf("普通の値が %d トークン", n)
	}
	if safe.Injected() {
		t.Fatal("普通の値が注入と判定された")
	}
	// 攻撃の値は、引用符を閉じたところで文字列でなくなり、構文になる。
	if n := len(bad.FromValue()); n < 3 {
		t.Fatalf("攻撃の値が %d トークンにしかならない", n)
	}
	if !bad.Injected() {
		t.Fatal("注入が検出されない")
	}
	// キーワードが値の側から出てきていることを確かめる。
	found := false
	for _, tk := range bad.FromValue() {
		if IsKeyword(tk) {
			found = true
			t.Logf("値から出たキーワード: %q", tk.Text)
		}
	}
	if !found {
		t.Fatal("値からキーワードが出ていない")
	}
}

// この章の中心その2。プレースホルダは、値を問い合わせに入れない。
func TestPlaceholderKeepsTheQueryShapeConstant(t *testing.T) {
	values := []string{"esh2n", `' OR '1'='1`, `'; DROP TABLE users;--`, ""}
	var shapes []string
	for _, v := range values {
		q := Build(Placeholder, Quoted, v)
		shapes = append(shapes, q.Text)
		t.Logf("値 %-26q → %s  (別経路 %q)", v, q.Text, q.Params)
		if q.Injected() {
			t.Fatalf("%q で注入された", v)
		}
		// 値が構文に寄与したトークンは 0。文字列に入っていないからだ。
		if n := len(q.FromValue()); n != 0 {
			t.Fatalf("%q が構文に %d トークン寄与した", v, n)
		}
	}
	// 入力が何であれ、問い合わせの形は同じ。
	for i := 1; i < len(shapes); i++ {
		if shapes[i] != shapes[0] {
			t.Fatalf("形が変わった: %q と %q", shapes[0], shapes[i])
		}
	}
	t.Log("入力が何であっても問い合わせの形は変わらない。だから構文が動きようがない")
}

// この章の中心その3。引用符を二重にする守りは、引用符の無い場所では効かない。
func TestEscapingDoesNothingWhereThereAreNoQuotes(t *testing.T) {
	t.Logf("%-24s %-16s %s", "場所", "混ぜ方", "止めた")
	for _, s := range Slots() {
		for _, m := range Modes() {
			if !Usable(m, s) {
				t.Logf("%-24s %-16s 使えない", s, m)
				continue
			}
			st, total := Score(m, s)
			t.Logf("%-24s %-16s %d / %d", s, m, st, total)
		}
	}

	// 引用符ありの場所では、二重にする守りが効く。
	if st, total := Score(QuoteEscape, Quoted); st != total {
		t.Fatalf("引用符ありで止まらない: %d/%d", st, total)
	}
	// 引用符なしの場所では、変換する対象が無いので何も起きない。
	before := Build(Concat, Bare, `1 OR 1=1`)
	after := Build(QuoteEscape, Bare, `1 OR 1=1`)
	t.Logf("引用符なしの場所")
	t.Logf("  連結     %s", before.Text)
	t.Logf("  二重化   %s", after.Text)
	if before.Text != after.Text {
		t.Fatal("引用符が無いのに変換されている")
	}
	if !after.Injected() {
		t.Fatal("引用符なしの場所で止まってしまった")
	}
	t.Log("変換すべき文字が入力に無ければ、その守りは何もしない")
}

// この章の中心その4。識別子にはプレースホルダを使えない。
func TestIdentifiersCannotUsePlaceholdersAtAll(t *testing.T) {
	// 列名は値ではなく構文の一部なので、実行時に差し込めない。
	if Usable(Placeholder, Ident) {
		t.Fatal("識別子でプレースホルダが使えることになっている")
	}
	for _, s := range []Slot{Quoted, Bare} {
		if !Usable(Placeholder, s) {
			t.Fatalf("%v で使えないことになっている", s)
		}
	}

	// 連結も二重化も止められない。
	a := `name; DROP TABLE users;--`
	for _, m := range []Mode{Concat, QuoteEscape} {
		q := Build(m, Ident, a)
		t.Logf("%-16s %s → 注入 %v", m, q.Text, q.Injected())
		if !q.Injected() {
			t.Fatalf("%v で止まってしまった", m)
		}
	}

	// 残るのは許可制になる。
	allow := []string{"id", "name", "created_at"}
	if got := AllowIdent(allow, "created_at"); got != "created_at" {
		t.Fatalf("許可した名前が通らない: %q", got)
	}
	if got := AllowIdent(allow, a); got != "id" {
		t.Fatalf("知らない名前が既定へ倒れない: %q", got)
	}
	q := Build(Concat, Ident, AllowIdent(allow, a))
	t.Logf("許可制を通したあと: %s → 注入 %v", q.Text, q.Injected())
	if q.Injected() {
		t.Fatal("許可制を通しても注入された")
	}
	t.Log("値として渡せない場所は、選別で守るしかない")
}

// 3つの混ぜ方を、3つの場所すべてに当てた総当たり。
func TestNoSingleWayCoversEveryPlace(t *testing.T) {
	t.Logf("%-16s %s", "混ぜ方", "全部止めた場所")
	for _, m := range Modes() {
		good := 0
		for _, s := range Slots() {
			if !Usable(m, s) {
				continue
			}
			st, total := Score(m, s)
			if st == total {
				good++
			}
		}
		t.Logf("%-16v %d / %d", m, good, len(Slots()))
		if good == len(Slots()) {
			t.Fatalf("%v が3か所すべてを守れている。それはおかしい", m)
		}
	}
	t.Log("プレースホルダでも識別子は守れない。そこだけは許可制になる")
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	// 表示名。
	for _, m := range Modes() {
		if m.String() == "" {
			t.Fatal("混ぜ方の名前が空")
		}
	}
	for _, s := range Slots() {
		if s.String() == "" {
			t.Fatal("場所の名前が空")
		}
	}
	// 空の値は注入にならない。
	if Build(Concat, Quoted, "").Injected() {
		t.Fatal("空の値が注入と判定された")
	}
	// 1 トークンでも、それがキーワードなら注入になる。
	// 識別子の場所に or と書けば、そこは値ではなく構文になる。
	kw := Build(Concat, Ident, "or")
	if n := len(kw.FromValue()); n != 1 {
		t.Fatalf("寄与が %d トークン", n)
	}
	if !kw.Injected() {
		t.Fatal("1 トークンのキーワードが見逃された")
	}
	// 1 トークンで、キーワードでもなければ通る。
	if Build(Concat, Ident, "name").Injected() {
		t.Fatal("普通の列名が注入と判定された")
	}
	// 文字列の中の '' は閉じない。
	toks := Lex(`SELECT * FROM t WHERE a = 'it''s'`)
	last := toks[len(toks)-1]
	if last.Kind != Str || last.Text != `'it''s'` {
		t.Fatalf("二重引用符の扱い: %v %q", last.Kind, last.Text)
	}
	// 閉じられていない文字列は、末尾までで1つ。
	u := Lex(`a = 'open`)
	if u[len(u)-1].Kind != Str {
		t.Fatal("閉じていない文字列が文字列になっていない")
	}
	// コメントは行末まで。
	c := Lex("SELECT 1 -- drop\nSELECT 2")
	n := 0
	for _, tk := range c {
		if tk.Kind == Comment {
			n++
			if tk.Text != "-- drop" {
				t.Fatalf("コメント: %q", tk.Text)
			}
		}
	}
	if n != 1 {
		t.Fatalf("コメントの数: %d", n)
	}
	// 数値と演算子。
	if k := Lex("1=1")[0].Kind; k != Num {
		t.Fatalf("数値の種類: %v", k)
	}
	if k := Lex("1=1")[1].Kind; k != Op {
		t.Fatalf("演算子の種類: %v", k)
	}
	// 攻撃はどれか1つ以上の場所を狙っている。
	for _, a := range Attacks() {
		if len(a.Slots) == 0 {
			t.Fatalf("狙う場所が無い: %s", a.Name)
		}
		if Hits(a, Slot(99)) {
			t.Fatal("知らない場所に当たっている")
		}
	}
}
