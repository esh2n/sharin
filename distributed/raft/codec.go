package raft

import "encoding/binary"

// encodeConfChange は ConfChange を9バイトに直列化する(1バイト種別 + 8バイトID)。
// ログの Data に載せて他ノードへ運ぶための最小フォーマット。
func encodeConfChange(cc ConfChange) []byte {
	b := make([]byte, 9)
	b[0] = byte(cc.Type)
	binary.BigEndian.PutUint64(b[1:], cc.NodeID)
	return b
}

func decodeConfChange(b []byte) ConfChange {
	return ConfChange{
		Type:   ConfChangeType(b[0]),
		NodeID: binary.BigEndian.Uint64(b[1:]),
	}
}
