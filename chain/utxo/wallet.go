package utxo

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
)

// #region wallet

// Wallet は 1 人の鍵ペア。公開鍵がそのままアドレス(出力の受取先)になる。
// 秘密鍵は署名にだけ使い、外へは出さない。
type Wallet struct {
	priv ed25519.PrivateKey
	Pub  ed25519.PublicKey
}

// NewWallet は seed 文字列から決定的に鍵ペアを作る。
// 同じ seed からは必ず同じ鍵——テストとデモを再現可能にするため。
// (実運用の鍵は乱数から作る。ここは学習用に決定的化している)
func NewWallet(seed string) *Wallet {
	h := sha256.Sum256([]byte(seed))
	priv := ed25519.NewKeyFromSeed(h[:]) // 32 バイト seed から生成
	return &Wallet{priv: priv, Pub: priv.Public().(ed25519.PublicKey)}
}

// Address は公開鍵の短い 16 進表現。表示・比較用の「宛先」。
func (w *Wallet) Address() string {
	return hex.EncodeToString(w.Pub)[:8]
}

// Sign は取引本体(signingBytes)に署名する。この署名が input に載り、
// 「この出力を消費してよい」ことの証明になる。
func (w *Wallet) Sign(tx Tx) []byte {
	return ed25519.Sign(w.priv, tx.signingBytes())
}

// AddressOf は任意の公開鍵の短い表示を返す(所有者の照合・デモ表示に使う)。
func AddressOf(pub ed25519.PublicKey) string {
	if len(pub) == 0 {
		return "(none)"
	}
	return hex.EncodeToString(pub)[:8]
}

// #endregion wallet
