package config

const (
	// PCGStreamSeed 是 PCG 伪随机数生成器的流选择器，用于与哈希种子异或后确定 PCG 的流参数。
	// 取自经典的构造性魔数 0xdeadbeef（死牛肉）与 0xcafe1234 的拼接，
	// 目的是让两个 PCG 参数之间保持足够的汉明距离，避免生成序列相关。
	PCGStreamSeed uint64 = 0xdeadbeefcafe1234

	// MurmurHash3Mix1 是 MurmurHash3（64 位版本）终结化阶段第一个混合乘数。
	// 来源：Austin Appleby 在 MurmurHash3 实现中引入，用于打散低位熵。
	// 参考：https://github.com/aappleby/smhasher (MurmurHash3, fmix64)
	MurmurHash3Mix1 uint64 = 0xff51afd7ed558ccd

	// MurmurHash3Mix2 是 MurmurHash3（64 位版本）终结化阶段第二个混合乘数。
	// 来源：同 MurmurHash3Mix1，与其配合使图形雪崩效果达标（avalanche score ≈ 0.020）。
	// 参考：https://github.com/aappleby/smhasher (MurmurHash3, fmix64)
	MurmurHash3Mix2 uint64 = 0xc4ceb9fe1a85ec53
)
