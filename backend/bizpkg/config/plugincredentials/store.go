// Package plugincredentials 保存平台共用的插件授权，不向管理页面返回密钥。
package plugincredentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const Namespace = "plugin_service_credentials"
const KeyFileEnv = "PLUGIN_CREDENTIAL_KEY_FILE"

type Provider struct {
	ID              string `json:"id"`
	PluginID        string `json:"plugin_id"`
	Name            string `json:"name"`
	CredentialLabel string `json:"credential_label"`
	ApplyURL        string `json:"apply_url"`
	Description     string `json:"description"`
}

// 目录只定义受支持的供应商，测试地址和认证方式由服务端固定，避免任意地址携带密钥。
func Providers() []Provider {
	return []Provider{
		{ID: "tianyancha", PluginID: "12", Name: "天眼查", CredentialLabel: "API Token", ApplyURL: "https://open.tianyancha.com/console/mydata", Description: "企业搜索、基本信息和经营异常等接口；实际可用范围取决于供应商授权。"},
		{ID: "amap", PluginID: "22", Name: "高德地图", CredentialLabel: "Web 服务 Key", ApplyURL: "https://lbs.amap.com/api/webservice/create-project-and-key", Description: "地理编码、地点搜索和周边搜索等工具使用的 Web 服务授权。"},
	}
}
func Find(id string) (Provider, bool) {
	for _, p := range Providers() {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}
func FindPlugin(id int64) (Provider, bool) {
	for _, p := range Providers() {
		if p.PluginID == strconv.FormatInt(id, 10) {
			return p, true
		}
	}
	return Provider{}, false
}

type Record struct {
	Ciphertext string `json:"ciphertext"`
	UpdatedAt  int64  `json:"updated_at"`
	UpdatedBy  string `json:"updated_by"`
}
type Metadata struct {
	Provider
	Configured bool   `json:"configured"`
	Source     string `json:"source"`
	UpdatedAt  int64  `json:"updated_at"`
}
type repository interface {
	Get(context.Context, string, string) (*Record, error)
	Save(context.Context, string, string, *Record) error
}
type Store struct {
	data repository
	key  func() ([]byte, error)
}

func New(db *gorm.DB) *Store {
	if db != nil {
		db = db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)})
	}
	return &Store{data: kvstore.New[Record](db), key: loadKey}
}
func loadKey() ([]byte, error) {
	path := os.Getenv(KeyFileEnv)
	if path == "" {
		return nil, errors.New("尚未配置插件凭据加密密钥文件")
	}
	value, err := os.ReadFile(path)
	if err != nil || len(value) != 32 {
		return nil, errors.New("插件凭据加密密钥文件不可用，需要持久保存的 32 字节密钥")
	}
	return value, nil
}
func (s *Store) Ready() bool { _, err := s.key(); return err == nil }
func seal(key []byte, provider, secret string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(secret), []byte(Namespace+":"+provider))), nil
}
func open(key []byte, provider, value string) (string, error) {
	fail := errors.New("插件凭据解密失败，请检查原加密密钥或重新配置授权")
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fail
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fail
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(data) < aead.NonceSize() {
		return "", fail
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(Namespace+":"+provider))
	if err != nil {
		return "", fail
	}
	return string(plain), nil
}

// 每次调用读取持久配置，支持多进程及重启后的生效，不修改共享插件元数据。
func (s *Store) Resolve(ctx context.Context, id, fallback string) (string, string, error) {
	if _, ok := Find(id); !ok {
		return "", "", errors.New("不支持的插件供应商")
	}
	record, err := s.data.Get(ctx, Namespace, id)
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		source := "none"
		if fallback != "" {
			source = "file"
		}
		return fallback, source, nil
	}
	if err != nil {
		return "", "", errors.New("读取插件授权配置失败")
	}
	// 空密文是显式清除授权，不能重新回退到配置文件中的旧密钥。
	if record.Ciphertext == "" {
		return "", "admin", nil
	}
	key, err := s.key()
	if err != nil {
		return "", "", err
	}
	value, err := open(key, id, record.Ciphertext)
	return value, "admin", err
}
func (s *Store) Metadata(ctx context.Context, id, fallback string) (Metadata, error) {
	provider, ok := Find(id)
	if !ok {
		return Metadata{}, errors.New("不支持的插件供应商")
	}
	value, source, err := s.Resolve(ctx, id, fallback)
	if err != nil {
		return Metadata{}, err
	}
	meta := Metadata{Provider: provider, Configured: value != "", Source: source}
	record, err := s.data.Get(ctx, Namespace, id)
	if err == nil {
		meta.UpdatedAt = record.UpdatedAt
	} else if !errors.Is(err, kvstore.ErrKeyNotFound) {
		return Metadata{}, errors.New("读取插件授权状态失败")
	}
	return meta, nil
}
func (s *Store) Save(ctx context.Context, id, value string, clear bool, actor int64) error {
	if _, ok := Find(id); !ok {
		return errors.New("不支持的插件供应商")
	}
	if clear && value != "" {
		return errors.New("清除授权时不能同时提交新密钥")
	}
	if !clear && value == "" {
		return nil
	} // 留空保留，不把未填写当作清除。
	if !clear {
		if len(value) < 8 || len(value) > 4096 || strings.TrimSpace(value) != value {
			return errors.New("密钥须为 8～4096 个字符，不能包含首尾空白")
		}
		for _, r := range value {
			if r < 33 || r > 126 {
				return errors.New("密钥只能包含可见的英文字符、数字或符号")
			}
		}
	}
	key, err := s.key()
	if err != nil {
		return err
	}
	record := &Record{UpdatedAt: time.Now().UnixMilli(), UpdatedBy: strconv.FormatInt(actor, 10)}
	if !clear {
		record.Ciphertext, err = seal(key, id, value)
		if err != nil {
			return errors.New("插件凭据加密失败")
		}
	}
	if err = s.data.Save(ctx, Namespace, id, record); err != nil {
		return fmt.Errorf("保存插件授权失败")
	}
	return nil
}
