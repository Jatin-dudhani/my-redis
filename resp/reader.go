package resp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
)

type Reader struct {
	rd *bufio.Reader
}

var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 1024)
		return b
	},
}

func NewReader(rd io.Reader) *Reader {
	return &Reader{rd: bufio.NewReaderSize(rd, 65536)}
}

func NewReaderSize(rd io.Reader, size int) *Reader {
	return &Reader{rd: bufio.NewReaderSize(rd, size)}
}

func (r *Reader) Read() (Value, error) {
	b, err := r.rd.ReadByte()
	if err != nil {
		return Value{}, err
	}
	switch b {
	case '+':
		return r.readSimpleString()
	case '-':
		return r.readError()
	case ':':
		return r.readInteger()
	case '$':
		return r.readBulkString()
	case '*':
		return r.readArray()
	default:
		return Value{}, fmt.Errorf("unknown type byte: %c", b)
	}
}

func (r *Reader) readLine() (string, error) {
	line, err := r.rd.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", errors.New("missing CRLF")
	}
	return line[:len(line)-2], nil
}

func (r *Reader) readSimpleString() (Value, error) {
	s, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	return Value{Typ: TypeSimpleString, Str: s}, nil
}

func (r *Reader) readError() (Value, error) {
	s, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	return Value{Typ: TypeError, Str: s}, nil
}

func (r *Reader) readInteger() (Value, error) {
	s, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid integer: %s", s)
	}
	return Value{Typ: TypeInteger, Num: n}, nil
}

func (r *Reader) readBulkString() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	if line == "-1" {
		return Value{Typ: TypeNull}, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil {
		return Value{}, fmt.Errorf("invalid bulk string length: %s", line)
	}
	buf := bufPool.Get().([]byte)
	if cap(buf) < n {
		buf = make([]byte, n)
	}
	buf = buf[:n]
	_, err = io.ReadFull(r.rd, buf)
	if err != nil {
		bufPool.Put(buf[:0])
		return Value{}, err
	}
	_, err = r.readLine()
	if err != nil {
		bufPool.Put(buf[:0])
		return Value{}, err
	}
	s := string(buf)
	bufPool.Put(buf[:0])
	return Value{Typ: TypeBulkString, Str: s}, nil
}

func (r *Reader) readArray() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	if line == "-1" {
		return Value{Typ: TypeNull}, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil {
		return Value{}, fmt.Errorf("invalid array length: %s", line)
	}
	arr := make([]Value, n)
	for i := 0; i < n; i++ {
		v, err := r.Read()
		if err != nil {
			return Value{}, err
		}
		arr[i] = v
	}
	return Value{Typ: TypeArray, Array: arr}, nil
}
