// Package ratelimiter は代表的なレートリミットアルゴリズムの比較実装。
//
// token bucket / leaky bucket / fixed window / sliding window log の
// 4方式を同じインターフェースで実装し、挙動の違いをテストで固定する。
package ratelimiter

// #region limiter
// Limiter は1リクエストを通すか判定する共通インターフェース。
type Limiter interface {
	// Allow は今このリクエストを通してよければ true を返す。
	Allow() bool
}

// #endregion limiter
