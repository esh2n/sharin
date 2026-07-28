// Package leaderelection はコントローラの leader election を最小構成で実装する。
//
// この編のコントローラは、ずっと1つだけ動いている前提だった。だが本番では
// コントローラ自身も落ちる。落ちたら誰も調整しなくなるので、複数台で動かして
// おきたい。ところが複数台が同時に調整すると、同じ Pod を二重に作ってしまう。
// 冗長化したいが、同時に働いてほしくはない。
//
// 答えは、置き場に1つオブジェクトを作って、それを持っている者だけが働く形に
// なる。持ち主は定期的に更新し、他は更新が止まったのを見て奪う。
//
// ここで難しいのは、時計が共有されていないことだ。持ち主の書いた時刻を
// 他が読んでも、その時刻が自分の時計とどれだけずれているかは分からない。
// だから待つ側は絶対時刻を比べない。「最後に変化を見てから、自分の時計で
// どれだけ経ったか」だけを見る。時刻は共有できないが、経過は共有できる。
//
// そのうえで肝心なのは、降りるほうが先でなければならない点になる。持ち主が
// 更新に失敗し続けたとき、他が奪うより先に自分から降りる。降りるまでの猶予を
// 奪われるまでの期間より短くしておけば、2人が同時に持ち主だと思う時間は
// できない。この大小が逆だと、逆転した幅だけ重なる。
package leaderelection

import "sort"

// #region config

// Config は3つの時間で leader election の性質を決める。
//
// 大小の関係が正しさそのものになっている。RenewDeadline が LeaseDuration より
// 短くなければ、持ち主が降りるより先に他が奪ってしまう。
type Config struct {
	// LeaseDuration は、変化を見なくなってから他が奪ってよいと判断するまでの長さ。
	LeaseDuration int
	// RenewDeadline は、持ち主が更新に失敗し続けたとき自分から降りるまでの猶予。
	RenewDeadline int
	// RetryPeriod は更新や奪取を試みる間隔。
	RetryPeriod int
}

// Safe は重なりが起きえない設定かを返す。
func (c Config) Safe() bool {
	return c.RenewDeadline < c.LeaseDuration && c.RetryPeriod < c.RenewDeadline
}

// Default は実物の既定値と同じ比を持つ設定を返す(15 / 10 / 2)。
func Default() Config {
	return Config{LeaseDuration: 15, RenewDeadline: 10, RetryPeriod: 2}
}

// #endregion config

// #region lease

// Lease は置き場にある1つのオブジェクト。これを持っている者だけが働く。
//
// Version が更新のたびに増える。待つ側は、書かれた時刻ではなくこの値の
// 変化を見る。時刻は他人の時計の値なので比べられないが、変化したかどうかは
// 自分の時計で測れる。
type Lease struct {
	Holder  string
	Version int
}

type cand struct {
	name       string
	leader     bool
	lastRenew  int // 自分が最後に更新できた時刻(自分の時計)
	obsVersion int // 最後に見た Version
	obsAt      int // それを見た時刻(自分の時計)
	nextAct    int
	down       bool
}

// Sim は複数の候補が1つの Lease を取り合う様子を、時刻を1つずつ進めながら再現する。
type Sim struct {
	cfg   Config
	now   int
	lease Lease
	cands []*cand

	// Overlap は2人以上が自分を持ち主だと思っていた時刻の数。
	Overlap int
	// DoubleActs は、その重なりによって二重になった操作の回数。
	DoubleActs int
	// Vacant は誰も持ち主が居なかった時刻の数。安全にした代償がここに出る。
	Vacant int
	Log    []string
}

// New は名前の候補たちで選出を始める。名前順に行動するので結果は決定的になる。
func New(cfg Config, names ...string) *Sim {
	s := &Sim{cfg: cfg}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	for _, n := range sorted {
		s.cands = append(s.cands, &cand{name: n, obsAt: 0, nextAct: 0})
	}
	return s
}

// Now は現在の論理時刻を返す。
func (s *Sim) Now() int { return s.now }

// Holder は置き場のオブジェクトが誰のものになっているかを返す。
func (s *Sim) Holder() string { return s.lease.Holder }

// Believers は「自分が持ち主だ」と思っている候補の名前を返す。
// これが2つ以上になっている時刻が、この章で数えたいものになる。
func (s *Sim) Believers() []string {
	var out []string
	for _, c := range s.cands {
		if c.leader {
			out = append(out, c.name)
		}
	}
	return out
}

// Partition は候補を置き場から切り離す。読むことも書くこともできなくなる。
func (s *Sim) Partition(name string) {
	if c := s.find(name); c != nil && !c.down {
		c.down = true
		s.logf(name + " が置き場に届かなくなった")
	}
}

// Heal は切り離しを解く。
func (s *Sim) Heal(name string) {
	if c := s.find(name); c != nil && c.down {
		c.down = false
		s.logf(name + " が置き場に届くようになった")
	}
}

// #endregion lease

// #region tick

// Tick は時刻を1つ進める。候補は名前順に行動する。
func (s *Sim) Tick() {
	for _, c := range s.cands {
		s.act(c)
	}
	s.tally()
	s.now++
}

// act は候補1人ぶんの行動。
//
// 観測が先で、判断が後になっている。この順序でないと、切り離しから復帰した
// 候補が古い観測のまま「期限が切れている」と判断して奪ってしまう。
func (s *Sim) act(c *cand) {
	if c.down {
		// 届かないので、読むことも更新もできない。できるのは降りることだけ。
		if c.leader && s.now-c.lastRenew >= s.cfg.RenewDeadline {
			c.leader = false
			s.logf(c.name + " は更新できないまま猶予 " + itoa(s.cfg.RenewDeadline) +
				" を使い切った。自分から持ち主を降りる")
		}
		return
	}

	// ① 観測。Version が変わっていれば、その変化を見た時刻を自分の時計で記録する。
	if s.lease.Version != c.obsVersion {
		c.obsVersion = s.lease.Version
		c.obsAt = s.now
	}

	if s.now < c.nextAct {
		return
	}
	c.nextAct = s.now + s.cfg.RetryPeriod

	// ② 判断。
	if c.leader {
		if s.lease.Holder != c.name {
			// 更新しようとしたら、すでに他人のものになっていた。
			c.leader = false
			s.logf(c.name + " は更新しようとして、持ち主が " + s.lease.Holder + " に変わっているのを見た。降りる")
			return
		}
		s.write(c)
		return
	}
	if s.lease.Holder == "" || s.now-c.obsAt >= s.cfg.LeaseDuration {
		// 自分の時計で LeaseDuration ぶん、変化を見ていない。奪う。
		prev, waited := s.lease.Holder, s.now-c.obsAt
		s.lease.Holder = c.name
		s.write(c)
		c.leader = true
		if prev == "" {
			s.logf(c.name + " が持ち主になった")
		} else {
			s.logf(c.name + " が " + itoa(waited) + " のあいだ変化を見なかったので " + prev + " から奪った")
		}
	}
}

// write は Version を進め、自分の観測も同時に更新する。
func (s *Sim) write(c *cand) {
	s.lease.Version++
	c.lastRenew = s.now
	c.obsVersion = s.lease.Version
	c.obsAt = s.now
}

// tally はこの時刻の重なりと空位を数える。
//
// 重なっている間、持ち主だと思っている全員が働く。冪等な操作なら結果は
// 変わらないが、外部への操作は人数ぶん実行される。
func (s *Sim) tally() {
	n := len(s.Believers())
	switch {
	case n == 0:
		s.Vacant++
	case n > 1:
		s.Overlap++
		s.DoubleActs += n - 1
	}
}

// #endregion tick

func (s *Sim) find(name string) *cand {
	for _, c := range s.cands {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (s *Sim) logf(msg string) { s.Log = append(s.Log, "t="+itoa(s.now)+" "+msg) }

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
