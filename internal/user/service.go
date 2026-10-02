package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/jwtx"
)

// UserInfo 注册成功后返回给前端的用户信息 DTO，结构上不含任何敏感字段。
type UserInfo struct {
	ID     int64      `json:"id"`
	Phone  string     `json:"phone"`
	Status UserStatus `json:"status"`
}

// TokenPair 登录/刷新成功后返回的 token 对。
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// UserService 用户模块业务接口，供 handler 层依赖。
type UserService interface {
	// Register 注册新用户，手机号已存在返回 apperror.ErrUserExists。
	Register(ctx context.Context, phone, password string) (*UserInfo, error)
	// Login 手机号+密码登录，凭证错误统一返回 ErrUserCreds（防撞库探测）。
	Login(ctx context.Context, phone, password string) (*TokenPair, error)
	// RefreshToken 用 refresh token 换新 token 对，旧 token 立即轮换作废。
	RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)
}

// userService 依赖注入：repo 负责数据，jwt 负责签发/解析，配置由 Manager 内部持有。
type userService struct {
	repo UserRepository
	jwt  *jwtx.Manager
}

// NewUserService 构造函数。
func NewUserService(repo UserRepository, jwt *jwtx.Manager) UserService {
	return &userService{repo: repo, jwt: jwt}
}

// Register 注册新用户。
func (s *userService) Register(ctx context.Context, phone, password string) (*UserInfo, error) {
	if phone == "" || password == "" {
		return nil, apperror.New(apperror.CodeParamInvalid, "手机号或密码不能为空")
	}
	if len(password) < 6 {
		return nil, apperror.New(apperror.CodeParamInvalid, "密码至少6位")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "密码加密失败", err)
	}
	u := &User{
		Phone:        phone,
		PasswordHash: string(hash),
		Status:       UserStatusActive,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return &UserInfo{ID: u.ID, Phone: u.Phone, Status: u.Status}, nil
}

// Login 登录。安全约定：手机号不存在与密码错误返回同一个 ErrUserCreds；
// 封禁检查放在 bcrypt 之前，避免为注定失败的请求浪费慢哈希 CPU。
func (s *userService) Login(ctx context.Context, phone, password string) (*TokenPair, error) {
	user, err := s.repo.FindByPhone(ctx, phone)
	if err != nil {
		if errors.Is(err, apperror.ErrUserNotFound) {
			return nil, apperror.ErrUserCreds
		}
		return nil, err
	}
	if user.Status == UserStatusBanned {
		return nil, apperror.ErrUserBanned
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, apperror.ErrUserCreds
	}
	return s.issueTokenPair(ctx, user.ID)
}

// RefreshToken 刷新流程：验签 → 哈希查库 → 校验有效期 → 原子作废旧 token → 签发新对。
// 先作废再签发：签发失败用户重新登录即可；反之若先签发后作废失败，会出现新旧并存。
func (s *userService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.jwt.Parse(refreshToken, jwtx.TypRefresh)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(refreshToken))
	rt, err := s.repo.GetRefreshTokenByHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, err
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, apperror.ErrTokenInvalid
	}
	if err := s.repo.RevokeRefreshToken(ctx, rt.TokenHash); err != nil {
		return nil, err
	}
	return s.issueTokenPair(ctx, claims.UserID)
}

// issueTokenPair 签发 access/refresh 对，refresh 哈希入库（expires_at 与签发 TTL 同源）。
func (s *userService) issueTokenPair(ctx context.Context, userID int64) (*TokenPair, error) {
	access, err := s.jwt.Issue(userID, jwtx.TypAccess)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "签发access token失败", err)
	}
	refresh, err := s.jwt.Issue(userID, jwtx.TypRefresh)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnknown, "签发refresh token失败", err)
	}
	sum := sha256.Sum256([]byte(refresh))
	rt := &RefreshToken{
		UserID:    userID,
		TokenHash: hex.EncodeToString(sum[:]),
		ExpiresAt: time.Now().Add(s.jwt.TTL(jwtx.TypRefresh)),
	}
	if err := s.repo.CreateRefreshToken(ctx, rt); err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}
