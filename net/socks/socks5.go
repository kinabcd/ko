package socks

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"

	koIo "github.com/kinabcd/ko/io"
)

func readSOCKS5Header(conn net.Conn) (method []byte, err error) {
	/**
	  +-----+------+-----------+
	  | VER | SIZE |  METHODS  |
	  +-----+------+-----------+
	  | '5' |  1   |   1~255   |
	  +-----+------+-----------+
	*/
	var size byte
	if size, err = koIo.ReadByte(conn); err != nil {
		return nil, ErrWrongProtocol
	} else {
		return koIo.ReadN(conn, int(size))
	}
}

func writeSOCKS5AuthMethod(conn net.Conn, method byte) (err error) {
	/**
	  +-----+--------+
	  | VER | METHOD |
	  +-----+--------+
	  | '5' |   1    |
	  +-----+--------+
	*/
	_, err = conn.Write([]byte{Version5, method})
	return
}

func readSOCKS5AuthUsernamePassword(conn net.Conn) (account string, password string, err error) {
	/**
	  +-----+--------------+--------------+
	  | VER |    ACCOUNT   |   PASSWORD   |
	  +-----+--------------+--------------+
	  | '1' | PascalString | PascalString |
	  +-----+--------------+--------------+
	*/
	var ver byte
	if ver, err = koIo.ReadByte(conn); err != nil {
		err = fmt.Errorf("read ver failed: %w", err)
	} else if ver != 0x01 {
		err = ErrWrongProtocol
	} else if account, err = koIo.ReadPascalString(conn); err != nil {
		err = fmt.Errorf("read account failed: %w", err)
	} else if password, err = koIo.ReadPascalString(conn); err != nil {
		err = fmt.Errorf("read password failed: %w", err)
	}
	return
}
func writeSOCKS5AuthResult(conn net.Conn, ok bool) (err error) {
	/**
	  +-----+---------+
	  | VER | SUCCESS |
	  +-----+---------+
	  | '1' |    1    |
	  +-----+---------+
	*/
	status := StatusSucceeded
	if !ok {
		status = StatusFailed
	}
	_, err = conn.Write([]byte{0x01, status})
	return
}

func readSOCKS5Request(conn net.Conn) (address string, err error) {
	/**
	  +-----+-----+-----+------+----------+----------+
	  | VER | CMD | RSV | ATYP | DST.ADDR | DST.PORT |
	  +-----+-----+-----+------+----------+----------+
	  | '5' |  1  | '0' |  1   |    VAR   |    2     |
	  +-----+-----+-----+------+----------+----------+
	*/
	var bs []byte
	if bs, err = koIo.ReadN(conn, 4); err != nil {
		err = fmt.Errorf("read header failed: %w", err)
		return
	}
	cmd := bs[1]
	addressType := bs[3]
	if cmd != CmdConnect {
		err = ErrCmdNotSupported
		return
	}

	var host string
	if addressType == AddrTypeIPv4 {
		bs, err = koIo.ReadN(conn, 4)
	} else if addressType == AddrTypeIPv6 {
		bs, err = koIo.ReadN(conn, 16)
	} else if addressType == AddrTypeFQDN {
		host, err = koIo.ReadPascalString(conn)
	} else {
		err = ErrWrongFormat
	}
	if err != nil {
		err = fmt.Errorf("read address failed: %w", err)
		return
	}
	if addressType == AddrTypeIPv4 || addressType == AddrTypeIPv6 {
		host = net.IP(bs).String()
	}
	if host == "" {
		err = ErrWrongFormat
		return
	}

	var port string
	if bs, err = koIo.ReadN(conn, 2); err != nil {
		err = fmt.Errorf("read port failed: %w", err)
	} else {
		port = strconv.Itoa(int(binary.BigEndian.Uint16(bs[:])))
	}
	address = net.JoinHostPort(host, port)
	return
}

func writeSOCKS5Response(conn net.Conn, status byte) error {
	/**
	  +-----+--------+-----+----------+----------+----------+
	  | VER | STATUS | RSV | BND.TYPE | BND.ADDR | BND.PORT |
	  +-----+--------+-----+----------+----------+----------+
	  | '5' |   1    | '0' |    1     | Variable |    2     |
	  +-----+--------+-----+----------+----------+----------+
	*/
	_, err := conn.Write([]byte{Version5, status, 0x00, AddrTypeIPv4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	return err
}
