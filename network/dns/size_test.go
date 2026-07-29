package dns

import "testing"

// 長さの違う実在しそうな名前で測る。
var names = []string{
	"example.com",
	"www.example.com",
	"d111111abcdef8.cloudfront.net",
	"ec2-203-0-113-25.ap-northeast-1.compute.amazonaws.com",
}

// この章の中心。圧縮すると、回答1件の大きさが名前の長さから切り離される。
func TestCompressedAnswerIsFixedSize(t *testing.T) {
	for _, n := range names {
		if got := AnswerSize(n, true); got != 16 {
			t.Errorf("%s: 圧縮ありの1件 = %d バイト, want 16", n, got)
		}
		// 圧縮しなければ、名前がそのままレコードの大きさになる。
		want := len(encodeName(n)) + 14
		if got := AnswerSize(n, false); got != want {
			t.Errorf("%s: 圧縮なしの1件 = %d, want %d", n, got, want)
		}
	}
	// 名前が長いほど差が開く。
	short := AnswerSize(names[0], false)
	long := AnswerSize(names[3], false)
	if long <= short {
		t.Fatalf("名前が長いほど太るはず: %d, %d", short, long)
	}
}

// 512 バイトに入る件数。圧縮ありはほぼ動かず、圧縮なしは名前の長さで崩れる。
func TestCapacityAgainstUDPLimit(t *testing.T) {
	for _, n := range names {
		on, off := Capacity(n, true), Capacity(n, false)
		t.Logf("%-54s 名前 %2dB  圧縮あり %2d 件  圧縮なし %2d 件",
			n, len(encodeName(n)), on, off)
		if on <= off {
			t.Errorf("%s: 圧縮したほうが入るはず: %d, %d", n, on, off)
		}
	}

	// 名前を 13B から 55B へ、4倍に伸ばしたときの落ち方を比べる。
	// 圧縮ありは 8割を保つ(30 → 27)。
	on0, on3 := Capacity(names[0], true), Capacity(names[3], true)
	if on3*10 < on0*8 {
		t.Errorf("圧縮ありが名前の長さで大きく減った: %d → %d", on0, on3)
	}
	// 圧縮なしは4割を下回る(17 → 6)。
	off0, off3 := Capacity(names[0], false), Capacity(names[3], false)
	if off3*5 >= off0*2 {
		t.Errorf("圧縮なしはもっと落ちるはず: %d → %d", off0, off3)
	}
}

func ips(n int) [][4]byte {
	out := make([][4]byte, n)
	for i := range out {
		out[i] = [4]byte{192, 0, 2, byte(i + 1)}
	}
	return out
}

// 組み立てた応答は、どちらの書き方でも同じように読める。
func TestBuildResponseRoundTrip(t *testing.T) {
	for _, compress := range []bool{true, false} {
		msg := BuildResponse(0x1234, "www.example.com", ips(3), compress)
		if Truncated(msg) {
			t.Fatalf("compress=%v: 3件で切れた", compress)
		}
		got, err := ParseResponse(msg, 0x1234)
		if err != nil {
			t.Fatalf("compress=%v: %v", compress, err)
		}
		want := []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}
		if len(got) != len(want) {
			t.Fatalf("compress=%v: %v", compress, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("compress=%v: %v", compress, got)
			}
		}
	}
}

// 入りきらなければ TC を立てて打ち切る。載る件数は Capacity と一致する。
func TestTruncationMatchesCapacity(t *testing.T) {
	const name = "ec2-203-0-113-25.ap-northeast-1.compute.amazonaws.com"
	for _, compress := range []bool{true, false} {
		want := Capacity(name, compress)
		msg := BuildResponse(0x1234, name, ips(want+5), compress)
		if !Truncated(msg) {
			t.Fatalf("compress=%v: 溢れたのに TC が立っていない", compress)
		}
		if len(msg) > UDPLimit {
			t.Fatalf("compress=%v: 512 を超えた: %d", compress, len(msg))
		}
		got, err := ParseResponse(msg, 0x1234)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != want {
			t.Errorf("compress=%v: 載った件数 %d, want %d", compress, len(got), want)
		}
	}

	// 実際に効く場面。同じ8件が、圧縮ありなら入り、圧縮なしなら入らない。
	eight := ips(8)
	if m := BuildResponse(1, name, eight, true); Truncated(m) {
		t.Errorf("圧縮ありなら8件入るはず: %d バイト", len(m))
	}
	if m := BuildResponse(1, name, eight, false); !Truncated(m) {
		t.Errorf("圧縮なしでは8件入らないはず: %d バイト", len(m))
	}
	// 溢れなければ TC は立たない。
	if Truncated(BuildResponse(1, name, ips(1), false)) {
		t.Error("1件で TC が立った")
	}
	if Truncated(nil) {
		t.Error("空のメッセージで TC 判定が落ちた")
	}
}
