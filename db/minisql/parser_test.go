package minisql

import "testing"

func TestParseInsert(t *testing.T) {
	stmt, err := Parse("INSERT INTO users VALUES (1, 42)")
	if err != nil {
		t.Fatal(err)
	}
	ins, ok := stmt.(*InsertStmt)
	if !ok {
		t.Fatalf("型が InsertStmt でない: %T", stmt)
	}
	if ins.Table != "users" || ins.Key != 1 || ins.Value != 42 {
		t.Errorf("got %+v", ins)
	}
}

func TestParseSelectAll(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users")
	if err != nil {
		t.Fatal(err)
	}
	sel, ok := stmt.(*SelectStmt)
	if !ok {
		t.Fatalf("型が SelectStmt でない: %T", stmt)
	}
	if sel.Table != "users" || sel.WhereKey != nil {
		t.Errorf("got %+v", sel)
	}
}

func TestParseSelectWhere(t *testing.T) {
	stmt, err := Parse("SELECT * FROM users WHERE id = 7")
	if err != nil {
		t.Fatal(err)
	}
	sel := stmt.(*SelectStmt)
	if sel.WhereKey == nil || *sel.WhereKey != 7 {
		t.Errorf("WHERE が解釈されていない: %+v", sel)
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		"INSERT users VALUES (1, 2)",     // INTO 抜け
		"INSERT INTO users VALUES (1)",   // 値が1つ
		"SELECT FROM users",              // * 抜け
		"SELECT * users",                 // FROM 抜け
		"SELECT * FROM users WHERE id 1", // = 抜け
		"DELETE FROM users",              // 未対応の文
		"",                               // 空
	}
	for _, in := range bad {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) はエラーになるべき", in)
		}
	}
}
