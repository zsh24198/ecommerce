package cache

import (
	"context"
	"sync"

	bloom "github.com/bits-and-blooms/bloom/v3"
)

// BloomFilter 位数组 + k 个哈希函数的概率数据结构。
// 用于防缓存穿透：在查 DB 前预判 key 是否"可能存在"，
// 误判可接受（多查一次 DB），漏判不可接受（必须 100% 拦截已写入元素）。
// 接口预留 ctx + error，便于未来扩展为 Redis 分布式布隆或持久化加载。
type BloomFilter interface {
	Add(ctx context.Context, id int64) error
	Test(ctx context.Context, id int64) (bool, error)
	LoadBulk(ctx context.Context, ids []int64) error
}

// memoryBloom bits-and-blooms/bloom 的进程内实现。
type memoryBloom struct {
	mu  sync.RWMutex
	bf  *bloom.BloomFilter
	cap uint
	fpp float64
}

// NewBloomFilter capacity 为预期元素数，fpp 为期望误判率（如 0.001）。
// 底层 NewWithEstimates 根据 capacity 与 fpp 自动算 m 和 k。
// 工程经验：capacity 预留 2 倍余量防误判率飙升。
func NewBloomFilter(capacity uint, fpp float64) BloomFilter {
	return &memoryBloom{
		bf:  bloom.NewWithEstimates(capacity, fpp),
		cap: capacity,
		fpp: fpp,
	}
}

// Add 写入元素。int64 → 8 字节小端，保证 Add 和 Test 序列化方式一致。
// 历史教训：序列化方式不一致会导致哈希位置对不上，布隆完全失效。
func (b *memoryBloom) Add(ctx context.Context, id int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bf.Add(intToBytes(id))
	return nil
}

// Test 查询元素。返回 true 表示"可能存在"（误判），false 表示"一定不存在"。
// 必须与 Add 用同一套字节流，否则位置对不上。
func (b *memoryBloom) Test(ctx context.Context, id int64) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.bf.Test(intToBytes(id)), nil
}

// LoadBulk 启动时全量加载（A 方案）。
func (b *memoryBloom) LoadBulk(ctx context.Context, ids []int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range ids {
		b.bf.Add(intToBytes(id))
	}
	return nil
}

// intToBytes int64 → 8 字节小端表示。
// bytes 包有 binary.BigEndian.PutUint64，但手写避免额外 import。
func intToBytes(n int64) []byte {
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte(n >> (i * 8))
	}
	return buf
}
