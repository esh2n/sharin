// Package dlock は分散ロックの核心——**リース(失効するロック)とフェンシング
// トークン**——を Go で決定的にモデル化する。分散システム編のパーツ。
//
// 1 台のマシンなら mutex で排他できる。だが複数ノードにまたがると難しくなる:
// ロックを持ったノードが**いつでもクラッシュしうる**。持ったまま死なれると、
// ロックは永遠に解放されない。そこで分散ロックはロックに**リース(有効期限)**を
// 付ける——一定時間で自動的に失効させ、持ち主が死んでも他が取れるようにする。
//
// ところがリースは新たな罠を生む。持ち主が死んでいなくても、GC の一時停止・
// ネットワーク遅延で**「まだ持っているつもり」のまま時間だけ過ぎる**ことがある。
// その間にリースが失効し、別のノードが同じロックを取ってしまう——二重取得だ。
// これを防ぐのが**フェンシングトークン**: ロックを与えるたびに単調増加する番号を
// 発行し、保護対象(リソース)は「今まで見た最大の番号より小さい書き込みを拒む」。
// 出遅れた古い持ち主の書き込みは、番号が古いので弾かれる。
//
// lock.go はロックサービス(リース + トークン発行)、resource.go は保護対象。
package dlock

// #region lock

// LockService は 1 本のロックを貸し出すサービス。論理時計を持ち、リースで
// 自動失効させ、貸すたびに単調増加するフェンシングトークンを発行する。
// 合意は取れている前提。実際は Raft 等の上に載せる。
// ここでは 1 本のロックの、リースとトークンの意味論だけを取り出す。
type LockService struct {
	clock     int    // 論理時刻
	holder    string // 現在の持ち主("" なら空き)
	expiry    int    // 現在のリースが失効する時刻
	token     int64  // 現在の持ち主に発行したトークン
	nextToken int64  // 次に発行するトークン(単調増加)
}

// New は空のロックサービスを作る。トークンは 1 から始まる。
func New() *LockService {
	return &LockService{nextToken: 1}
}

// Now は現在の論理時刻を返す。
func (s *LockService) Now() int { return s.clock }

// Tick は論理時刻を d だけ進める。時間が進むとリースが失効しうる。
func (s *LockService) Tick(d int) {
	if d > 0 {
		s.clock += d
	}
}

// refresh はリースが失効していたらロックを空きに戻す。
func (s *LockService) refresh() {
	if s.holder != "" && s.clock >= s.expiry {
		s.holder = ""
	}
}

// Acquire はロックの取得を試みる。空き(または失効済み)なら client に lease だけ
// 貸し、新しいフェンシングトークンを発行して (token, true) を返す。他が保持中なら
// (0, false)。トークンが**取得のたびに増える**のが肝——後で持ち主を見分ける鍵になる。
func (s *LockService) Acquire(client string, lease int) (int64, bool) {
	s.refresh()
	if s.holder != "" {
		return 0, false // 誰かが保持中(まだ失効していない)
	}
	if lease < 1 {
		lease = 1
	}
	s.holder = client
	s.expiry = s.clock + lease
	s.token = s.nextToken
	s.nextToken++
	return s.token, true
}

// Renew は現在の持ち主がリースを延長する(ハートビート)。持ち主でなければ false。
// トークンは変わらない(同じ保持の延長だから)。
func (s *LockService) Renew(client string, lease int) bool {
	s.refresh()
	if s.holder != client {
		return false
	}
	if lease < 1 {
		lease = 1
	}
	s.expiry = s.clock + lease
	return true
}

// Release は持ち主が自発的にロックを手放す。持ち主でなければ何もしない。
func (s *LockService) Release(client string) {
	s.refresh()
	if s.holder == client {
		s.holder = ""
	}
}

// Holder は現在の持ち主とそのトークンを返す(失効を反映)。空きなら ("", 0)。
func (s *LockService) Holder() (string, int64) {
	s.refresh()
	if s.holder == "" {
		return "", 0
	}
	return s.holder, s.token
}

// Expiry は現在のリースの失効時刻を返す(観察用)。
func (s *LockService) Expiry() int { return s.expiry }

// #endregion lock
