package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/zsh24198/ecommerce/internal/user"
	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/config"
	"github.com/zsh24198/ecommerce/shared/jwtx"
)

func main() {
	cfg, err := config.Load("configs/config.yaml")
	must("加载配置", err)

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.MySQL.User, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.DBName)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	must("连接数据库", err)

	svc := user.NewUserService(user.NewUserRepository(db), jwtx.NewManager(&cfg.JWT))
	ctx := context.Background()
	phone := fmt.Sprintf("138%08d", rand.Intn(100000000))
	password := "test123456"

	// 1. 注册
	info, err := svc.Register(ctx, phone, password)
	must("注册", err)
	fmt.Printf("✓ 注册成功 id=%d phone=%s\n", info.ID, info.Phone)

	// 2. 重复注册 → ErrUserExists
	_, err = svc.Register(ctx, phone, password)
	expectErr("重复注册", err, apperror.ErrUserExists)

	// 3. 密码错误 → ErrUserCreds
	_, err = svc.Login(ctx, phone, "wrong!")
	expectErr("密码错误", err, apperror.ErrUserCreds)

	// 4. 手机号不存在 → 同样返回 ErrUserCreds（防撞库探测的关键断言）
	_, err = svc.Login(ctx, "19900000001", password)
	expectErr("手机号不存在", err, apperror.ErrUserCreds)

	// 5. 正常登录
	pair1, err := svc.Login(ctx, phone, password)
	must("登录", err)
	fmt.Printf("✓ 登录成功 access=%d字符 refresh=%d字符\n", len(pair1.AccessToken), len(pair1.RefreshToken))

	// 6. 解析 access token，userID 必须一致
	claims, err := jwtx.NewManager(&cfg.JWT).Parse(pair1.AccessToken, jwtx.TypAccess)
	must("解析access token", err)
	if claims.UserID != info.ID {
		log.Fatalf("✗ userID不匹配: 期望%d 实际%d", info.ID, claims.UserID)
	}
	fmt.Printf("✓ access token 解析正确 userID=%d\n", claims.UserID)

	// 7. token 混用防护：access 冒充 refresh 必须被拒
	_, err = jwtx.NewManager(&cfg.JWT).Parse(pair1.AccessToken, jwtx.TypRefresh)
	expectErr("access冒充refresh", err, apperror.ErrTokenInvalid)

	// 8. 刷新轮换：换出新对
	pair2, err := svc.RefreshToken(ctx, pair1.RefreshToken)
	must("刷新", err)
	fmt.Printf("✓ 刷新成功，新旧token不同: %v\n", pair1.AccessToken != pair2.AccessToken)

	// 9. 旧 refresh token 二次使用 → ErrTokenInvalid（轮换+CAS防并发的最终证明）
	_, err = svc.RefreshToken(ctx, pair1.RefreshToken)
	expectErr("旧refresh二次使用", err, apperror.ErrTokenInvalid)

	fmt.Println("\n=== 九步全部通过，任务9核心流程验证完毕 ===")
}

func must(step string, err error) {
	if err != nil {
		log.Fatalf("✗ %s失败: %v", step, err)
	}
}

func expectErr(step string, got, want error) {
	if got == nil || !errors.Is(got, want) {
		log.Fatalf("✗ %s: 期望 %v，实际 %v", step, want, got)
	}
	fmt.Printf("✓ %s → 正确返回: %v\n", step, got)
}
