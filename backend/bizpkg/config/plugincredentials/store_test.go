package plugincredentials

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
	"strings"
	"testing"
)

type memoryCredentials struct{ records map[string]Record }

func (m *memoryCredentials) Get(_ context.Context, _ string, key string) (*Record, error) {
	v, ok := m.records[key]
	if !ok {
		return nil, kvstore.ErrKeyNotFound
	}
	return &v, nil
}
func (m *memoryCredentials) Save(_ context.Context, _ string, key string, v *Record) error {
	m.records[key] = *v
	return nil
}
func credentialFixture() (*Store, *memoryCredentials) {
	repo := &memoryCredentials{records: map[string]Record{}}
	return &Store{data: repo, key: func() ([]byte, error) { return []byte(strings.Repeat("k", 32)), nil }}, repo
}
func TestPluginCredentialEncryptedPersistenceAndImmediateUpdate(t *testing.T) {
	ctx := context.Background()
	store, repo := credentialFixture()
	const secret = "test-only-credential-not-real-12345678"
	if err := store.Save(ctx, "amap", secret, false, 42); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(repo.records["amap"])
	if strings.Contains(string(data), secret) {
		t.Fatal("数据库记录不应含明文凭据")
	}
	other := &Store{data: repo, key: store.key}
	token, source, err := other.Resolve(ctx, "amap", "old-file-value")
	if err != nil || token != secret || source != "admin" {
		t.Fatalf("新实例未读取持久凭据：%s %v", source, err)
	}
	meta, err := other.Metadata(ctx, "amap", "")
	encoded, _ := json.Marshal(meta)
	if err != nil || !meta.Configured || strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "ciphertext") {
		t.Fatal("管理状态不得回显明文或密文")
	}
	if err = store.Save(ctx, "amap", "new-test-key-87654321", false, 42); err != nil {
		t.Fatal(err)
	}
	token, _, err = other.Resolve(ctx, "amap", "")
	if err != nil || token != "new-test-key-87654321" {
		t.Fatal("已有实例未即时读取新凭据")
	}
}
func TestPluginCredentialKeepClearAndFallback(t *testing.T) {
	ctx := context.Background()
	store, _ := credentialFixture()
	token, source, err := store.Resolve(ctx, "amap", "file-test-value")
	if err != nil || token != "file-test-value" || source != "file" {
		t.Fatal("未设置时应兼容文件凭据")
	}
	if err = store.Save(ctx, "amap", "", false, 42); err != nil {
		t.Fatal(err)
	}
	token, _, _ = store.Resolve(ctx, "amap", "file-test-value")
	if token != "file-test-value" {
		t.Fatal("空输入不能误清除原值")
	}
	if err = store.Save(ctx, "amap", "", true, 42); err != nil {
		t.Fatal(err)
	}
	token, source, err = store.Resolve(ctx, "amap", "file-test-value")
	if err != nil || token != "" || source != "admin" {
		t.Fatal("显式清除后不能回退旧文件密钥")
	}
}
func TestPluginCredentialRejectsCorruptionAndWrongProvider(t *testing.T) {
	ctx := context.Background()
	store, repo := credentialFixture()
	if err := store.Save(ctx, "amap", "test-key-12345678", false, 42); err != nil {
		t.Fatal(err)
	}
	repo.records["tianyancha"] = repo.records["amap"]
	if _, _, err := store.Resolve(ctx, "tianyancha", ""); err == nil {
		t.Fatal("不同供应商的密文不能互换")
	}
	store.key = func() ([]byte, error) { return []byte(strings.Repeat("x", 32)), nil }
	if _, _, err := store.Resolve(ctx, "amap", ""); err == nil {
		t.Fatal("错误的加密密钥不能继续读取凭据")
	}
	store.key = func() ([]byte, error) { return nil, errors.New("缺少密钥") }
	if err := store.Save(ctx, "amap", "test-key-12345678", false, 42); err == nil {
		t.Fatal("缺少密钥时不能保存明文")
	}
}
func TestPluginCredentialInputValidation(t *testing.T) {
	for _, value := range []string{"short", " key-12345678", "key-12345678\n", "test-key-12345678\r\nAuthorization: injected", strings.Repeat("x", 4097)} {
		store, _ := credentialFixture()
		if err := store.Save(context.Background(), "amap", value, false, 42); err == nil {
			t.Fatal("未拒绝非法密钥输入")
		}
	}
	store, _ := credentialFixture()
	if err := store.Save(context.Background(), "unknown", "test-key-12345678", false, 42); err == nil {
		t.Fatal("未拒绝未知供应商")
	}
}
