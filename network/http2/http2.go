// Package http2 は HTTP/2 の要である多重化とヘッダ圧縮を最小構成で実装する。
//
// HTTP/1.1 の弱点は、1 本の接続で一度に 1 つの要求しか処理できないことだ。
// 大きな応答が 1 つ詰まると、その後ろに並んだ小さな応答は、前が終わるまで
// 待たされる(ヘッドオブラインブロッキング)。ブラウザは接続を 6 本ほど張って
// 誤魔化すが、根本解決ではない。HTTP/2 は 1 本の接続の上に複数のストリームを
// 作り、各ストリームのデータをフレームという小片に刻んで交互に流す。大きな
// 応答の合間に小さな応答のフレームを差し込めるので、小さい要求が先に完了できる。
// あわせて HPACK でヘッダを圧縮する。毎回ほぼ同じヘッダ(Host、User-Agent…)を
// 送る無駄を、一度送ったヘッダを索引で参照することで省く。
package http2

// #region frame

// FrameType はフレームの種類。
type FrameType int

const (
	FrameHeaders FrameType = iota // ヘッダ(HPACK 圧縮済み)
	FrameData                     // 本文の一片
)

// Frame は 1 本の接続を流れる最小単位。どのストリームのものかを StreamID で示す。
// 複数ストリームのフレームが交互に流れる(多重化)。
type Frame struct {
	StreamID int
	Type     FrameType
	Data     []byte
	End      bool // このストリームの最後のフレームか
}

// #endregion frame

// #region hpack

// Header は 1 つのヘッダ行。
type Header struct{ Name, Value string }

// HField は HPACK の符号化結果。Index>0 なら表の参照、0 なら literal(表にも追加)。
type HField struct {
	Index int // 動的表のインデックス(1 始まり)。0 は literal
	Name  string
	Value string
}

// Encoder はヘッダを動的表を使って圧縮する。一度送ったヘッダは索引で参照する。
type Encoder struct{ table []Header }

// NewEncoder は空の動的表でエンコーダを作る。
func NewEncoder() *Encoder { return &Encoder{} }

// Encode はヘッダ列を HField 列にする。表にあれば索引、なければ literal で送り表に追加。
func (e *Encoder) Encode(headers []Header) []HField {
	out := make([]HField, 0, len(headers))
	for _, h := range headers {
		if i := indexOf(e.table, h); i >= 0 {
			out = append(out, HField{Index: i + 1})
		} else {
			out = append(out, HField{Name: h.Name, Value: h.Value})
			e.table = append(e.table, h) // 次からは索引で送れる
		}
	}
	return out
}

// Decoder はエンコーダと同じ手順で動的表を育て、HField 列を復元する。
type Decoder struct{ table []Header }

// NewDecoder は空の動的表でデコーダを作る。
func NewDecoder() *Decoder { return &Decoder{} }

// Decode は HField 列をヘッダ列に戻す。literal は表に追加してエンコーダと同期する。
func (d *Decoder) Decode(fields []HField) []Header {
	out := make([]Header, 0, len(fields))
	for _, f := range fields {
		if f.Index > 0 {
			out = append(out, d.table[f.Index-1])
		} else {
			h := Header{Name: f.Name, Value: f.Value}
			d.table = append(d.table, h)
			out = append(out, h)
		}
	}
	return out
}

func indexOf(table []Header, h Header) int {
	for i, t := range table {
		if t == h {
			return i
		}
	}
	return -1
}

// EncodedSize は HField 列のおよそのバイト数。索引は 1、literal は名前と値の長さ分。
func EncodedSize(fields []HField) int {
	n := 0
	for _, f := range fields {
		if f.Index > 0 {
			n += 1 // 索引参照は 1 バイト
		} else {
			n += 1 + len(f.Name) + len(f.Value)
		}
	}
	return n
}

// RawSize は圧縮しない場合のヘッダのバイト数。
func RawSize(headers []Header) int {
	n := 0
	for _, h := range headers {
		n += len(h.Name) + len(h.Value)
	}
	return n
}

// #endregion hpack

// #region multiplex

// CompletionTicksH1 は HTTP/1.1 の完了時刻を返す。応答は要求順に直列送信され、
// 各応答は前の応答がすべて終わってから始まる(ヘッドオブラインブロッキング)。
// sizes は各応答のフレーム数。tick は 1 フレーム送信を 1 と数える論理時間。
func CompletionTicksH1(sizes []int) []int {
	done := make([]int, len(sizes))
	t := 0
	for i, s := range sizes {
		t += s
		done[i] = t
	}
	return done
}

// CompletionTicksH2 は HTTP/2 の完了時刻を返す。1 本の接続の上で全ストリームの
// フレームを順繰りに 1 つずつ流す(多重化)。大きな応答の合間に小さな応答の
// フレームが差し込まれるので、小さいものが先に完了する。
func CompletionTicksH2(sizes []int) []int {
	remaining := make([]int, len(sizes))
	copy(remaining, sizes)
	done := make([]int, len(sizes))
	tick := 0
	for {
		active := false
		for i := range remaining {
			if remaining[i] > 0 {
				active = true
				tick++
				remaining[i]--
				if remaining[i] == 0 {
					done[i] = tick
				}
			}
		}
		if !active {
			break
		}
	}
	return done
}

// Multiplex は各ストリームを順繰りにフレーム化した、接続を流れる順序を返す。
// CompletionTicksH2 と同じ順繰りで、実際のフレーム列を組み立てる。
func Multiplex(sizes []int) []Frame {
	remaining := make([]int, len(sizes))
	copy(remaining, sizes)
	var frames []Frame
	for {
		active := false
		for i := range remaining {
			if remaining[i] > 0 {
				active = true
				remaining[i]--
				frames = append(frames, Frame{
					StreamID: i + 1,
					Type:     FrameData,
					End:      remaining[i] == 0,
				})
			}
		}
		if !active {
			break
		}
	}
	return frames
}

// #endregion multiplex
