package minisql

import (
	"fmt"
	"testing"
)

// fill は n 行入れた DB を返す。
func fill(t *testing.T, n int) *DB {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for i := 1; i <= n; i++ {
		if _, err := db.Exec(fmt.Sprintf("INSERT INTO users VALUES (%d, %d)", i, i*10)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// この章の中心。同じ表に対して、WHERE の有無で読むページ数が桁で変わる。
func TestPlanIndexVersusScan(t *testing.T) {
	var ratios []float64
	for _, n := range []int{100, 400, 1600} {
		db := fill(t, n)

		idx, rows, err := db.Explain(fmt.Sprintf("SELECT * FROM users WHERE id = %d", n/2))
		if err != nil {
			t.Fatal(err)
		}
		scan, all, err := db.Explain("SELECT * FROM users")
		if err != nil {
			t.Fatal(err)
		}
		r := float64(scan.Reads) / float64(idx.Reads)
		ratios = append(ratios, r)

		t.Logf("%5d 行   index %2d ページ → %d 行    scan %4d ページ → %5d 行    比 %.0f倍",
			n, idx.Reads, len(rows), scan.Reads, len(all), r)

		if idx.Access != "index" || scan.Access != "scan" {
			t.Fatalf("道の選び方が違う: %q, %q", idx.Access, scan.Access)
		}
		if len(rows) != 1 || len(all) != n {
			t.Fatalf("行数: %d, %d", len(rows), len(all))
		}
		// 1点引きは高さぶんしか読まない。行を16倍にしても2桁には乗らない。
		if idx.Reads > 10 {
			t.Errorf("%d 行: 1点引きが %d ページ読んだ", n, idx.Reads)
		}
	}
	// 行を16倍にすると、比も一桁上がる。1点引きの手数がほぼ動かないため。
	if ratios[2] < ratios[0]*8 {
		t.Errorf("比が開いていない: %.0f倍 → %.0f倍", ratios[0], ratios[2])
	}
}

// 直す前の書き方(全走査してから1件ずつ引き直す)との差。
func TestScanThenGetIsNPlusOne(t *testing.T) {
	const n = 1600
	db := fill(t, n)

	scan, rows, err := db.Explain("SELECT * FROM users")
	if err != nil {
		t.Fatal(err)
	}
	old, oldRows, err := db.measure("scan+get", db.scanThenGet)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("1回歩く         %4d ページ → %d 行", scan.Reads, len(rows))
	t.Logf("1件ずつ引き直す %4d ページ → %d 行  (1行あたり %.1f ページ増)",
		old.Reads, len(oldRows), float64(old.Reads-scan.Reads)/float64(n))

	// 返る結果は同じ。違うのは代償だけ。
	if len(rows) != n || len(oldRows) != n {
		t.Fatalf("行数が違う: %d, %d", len(rows), len(oldRows))
	}
	for i := range rows {
		if rows[i] != oldRows[i] {
			t.Fatalf("%d 行目が違う: %+v, %+v", i, rows[i], oldRows[i])
		}
	}
	// 引き直すぶんは行数に比例して積み上がる。1回歩くのは木のノード数で止まる。
	if old.Reads < scan.Reads*5 {
		t.Errorf("差が出ていない: %d, %d", scan.Reads, old.Reads)
	}
	if scan.Reads > n {
		t.Errorf("1回歩くだけで行数を超えた: %d", scan.Reads)
	}
}

// バッファプールは 128 ページ。1点引きは根の付近しか触らない。
func TestIndexStaysInTheBufferPool(t *testing.T) {
	db := fill(t, 1600)

	idx, _, err := db.Explain("SELECT * FROM users WHERE id = 800")
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := db.measure("scan+get", db.scanThenGet)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("index    ヒット %5d / ミス %5d", idx.Hits, idx.Misses)
	t.Logf("scan+get ヒット %5d / ミス %5d", old.Hits, old.Misses)

	if idx.Misses > idx.Reads {
		t.Errorf("ミスが読んだ数を超えた: %d > %d", idx.Misses, idx.Reads)
	}
	// 何度も降り直すと、プールに載りきらない下の段を取りに行くことになる。
	if old.Misses <= idx.Misses {
		t.Errorf("降り直してもミスが増えていない: %d, %d", idx.Misses, old.Misses)
	}
}

// EXPLAIN は SELECT だけ。壊れた SQL はそのままエラーになる。
func TestExplainRejectsNonSelect(t *testing.T) {
	db := fill(t, 1)
	if _, _, err := db.Explain("INSERT INTO users VALUES (1, 2)"); err == nil {
		t.Error("INSERT を EXPLAIN できてしまった")
	}
	if _, _, err := db.Explain("SELECT users"); err == nil {
		t.Error("壊れた SQL がエラーにならなかった")
	}
	// 見つからないキーでも道の選び方は同じ。
	p, rows, err := db.Explain("SELECT * FROM users WHERE id = 999")
	if err != nil {
		t.Fatal(err)
	}
	if p.Access != "index" || len(rows) != 0 {
		t.Errorf("%+v %v", p, rows)
	}
}
