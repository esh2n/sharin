// Package tcp は TCP の肝——**信頼できないパケット網の上に、信頼できて順序どおりの
// バイトストリームを作る**——を、実ソケット不使用で決定的にモデル化する。
// ネットワーク下層編のパーツ。
//
// 下層(IP)は、パケットを届く保証も順序の保証も無く運ぶだけ(落ちる・遅れる・
// 入れ替わる)。TCP はその上で、次の道具立てでバイトストリームの錯覚を作る:
//   - 3-way ハンドシェイク(SYN / SYN-ACK / ACK)で接続を確立し、初期シーケンス番号を交換
//   - シーケンス番号(送る各バイトに通し番号)と累積 ACK(「ここまで受け取った」)
//   - 再送(ACK が返らないバイトをタイマ満了で送り直す)= 信頼性
//   - スライディングウィンドウ(未確認のまま送れる量の上限)= フロー制御
//   - 受信側の並べ替え(順序が乱れて届いても、連続する先頭から順にアプリへ渡す)
//
// tcp.go はエンドポイント(接続の片側)の状態機械、sim.go はパケット網の模擬。
package tcp

import (
	"fmt"
	"sort"
)

// #region types

// State は接続の状態(RFC 793 の主要サブセット)。
type State int

const (
	Closed      State = iota
	Listen            // 受け身で SYN を待つ(サーバ)
	SynSent           // SYN を送って SYN-ACK を待つ(クライアント)
	SynRcvd           // SYN を受け、SYN-ACK を送って ACK を待つ
	Established       // 確立。データを送受信できる
	CloseWait         // 相手の FIN を受けた(こちらはまだ送るかも)
	LastAck           // こちらも FIN を送り、その ACK を待つ
	FinWait           // こちらから FIN を送り、相手の ACK/FIN を待つ
)

func (s State) String() string {
	switch s {
	case Closed:
		return "CLOSED"
	case Listen:
		return "LISTEN"
	case SynSent:
		return "SYN_SENT"
	case SynRcvd:
		return "SYN_RCVD"
	case Established:
		return "ESTABLISHED"
	case CloseWait:
		return "CLOSE_WAIT"
	case LastAck:
		return "LAST_ACK"
	case FinWait:
		return "FIN_WAIT"
	default:
		return "?"
	}
}

// Flag はセグメントの制御ビット。
type Flag uint8

const (
	SYN Flag = 1 << iota // 接続開始・初期シーケンス番号の同期
	ACK                  // Ack 番号が有効(確認応答)
	FIN                  // 送信終了
)

func (f Flag) String() string {
	s := ""
	if f&SYN != 0 {
		s += "SYN "
	}
	if f&ACK != 0 {
		s += "ACK "
	}
	if f&FIN != 0 {
		s += "FIN "
	}
	if s == "" {
		return "-"
	}
	return s[:len(s)-1]
}

// Segment は 1 つの TCP セグメント(パケットに載る単位)。Seq はこのセグメント先頭の
// シーケンス番号、Ack は「次に期待するバイト番号」、Window は受信側の空き容量。
type Segment struct {
	Seq     uint32
	Ack     uint32
	Flags   Flag
	Window  uint32
	Payload []byte
}

func (s Segment) String() string {
	return fmt.Sprintf("%s seq=%d ack=%d win=%d len=%d", s.Flags, s.Seq, s.Ack, s.Window, len(s.Payload))
}

// #endregion types

// #region endpoint

const defaultMSS = 4 // 1 セグメントに載せる最大バイト数(教材用に小さく)

// Endpoint は接続の片側。送信側(seq/ack/ウィンドウ)と受信側(並べ替えバッファ)の
// 状態を持つ。SYN と FIN はそれぞれシーケンス番号を 1 つ消費する。
type Endpoint struct {
	Name  string
	state State

	// 送信側
	iss      uint32 // 初期送信シーケンス番号
	sndUna   uint32 // 未確認の最も古い seq(ここまでは相手が受け取った)
	sndNxt   uint32 // 次に送る seq
	sndMax   uint32 // これまで送った最大 seq(+1)。再送判定に使う
	sndWnd   uint32 // 相手が広告したウィンドウ(送ってよい残量)
	toSend   []byte // アプリが送りたいバイト列(全体)
	dataBase uint32 // 最初のデータバイトの seq(= iss+1)
	wantFin  bool   // 送信終了(FIN)を出したいか
	finSeq   uint32 // FIN に割り当てた seq
	finDone  bool   // FIN を送信済みか

	// 受信側
	rcvNxt uint32            // 次に期待するバイト番号(累積 ACK の値)
	rcvWnd uint32            // 受信ウィンドウ(空き容量)
	ooo    map[uint32][]byte // 順序が乱れて届いたセグメント(seq → payload)
	recvd  []byte            // 連続してアプリへ渡したバイト列
	gotFin bool

	// タイマ・保留
	rto     int  // 再送タイムアウト(ステップ数)
	sinceUp int  // sndUna が最後に進んでからのステップ数
	needAck bool // 単独 ACK を返す必要があるか
	started bool // SYN 系を一度でも送ったか

	Retransmits int // 再送回数(観察用)
	mss         int
}

// NewEndpoint は接続の片側を作る。iss は初期シーケンス番号、rcvWnd は受信ウィンドウ。
func NewEndpoint(name string, iss uint32, rcvWnd uint32) *Endpoint {
	return &Endpoint{
		Name: name, state: Closed, iss: iss,
		sndUna: iss, sndNxt: iss, sndMax: iss,
		rcvWnd: rcvWnd, ooo: map[uint32][]byte{},
		rto: 3, mss: defaultMSS,
	}
}

// State は現在の接続状態を返す。
func (e *Endpoint) State() State { return e.state }

// Received はこれまでアプリへ渡した(順序どおりの)バイト列を返す。
func (e *Endpoint) Received() []byte { return append([]byte(nil), e.recvd...) }

// InFlight は未確認のまま飛んでいるバイト数を返す(ウィンドウの使用量)。
func (e *Endpoint) InFlight() uint32 { return e.sndMax - e.sndUna }

// Listen はこのエンドポイントを受け身にする(サーバ側)。
func (e *Endpoint) Listen() { e.state = Listen }

// Connect は能動オープンを始める(クライアント側)。SYN は次の emit で飛ぶ。
func (e *Endpoint) Connect() {
	e.state = SynSent
	e.dataBase = e.iss + 1
	e.started = false
}

// Send はアプリのバイト列を送信キューに積む。実際の送出は emit が担う。
func (e *Endpoint) Send(data []byte) {
	e.toSend = append(e.toSend, data...)
	if e.dataBase == 0 {
		e.dataBase = e.iss + 1
	}
}

// Close は送信終了を要求する。全データを送り切ったあと FIN を出す。
func (e *Endpoint) Close() { e.wantFin = true }

// #endregion endpoint

// #region emit

// emit はこのステップで送り出すセグメントを返す。再送タイマの満了・新規データ・
// ハンドシェイク・ACK・FIN をここで一手にさばく。
func (e *Endpoint) emit() []Segment {
	var out []Segment
	switch e.state {
	case SynSent:
		if !e.started || e.timerExpired() {
			if e.started {
				e.Retransmits++
			}
			out = append(out, Segment{Seq: e.iss, Flags: SYN, Window: e.rcvWnd})
			e.sndNxt, e.sndMax = e.iss+1, e.iss+1 // SYN は seq を 1 消費する
			e.mark()
		}
	case SynRcvd:
		if !e.started || e.timerExpired() {
			if e.started {
				e.Retransmits++
			}
			out = append(out, Segment{Seq: e.iss, Ack: e.rcvNxt, Flags: SYN | ACK, Window: e.rcvWnd})
			e.sndNxt, e.sndMax = e.iss+1, e.iss+1 // SYN-ACK の SYN も 1 消費
			e.mark()
		}
	case Established, CloseWait, FinWait, LastAck:
		out = append(out, e.emitData()...)
	}
	// 送るものが無くても、応答すべき ACK があれば単独で返す。
	if e.needAck && len(out) == 0 {
		out = append(out, Segment{Seq: e.sndNxt, Ack: e.rcvNxt, Flags: ACK, Window: e.rcvWnd})
	}
	if len(out) > 0 {
		e.needAck = false
	}
	return out
}

// emitData はデータ・FIN の(再)送を組み立てる。再送タイマが満了していたら、
// 未確認の先頭(sndUna)まで巻き戻して送り直す(go-back-N)。
func (e *Endpoint) emitData() []Segment {
	if e.timerExpired() && e.sndUna < e.sndMax {
		e.sndNxt = e.sndUna // 未確認の先頭から送り直す
		e.Retransmits++
		e.sinceUp = 0
	}
	var out []Segment
	dataEnd := e.dataBase + uint32(len(e.toSend))
	limit := e.sndUna + e.sndWnd // フロー制御: ウィンドウの右端

	for e.sndNxt < dataEnd && e.sndNxt < limit {
		start := int(e.sndNxt - e.dataBase)
		n := e.mss
		if start+n > len(e.toSend) {
			n = len(e.toSend) - start
		}
		if e.sndNxt+uint32(n) > limit {
			n = int(limit - e.sndNxt)
		}
		if n <= 0 {
			break
		}
		payload := append([]byte(nil), e.toSend[start:start+n]...)
		out = append(out, Segment{Seq: e.sndNxt, Ack: e.rcvNxt, Flags: ACK, Window: e.rcvWnd, Payload: payload})
		e.sndNxt += uint32(n)
		if e.sndNxt > e.sndMax {
			e.sndMax = e.sndNxt
		}
	}

	// 全データを送り切り、FIN を出したいなら FIN を送る(seq を 1 消費)。
	if e.wantFin && !e.finDone && e.sndNxt == dataEnd && e.sndNxt < limit {
		e.finSeq = dataEnd
		out = append(out, Segment{Seq: e.finSeq, Ack: e.rcvNxt, Flags: FIN | ACK, Window: e.rcvWnd})
		e.sndNxt++
		if e.sndNxt > e.sndMax {
			e.sndMax = e.sndNxt
		}
		e.finDone = true
		if e.state == Established {
			e.state = FinWait
		} else if e.state == CloseWait {
			e.state = LastAck
		}
	}
	if len(out) > 0 && !e.started {
		e.mark()
	}
	return out
}

func (e *Endpoint) mark()          { e.started = true; e.sinceUp = 0 }
func (e *Endpoint) timerExpired() bool { return e.started && e.sinceUp >= e.rto }

// tick は 1 ステップぶん時間を進める(再送タイマを刻む)。
func (e *Endpoint) tick() {
	if e.started {
		e.sinceUp++
	}
}

// #endregion emit

// #region deliver

// deliver は届いたセグメントを処理する。ACK でウィンドウを進め、SYN/FIN で状態を
// 遷移し、データは順序を見て受信バッファへ入れる。
func (e *Endpoint) deliver(s Segment) {
	// --- ACK 処理: 未確認の先頭を進める ---
	if s.Flags&ACK != 0 && s.Ack > e.sndUna && s.Ack <= e.sndMax {
		e.sndUna = s.Ack
		e.sinceUp = 0 // 前進した → 再送タイマをリセット
	}
	if s.Flags&ACK != 0 {
		e.sndWnd = s.Window
	}

	// --- ハンドシェイク/状態遷移 ---
	switch e.state {
	case Listen:
		if s.Flags&SYN != 0 {
			e.rcvNxt = s.Seq + 1
			e.dataBase = e.iss + 1
			e.state = SynRcvd
			e.started = false
			return
		}
	case SynSent:
		if s.Flags&SYN != 0 && s.Flags&ACK != 0 {
			e.rcvNxt = s.Seq + 1
			e.state = Established
			e.needAck = true // 最終 ACK を返す
			return
		}
	case SynRcvd:
		if s.Flags&ACK != 0 && s.Ack >= e.iss+1 {
			e.state = Established
			// このセグメントにデータや FIN が載っていれば下で処理する
		}
	}

	// --- データ受信(Established 以降) ---
	if len(s.Payload) > 0 {
		e.recvData(s.Seq, s.Payload)
		e.needAck = true
	}

	// --- FIN 受信(順序どおりに来たら) ---
	if s.Flags&FIN != 0 && s.Seq == e.rcvNxt {
		e.gotFin = true
		e.rcvNxt++ // FIN も seq を 1 消費
		e.needAck = true
		switch e.state {
		case Established:
			e.state = CloseWait
		case FinWait:
			e.state = Closed
		}
	}
	if e.state == LastAck && e.sndUna > e.finSeq {
		e.state = Closed // こちらの FIN が ACK された
	}
}

// recvData は 1 セグメントのデータを受信バッファへ入れ、順序どおりに繋がる範囲を
// アプリへ渡す。順序が乱れていたら ooo に退避し、隙間が埋まったらまとめて渡す。
func (e *Endpoint) recvData(seq uint32, payload []byte) {
	if seq < e.rcvNxt {
		return // 既に受け取った(再送の重複)
	}
	if seq == e.rcvNxt {
		e.recvd = append(e.recvd, payload...)
		e.rcvNxt += uint32(len(payload))
		e.drainOOO()
		return
	}
	// 順序が先走っている → 退避(累積 ACK は rcvNxt のままなので相手は隙間を埋め直す)
	if _, ok := e.ooo[seq]; !ok {
		e.ooo[seq] = append([]byte(nil), payload...)
	}
}

// drainOOO は退避したセグメントのうち、rcvNxt に連続する分を順に取り込む。
func (e *Endpoint) drainOOO() {
	for {
		p, ok := e.ooo[e.rcvNxt]
		if !ok {
			return
		}
		delete(e.ooo, e.rcvNxt)
		e.recvd = append(e.recvd, p...)
		e.rcvNxt += uint32(len(p))
	}
}

// OOOKeys は退避中の out-of-order セグメントの seq を昇順で返す(観察用)。
func (e *Endpoint) OOOKeys() []uint32 {
	ks := make([]uint32, 0, len(e.ooo))
	for k := range e.ooo {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}

// #endregion deliver
