package resp

import (
	"fmt"
	"io"
	"strconv"
)

type Writer struct {
	w io.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (wr *Writer) Write(v Value) error {
	switch v.Typ {
	case TypeSimpleString:
		return wr.writeSimpleString(v)
	case TypeError:
		return wr.writeError(v)
	case TypeInteger:
		return wr.writeInteger(v)
	case TypeBulkString:
		return wr.writeBulkString(v)
	case TypeNull:
		return wr.writeNull()
	case TypeArray:
		return wr.writeArray(v)
	default:
		return fmt.Errorf("unknown type: %c", v.Typ)
	}
}

func (wr *Writer) writeSimpleString(v Value) error {
	_, err := fmt.Fprintf(wr.w, "+%s\r\n", v.Str)
	return err
}

func (wr *Writer) writeError(v Value) error {
	_, err := fmt.Fprintf(wr.w, "-%s\r\n", v.Str)
	return err
}

func (wr *Writer) writeInteger(v Value) error {
	_, err := fmt.Fprintf(wr.w, ":%s\r\n", strconv.FormatInt(v.Num, 10))
	return err
}

func (wr *Writer) writeBulkString(v Value) error {
	_, err := fmt.Fprintf(wr.w, "$%d\r\n%s\r\n", len(v.Str), v.Str)
	return err
}

func (wr *Writer) writeNull() error {
	_, err := fmt.Fprintf(wr.w, "$-1\r\n")
	return err
}

func (wr *Writer) writeArray(v Value) error {
	_, err := fmt.Fprintf(wr.w, "*%d\r\n", len(v.Array))
	if err != nil {
		return err
	}
	for _, elem := range v.Array {
		if err := wr.Write(elem); err != nil {
			return err
		}
	}
	return nil
}
