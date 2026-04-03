package text

import (
	"math"
	"math/bits"
	"strings"
	"testing"

	"github.com/yzhlove/peids/app/config"
)

// newTestBook 以固定种子初始化密码本，测试结果可复现。
func newTestBook(tb testing.TB) {
	tb.Helper()
	New(&config.Config{
		TimeSeed:     "pedis-test-seed",
		CharacterSet: "",
	})
}

// ── 基础正确性 ──────────────────────────────────────────────────────────────

func TestEncodeDecodeRoundTrip(t *testing.T) {
	newTestBook(t)

	cases := []uint64{
		0,
		1,
		71,
		72,
		255,
		1000,
		math.MaxUint32,
		math.MaxUint32 + 1,
		math.MaxInt64,
		math.MaxUint64 - 1,
		math.MaxUint64,
	}

	for _, v := range cases {
		s := Encode(v)
		got, err := Decode(s)
		if err != nil {
			t.Errorf("Decode(%q) error: %v  (encoded from %d)", s, err, v)
			continue
		}
		if got != v {
			t.Errorf("round-trip mismatch: Encode(%d)=%q, Decode→%d", v, s, got)
		}
	}
}

func TestEncodedLength(t *testing.T) {
	newTestBook(t)

	for _, v := range []uint64{0, 1, math.MaxUint64} {
		s := Encode(v)
		if len(s) != strLen {
			t.Errorf("Encode(%d) length = %d, want %d", v, len(s), strLen)
		}
	}
}

// 相同值使用相同种子必须产生完全一致的编码结果。
func TestEncodeDeterministic(t *testing.T) {
	newTestBook(t)

	for _, v := range []uint64{0, 42, math.MaxUint64} {
		a, b := Encode(v), Encode(v)
		if a != b {
			t.Errorf("Encode(%d) not deterministic: %q vs %q", v, a, b)
		}
	}
}

// 不同值编码结果必须不同（在测试覆盖范围内无碰撞）。
func TestEncodeDistinct(t *testing.T) {
	newTestBook(t)

	seen := make(map[string]uint64, 1000)
	for i := uint64(0); i < 1000; i++ {
		s := Encode(i)
		if prev, ok := seen[s]; ok {
			t.Fatalf("collision: Encode(%d) == Encode(%d) == %q", i, prev, s)
		}
		seen[s] = i
	}
}

// 编码字符串只包含密码本基础字符集内的字符。
func TestEncodedCharsInCharset(t *testing.T) {
	newTestBook(t)

	charsetSet := make(map[byte]bool, bookLen)
	for i := 0; i < bookLen; i++ {
		charsetSet[_book.table[0][0]] = true // 占位，下面直接遍历 table
	}
	// 收集所有出现在 table[0] 中的字符（即基础字符集）
	base := make(map[byte]bool, bookLen)
	for _, row := range _book.table {
		for _, c := range row {
			base[c] = true
		}
	}

	for _, v := range []uint64{0, 1, math.MaxUint64, 999999} {
		for _, c := range []byte(Encode(v)) {
			if !base[c] {
				t.Errorf("Encode(%d) contains char %q not in charset", v, c)
			}
		}
	}
}

// ── 解码拒绝非法输入 ─────────────────────────────────────────────────────────

func TestDecodeWrongLength(t *testing.T) {
	newTestBook(t)

	cases := []string{"", "a", strings.Repeat("a", strLen-1), strings.Repeat("a", strLen+1)}
	for _, s := range cases {
		if _, err := Decode(s); err != ErrInvalidLength {
			t.Errorf("Decode(%q) error = %v, want ErrInvalidLength", s, err)
		}
	}
}

func TestDecodeTamperedSingleBit(t *testing.T) {
	newTestBook(t)

	for _, v := range []uint64{0, 42, math.MaxUint64} {
		original := Encode(v)
		bs := []byte(original)
		// 逐位翻转每个字节，任意 1-bit 篡改都必须被检测到
		for pos := 0; pos < len(bs); pos++ {
			for bit := uint(0); bit < 8; bit++ {
				tampered := make([]byte, len(bs))
				copy(tampered, bs)
				tampered[pos] ^= 1 << bit
				if string(tampered) == original {
					continue // 翻转后恰好相同，跳过
				}
				_, err := Decode(string(tampered))
				if err == nil {
					t.Errorf("tampered string accepted: original=%q pos=%d bit=%d tampered=%q val=%d",
						original, pos, bit, tampered, v)
				}
			}
		}
	}
}

func TestDecodeTamperedSingleChar(t *testing.T) {
	newTestBook(t)

	// 单字符篡改后，解码结果不能与原值相同。
	// 注：篡改后的字符串可能恰好是另一个合法值的编码，那是正常的；
	// 但它不能解码回原值，否则说明存在碰撞漏洞。
	for _, v := range []uint64{1, 1000, math.MaxUint32} {
		original := Encode(v)
		bs := []byte(original)
		for pos := 0; pos < len(bs); pos++ {
			orig := bs[pos]
			for _, c := range []byte(defaultCharset) {
				if c == orig {
					continue
				}
				bs[pos] = c
				got, err := Decode(string(bs))
				if err == nil && got == v {
					t.Errorf("tampered string decodes to same value: original=%q pos=%d char=%q val=%d",
						original, pos, c, v)
				}
			}
			bs[pos] = orig
		}
	}
}

func TestDecodeAllZeroString(t *testing.T) {
	newTestBook(t)

	s := strings.Repeat("\x00", strLen)
	_, err := Decode(s)
	if err == nil {
		t.Error("all-zero string should not decode successfully")
	}
}

func TestDecodeRandomString(t *testing.T) {
	newTestBook(t)

	// 用一个非密码本字符填充，应直接被拒绝
	s := strings.Repeat("~", strLen)
	_, err := Decode(s)
	if err == nil {
		t.Error("random string should not decode successfully")
	}
}

// ── 边界值 ────────────────────────────────────────────────────────────────────

func TestEncodeDecodeBoundary(t *testing.T) {
	newTestBook(t)

	boundaries := []uint64{
		0,
		1,
		71,        // bookLen - 1
		72,        // bookLen
		72*72 - 1, // 两位 base-72 最大值
		math.MaxUint32,
		math.MaxUint32 + 1,
		1<<63 - 1, // MaxInt64
		1 << 63,   // MinInt64 (as uint64)
		math.MaxUint64 - 1,
		math.MaxUint64,
	}

	for _, v := range boundaries {
		s := Encode(v)
		got, err := Decode(s)
		if err != nil || got != v {
			t.Errorf("boundary %d: encode=%q decode=%d err=%v", v, s, got, err)
		}
	}
}

// ── 种子隔离性：不同种子产生不同编码 ─────────────────────────────────────────

func TestDifferentSeedsDifferentEncoding(t *testing.T) {
	val := uint64(12345)

	New(&config.Config{TimeSeed: "seed-A"})
	encA := Encode(val)

	New(&config.Config{TimeSeed: "seed-B"})
	encB := Encode(val)

	if encA == encB {
		t.Errorf("different seeds produced same encoding: %q", encA)
	}

	// 各自解码必须正确
	New(&config.Config{TimeSeed: "seed-A"})
	if got, err := Decode(encA); err != nil || got != val {
		t.Errorf("seed-A decode failed: got=%d err=%v", got, err)
	}

	New(&config.Config{TimeSeed: "seed-B"})
	if got, err := Decode(encB); err != nil || got != val {
		t.Errorf("seed-B decode failed: got=%d err=%v", got, err)
	}
}

// 用 seed-A 编码的字符串不能被 seed-B 正确解码。
func TestCrossDecodeFails(t *testing.T) {
	val := uint64(99999)

	New(&config.Config{TimeSeed: "seed-X"})
	encX := Encode(val)

	New(&config.Config{TimeSeed: "seed-Y"})
	got, err := Decode(encX)
	if err == nil && got == val {
		t.Error("cross-seed decode should fail")
	}
}

// ── 自定义字符集 ──────────────────────────────────────────────────────────────

func TestCustomCharacterSet(t *testing.T) {
	// 用连续可打印 ASCII (0x21–0x68) 构造恰好 72 个不重复字符，
	// 与 defaultCharset 完全不同，验证自定义字符集路径。
	var buf [bookLen]byte
	for i := range buf {
		buf[i] = byte('!' + i) // 0x21 .. 0x68
	}
	charset := string(buf[:])

	New(&config.Config{TimeSeed: "custom-seed", CharacterSet: charset})

	for _, v := range []uint64{0, 42, math.MaxUint32, math.MaxUint64} {
		s := Encode(v)
		got, err := Decode(s)
		if err != nil || got != v {
			t.Errorf("custom charset: val=%d enc=%q got=%d err=%v", v, s, got, err)
		}
	}
}

// 字符集不足时自动从 defaultCharset 补充，不应 panic。
func TestInsufficientCharacterSetFallback(t *testing.T) {
	New(&config.Config{TimeSeed: "fallback-seed", CharacterSet: "abc"})
	v := uint64(42)
	s := Encode(v)
	got, err := Decode(s)
	if err != nil || got != v {
		t.Errorf("fallback charset: val=%d enc=%q got=%d err=%v", v, s, got, err)
	}
}

// ── rowOf 验证 ────────────────────────────────────────────────────────────────

// rowOf 必须将所有值均匀分布到 0–71，偏差不能过大。
func TestRowOfDistribution(t *testing.T) {
	const N = 72 * 1000
	counts := make([]int, bookLen)
	for i := uint64(0); i < N; i++ {
		counts[rowOf(i)]++
	}
	for r, c := range counts {
		if c < N/bookLen/2 || c > N/bookLen*2 {
			t.Errorf("rowOf distribution uneven at row %d: count=%d (expected ~%d)", r, c, N/bookLen)
		}
	}
}

// rowOf 对相邻值应落在不同行（雪崩效应粗检）。
func TestRowOfAvalanche(t *testing.T) {
	sameRow := 0
	const N = 10000
	for i := uint64(0); i < N; i++ {
		if rowOf(i) == rowOf(i+1) {
			sameRow++
		}
	}
	// 相邻值落同行的概率期望约 1/72 ≈ 1.4%，放宽到 5%
	if sameRow > N/20 {
		t.Errorf("rowOf has poor avalanche: %d/%d adjacent pairs share the same row", sameRow, N)
	}
}

// ── 索引字符位置校验 ──────────────────────────────────────────────────────────

// 对于任意 val，索引字符必须出现在位置 row%13，且确实属于索引行。
func TestIndexCharPosition(t *testing.T) {
	newTestBook(t)

	for _, v := range []uint64{0, 1, 42, math.MaxUint32, math.MaxUint64} {
		row := rowOf(v)
		pos := row % strLen
		s := Encode(v)
		bs := []byte(s)

		expectedIndexChar := _book.table[bookLen][row]
		if bs[pos] != expectedIndexChar {
			t.Errorf("val=%d: index char at pos %d = %q, want %q",
				v, pos, bs[pos], expectedIndexChar)
		}
	}
}

// ── Benchmark ─────────────────────────────────────────────────────────────────

func BenchmarkEncode(b *testing.B) {
	newTestBook(b)
	for b.Loop() {
		Encode(math.MaxUint64)
	}
}

func BenchmarkDecode(b *testing.B) {
	newTestBook(b)
	s := Encode(math.MaxUint64)
	for b.Loop() {
		Decode(s) //nolint:errcheck
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	newTestBook(b)
	for b.Loop() {
		v := uint64(bits.RotateLeft64(math.MaxUint64, b.N%64))
		s := Encode(v)
		Decode(s) //nolint:errcheck
	}
}
