package resp

type Type byte

const (
	TypeSimpleString Type = '+'
	TypeError        Type = '-'
	TypeInteger      Type = ':'
	TypeBulkString   Type = '$'
	TypeArray        Type = '*'
	TypeNull         Type = '_'
)

type Value struct {
	Typ   Type
	Str   string
	Num   int64
	Array []Value
}

func SimpleString(s string) Value {
	return Value{Typ: TypeSimpleString, Str: s}
}

func Error(s string) Value {
	return Value{Typ: TypeError, Str: s}
}

func Integer(n int64) Value {
	return Value{Typ: TypeInteger, Num: n}
}

func BulkString(s string) Value {
	return Value{Typ: TypeBulkString, Str: s}
}

func Null() Value {
	return Value{Typ: TypeNull}
}

func Array(vals []Value) Value {
	return Value{Typ: TypeArray, Array: vals}
}
