package audioasset

import (
	"bytes"
	"context"
	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAudioFormatsSignaturesAndAssetOwnership(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "assets.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if e = db.AutoMigrate(&model.AudioAsset{}); e != nil {
		t.Fatal(e)
	}
	oldDB := model.DB
	model.DB = db
	defer func() { model.DB = oldDB }()
	model.OptionMapMutex.Lock()
	oldMap := model.OptionMap
	model.OptionMap = map[string]string{"ServerAddress": "https://audio.example", "AudioGenRetentionHours": "24"}
	model.OptionMapMutex.Unlock()
	defer func() { model.OptionMapMutex.Lock(); model.OptionMap = oldMap; model.OptionMapMutex.Unlock() }()
	store, e := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	wav, e := WAV([]byte{0, 0, 1, 0}, 24000, 1, 16)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = Inspect(wav, true); e != nil {
		t.Fatal(e)
	}
	for _, b := range [][]byte{[]byte("this is not an mp3 file"), wav[:20], []byte("RIFF0000WAVEjunk0000")} {
		if _, _, e = Inspect(b, true); e == nil {
			t.Fatal("accepted corrupt audio")
		}
	}
	if _, e = WAV([]byte{0}, 0, 1, 16); e == nil {
		t.Fatal("guessed missing PCM parameters")
	}
	asset, e := Save(context.Background(), db, store, 1, "", "upload", wav)
	if e != nil {
		t.Fatal(e)
	}
	ref := URL("GET", asset.ShortID)
	b, format, e := Load(context.Background(), db, store, 1, ref, InputLimit())
	if e != nil || format != "wav" || !bytes.Equal(wav, b) {
		t.Fatal(e)
	}
	if _, _, e = Load(context.Background(), db, store, 2, ref, InputLimit()); e == nil {
		t.Fatal("cross-user asset read")
	}
	if Verify("PUT", asset.ShortID, Sign("GET", asset.ShortID)) {
		t.Fatal("GET token authorized PUT")
	}
	if !Verify("HEAD", asset.ShortID, Sign("GET", asset.ShortID)) {
		t.Fatal("HEAD cannot read")
	}
	if Verify("GET", asset.ShortID, storage.SignShort("GET", asset.ShortID)) {
		t.Fatal("image token authorized audio")
	}
	past := time.Now().Add(-time.Hour)
	db.Model(asset).Update("expires_at", past)
	if _, _, e = Load(context.Background(), db, store, 1, ref, InputLimit()); e == nil {
		t.Fatal("expired audio read")
	}
	if _, _, e = Load(context.Background(), db, store, 1, "http://127.0.0.1/private.wav", InputLimit()); e == nil || !strings.Contains(e.Error(), "private") {
		t.Fatalf("private URL accepted: %v", e)
	}
}
