package eventloop

// #region poller

// Ready は Wait が返す 1 件: どの FD で、どの Interest が発火したか。
type Ready struct {
	FD     int
	Events Interest
}

// Poller は epoll オブジェクトのモデル。関心のある FD をあらかじめ登録して
// おき、Wait 1 回で「今 ready な FD 全部」をまとめて受け取る。多数の接続を
// 1 スレッドで見張れるのは、この一括問い合わせのおかげだ。
//
// 素朴な select(2) は毎回「見たい FD の集合」を丸ごと渡し直し、カーネルは
// 全部を線形に走査した。epoll は関心を一度登録したら再利用でき、準備できた
// FD だけを返すので、接続数が増えても効率が落ちにくい。ここではその「登録
// しておく → 準備できたものだけ返る」という性質をモデル化する。
type Poller struct {
	fds      map[int]*FD
	interest map[int]Interest
	order    []int // 登録順。走査順を決定的にする
}

// NewPoller は空の Poller を作る。
func NewPoller() *Poller {
	return &Poller{fds: map[int]*FD{}, interest: map[int]Interest{}}
}

// Add は FD を関心付きで登録する(epoll_ctl ADD)。
func (p *Poller) Add(f *FD, in Interest) {
	if _, ok := p.fds[f.id]; !ok {
		p.order = append(p.order, f.id)
	}
	p.fds[f.id] = f
	p.interest[f.id] = in
}

// Modify は登録済み FD の関心を差し替える(epoll_ctl MOD)。
// 「送りたいデータが溜まったら Writable を足し、送り終えたら外す」のに使う。
// これを怠って Writable を張りっぱなしにすると、送信バッファが空いている限り
// 毎回 ready と報告され、ループが空回りする(busy loop)——実務の頻出バグ。
func (p *Poller) Modify(id int, in Interest) {
	if _, ok := p.fds[id]; ok {
		p.interest[id] = in
	}
}

// Remove は FD を監視対象から外す(epoll_ctl DEL)。接続を閉じたとき。
func (p *Poller) Remove(id int) {
	delete(p.fds, id)
	delete(p.interest, id)
	for i, x := range p.order {
		if x == id {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
}

// Wait は今この瞬間に ready な FD を、登録順(決定的)に返す。各 FD の現在の
// readiness と登録した関心の積(AND)を取り、空でなければ 1 件立てる。
// これが epoll_wait の中核——ブロッキングを除けば、監視中の FD を走査して
// 準備できたものだけ返す操作そのものだ。level-triggered(条件が続く限り毎回)。
func (p *Poller) Wait() []Ready {
	var out []Ready
	for _, id := range p.order {
		ev := p.fds[id].ready() & p.interest[id]
		if ev != 0 {
			out = append(out, Ready{FD: id, Events: ev})
		}
	}
	return out
}

// Len は監視中の FD 数を返す。
func (p *Poller) Len() int { return len(p.order) }

// Watching は監視中の FD 番号を登録順に返す(観察用)。
func (p *Poller) Watching() []int {
	return append([]int(nil), p.order...)
}

// InterestOf は登録された関心を返す。未登録なら 0。
func (p *Poller) InterestOf(id int) Interest { return p.interest[id] }

// #endregion poller
