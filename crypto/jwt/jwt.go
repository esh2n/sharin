// Package jwt は JWT(JSON Web Token)を HMAC-SHA256 署名で自作したもの。
//
// JWT は「サーバが署名した通行証」。ログイン時にサーバが「この人は user-42 です」と
// 書いた紙に署名して渡す。以降クライアントはその紙を見せるだけでよく、
// サーバは署名を検証するだけで「確かに自分が発行した、改竄されていない」と分かる。
// サーバがセッションを覚えておく必要がない(ステートレス)のが利点。
//
// 署名の安全性は「秘密鍵を知らないと正しい署名を作れない」こと。
// crypto 編の公開鍵署名とは違い、JWT の HS256 は秘密鍵を共有する共通鍵方式。
package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// #region claims
// Claims はトークンに載せる主張(誰が、いつまで有効か)。
type Claims struct {
	Sub string `json:"sub"` // subject: 誰のトークンか
	Exp int64  `json:"exp"` // expiration: 有効期限(Unix 秒)
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// #endregion claims

// #region sign
// encodeSegment は base64url(パディングなし)で符号化する。JWT の各パートの形式。
func encodeSegment(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// sign は "header.payload" に HMAC-SHA256 をかけた署名を base64url で返す。
// HMAC = 秘密鍵とメッセージからハッシュを作る仕組み。鍵を知らないと同じ値を作れない。
func sign(signingInput string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return encodeSegment(mac.Sum(nil))
}

// Sign は Claims を JWT 文字列 "header.payload.signature" にする。
func Sign(claims Claims, secret []byte) (string, error) {
	h, err := json.Marshal(header{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	p, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := encodeSegment(h) + "." + encodeSegment(p)
	return signingInput + "." + sign(signingInput, secret), nil
}

// #endregion sign

// #region verify
// Verify はトークンを検証し、正しければ Claims を返す。
// チェックは3段: (1)形式 (2)署名が一致するか (3)期限切れでないか。
func Verify(token string, secret []byte) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("jwt: token must have 3 parts, got %d", len(parts))
	}
	signingInput := parts[0] + "." + parts[1]

	// (2) 署名検証。自分で計算した署名と、トークンの署名を突き合わせる。
	// タイミング攻撃を避けるため hmac.Equal(定数時間比較)を使う。
	// これが「自分が発行した、改竄されていない」の保証。alg=none 攻撃も、
	// トークンの alg を読まず HS256 で計算するのでここで弾かれる。
	expected := sign(signingInput, secret)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return Claims{}, errors.New("jwt: signature mismatch")
	}

	// (1') ペイロードを復元。
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("jwt: bad payload encoding: %w", err)
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, fmt.Errorf("jwt: bad payload json: %w", err)
	}

	// (3) 期限チェック。
	if claims.Exp != 0 && time.Now().Unix() >= claims.Exp {
		return Claims{}, errors.New("jwt: token expired")
	}
	return claims, nil
}

// #endregion verify
