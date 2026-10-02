// Package jwtx 提供项目统一的 JWT 签发与解析能力，供各模块与认证中间件共用。
package jwtx

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/config"
)

// token 类型标识，写入 Claims.Typ，校验时防止两类 token 混用。
const (
	TypAccess  = "access"
	TypRefresh = "refresh"
)

// Claims JWT 载荷：业务字段 + 标准字段（exp/iat 等）。
type Claims struct {
	UserID int64  `json:"user_id"`
	Typ    string `json:"typ"`
	jwt.RegisteredClaims
}

// Manager JWT 签发器。配置在构造时注入一次，之后所有方法复用。
type Manager struct {
	cfg *config.JWTConfig
}

// NewManager 构造签发器。
func NewManager(cfg *config.JWTConfig) *Manager {
	return &Manager{cfg: cfg}
}

// TTL 返回指定类型 token 的有效期，供持久化层写 expires_at，与签发逻辑同源。
func (m *Manager) TTL(typ string) time.Duration {
	if typ == TypRefresh {
		return m.cfg.RefreshTTL
	}
	return m.cfg.AccessTTL
}

// Issue 签发 token。typ 取 TypAccess / TypRefresh，分别使用对应 TTL。
func (m *Manager) Issue(userID int64, typ string) (string, error) {
	ttl := m.TTL(typ)
	now := time.Now()
	// jti：每 token 唯一随机标识。iat/exp 只有秒级精度，同一用户同一秒内
	// 重复签发会得到字节级相同的 token（HS256 确定性签名），必须靠 jti 区分。
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", apperror.Wrap(apperror.CodeUnknown, "生成jti失败", err)
	}
	claims := Claims{
		UserID: userID,
		Typ:    typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        hex.EncodeToString(jti),
			Issuer:    "ecommerce",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString([]byte(m.cfg.Secret))
}

// Parse 解析并校验 token。wantTyp 指定期望的类型，不匹配视为无效，防止混用。
func (m *Manager) Parse(tokenStr, wantTyp string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return []byte(m.cfg.Secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperror.New(apperror.CodeTokenExpired, "token 已过期")
		}
		return nil, apperror.New(apperror.CodeTokenInvalid, "token 无效")
	}
	if !token.Valid || claims.Typ != wantTyp {
		return nil, apperror.New(apperror.CodeTokenInvalid, "token 无效")
	}
	return claims, nil
}
