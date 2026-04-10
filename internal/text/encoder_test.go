package text

import (
	"math"
	"math/bits"
	"strings"
	"testing"

	"github.com/yzhlove/pedis/internal/config"
)

func newTestEncoder(tb testing.TB) {
	tb.Helper()
	New(&config.Config{
		TimeSeed:     "pedis-test-seed",
		CharacterSet: "",
	}).(*encoder).Apply() //nolint:errcheck
}

// ── 基础正确性 ──────────────────────────────────────────────────────────────

func TestEncodeDecodeRoundTrip(t *testing.T) {
	newTestEncoder(t)

	cases := []uint64{
		0, 1, 71, 72, 255, 1000,
		math.MaxUint32, math.MaxUint32 + 1,
		math.MaxInt64, math.MaxUint64 - 1, math.MaxUint64,
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
	newTestEncoder(t)
	for _, v := range []uint64{0, 1, math.MaxUint64} {
		s := Encode(v)
		if len(s) != strLen {
			t.Errorf("Encode(%d) length = %d, want %d", v, len(s), strLen)
		}
	}
}

func TestEncodeDeterministic(t *testing.T) {
	newTestEncoder(t)
	for _, v := range []uint64{0, 42, math.MaxUint64} {
		a, b := Encode(v), Encode(v)
		if a != b {
			t.Errorf("Encode(%d) not deterministic: %q vs %q", v, a, b)
		}
	}
}

func TestEncodeDistinct(t *testing.T) {
	newTestEncoder(t)
	seen := make(map[string]uint64, 1000)
	for i := uint64(0); i < 1000; i++ {
		s := Encode(i)
		if prev, ok := seen[s]; ok {
			t.Fatalf("collision: Encode(%d) == Encode(%d) == %q", i, prev, s)
		}
		seen[s] = i
	}
}

// ── 解码拒绝非法输入 ─────────────────────────────────────────────────────────

func TestDecodeWrongLength(t *testing.T) {
	newTestEncoder(t)
	cases := []string{"", "a", strings.Repeat("a", strLen-1), strings.Repeat("a", strLen+1)}
	for _, s := range cases {
		if _, err := Decode(s); err != ErrInvalidLength {
			t.Errorf("Decode(%q) error = %v, want ErrInvalidLength", s, err)
		}
	}
}

func TestDecodeTamperedSingleBit(t *testing.T) {
	newTestEncoder(t)
	for _, v := range []uint64{0, 42, math.MaxUint64} {
		original := Encode(v)
		bs := []byte(original)
		for pos := range len(bs) {
			for bit := uint(0); bit < 8; bit++ {
				tampered := make([]byte, len(bs))
				copy(tampered, bs)
				tampered[pos] ^= 1 << bit
				if string(tampered) == original {
					continue
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

func TestDecodeAllZeroString(t *testing.T) {
	newTestEncoder(t)
	s := strings.Repeat("\x00", strLen)
	_, err := Decode(s)
	if err == nil {
		t.Error("all-zero string should not decode successfully")
	}
}

func TestDecodeRandomString(t *testing.T) {
	newTestEncoder(t)
	s := strings.Repeat("~", strLen)
	_, err := Decode(s)
	if err == nil {
		t.Error("random string should not decode successfully")
	}
}

// ── 边界值 ────────────────────────────────────────────────────────────────────

func TestEncodeDecodeBoundary(t *testing.T) {
	newTestEncoder(t)
	boundaries := []uint64{
		0, 1, 71, 72, 72*72 - 1,
		math.MaxUint32, math.MaxUint32 + 1,
		1<<63 - 1, 1 << 63,
		math.MaxUint64 - 1, math.MaxUint64,
	}
	for _, v := range boundaries {
		s := Encode(v)
		got, err := Decode(s)
		if err != nil || got != v {
			t.Errorf("boundary %d: encode=%q decode=%d err=%v", v, s, got, err)
		}
	}
}

// ── 种子隔离性 ─────────────────────────────────────────────────────────────────

func TestDifferentSeedsDifferentEncoding(t *testing.T) {
	val := uint64(12345)

	New(&config.Config{TimeSeed: "seed-A"}).(*encoder).Apply() //nolint:errcheck
	encA := Encode(val)

	New(&config.Config{TimeSeed: "seed-B"}).(*encoder).Apply() //nolint:errcheck
	encB := Encode(val)

	if encA == encB {
		t.Errorf("different seeds produced same encoding: %q", encA)
	}

	New(&config.Config{TimeSeed: "seed-A"}).(*encoder).Apply() //nolint:errcheck
	if got, err := Decode(encA); err != nil || got != val {
		t.Errorf("seed-A decode failed: got=%d err=%v", got, err)
	}

	New(&config.Config{TimeSeed: "seed-B"}).(*encoder).Apply() //nolint:errcheck
	if got, err := Decode(encB); err != nil || got != val {
		t.Errorf("seed-B decode failed: got=%d err=%v", got, err)
	}
}

func TestCrossDecodeFails(t *testing.T) {
	val := uint64(99999)
	New(&config.Config{TimeSeed: "seed-X"}).(*encoder).Apply() //nolint:errcheck
	encX := Encode(val)

	New(&config.Config{TimeSeed: "seed-Y"}).(*encoder).Apply() //nolint:errcheck
	got, err := Decode(encX)
	if err == nil && got == val {
		t.Error("cross-seed decode should fail")
	}
}

// ── 自定义字符集 ──────────────────────────────────────────────────────────────

func TestCustomCharacterSet(t *testing.T) {
	var buf [bookLen]byte
	for i := range buf {
		buf[i] = byte('!' + i)
	}
	New(&config.Config{TimeSeed: "custom-seed", CharacterSet: string(buf[:])}).(*encoder).Apply() //nolint:errcheck

	for _, v := range []uint64{0, 42, math.MaxUint32, math.MaxUint64} {
		s := Encode(v)
		got, err := Decode(s)
		if err != nil || got != v {
			t.Errorf("custom charset: val=%d enc=%q got=%d err=%v", v, s, got, err)
		}
	}
}

func TestInsufficientCharacterSetFallback(t *testing.T) {
	New(&config.Config{TimeSeed: "fallback-seed", CharacterSet: "abc"}).(*encoder).Apply() //nolint:errcheck
	v := uint64(42)
	s := Encode(v)
	got, err := Decode(s)
	if err != nil || got != v {
		t.Errorf("fallback charset: val=%d enc=%q got=%d err=%v", v, s, got, err)
	}
}

// ── rowOf 分布 ─────────────────────────────────────────────────────────────────

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

func TestRowOfAvalanche(t *testing.T) {
	sameRow := 0
	const N = 10000
	for i := uint64(0); i < N; i++ {
		if rowOf(i) == rowOf(i+1) {
			sameRow++
		}
	}
	if sameRow > N/20 {
		t.Errorf("rowOf has poor avalanche: %d/%d adjacent pairs share the same row", sameRow, N)
	}
}

// ── Benchmark ─────────────────────────────────────────────────────────────────

func BenchmarkEncode(b *testing.B) {
	newTestEncoder(b)
	for b.Loop() {
		Encode(math.MaxUint64)
	}
}

func BenchmarkDecode(b *testing.B) {
	newTestEncoder(b)
	s := Encode(math.MaxUint64)
	for b.Loop() {
		Decode(s) //nolint:errcheck
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	newTestEncoder(b)
	for b.Loop() {
		v := uint64(bits.RotateLeft64(math.MaxUint64, b.N%64))
		s := Encode(v)
		Decode(s) //nolint:errcheck
	}
}
