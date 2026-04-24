package text

import (
	"errors"
	"hash/fnv"
	"math/rand/v2"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/module"
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
	bookLen    = 72
	bookRows   = bookLen + 1
	dataDigits = 11
	strLen     = dataDigits + 2
)

var (
	errInvalidString = errors.New("text: invalid encoded string")
	errInvalidLength = errors.New("text: encoded string must be 13 characters")
	errTextCharacter = errors.New("text: invalid text character")
)

type encoder struct {
	cfg     *config.Config
	table   [bookRows][bookLen]byte
	inverse [bookRows][256]int8
}

var defaultBook *encoder

func New(cfg *config.Config) module.Module {
	e := &encoder{cfg: cfg}
	return e
}

func (e *encoder) Apply() error {
	if err := e.build(e.cfg); err != nil {
		return err
	}
	defaultBook = e
	return nil
}

func (e *encoder) build(cfg *config.Config) error {
	if len(cfg.CharacterSet) != bookLen {
		return errTextCharacter
	}

	h := fnv.New64a()
	h.Write([]byte(cfg.TimeSeed))
	seed := h.Sum64()
	rng := rand.New(rand.NewPCG(seed, seed^config.PCGStreamSeed))

	for i := range e.table {
		copy(e.table[i][:], cfg.CharacterSet)
		for j := bookLen - 1; j > 0; j-- {
			k := rng.IntN(j + 1)
			e.table[i][j], e.table[i][k] = e.table[i][k], e.table[i][j]
		}
	}

	for i := range e.inverse {
		for j := range e.inverse[i] {
			e.inverse[i][j] = -1
		}
		for j := range bookLen {
			e.inverse[i][e.table[i][j]] = int8(j)
		}
	}
	return nil
}

func rowOf(val uint64) int {
	h := val
	h ^= h >> 33
	h *= config.MurmurHash3Mix1
	h ^= h >> 33
	h *= config.MurmurHash3Mix2
	h ^= h >> 33
	return int(h % uint64(bookLen))
}

// Encode encodes a uint64 to a 13-character string.
func Encode(val uint64) string { return defaultBook.encode(val) }

func (e *encoder) encode(val uint64) string {
	row := rowOf(val)

	var digits [dataDigits]byte
	v := val
	for i := dataDigits - 1; i >= 0; i-- {
		digits[i] = byte(v % uint64(bookLen))
		v /= uint64(bookLen)
	}

	var chk byte
	for _, d := range digits {
		chk ^= d
	}
	chk %= bookLen

	var encoded [dataDigits + 1]byte
	for i, d := range digits {
		encoded[i] = e.table[row][d]
	}
	encoded[dataDigits] = e.table[row][chk]

	indexChar := e.table[bookLen][row]
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

// Decode decodes a 13-character string back to a uint64.
func Decode(s string) (uint64, error) { return defaultBook.decode(s) }

func (e *encoder) decode(s string) (uint64, error) {
	if len(s) != strLen {
		return 0, errInvalidLength
	}
	bs := []byte(s)

	for r := range bookLen {
		pos := r % strLen
		if bs[pos] != e.table[bookLen][r] {
			continue
		}
		if val, err := e.tryRow(bs, r, pos); err == nil {
			return val, nil
		}
	}
	return 0, errInvalidString
}

func (e *encoder) tryRow(bs []byte, row, indexPos int) (uint64, error) {
	var encoded [dataDigits + 1]byte
	j := 0
	for i, c := range bs {
		if i != indexPos {
			encoded[j] = c
			j++
		}
	}

	var digits [dataDigits]byte
	for i := range dataDigits {
		v := e.inverse[row][encoded[i]]
		if v < 0 {
			return 0, errInvalidString
		}
		digits[i] = byte(v)
	}

	chkIdx := e.inverse[row][encoded[dataDigits]]
	if chkIdx < 0 {
		return 0, errInvalidString
	}
	var chk byte
	for _, d := range digits {
		chk ^= d
	}
	chk %= bookLen
	if chk != byte(chkIdx) {
		return 0, errInvalidString
	}

	const base = uint64(bookLen)
	var val uint64
	for _, d := range digits {
		if val > (^uint64(0))/base {
			return 0, errInvalidString
		}
		next := val * base
		if next > (^uint64(0))-uint64(d) {
			return 0, errInvalidString
		}
		val = next + uint64(d)
	}

	if rowOf(val) != row {
		return 0, errInvalidString
	}
	return val, nil
}
