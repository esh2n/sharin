package minisql

import "testing"

func TestLexer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Token
	}{
		{
			name:  "INSERT 文",
			input: "INSERT INTO users VALUES (1, 42)",
			want: []Token{
				{Kind: TokKeyword, Text: "INSERT"},
				{Kind: TokKeyword, Text: "INTO"},
				{Kind: TokIdent, Text: "users"},
				{Kind: TokKeyword, Text: "VALUES"},
				{Kind: TokLParen, Text: "("},
				{Kind: TokNumber, Text: "1"},
				{Kind: TokComma, Text: ","},
				{Kind: TokNumber, Text: "42"},
				{Kind: TokRParen, Text: ")"},
				{Kind: TokEOF},
			},
		},
		{
			name:  "SELECT 文",
			input: "SELECT * FROM users WHERE id = 1",
			want: []Token{
				{Kind: TokKeyword, Text: "SELECT"},
				{Kind: TokStar, Text: "*"},
				{Kind: TokKeyword, Text: "FROM"},
				{Kind: TokIdent, Text: "users"},
				{Kind: TokKeyword, Text: "WHERE"},
				{Kind: TokIdent, Text: "id"},
				{Kind: TokEq, Text: "="},
				{Kind: TokNumber, Text: "1"},
				{Kind: TokEOF},
			},
		},
		{
			name:  "小文字キーワードも大文字扱い(大小無視)",
			input: "select * from t",
			want: []Token{
				{Kind: TokKeyword, Text: "SELECT"},
				{Kind: TokStar, Text: "*"},
				{Kind: TokKeyword, Text: "FROM"},
				{Kind: TokIdent, Text: "t"},
				{Kind: TokEOF},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Lex(tt.input)
			if err != nil {
				t.Fatalf("Lex: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("トークン数 = %d, want %d\n got=%v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i].Kind != tt.want[i].Kind || got[i].Text != tt.want[i].Text {
					t.Errorf("token[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLexerError(t *testing.T) {
	if _, err := Lex("SELECT @ FROM t"); err == nil {
		t.Error("未知の文字 @ はエラーになるべき")
	}
}
