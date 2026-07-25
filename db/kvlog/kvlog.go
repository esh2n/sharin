// Package kvlog は「追記だけの KV ストア」(Bitcask 型)の最小実装。
//
// ディスク上のファイルには追記しかしない。上書きも削除も「新しいレコードの追記」で
// 表現し、「どのキーの最新がどこにあるか」はメモリ上のインデックスが覚える。
// 追記だけにする理由は2つ:
//   - シーケンシャル書き込みは書き込みの中で最速(ディスクとページ編)
//   - 途中でクラッシュしても壊れる可能性があるのは「末尾の書きかけ」だけ
package kvlog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// #region format
// レコードの並び: [keyLen 4B][valLen 4B][tombstone 1B][key][value]
// tombstone(墓石) = 1 は「このキーは削除された」という印のレコード。
const headerSize = 9

func encodeRecord(key, value string, tombstone bool) []byte {
	buf := make([]byte, headerSize+len(key)+len(value))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(key)))
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(value)))
	if tombstone {
		buf[8] = 1
	}
	copy(buf[headerSize:], key)
	copy(buf[headerSize+len(key):], value)
	return buf
}

// readRecord は off の位置のレコードを読む。ファイル末尾や書きかけのレコードなら
// io.EOF / io.ErrUnexpectedEOF を返す(replay がこれを「ここまで」の合図に使う)。
func readRecord(r io.ReaderAt, off int64) (key, value string, tombstone bool, size int64, err error) {
	var header [headerSize]byte
	if _, err = r.ReadAt(header[:], off); err != nil {
		return
	}
	keyLen := binary.BigEndian.Uint32(header[0:4])
	valLen := binary.BigEndian.Uint32(header[4:8])
	tombstone = header[8] == 1

	body := make([]byte, keyLen+valLen)
	if _, err = r.ReadAt(body, off+headerSize); err != nil {
		return
	}
	key = string(body[:keyLen])
	value = string(body[keyLen:])
	size = headerSize + int64(len(body))
	return
}

// #endregion format

// #region store
// Store は1本のログファイルと、キー → レコード先頭オフセットのインデックス。
// ディスクは「全履歴」、インデックスは「最新の場所」だけを知っている。
type Store struct {
	mu    sync.Mutex
	f     *os.File
	index map[string]int64
	end   int64 // 次に追記する位置(=有効なログの長さ)
}

// Open はログファイルを開き、再生(replay)してインデックスを組み立てる。
func Open(path string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("kvlog: open: %w", err)
	}
	s := &Store{f: f, index: map[string]int64{}}
	if err := s.replay(); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// Close はファイルを閉じる。
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Close()
}

// #endregion store

// #region replay
// replay はログを先頭から読めるところまで読み、インデックスを組み立てる。
// 途中で切れたレコード(クラッシュの痕跡)が見つかったら、そこから後ろを切り捨てる。
// 「再起動 = ログの再生」であり、これが WAL とイベントソーシングに共通する核。
func (s *Store) replay() error {
	var off int64
	for {
		key, _, tombstone, size, err := readRecord(s.f, off)
		if err != nil {
			break // 末尾に到達、または書きかけレコード。ここまでが有効
		}
		if tombstone {
			delete(s.index, key)
		} else {
			s.index[key] = off
		}
		off += size
	}
	s.end = off
	// 書きかけの末尾を物理的にも捨てて、次の追記が壊れた上に乗らないようにする。
	return s.f.Truncate(off)
}

// #endregion replay

// #region putget
// Put は「key の最新は value」というレコードを末尾に追記する。
// 古い値はログに残り続けるが、インデックスが新しいオフセットを指すので見えなくなる。
func (s *Store) Put(key, value string) error {
	if key == "" {
		return errors.New("kvlog: key must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rec := encodeRecord(key, value, false)
	if _, err := s.f.WriteAt(rec, s.end); err != nil {
		return fmt.Errorf("kvlog: append: %w", err)
	}
	s.index[key] = s.end
	s.end += int64(len(rec))
	return nil
}

// Get はインデックスでオフセットを引き、その1レコードだけを読む。
// ログ全体を走査することはない。
func (s *Store) Get(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	off, ok := s.index[key]
	if !ok {
		return "", false, nil
	}
	_, value, _, _, err := readRecord(s.f, off)
	if err != nil {
		return "", false, fmt.Errorf("kvlog: read: %w", err)
	}
	return value, true, nil
}

// Delete は tombstone(削除の印)を追記し、インデックスから外す。
// ファイルからは何も消えない。「削除も追記」なのがこの方式の徹底ぶり。
func (s *Store) Delete(key string) error {
	if key == "" {
		return errors.New("kvlog: key must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rec := encodeRecord(key, "", true)
	if _, err := s.f.WriteAt(rec, s.end); err != nil {
		return fmt.Errorf("kvlog: append: %w", err)
	}
	delete(s.index, key)
	s.end += int64(len(rec))
	return nil
}

// #endregion putget
