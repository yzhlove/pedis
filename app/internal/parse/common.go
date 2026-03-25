package parse

import "sync"

var (
	Sep = "\r\n"
	CR  = byte('\r')
	LF  = byte('\n')
)

type Object interface {
	ToBytes() []byte
	Type() Type
}

type Type int

const (
	StatusType Type = iota
	ErrorType
	IntegerType
	BulkType
	ArrBulkType
)

var statusPool = sync.Pool{New: func() any { return &Status{} }}
var errorPool = sync.Pool{New: func() any { return &Error{} }}
var integerPool = sync.Pool{New: func() any { return &Integer{} }}
var bulkPool = sync.Pool{New: func() any { return &Bulk{} }}
var arrBulkPool = sync.Pool{New: func() any { return &ArrBulk{} }}

func GetStatus() *Status   { return statusPool.Get().(*Status) }
func FreeStatus(s *Status) { s.value = ""; statusPool.Put(s) }

func GetError() *Error   { return errorPool.Get().(*Error) }
func FreeError(e *Error) { e.value = ""; errorPool.Put(e) }

func GetInteger() *Integer   { return integerPool.Get().(*Integer) }
func FreeInteger(i *Integer) { i.value = 0; integerPool.Put(i) }

func GetBulk() *Bulk   { return bulkPool.Get().(*Bulk) }
func FreeBulk(b *Bulk) { b.value = ""; bulkPool.Put(b) }

func GetArrBulk() *ArrBulk   { return arrBulkPool.Get().(*ArrBulk) }
func FreeArrBulk(a *ArrBulk) { a.value = a.value[:0]; arrBulkPool.Put(a) }
