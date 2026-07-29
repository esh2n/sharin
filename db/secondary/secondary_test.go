package secondary

import (
	"fmt"
	"testing"
)

// この章の中心その1。索引を引いても行は手に入らない。木を2回降りることになる。
func TestSecondaryGoesDownTwice(t *testing.T) {
	// 1万行、年齢は 0..9999 なので1つの値に1件だけ当たる。
	tbl := NewTable(10000, 10000)
	ix := Build(tbl, false)

	scan := ByScan(tbl, 42)
	sec := BySecondary(tbl, ix, 42)

	t.Logf("全走査       索引 %2d + 本体 %3d = %3d ページ / %d 行",
		scan.IndexReads, scan.TableReads, scan.Total(), scan.Rows)
	t.Logf("索引 + 本体  索引 %2d + 本体 %3d = %3d ページ / %d 行",
		sec.IndexReads, sec.TableReads, sec.Total(), sec.Rows)

	if scan.Rows != 1 || sec.Rows != 1 {
		t.Fatalf("%d, %d", scan.Rows, sec.Rows)
	}
	// 索引を降りて、そのあと本体を1ページ読む。
	if sec.IndexReads != ix.Height() || sec.TableReads != 1 {
		t.Fatalf("%+v (高さ %d)", sec, ix.Height())
	}
	// 1件だけ当たる問い合わせなら、索引のほうが圧倒的に安い。
	if sec.Total()*10 > scan.Total() {
		t.Errorf("索引の得が小さい: %d, %d", sec.Total(), scan.Total())
	}
}

// この章の中心その2。当たる件数が増えると、どこかで全走査に負ける。
func TestSelectivityHasACrossover(t *testing.T) {
	const n = 10000
	full := (n + RowsPerPage - 1) / RowsPerPage

	t.Logf("表は %d 行 = %d ページ。全走査はいつでも %d ページ", n, full, full)
	t.Logf("%8s %8s %10s %10s %10s", "当たる率", "件数", "全走査", "索引+本体", "並びそろえ")

	var crossover int
	for _, spread := range []int{10000, 1000, 200, 100, 50, 20, 10} {
		tbl := NewTable(n, spread)
		ix := Build(tbl, false)
		scan := ByScan(tbl, 0)
		sec := BySecondary(tbl, ix, 0)

		// 並びをそろえた表は、同じ年齢の行が隣り合う。
		ctbl := NewClustered(n, spread)
		clu := ByClustered(ctbl, Build(ctbl, false), 0)

		t.Logf("%7.1f%% %8d %10d %10d %10d",
			float64(sec.Rows)/float64(n)*100, sec.Rows, scan.Total(), sec.Total(), clu.Total())

		if crossover == 0 && sec.Total() > scan.Total() {
			crossover = sec.Rows
		}
		// 並びをそろえた索引は、どの選択率でも全走査を超えない。
		if clu.Total() > scan.Total() {
			t.Errorf("並びをそろえたのに全走査より読んだ: %d > %d", clu.Total(), scan.Total())
		}
	}

	t.Logf("索引が負け始めるのは %d 件(表の %.1f%%)あたり", crossover, float64(crossover)/float64(n)*100)
	if crossover == 0 {
		t.Fatal("負ける点が見つからない")
	}
	// 1割も当たらないうちに逆転する。
	if crossover > n/10 {
		t.Errorf("逆転が遅すぎる: %d 件", crossover)
	}
}

// この章の中心その3。欲しい列が索引に揃っていれば、本体に戻らない。
func TestCoveringIndexNeverGoesBack(t *testing.T) {
	const n = 10000
	tbl := NewTable(n, 100) // 1つの値に 100 件当たる
	plain := Build(tbl, false)
	covering := Build(tbl, true)

	sec := BySecondary(tbl, plain, 0)
	cov := ByCovering(tbl, covering, 0)
	scan := ByScan(tbl, 0)

	t.Logf("全走査             %3d ページ", scan.Total())
	t.Logf("索引 + 本体        %3d ページ(索引 %d + 本体 %d)", sec.Total(), sec.IndexReads, sec.TableReads)
	t.Logf("本体に戻らない索引 %3d ページ(索引 %d + 本体 %d)", cov.Total(), cov.IndexReads, cov.TableReads)

	if cov.Rows != sec.Rows {
		t.Fatalf("件数が違う: %d, %d", cov.Rows, sec.Rows)
	}
	if cov.TableReads != 0 {
		t.Errorf("本体を読んだ: %d", cov.TableReads)
	}
	// 本体へ戻らないだけで、読むページが2桁変わる。
	if sec.Total() < cov.Total()*20 {
		t.Errorf("差が出ていない: %d, %d", sec.Total(), cov.Total())
	}
}

// 索引の高さは件数が増えても対数でしか伸びない。
func TestIndexHeightGrowsSlowly(t *testing.T) {
	for _, n := range []int{1000, 100000, 10000000} {
		ix := Build(NewTable(n, n), false)
		t.Logf("%9d 件   高さ %d", n, ix.Height())
		if ix.Height() > 4 {
			t.Errorf("%d 件で高さ %d", n, ix.Height())
		}
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	tbl := NewTable(500, 10)
	ix := Build(tbl, false)

	// 当たらないキーでも索引は降りる。降りてみないと無いと分からない。
	p := BySecondary(tbl, ix, 999)
	if p.Rows != 0 || p.IndexReads == 0 {
		t.Errorf("%+v", p)
	}
	if p.TableReads != 0 {
		t.Errorf("当たっていないのに本体を読んだ: %d", p.TableReads)
	}
	// 範囲の外の主キーは取りに行かない。
	if _, ok := tbl.Fetch(-1); ok {
		t.Error("負の主キーが引けた")
	}
	if _, ok := tbl.Fetch(500); ok {
		t.Error("範囲外の主キーが引けた")
	}
	tbl.ResetStats()
	if got := tbl.FetchAll([]int{-1, 999}); len(got) != 0 || tbl.Reads() != 0 {
		t.Errorf("%v %d", got, tbl.Reads())
	}
	// 空の表でも落ちない。
	empty := NewTable(0, 1)
	if empty.Pages() != 0 || Build(empty, false).Height() != 1 {
		t.Error("空の表で崩れた")
	}
	if s := fmt.Sprint(ByScan(empty, 0).Rows); s != "0" {
		t.Errorf("%s", s)
	}
}
