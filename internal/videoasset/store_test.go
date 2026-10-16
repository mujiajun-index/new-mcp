package videoasset

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBoundedVideoStorageAndSigning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "options.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&model.Option{})
	oldDB := model.DB
	model.DB = db
	model.InitOptionMap()
	defer func() { sql, _ := db.DB(); sql.Close(); model.DB = oldDB }()
	store, err := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	video := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm', 'm', 'p', '4', '2'}, make([]byte, 64)...)
	result, err := Save(context.Background(), store, 1, "valid", bytes.NewReader(video))
	if err != nil || result.MIME != "video/mp4" || result.Size != int64(len(video)) {
		t.Fatalf("%+v %v", result, err)
	}
	reader, err := store.(storage.SeekableStorage).OpenSeek(context.Background(), result.Key)
	if err != nil {
		t.Fatal(err)
	}
	reader.Close()
	if _, err := Save(context.Background(), store, 1, "html", strings.NewReader("<html>error</html>")); err == nil {
		t.Fatal("accepted HTML as video")
	}
	model.UpdateOption("VideoGenMaxBytes", "32")
	if _, err := Save(context.Background(), store, 1, "too-large", bytes.NewReader(video)); err == nil {
		t.Fatal("accepted oversized video")
	}
	if _, err := store.Stat(context.Background(), "video/1/too-large"); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatal("oversized video written")
	}
	if !Verify("id", Sign("id")) || Verify("other", Sign("id")) || Verify("id", storage.SignShort("GET", "id")) {
		t.Fatal("video signature domain invalid")
	}
}
