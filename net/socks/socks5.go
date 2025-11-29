package socks

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"

	koIo "github.com/kinabcd/ko/io"
)

func readSOCKS5Header(conn net.Conn) (methods []AuthMethod, err error) {
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
	} else if methodBytes, err := koIo.ReadN(conn, int(size)); err != nil {
		return nil, err
	} else {
		methods = make([]AuthMethod, len(methodBytes))
		for i, b := range methodBytes {
			methods[i] = AuthMethod(b)
		}
		return methods, nil
	}
}

func writeSOCKS5Header(conn net.Conn, methods []AuthMethod) error {
	if len(methods) > 255 {
		return errors.New("too many authentication methods")
	}
	b := make([]byte, 2+len(methods))
	b[0] = Version5
	b[1] = byte(len(methods))

	for i, am := range methods {
		b[i+2] = byte(am)
	}
	_, err := conn.Write(b)
	return err
}
func readSOCKS5AuthMethod(conn net.Conn) (AuthMethod, error) {
	/**
	  +-----+--------+
	  | VER | METHOD |
	  +-----+--------+
	  | '5' |   1    |
	  +-----+--------+
	*/
	if b, err := koIo.ReadN(conn, 2); err != nil {
		return AuthMethodNoAcceptableMethods, err
	} else if b[0] != Version5 {
		return AuthMethodNoAcceptableMethods, errors.New("unexpected protocol version " + strconv.Itoa(int(b[0])))
	} else {
		return AuthMethod(b[1]), nil
	}
}

func writeSOCKS5AuthMethod(conn net.Conn, method AuthMethod) (err error) {
	_, err = conn.Write([]byte{Version5, byte(method)})
	return
}

func readSOCKS5AuthUsernamePassword(conn net.Conn) (username string, password string, err error) {
	/**
	  +-----+--------------+--------------+
	  | VER |   USERNAME   |   PASSWORD   |
	  +-----+--------------+--------------+
	  | '1' | PascalString | PascalString |
	  +-----+--------------+--------------+
	*/
	var ver byte
	if ver, err = koIo.ReadByte(conn); err != nil {
		err = fmt.Errorf("read ver failed: %w", err)
	} else if ver != AuthUsernamePasswordVersion {
		err = ErrWrongProtocol
	} else if username, err = koIo.ReadPascalString(conn); err != nil {
		err = fmt.Errorf("read account failed: %w", err)
	} else if password, err = koIo.ReadPascalString(conn); err != nil {
		err = fmt.Errorf("read password failed: %w", err)
	}
	return
}
func writeSOCKS5AuthUsernamePassword(conn net.Conn, username string, password string) error {
	if len(username) == 0 || len(username) > 255 || len(password) > 255 {
		return errors.New("invalid username/password")
	}
	buffer := bytes.NewBuffer(make([]byte, 0, 3+len(username)+len(password)))
	buffer.WriteByte(AuthUsernamePasswordVersion)
	koIo.WritePascalString(buffer, username)
	koIo.WritePascalString(buffer, password)
	_, err := conn.Write(buffer.Bytes())
	return err
}
func readSOCKS5AuthResult(conn net.Conn) error {
	/**
	  +-----+---------+
	  | VER | SUCCESS |
	  +-----+---------+
	  | '1' |    1    |
	  +-----+---------+
	*/
	if b, err := koIo.ReadN(conn, 2); err != nil {
		return err
	} else if Reply(b[0]) != AuthUsernamePasswordVersion {
		return errors.New("invalid username/password version")
	} else if Reply(b[1]) != StatusSucceeded {
		return ErrAuthFailed
	} else {
		return nil
	}
}
func writeSOCKS5AuthResult(conn net.Conn, ok bool) (err error) {
	status := StatusSucceeded
	if !ok {
		status = StatusFailed
	}
	_, err = conn.Write([]byte{0x01, byte(status)})
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
	if bs, err = koIo.ReadN(conn, 3); err != nil {
		err = fmt.Errorf("read header failed: %w", err)
		return
	}
	cmd := Command(bs[1])
	if cmd != CmdConnect {
		err = ErrCmdNotSupported
		return
	}
	return readSOCKS5Addr(conn)
}

func writeSOCKS5Request(conn net.Conn, cmd Command, address string) error {
	host, port, err := splitHostPort(address)
	if err != nil {
		return err
	}

	buffer := bytes.NewBuffer(make([]byte, 0, 6+len(host)))
	buffer.Write([]byte{Version5, byte(cmd), 0})
	if err := writeSOCKS5Addr(buffer, host, port); err != nil {
		return err
	}
	_, err = conn.Write(buffer.Bytes())
	return err
}
func readSOCKS5Response(conn net.Conn) (string, error) {
	/**
	  +-----+--------+-----+----------+----------+----------+
	  | VER | STATUS | RSV | BND.TYPE | BND.ADDR | BND.PORT |
	  +-----+--------+-----+----------+----------+----------+
	  | '5' |   1    | '0' |    1     | Variable |    2     |
	  +-----+--------+-----+----------+----------+----------+
	*/

	if b, err := koIo.ReadN(conn, 3); err != nil {
		return "", err
	} else if b[0] != Version5 {
		return "", errors.New("unexpected protocol version " + strconv.Itoa(int(b[0])))
	} else if cmdErr := Reply(b[1]); cmdErr != StatusSucceeded {
		return "", errors.New("unknown error " + cmdErr.String())
	} else if b[2] != 0 {
		return "", errors.New("non-zero reserved field")
	}

	address, err := readSOCKS5Addr(conn)
	if err != nil {
		return "", err
	}
	return address, nil
}
func writeSOCKS5Response(conn net.Conn, status Reply, address string) error {
	host, port, err := splitHostPort(address)
	if err != nil {
		return err
	}

	buffer := bytes.NewBuffer(make([]byte, 0, 6+len(host)))
	buffer.Write([]byte{byte(Version5), byte(status), 0})
	err = writeSOCKS5Addr(buffer, host, port)
	if err != nil {
		return err
	}
	_, err = conn.Write(buffer.Bytes())
	return err
}

func readSOCKS5Addr(conn net.Conn) (address string, err error) {
	/**
	+----------+----------+----------+
	| BND.TYPE | BND.ADDR | BND.PORT |
	+----------+----------+----------+
	|    1     | Variable |    2     |
	+----------+----------+----------+
	*/
	var addressType byte
	if addressType, err = koIo.ReadByte(conn); err != nil {
		return
	}
	var bs []byte
	var host string
	switch addressType {
	case AddrTypeIPv4:
		bs, err = koIo.ReadN(conn, net.IPv4len)
	case AddrTypeIPv6:
		bs, err = koIo.ReadN(conn, net.IPv6len)
	case AddrTypeFQDN:
		host, err = koIo.ReadPascalString(conn)
	default:
		err = ErrUnknownAddressType
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

func writeSOCKS5Addr(w *bytes.Buffer, host string, port int) error {
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			w.WriteByte(AddrTypeIPv4)
			w.Write(ip4)
		} else if ip6 := ip.To16(); ip6 != nil {
			w.WriteByte(AddrTypeIPv6)
			w.Write(ip6)
		} else {
			return ErrUnknownAddressType
		}
	} else {
		if len(host) > 255 {
			return ErrFQDNTooLong
		}
		w.WriteByte(AddrTypeFQDN)
		koIo.WritePascalString(w, host)
	}
	w.Write([]byte{byte(port >> 8), byte(port)})
	return nil
}

func splitHostPort(address string) (string, int, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	portnum, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, err
	}
	if 1 > portnum || portnum > 0xffff {
		return "", 0, errors.New("port number out of range " + port)
	}
	return host, portnum, nil
}
