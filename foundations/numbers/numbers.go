// Package numbers は負の数の表し方とバイト順を最小構成で実装する。
//
// 負の数をビットでどう表すか、という問いには何通りも答えがある。素直なのは
// 符号のビットを1本立てて残りを絶対値にする形で、人間が読むにはこれがいちばん
// 分かりやすい。だが実際の計算機はこれを使わない。ほぼすべてが2の補数を使う。
//
// なぜかというと、2の補数だけが「引き算を足し算にできる」からだ。a - b を
// a + (-b) として、同じ回路で計算できる。符号を見て加算と減算を切り替える必要が
// 無い。表し方の美しさではなく、回路が1つで済むという都合で選ばれている。
//
// その都合には代償がついてくる。表せる範囲が非対称になり、いちばん小さい数の
// 符号を反転できない。絶対値を取る関数が、たった1つの入力に対して答えを返せない。
// この歪みは、ハードウェアの都合として消えずに言語の意味論まで出てくる。
//
// もう1つ、同じ数をメモリに並べるときの順序という別の問題がある。こちらは
// 正しさの問題ではなく、ただの取り決めになる。取り決めなので、境界を越えるとき、
// つまりファイルに書くときと通信で送るときには、必ず明示しなければならない。
package numbers

// #region repr

// Width はビット幅。4 から 32 までを扱う。
type Width int

// Kind は負の数の表し方。
type Kind int

const (
	// Twos は2の補数。負の数は「2^n から引いた値」で表す。
	Twos Kind = iota
	// Ones は1の補数。負の数は全ビットを反転して表す。
	Ones
	// SignMag は符号と絶対値。最上位を符号のビットにして、残りを絶対値にする。
	SignMag
)

func (k Kind) String() string {
	return [...]string{"2の補数", "1の補数", "符号と絶対値"}[k]
}

func mask(w Width) uint64 { return (uint64(1) << uint(w)) - 1 }

// signBit は最上位のビットだけが立った値を返す。
func signBit(w Width) uint64 { return uint64(1) << uint(w-1) }

// Encode は数 v を、表し方 kind の w ビットのビット列にする。
func Encode(v int64, w Width, kind Kind) uint64 {
	m := mask(w)
	switch kind {
	case Ones:
		if v < 0 {
			return ^uint64(-v) & m // 絶対値の全ビット反転
		}
		return uint64(v) & m
	case SignMag:
		if v < 0 {
			return (uint64(-v) & (m >> 1)) | signBit(w)
		}
		return uint64(v) & (m >> 1)
	default: // Twos
		return uint64(v) & m
	}
}

// Decode はビット列を、表し方 kind の符号つきの数として読む。
func Decode(bits uint64, w Width, kind Kind) int64 {
	bits &= mask(w)
	neg := bits&signBit(w) != 0
	switch kind {
	case Ones:
		if neg {
			return -int64(^bits & mask(w))
		}
		return int64(bits)
	case SignMag:
		if neg {
			return -int64(bits &^ signBit(w))
		}
		return int64(bits)
	default: // Twos
		if neg {
			// 上位を全部1で埋めてから符号つきとして読む。
			return int64(bits | ^mask(w))
		}
		return int64(bits)
	}
}

// Zeros は 0 を表すビット列を、その表し方で何通りあるかを含めて返す。
//
// 2の補数だけが1通りになる。他の2つには「正の 0」と「負の 0」があり、
// 同じ数なのにビットが違うので、比較のたびに特別扱いが要る。
func Zeros(w Width, kind Kind) []uint64 {
	switch kind {
	case Ones:
		return []uint64{0, mask(w)} // 全部 0 と 全部 1
	case SignMag:
		return []uint64{0, signBit(w)} // 符号ビットだけ立った 0
	default:
		return []uint64{0}
	}
}

// Range は表せる範囲を返す。
//
// 2の補数だけが非対称になる。0 の表し方が1通りしかないぶん、負の側が1つ多い。
func Range(w Width, kind Kind) (min, max int64) {
	half := int64(1) << uint(w-1)
	switch kind {
	case Twos:
		return -half, half - 1
	default:
		return -(half - 1), half - 1
	}
}

// #endregion repr

// #region adder

// Result は加算の結果と、2種類のはみ出し。
type Result struct {
	Bits uint64
	// Carry は符号なしとして見たときのはみ出し。最上位から繰り上がりが出たか。
	Carry bool
	// Overflow は符号つきとして見たときのはみ出し。符号が壊れたか。
	Overflow bool
}

// Add は w ビットの加算を行う。
//
// 見どころは、はみ出しの判定が2つあることになる。同じビット列を符号なしと見るか
// 符号つきと見るかで、壊れる条件が違う。回路は同じで、どちらの旗を見るかだけが違う。
//
// 符号つきのはみ出しは「最上位への繰り上がり」と「最上位からの繰り上がり」が
// 食い違ったときに起きる。同じ符号どうしを足して符号が変わったとき、と言っても同じ。
func Add(a, b uint64, w Width) Result {
	m := mask(w)
	a &= m
	b &= m
	sum := (a + b) & m

	carry := a+b > m
	// 同じ符号どうしを足して、結果の符号が変わったら壊れている。
	sameSign := (a^b)&signBit(w) == 0
	flipped := (a^sum)&signBit(w) != 0
	return Result{Bits: sum, Carry: carry, Overflow: sameSign && flipped}
}

// Neg は符号を反転する。全ビットを反転して1を足す。
//
// いちばん小さい数だけ、これが自分自身を返す。符号を反転した結果が範囲に
// 収まらないので、行き場がない。絶対値が取れない数がある、という歪みの正体になる。
func Neg(bits uint64, w Width) uint64 {
	return (^bits + 1) & mask(w)
}

// Sub は減算を行う。引く数の符号を反転して足すだけで、加算と同じ回路になる。
// これが2の補数を選ぶ理由そのものになる。
func Sub(a, b uint64, w Width) Result {
	r := Add(a, Neg(b, w), w)
	// 引き算では、繰り上がりの意味が逆になる(借りが出なければ立つ)。
	if b&mask(w) == 0 {
		r.Carry = true
	}
	return r
}

// #endregion adder

// #region extend

// SignExtend は幅を広げる。空いた上位を符号のビットで埋める。
func SignExtend(bits uint64, from, to Width) uint64 {
	bits &= mask(from)
	if bits&signBit(from) == 0 {
		return bits
	}
	return (bits | ^mask(from)) & mask(to)
}

// ZeroExtend は幅を広げる。空いた上位を 0 で埋める。
//
// 符号つきの値にこちらを使うと、負の数が巨大な正の数になる。同じビット列でも
// 意味が違うので、広げ方は型で決まる。
func ZeroExtend(bits uint64, from, to Width) uint64 {
	return bits & mask(from) & mask(to)
}

// ShiftRight は右にずらす。arithmetic なら符号のビットで埋め、そうでなければ 0 で埋める。
//
// 負の数を2で割りたいときは算術シフトが要る。論理シフトを使うと、
// 負の数が巨大な正の数になる。同じ「右にずらす」でも別の命令になっている。
func ShiftRight(bits uint64, n int, w Width, arithmetic bool) uint64 {
	bits &= mask(w)
	out := bits >> uint(n)
	if arithmetic && bits&signBit(w) != 0 {
		// 上から n ビットぶんを1で埋める。
		out |= (mask(w) << uint(int(w)-n)) & mask(w)
	}
	return out & mask(w)
}

// #endregion extend

// #region endian

// PutLittle はビット列をバイトに分けて、下位のバイトから並べる。
func PutLittle(bits uint64, w Width) []byte {
	n := int(w) / 8
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = byte(bits >> uint(8*i))
	}
	return out
}

// PutBig は上位のバイトから並べる。通信で使う順序はこちらになる。
func PutBig(bits uint64, w Width) []byte {
	n := int(w) / 8
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = byte(bits >> uint(8*(n-1-i)))
	}
	return out
}

// GetLittle は下位のバイトから並んでいるとして読む。
//
// 途中で止めても意味が壊れないのが、この並びの取り柄になる。先頭の1バイトだけ
// 読めば、それが下位8ビットになる。幅の違う読み出しが同じアドレスでできる。
func GetLittle(b []byte) uint64 {
	var v uint64
	for i := len(b) - 1; i >= 0; i-- {
		v = v<<8 | uint64(b[i])
	}
	return v
}

// GetBig は上位のバイトから並んでいるとして読む。
func GetBig(b []byte) uint64 {
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v
}

// #endregion endian
