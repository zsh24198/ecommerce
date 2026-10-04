package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Signer 支付回调签名器，模拟微信支付的 HMAC-SHA256 签名机制。
// 真实场景中密钥从配置/密钥管理服务读取，这里通过构造函数注入便于测试。
type Signer struct {
	secret string // 商户密钥（类似微信 APIv3 密钥）
}

// NewSigner 构造签名器，secret 为商户与支付平台约定的共享密钥。
func NewSigner(secret string) *Signer {
	return &Signer{secret: secret}
}

// GenerateSign 根据参数生成签名。
// 算法：参数按 key 字典序排序 → 拼接 key=value&... → 追加 &key=secret → HMAC-SHA256 → hex 大写。
func (s *Signer) GenerateSign(params map[string]string) string {
	// 1. 提取所有 key（排除 sign 本身）并排序
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 2. 拼接 key=value&...
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(params[k])
	}
	// 3. 追加密钥
	buf.WriteString("&key=")
	buf.WriteString(s.secret)

	// 4. HMAC-SHA256 + hex 大写
	h := hmac.New(sha256.New, []byte(s.secret))
	h.Write([]byte(buf.String()))
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
}

// VerifySign 验证回调签名是否合法。
// params 包含回调的全部参数（含 sign 字段）。
func (s *Signer) VerifySign(params map[string]string) bool {
	sign, ok := params["sign"]
	if !ok || sign == "" {
		return false
	}
	expected := s.GenerateSign(params)
	// 用 hmac.Equal 做常量时间比较，防止时序攻击
	return hmac.Equal([]byte(sign), []byte(expected))
}
