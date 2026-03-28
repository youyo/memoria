package cli

import (
	"sync"

	"github.com/youyo/memoria/internal/db"
)

// LazyDB は初回アクセス時にのみ DB を開く遅延初期化ラッパー。
// DB が不要なコマンド（version, config init/show/path/print-hook 等）では
// DB のオープンが発生しないため、初回起動時や DB ファイル未作成時でも
// これらのコマンドが正常に動作する。
type LazyDB struct {
	once     sync.Once
	database *db.DB
	err      error
	path     string
}

// NewLazyDB は指定パスへの LazyDB を返す。DB はまだ開かない。
func NewLazyDB(path string) *LazyDB {
	return &LazyDB{path: path}
}

// NewLazyDBFromDB は既に開いている *db.DB をラップした LazyDB を返す。
// テストで DI するために使用する。
func NewLazyDBFromDB(database *db.DB) *LazyDB {
	l := &LazyDB{}
	l.once.Do(func() {
		l.database = database
	})
	return l
}

// Get は DB を返す。初回呼び出し時にのみ db.Open を実行する。
func (l *LazyDB) Get() (*db.DB, error) {
	l.once.Do(func() {
		l.database, l.err = db.Open(l.path)
	})
	return l.database, l.err
}
