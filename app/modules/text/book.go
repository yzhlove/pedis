package text

import (
	"errors"
	"hash/fnv"
	"math/rand/v2"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/modules"
)

// 密码本布局：
//   rows 0–71  : 72 个编码行，每行是基础字符集的一种随机排列
//   row  72    : 索引行，将行号 (0–71) 映射为单个字符
//
// 编码后字符串共 13 个字符：
//   11 位 data   — uint64 的 base-72 表示，经选定行映射
//    1 位 chk    — 所有 data digit 的 XOR 校验，经同一行映射
//    1 位 index  — 行号经第 72 行映射得到的字符，插入位置 = row % 13

const (
	bookLen    = 72             // 每行字符数 / 编码行数
	bookRows   = bookLen + 1    // 73 行（含索引行）
	dataDigits = 11             // ceil(log_72(2^64))：72^11 > 2^64 > 72^10
	strLen     = dataDigits + 2 // 11 data + 1 chk + 1 index = 13
)

// defaultCharset 是配置未提供足够字符时的兜底集合（恰好 72 个可打印 ASCII）。
// 26 小写 + 26 大写 + 10 数字 + 10 符号 = 72
const defaultCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()"

var (
	ErrInvalidString = errors.New("text: invalid encoded string")
	ErrInvalidLength = errors.New("text: encoded string must be 13 characters")
)

type book struct {
	table   [bookRows][bookLen]byte
	inverse [bookRows][256]int8 // inverse[row][c] = digit (0–71)，-1 表示不在该行
}

var _book *book

func New(cfg *config.Config) modules.Modules {
	b := &book{}
	b.build(cfg)
	return b
}

func (b *book) Apply() error {
	_book = b
	return nil
}

// build 根据配置生成完整的 73×72 密码本。
func (b *book) build(cfg *config.Config) {
	base := extractBase(cfg.CharacterSet)

	// 用 FNV-1a 将 TimeSeed 字符串派生为 uint64 种子，避免对格式做假设。
	h := fnv.New64a()
	h.Write([]byte(cfg.TimeSeed))
	seed := h.Sum64()
	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeefcafe1234))

	// 每行用 Fisher-Yates 打乱生成独立排列。
	for i := range b.table {
		copy(b.table[i][:], base)
		for j := bookLen - 1; j > 0; j-- {
			k := rng.IntN(j + 1)
			b.table[i][j], b.table[i][k] = b.table[i][k], b.table[i][j]
		}
	}

	// 预计算逆查找表，解码时 O(1) 取 digit。
	for i := range b.inverse {
		for j := range b.inverse[i] {
			b.inverse[i][j] = -1
		}
		for j := 0; j < bookLen; j++ {
			b.inverse[i][b.table[i][j]] = int8(j)
		}
	}
}

// extractBase 从 s 中提取恰好 bookLen 个不重复字节，
// 若 s 不足则从 defaultCharset 补充。
func extractBase(s string) []byte {
	seen := make(map[byte]bool, bookLen)
	out := make([]byte, 0, bookLen)
	for _, src := range []string{s, defaultCharset} {
		for i := 0; i < len(src) && len(out) < bookLen; i++ {
			c := src[i]
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
		if len(out) >= bookLen {
			break
		}
	}
	return out
}

// rowOf 由 uint64 值计算所用编码行号（0–71）。
// 使用 MurmurHash3 64-bit finalizer 进行位混合，使行号难以从值直接推断。
func rowOf(val uint64) int {
	h := val
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return int(h % uint64(bookLen))
}

// Encode 将 uint64 编码为 13 字符的字符串。
func Encode(val uint64) string { return _book.encode(val) }

func (b *book) encode(val uint64) string {
	row := rowOf(val)

	// 将 val 分解为 dataDigits 位 base-72 数字（大端）。
	var digits [dataDigits]byte
	v := val
	for i := dataDigits - 1; i >= 0; i-- {
		digits[i] = byte(v % uint64(bookLen))
		v /= uint64(bookLen)
	}

	// 校验位：所有 digit 做 XOR 后取模，确保结果在 [0, bookLen) 内。
	var chk byte
	for _, d := range digits {
		chk ^= d
	}
	chk %= bookLen

	// 用选定行映射 digit + chk。
	var encoded [dataDigits + 1]byte
	for i, d := range digits {
		encoded[i] = b.table[row][d]
	}
	encoded[dataDigits] = b.table[row][chk]

	// 用索引行（row 72）将行号映射为索引字符，插入位置 = row % strLen。
	indexChar := b.table[bookLen][row]
	pos := row % strLen

	out := make([]byte, strLen)
	j := 0
	for i := range out {
		if i == pos {
			out[i] = indexChar
		} else {
			out[i] = encoded[j]
			j++
		}
	}
	return string(out)
}

// Decode 将 13 字符字符串解码回 uint64。
// 字符串格式不合法或被篡改时返回 ErrInvalidString。
func Decode(s string) (uint64, error) { return _book.decode(s) }

func (b *book) decode(s string) (uint64, error) {
	if len(s) != strLen {
		return 0, ErrInvalidLength
	}
	bs := []byte(s)

	// 遍历所有 72 个候选行：若行 r 是正确行，则 str[r%strLen] == table[72][r]。
	// 匹配后做完整验证（checksum + rowOf），只有正确行能同时通过两项检验。
	for r := 0; r < bookLen; r++ {
		pos := r % strLen
		if bs[pos] != b.table[bookLen][r] {
			continue
		}
		if val, err := b.tryRow(bs, r, pos); err == nil {
			return val, nil
		}
	}
	return 0, ErrInvalidString
}

// tryRow 以给定的 row 和索引字符位置尝试完整解码与验证。
func (b *book) tryRow(bs []byte, row, indexPos int) (uint64, error) {
	// 去掉索引字符，提取剩余 12 个字符。
	var encoded [dataDigits + 1]byte
	j := 0
	for i, c := range bs {
		if i != indexPos {
			encoded[j] = c
			j++
		}
	}

	// 逆映射：字符 → digit（0–71）。
	var digits [dataDigits]byte
	for i := 0; i < dataDigits; i++ {
		v := b.inverse[row][encoded[i]]
		if v < 0 {
			return 0, ErrInvalidString
		}
		digits[i] = byte(v)
	}

	// 逆映射并校验 checksum 字符。
	chkIdx := b.inverse[row][encoded[dataDigits]]
	if chkIdx < 0 {
		return 0, ErrInvalidString
	}
	var chk byte
	for _, d := range digits {
		chk ^= d
	}
	chk %= bookLen
	if chk != byte(chkIdx) {
		return 0, ErrInvalidString
	}

	// 从 base-72 digits 重建 uint64，带溢出保护。
	const base = uint64(bookLen)
	var val uint64
	for _, d := range digits {
		if val > (^uint64(0))/base {
			return 0, ErrInvalidString
		}
		next := val * base
		if next > (^uint64(0))-uint64(d) {
			return 0, ErrInvalidString
		}
		val = next + uint64(d)
	}

	// 终验证：解码值经 rowOf 必须还原出相同行号。
	if rowOf(val) != row {
		return 0, ErrInvalidString
	}
	return val, nil
}
