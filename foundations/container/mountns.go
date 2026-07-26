package container

import "strings"

// #region mountns

// MountNamespace は独立したファイルシステムの見え方。
// マウント表(マウント先パス -> 実体)を持ち、パスは最長一致するマウントの
// 実体に解決される。既定ではすべてがコンテナ自身の rootfs に載る。
// だから 2つのコンテナが同じ /data に別々のボリュームをマウントできる。
type MountNamespace struct {
	mounts map[string]string // マウント先 -> ソース(実体の名前)
}

// NewMountNamespace は / に rootfs だけがある名前空間を作る。
func NewMountNamespace() *MountNamespace {
	return &MountNamespace{mounts: map[string]string{"/": "rootfs"}}
}

// Mount は target のパスに source(実体)を割り当てる。
func (ns *MountNamespace) Mount(target, source string) {
	ns.mounts[target] = source
}

// Resolve は path を、最長一致するマウントの実体に解決する。
func (ns *MountNamespace) Resolve(path string) string {
	best, bestLen := "rootfs", -1
	for mp, src := range ns.mounts {
		if covers(mp, path) && len(mp) > bestLen {
			best, bestLen = src, len(mp)
		}
	}
	return best
}

// covers は path がマウント先 mp の配下かどうかを、セグメント境界で判定する。
// "/data" は "/data/x" を覆うが "/database" は覆わない。
func covers(mp, path string) bool {
	if mp == "/" {
		return true
	}
	return path == mp || strings.HasPrefix(path, mp+"/")
}

// #endregion mountns
