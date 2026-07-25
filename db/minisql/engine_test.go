package minisql

import (
	"testing"
)

func TestEngineInsertSelect(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec("INSERT INTO users VALUES (1, 100)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO users VALUES (2, 200)"); err != nil {
		t.Fatal(err)
	}

	// WHERE で1件
	rows, err := db.Exec("SELECT * FROM users WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Key != 1 || rows[0].Value != 100 {
		t.Errorf("WHERE id=1 の結果 = %+v", rows)
	}

	// 全件(昇順で返る)
	all, err := db.Exec("SELECT * FROM users")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Key != 1 || all[1].Key != 2 {
		t.Errorf("全件 = %+v", all)
	}
}

func TestEngineMissingKey(t *testing.T) {
	db, _ := Open(t.TempDir())
	defer db.Close()
	db.Exec("INSERT INTO users VALUES (1, 100)")

	rows, err := db.Exec("SELECT * FROM users WHERE id = 99")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("存在しないキーの結果は空のはず: %+v", rows)
	}
}

func TestEnginePersists(t *testing.T) {
	dir := t.TempDir()
	db, _ := Open(dir)
	db.Exec("INSERT INTO users VALUES (5, 500)")
	db.Close()

	db2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	rows, _ := db2.Exec("SELECT * FROM users WHERE id = 5")
	if len(rows) != 1 || rows[0].Value != 500 {
		t.Errorf("再open後 = %+v", rows)
	}
}

func TestEngineSyntaxError(t *testing.T) {
	db, _ := Open(t.TempDir())
	defer db.Close()
	if _, err := db.Exec("DROP TABLE users"); err == nil {
		t.Error("未対応の文はエラーになるべき")
	}
}
