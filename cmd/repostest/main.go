package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/zsh24198/ecommerce/internal/user"
	"github.com/zsh24198/ecommerce/shared/apperror"
	"github.com/zsh24198/ecommerce/shared/config"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		panic(fmt.Sprintf("load config: %v", err))
	}

	// 2. 连接 MySQL
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.MySQL.User, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.DBName)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(fmt.Sprintf("connect mysql: %v", err))
	}

	// 3. 构造 repository
	repo := user.NewUserRepository(db)
	ctx := context.Background()

	// 用一个带时间戳的手机号，避免和之前测试数据冲突
	phone := fmt.Sprintf("138%08d", time.Now().Unix()%100000000)
	fmt.Printf("测试手机号: %s\n\n", phone)

	// --- 场景 1: Create 成功 ---
	fmt.Println("=== 场景 1: Create 用户 ===")
	newUser := &user.User{
		Phone:        phone,
		PasswordHash: "$2a$10$placeholder_hash_for_test",
		Status:       user.UserStatusActive,
	}
	if err := repo.Create(ctx, newUser); err != nil {
		panic(fmt.Sprintf("Create 失败: %v", err))
	}
	fmt.Printf("✅ Create 成功，自增 ID = %d\n\n", newUser.ID)

	// --- 场景 2: FindByPhone 查到 ---
	fmt.Println("=== 场景 2: FindByPhone ===")
	foundByPhone, err := repo.FindByPhone(ctx, phone)
	if err != nil {
		panic(fmt.Sprintf("FindByPhone 失败: %v", err))
	}
	fmt.Printf("✅ FindByPhone 成功: ID=%d, Phone=%s, Status=%d\n\n",
		foundByPhone.ID, foundByPhone.Phone, foundByPhone.Status)

	// --- 场景 3: FindByID 查到 ---
	fmt.Println("=== 场景 3: FindByID ===")
	foundByID, err := repo.FindByID(ctx, newUser.ID)
	if err != nil {
		panic(fmt.Sprintf("FindByID 失败: %v", err))
	}
	fmt.Printf("✅ FindByID 成功: ID=%d, Phone=%s\n\n", foundByID.ID, foundByID.Phone)

	// --- 场景 4: 重复 Create 应返回 ErrUserExists ---
	fmt.Println("=== 场景 4: 重复 Create（应报 ErrUserExists）===")
	dupUser := &user.User{
		Phone:        phone,
		PasswordHash: "$2a$10$another_hash",
		Status:       user.UserStatusActive,
	}
	err = repo.Create(ctx, dupUser)
	if err == nil {
		panic("❌ 重复 Create 没有报错！唯一索引没生效？")
	}
	if errors.Is(err, apperror.ErrUserExists) {
		fmt.Printf("✅ 重复 Create 正确返回 ErrUserExists (code=%d)\n\n", apperror.CodeUserExists)
	} else {
		panic(fmt.Sprintf("❌ 返回了错误的错误类型: %v", err))
	}

	// --- 场景 5: 查不存在的用户应返回 ErrUserNotFound ---
	fmt.Println("=== 场景 5: FindByPhone 查不存在（应报 ErrUserNotFound）===")
	_, err = repo.FindByPhone(ctx, "19900000000")
	if err == nil {
		panic("❌ 查不存在的用户没有报错！")
	}
	if errors.Is(err, apperror.ErrUserNotFound) {
		fmt.Printf("✅ 查不存在的用户正确返回 ErrUserNotFound (code=%d)\n\n", apperror.CodeUserNotFound)
	} else {
		panic(fmt.Sprintf("❌ 返回了错误的错误类型: %v", err))
	}

	fmt.Println("🎉 全部场景验证通过！")
}
