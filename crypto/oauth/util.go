package oauth

// idGen は決定的なコード生成器(テスト再現性のため実乱数を使わない)。
type idGen struct{ state uint64 }

// next は次のコード文字列を返す。
func (g *idGen) next() string {
	g.state = g.state*6364136223846793005 + 1442695040888963407
	return "code_" + itoa(int(g.state>>40))
}

// constEq は 2 つの文字列を定数時間で比べる(タイミング攻撃対策)。
func constEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// itoa は非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// atoi は文字列を非負整数に変換する。壊れていれば false。
func atoi(s string) (int, bool) {
	if len(s) == 0 {
		return 0, false
	}
	var n int
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
