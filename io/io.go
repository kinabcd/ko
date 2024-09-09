package io

import (
	"io"
)

// Bind establishes a bidirectional data transfer between two connections.
// Two connections will be closed if anyone is closed.
func BidirectionalCopy(conn1, conn2 io.ReadWriteCloser) {
	go func() {
		io.Copy(conn1, conn2)
		conn1.Close()
		conn2.Close()
	}()
	io.Copy(conn2, conn1)
	conn2.Close()
	conn1.Close()
}

// ReadByte reads and returns the next byte from the Reader or any error encountered.
// If ReadByte returns an error, no input byte was consumed, and the returned byte value is undefined.
func ReadByte(r io.Reader) (byte, error) {
	if bytes, err := ReadN(r, 1); err == nil {
		return bytes[0], nil
	} else {
		return byte(0), err
	}
}

// ReadN reads and returns the next N bytes from the Reader or any error encountered.
// It returns bytes copied and an error if fewer bytes were read.
// The error is EOF only if no bytes were read.
// If an EOF happens after reading some but not all the bytes, ReadN returns ErrUnexpectedEOF.
func ReadN(r io.Reader, n int) (bs []byte, err error) {
	bs = make([]byte, n)
	var readN int
	readN, err = io.ReadFull(r, bs)
	bs = bs[:readN]
	return
}

// ReadPascalString reads one byte N as size and N bytes from Reader.
func ReadPascalString(r io.Reader) (str string, err error) {
	/**
	  +-----+--------+
	  | LEN | STRING |
	  +-----+--------+
	  |  1  | 1~255  |
	  +-----+--------+
	*/
	var len byte
	var bs []byte
	if len, err = ReadByte(r); err != nil {
	} else if bs, err = ReadN(r, int(uint(len))); err != nil {
	} else {
		str = string(bs)
	}
	return
}

// ReadCString reads bytes from Reader util '\0'.
// It returns string without '\0'
func ReadCString(r io.Reader) (str string, err error) {
	/**
	  +--------+------+
	  | STRING | NULL |
	  +--------+------+
	  |  VAR   |  1   |
	  +--------+------+
	*/
	bs := make([]byte, 0)
	var b byte
	for {
		if b, err = ReadByte(r); err != nil {
			return
		} else if b == 0 {
			break
		} else {
			bs = append(bs, b)
		}
	}
	return string(bs), nil
}

// WriteCString writes bytes to Writer and append a '\0'.
func WriteCString(w io.Writer, str string) (err error) {
	if _, err = w.Write([]byte(str)); err != nil {
		return
	}
	_, err = w.Write([]byte{0})
	return
}
