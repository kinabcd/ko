package socks

import (
	"encoding/binary"
	"net"
	"slices"
	"strconv"

	koIo "github.com/kinabcd/ko/io"
)

func readSOCKS4Header(conn net.Conn) (address string, portByte [2]byte, ipByte [4]byte, err error) {
	/** SOCKS4
	  +----+----+---------+-------+--------+------+
	  | VN | CD | DSTPORT | DSTIP | USERID | NULL |
	  +----+----+---------+-------+--------+------+
	  | 1  |  1 |    2    |   4   |  VAR   |  1   |
	  +----+----+---------+-------+--------+------+
	*/
	/** SOCKS4a
	  +----+----+---------+-------+--------+------+----------+------+
	  | VN | CD | DSTPORT | DSTIP | USERID | NULL | HOSTNAME | NULL |
	  +----+----+---------+-------+--------+------+----------+------+
	  | 1  |  1 |    2    |   4   |  VAR   |  1   |   VAR    |  1   |
	  +----+----+---------+-------+--------+------+----------+------+
	*/
	var bs []byte
	if bs, err = koIo.ReadN(conn, 7); err != nil {
		return
	}

	// CD
	if bs[0] != 0x01 {
		err = ErrCmdNotSupported
		return
	}
	// DSTPORT
	portByte = [2]byte(bs[1:3])
	port := strconv.Itoa(int(binary.BigEndian.Uint16(portByte[:])))
	// DSTIP
	ipByte = [4]byte(bs[3:7])

	// USERID: ignored
	if _, err = koIo.ReadCString(conn); err != nil {
		return
	}
	var host string
	if slices.Equal(ipByte[0:3], []byte{0, 0, 0}) {
		// HOSTNAME
		if host, err = koIo.ReadCString(conn); err != nil {
			return
		}
	} else {
		host = net.IP(ipByte[0:4]).String()
	}
	address = net.JoinHostPort(host, port)
	return

}

func writeSOCKS4Response(conn net.Conn, ok bool, portByte [2]byte, ipByte [4]byte) (err error) {
	/**
	  +-----+----+------+----+
	  | VN  | CD | PORT | IP |
	  +-----+----+------+----+
	  | '0' | 1  |  2   | 4  |
	  +-----+----+------+----+
	*/
	var status byte = 90
	if !ok {
		status = 91
	}

	_, err = conn.Write([]byte{0x00, status, portByte[0], portByte[1], ipByte[0], ipByte[1], ipByte[2], ipByte[3]})
	return
}
