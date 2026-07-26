package dlock

// #region resource

// Resource はロックで守りたい共有資源(DB 行・ファイル・外部 API など)。
// フェンシングが有効なら、**今まで見た最大トークン以上**の書き込みしか受けない。
// 出遅れた古い持ち主(小さいトークン)の書き込みは弾かれ、二重取得による破壊を防ぐ。
type Resource struct {
	Name     string
	fenced   bool   // フェンシングを有効にするか
	data     string // 現在の値
	maxToken int64  // これまで受け入れた最大のフェンシングトークン
	Rejected int    // フェンシングで弾いた回数(観察用)
	writes   int    // 受け入れた書き込み回数(観察用)
}

// NewResource はフェンシング有効のリソースを作る。
func NewResource(name string) *Resource {
	return &Resource{Name: name, fenced: true}
}

// NewUnfencedResource はフェンシング無効のリソースを作る(比較用——古い持ち主の
// 書き込みを素通しさせ、破壊が起きる様子を見せる)。
func NewUnfencedResource(name string) *Resource {
	return &Resource{Name: name, fenced: false}
}

// Write はトークン付きの書き込みを試みる。フェンシング有効なら token が今まで見た
// 最大値以上のときだけ受理する。古いトークンは拒否して true でなく false を返す。
// フェンシング無効なら常に受理する(=古い持ち主が新しい値を上書きしてしまう)。
func (r *Resource) Write(token int64, value string) bool {
	if r.fenced && token < r.maxToken {
		r.Rejected++
		return false // フェンス落ち: 古い持ち主の書き込み。破壊を未然に防ぐ
	}
	if token > r.maxToken {
		r.maxToken = token
	}
	r.data = value
	r.writes++
	return true
}

// Data は現在の値を返す。
func (r *Resource) Data() string { return r.data }

// MaxToken はこれまで受理した最大トークンを返す。
func (r *Resource) MaxToken() int64 { return r.maxToken }

// Fenced はフェンシングが有効かを返す。
func (r *Resource) Fenced() bool { return r.fenced }

// #endregion resource
