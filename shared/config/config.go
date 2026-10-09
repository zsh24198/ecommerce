package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	MySQL   MySQLConfig   `mapstructure:"mysql"`
	Redis   RedisConfig   `mapstructure:"redis"`
	Kafka   KafkaConfig   `mapstructure:"kafka"`
	Log     LogConfig     `mapstructure:"log"`
	JWT     JWTConfig     `mapstructure:"jwt"`
	Payment PaymentConfig `mapstructure:"payment"`
}

type ServerConfig struct {
	Mode string `mapstructure:"mode"`
	Port int    `mapstructure:"port"`
}

type MySQLConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	DBName          string        `mapstructure:"dbname"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type RedisConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Filename   string `mapstructure:"filename"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

// PaymentConfig 支付模块配置。
type PaymentConfig struct {
	// SignSecret 支付回调签名密钥（商户与支付平台约定的共享密钥），用于 HMAC-SHA256 验签。
	// 生产环境应通过环境变量 ECOMMERCE_PAYMENT_SIGN_SECRET 注入，禁止硬编码到代码。
	SignSecret string `mapstructure:"sign_secret"`
}

// KafkaConfig Kafka 配置。
type KafkaConfig struct {
	Brokers []string `mapstructure:"brokers"`
	// Producer 配置
	Producer KafkaProducerConfig `mapstructure:"producer"`
	// Consumer 配置
	Consumer KafkaConsumerConfig `mapstructure:"consumer"`
}

type KafkaProducerConfig struct {
	// RequiredAcks 生产者确认机制：0=不等确认，1=leader 确认，-1=all ISR 确认
	RequiredAcks int `mapstructure:"required_acks"`
	// RetryMax 发送失败最大重试次数
	RetryMax int `mapstructure:"retry_max"`
	// Compression 压缩算法：none/gzip/snappy/lz4/zstd
	Compression string `mapstructure:"compression"`
}

type KafkaConsumerConfig struct {
	// GroupID 消费者组 ID，同组内分区互斥消费
	GroupID string `mapstructure:"group_id"`
	// AutoOffsetReset 无 offset 或 offset 越界时的起始位置：earliest/latest
	AutoOffsetReset string `mapstructure:"auto_offset_reset"`
	// EnableAutoCommit 是否自动提交 offset（false 时需手动提交，保证消费幂等）
	EnableAutoCommit bool `mapstructure:"enable_auto_commit"`
}

func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(path)
	v.SetEnvPrefix("ECOMMERCE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
