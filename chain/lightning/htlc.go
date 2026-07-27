package lightning

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// htlc.go は「自分と直接チャネルを持たない相手へ、途中のノードを信頼せずに送る」仕組み。
//
// A→B→C と繋がっているとき、A は C と直接チャネルを持たない。B に「C へ渡して」と頼むが、
// B が金だけ受け取って渡さないかもしれない。これを暗号で縛るのが HTLC
// (Hash Time-Locked Contract)。受取人 C だけが知る秘密 preimage の「ハッシュ H」で
// 各ホップの支払いをロックする。「H の preimage を出せた者にだけ払う」という条件だ。
//
// C が preimage を出して B から受け取ると、その preimage は B に知られる。B はそれを使って
// A から受け取れる。こうして preimage が受取人側から送金側へ逆向きに伝播し、経路全体が
// 同時に成立するか、まったく成立しないか(all-or-nothing)になる。取りはぐれは起きない。
// タイムアウトは送金側ほど長く設定し、下流が確定してから上流を確定する猶予を作る。

// #region htlc

// Hash は preimage のハッシュ。HTLC のロック条件になる(本物は SHA-256)。
func Hash(preimage string) string {
	h := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(h[:])[:12]
}

// HTLC は 1 つのチャネル上でロックされた条件付き支払い。
// preimage が示されれば Payee が受け取り、示されなければ期限後に Offerer へ戻る。
type HTLC struct {
	Hash    string
	Amount  uint64
	Expiry  int    // この時刻以降は失効(offerer へ返る)
	Offerer string // 成立すれば amount を失う側
	Payee   string // 成立すれば amount を得る側
	Settled bool
	Failed  bool
}

// LockHTLC は offerer の残高から amount を切り出し、条件付き支払いとしてロックする。
// ロック中は誰の残高でもない「宙に浮いた」状態で、新しい commitment に反映される。
func (c *Channel) LockHTLC(offerer string, amount uint64, hash string, expiry int) (*HTLC, error) {
	if c.state != StateOpen {
		return nil, ErrChannelClosed
	}
	if offerer != c.A && offerer != c.B {
		return nil, ErrUnknownParty
	}
	na, nb := c.Balances()
	if offerer == c.A {
		if na < amount {
			return nil, ErrInsufficient
		}
		na -= amount
	} else {
		if nb < amount {
			return nil, ErrInsufficient
		}
		nb -= amount
	}
	c.advance(na, nb) // ロックも状態更新の一種(revocable)
	h := &HTLC{Hash: hash, Amount: amount, Expiry: expiry, Offerer: offerer, Payee: c.other(offerer)}
	c.htlcs = append(c.htlcs, h)
	return h, nil
}

// SettleHTLC は preimage を示して HTLC を成立させる。ハッシュが合えば amount は Payee のものになる。
func (c *Channel) SettleHTLC(h *HTLC, preimage string) error {
	if h.Settled || h.Failed {
		return ErrHTLCDone
	}
	if Hash(preimage) != h.Hash {
		return ErrHTLCPreimage
	}
	na, nb := c.Balances()
	if h.Payee == c.A {
		na += h.Amount
	} else {
		nb += h.Amount
	}
	c.advance(na, nb)
	h.Settled = true
	return nil
}

// FailHTLC はタイムアウト後に、ロックした amount を offerer へ返す。
func (c *Channel) FailHTLC(h *HTLC, now int) error {
	if h.Settled || h.Failed {
		return ErrHTLCDone
	}
	if now < h.Expiry {
		return ErrHTLCNotExpired
	}
	na, nb := c.Balances()
	if h.Offerer == c.A {
		na += h.Amount
	} else {
		nb += h.Amount
	}
	c.advance(na, nb)
	h.Failed = true
	return nil
}

// #endregion htlc

// #region route

// Network はチャネルの集まり。多段の経路探索・ルーティングの舞台。
type Network struct {
	chans map[string]*Channel
}

// pairKey は 2 者の無向キー(順序に依らない)。
func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// NewNetwork はチャネル群からネットワークを作る。
func NewNetwork(chs ...*Channel) *Network {
	n := &Network{chans: map[string]*Channel{}}
	for _, ch := range chs {
		n.chans[pairKey(ch.A, ch.B)] = ch
	}
	return n
}

// Channel は 2 者間のチャネルを返す(無ければ nil)。
func (n *Network) Channel(a, b string) *Channel { return n.chans[pairKey(a, b)] }

// Nodes は参加ノードを整列して返す(表示用)。
func (n *Network) Nodes() []string {
	seen := map[string]bool{}
	for _, ch := range n.chans {
		seen[ch.A] = true
		seen[ch.B] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// hop は経路上の 1 チャネルと、そこでロックした HTLC。
type hop struct {
	ch   *Channel
	htlc *HTLC
}

// Payment は経路上に張られた HTLC の連なり。
type Payment struct {
	Hash     string
	Preimage string
	hops     []hop
}

// Hops は経路上の HTLC を返す(表示・検査用、送金側から順)。
func (p *Payment) Hops() []*HTLC {
	out := make([]*HTLC, len(p.hops))
	for i := range p.hops {
		out[i] = p.hops[i].htlc
	}
	return out
}

// #region routecore

// Route は path(送金側→受取側)の各ホップに HTLC を張る。expiry は送金側を最も長くし、
// 1 ホップごとに delta 減らす——下流が確定してから上流を確定する時間差を作るため。
func (n *Network) Route(path []string, amount uint64, hash string, baseExpiry, delta int) (*Payment, error) {
	p := &Payment{Hash: hash}
	expiry := baseExpiry
	for i := 0; i < len(path)-1; i++ {
		ch := n.Channel(path[i], path[i+1])
		if ch == nil {
			return nil, ErrNoChannel
		}
		h, err := ch.LockHTLC(path[i], amount, hash, expiry)
		if err != nil {
			return nil, err
		}
		p.hops = append(p.hops, hop{ch: ch, htlc: h})
		expiry -= delta
	}
	return p, nil
}

// Settle は受取側が preimage を公開し、経路を下流から上流へ順に成立させる。
// preimage が上流へ伝播していく様子を、末尾から先頭への走査でモデル化する。
func (p *Payment) Settle(preimage string) error {
	if Hash(preimage) != p.Hash {
		return ErrHTLCPreimage
	}
	for i := len(p.hops) - 1; i >= 0; i-- {
		if err := p.hops[i].ch.SettleHTLC(p.hops[i].htlc, preimage); err != nil {
			return err
		}
	}
	p.Preimage = preimage
	return nil
}

// #endregion routecore

// Timeout は preimage が出ないまま期限を迎えた経路を、全ホップ失効させて資金を返す。
func (p *Payment) Timeout(now int) error {
	for i := len(p.hops) - 1; i >= 0; i-- {
		if err := p.hops[i].ch.FailHTLC(p.hops[i].htlc, now); err != nil {
			return err
		}
	}
	return nil
}

// #endregion route
