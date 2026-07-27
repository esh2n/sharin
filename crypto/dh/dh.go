// Package dh は Diffie–Hellman 鍵交換を最小構成で実装する。
//
// 二人が、盗聴されている公開の通信路だけを使って、二人にしか分からない共有秘密を
// 作れる。魔法のようだが、種明かしは剰余べき乗の一方向性にある。g^a mod p は
// 簡単に計算できるが、結果から a を逆算する(離散対数を解く)のは、p が大きいと
// 現実的な時間で解けない。Alice は秘密 a から g^a を、Bob は秘密 b から g^b を
// 公開する。互いの公開値を相手の秘密でべき乗すると、どちらも g^(ab) mod p に
// たどり着く。盗聴者は g^a と g^b は見えても、a も b も分からないので g^(ab) を
// 作れない。ただしこれは受動的な盗聴に対してだけ。間に割り込む能動的な攻撃者
// (中間者)には、認証がなければ無力であることも示す。
package dh

import "math/big"

// #region modexp

// Rand は決定的な擬似乱数源(テスト再現性のため実乱数を使わない)。
type Rand struct{ state uint64 }

// NewRand は seed から擬似乱数源を作る。
func NewRand(seed uint64) *Rand { return &Rand{state: seed*2862933555777941757 + 1} }

func (r *Rand) next() uint64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return r.state >> 11
}

// ModExp は base^exp mod mod を二乗と乗算の繰り返しで計算する。
// 指数のビットを下から見て、1 のビットで結果に現在の底を掛け、毎回底を二乗する。
// 指数が巨大でもビット数ぶんの掛け算で済む(素朴に exp 回掛けるのとは桁違い)。
func ModExp(base, exp, mod *big.Int) *big.Int {
	result := big.NewInt(1)
	b := new(big.Int).Mod(base, mod)
	e := new(big.Int).Set(exp)
	for e.Sign() > 0 {
		if e.Bit(0) == 1 {
			result.Mul(result, b)
			result.Mod(result, mod)
		}
		b.Mul(b, b)
		b.Mod(b, mod)
		e.Rsh(e, 1)
	}
	return result
}

// #endregion modexp

// #region keyexchange

// Params は共有する公開パラメータ。素数 P と生成元 G。
type Params struct {
	P *big.Int // 大きな素数(法)
	G *big.Int // 生成元
}

// Generate は秘密鍵 priv を無作為に選び、公開鍵 pub = G^priv mod P を返す。
// priv は決して送らない。pub だけを相手に渡す。
func (pr Params) Generate(r *Rand) (priv, pub *big.Int) {
	// priv を [2, P-2] から選ぶ。
	span := new(big.Int).Sub(pr.P, big.NewInt(3))
	priv = new(big.Int).SetUint64(r.next())
	priv.Mod(priv, span)
	priv.Add(priv, big.NewInt(2))
	pub = ModExp(pr.G, priv, pr.P)
	return priv, pub
}

// Shared は相手の公開鍵 otherPub を自分の秘密鍵 priv でべき乗し、共有秘密を導く。
// Alice が Shared(a, B)、Bob が Shared(b, A) を計算すると、どちらも G^(ab) mod P。
func (pr Params) Shared(priv, otherPub *big.Int) *big.Int {
	return ModExp(otherPub, priv, pr.P)
}

// #endregion keyexchange
