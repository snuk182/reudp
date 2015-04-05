package reudp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/snuk182/go-multierror"
)

const (
	_MARKER      = 0xdefcabbe
	_HEADER_SIZE = 24
	_PACKET_SIZE = 576
	_DATA_SIZE   = _PACKET_SIZE - _HEADER_SIZE

	_FLAG_ACK = 1

	MsgData = iota
	MsgAck
)

type Reudp struct {
	writeSync, readCacheSync, writeCacheSync sync.Mutex

	conn                    *net.UDPConn
	writebuffer, readbuffer [_PACKET_SIZE]byte

	writecache, readcache map[uint64][][]byte

	errfunc  func(who *net.UDPAddr, err error)
	datafunc func(who *net.UDPAddr, data []byte)
	logfunc  func(format string, args ...interface{})
}

func New(datafunc func(who *net.UDPAddr, data []byte), errfunc func(who *net.UDPAddr, err error), logfunc func(format string, args ...interface{})) *Reudp {
	return &Reudp{
		errfunc:  errfunc,
		datafunc: datafunc,
		logfunc:  logfunc,
	}
}

func (this *Reudp) IsRunning() bool {
	return this.conn != nil
}

func (this *Reudp) Close() error {
	if this.IsRunning() {
		err := this.conn.Close()
		this.conn = nil
		return err
	} else {
		return nil
	}
}

func (this *Reudp) LocalAddr() *net.UDPAddr {
	return this.conn.LocalAddr().(*net.UDPAddr)
}

func (this *Reudp) Listen(addr *net.UDPAddr) (err error) {
	this.conn, err = net.ListenUDP("udp", addr)

	this.readcache, this.writecache = make(map[uint64][][]byte), make(map[uint64][][]byte)

	if err == nil {
		if this.logfunc != nil {
			this.logfunc("reudp: listening to %v", this.LocalAddr())
		}

		go this.loop()
	} else {
		this.conn = nil
	}

	return err
}

func (this *Reudp) loop() {
	for this.IsRunning() {
		got, who, err := this.conn.ReadFromUDP(this.readbuffer[:])

		if this.logfunc != nil {
			this.logfunc("reudp: got %d bytes from %v, err %v", got, who, err)
			this.logfunc("reudp: data %v", this.readbuffer[:got])
		}

		if err != nil {
			this.errfunc(who, err)
		} else {
			this.process(got, who)
		}
	}
}

func (this *Reudp) process(length int, who *net.UDPAddr) {
	if length < 24 {
		this.errfunc(who, errors.New(fmt.Sprintf("Broken packet: %v", this.readbuffer[:length])))
		return
	}

	if binary.BigEndian.Uint32(this.readbuffer[:4]) != _MARKER {
		this.errfunc(who, errors.New(fmt.Sprintf("Not my packet: %v", this.readbuffer[:4])))
		return
	}

	clientId := binary.BigEndian.Uint32(this.readbuffer[4:])
	flags := binary.BigEndian.Uint16(this.readbuffer[8:])
	size := binary.BigEndian.Uint16(this.readbuffer[10:])
	packetId := binary.BigEndian.Uint32(this.readbuffer[12:])
	parts := binary.BigEndian.Uint32(this.readbuffer[16:])
	currentPart := binary.BigEndian.Uint32(this.readbuffer[20:])

	if this.logfunc != nil {
		this.logfunc("reudp: process: clientId %v / flags %v / %v bytes / packetId %v / part %v of %v / data %v", clientId, strconv.FormatUint(uint64(flags), 2), size, packetId, currentPart, parts, this.readbuffer[24:24+size])
	}

	if flags&_FLAG_ACK == 0 {
		this.send(who, clientId, packetId, flags|_FLAG_ACK, 0, parts, currentPart, []byte{})

		if parts < 2 {
			this.datafunc(who, this.readbuffer[24:24+size])
		} else {
			this.readCacheSync.Lock()
			defer this.readCacheSync.Unlock()

			key := makeCacheKey(clientId, packetId)

			cache, ok := this.readcache[key]
			if !ok {
				cache = make([][]byte, parts)
				this.readcache[key] = cache
			}

			cache[currentPart-1] = make([]byte, size)
			copy(cache[currentPart-1], this.readbuffer[24:24+size])

			cursor := 0
			for _, part := range cache {
				if part == nil {
					return
				} else {
					cursor += len(part)
				}
			}

			packet := make([]byte, cursor)
			cursor = 0
			for _, part := range cache {
				copy(packet[cursor:], part)
				cursor += len(part)
			}

			delete(this.readcache, key)

			this.datafunc(who, packet)
		}
	} else {
		this.writeCacheSync.Lock()
		defer this.writeCacheSync.Unlock()

		key := makeCacheKey(clientId, packetId)
		cache, ok := this.writecache[key]
		if ok && cache != nil && len(cache) > int(currentPart-1) {
			cache[currentPart-1] = nil

			for _, b := range cache {
				if b != nil {
					return
				}
			}

			delete(this.writecache, key)
		} else {

		}
	}
}

func (this *Reudp) Send(addr *net.UDPAddr, data []byte) error {
	parts := len(data) / _DATA_SIZE
	if len(data)%_DATA_SIZE > 0 {
		parts += 1
	}

	currentPart := 1
	cursor := 0
	packetId := uint32(time.Now().UnixNano())
	clientId := makeClientId(this.LocalAddr())

	var e *multierror.Error

	for currentPart <= parts {
		size := len(data) - cursor
		if size > _DATA_SIZE {
			size = _DATA_SIZE
		}

		this.writeSync.Lock()
		err := this.send(addr, clientId, packetId, 0, uint16(size), uint32(parts), uint32(currentPart), data[cursor:cursor+size])
		this.writeSync.Unlock()

		multierror.Append(e, err)

		cursor += size
		currentPart++
	}

	return e.ErrorOrNil()
}

func (this *Reudp) send(where *net.UDPAddr, clientId, packetId uint32, flags, size uint16, parts, currentPart uint32, data []byte) error {
	if this.logfunc != nil {
		this.logfunc("reudp: send to %v / clientid %v / packetid %v / flags %v / %v bytes / %v of %v / data %v", where, clientId, packetId, flags, size, currentPart, parts, data)
	}

	binary.BigEndian.PutUint32(this.writebuffer[0:4], _MARKER)
	binary.BigEndian.PutUint32(this.writebuffer[4:], clientId)
	binary.BigEndian.PutUint16(this.writebuffer[8:], flags)
	binary.BigEndian.PutUint16(this.writebuffer[10:], uint16(size))
	binary.BigEndian.PutUint32(this.writebuffer[12:], packetId)
	binary.BigEndian.PutUint32(this.writebuffer[16:], uint32(parts))
	binary.BigEndian.PutUint32(this.writebuffer[20:], uint32(currentPart))

	copy(this.writebuffer[24:], data)

	if flags&_FLAG_ACK == 0 {
		this.writeCacheSync.Lock()

		key := makeCacheKey(clientId, packetId)
		cache, ok := this.writecache[key]
		if !ok {
			cache = make([][]byte, parts)
			this.writecache[key] = cache
		}

		cache[currentPart-1] = make([]byte, _HEADER_SIZE+size)
		copy(cache[currentPart-1], this.writebuffer[:_HEADER_SIZE+size])

		this.writeCacheSync.Unlock()

		go this.waitForAck(where, key, currentPart, 3)
	}

	_, err := this.conn.WriteToUDP(this.writebuffer[:size+_HEADER_SIZE], where)

	return err
}

func (this *Reudp) waitForAck(where *net.UDPAddr, writecacheKey uint64, part uint32, attempts int) {
	<-time.Tick(time.Second * 30)

	cache, ok := this.writecache[writecacheKey]
	if ok && cache != nil && len(cache) > int(part-1) && cache[part-1] != nil {
		this.conn.WriteToUDP(cache[part-1], where)

		if attempts > 0 {
			go this.waitForAck(where, writecacheKey, part, attempts-1)
		}
	}
}

func makeCacheKey(clientid, packetId uint32) uint64 {
	key := uint64(0)
	key += uint64(clientid)
	key = key << 32
	key += uint64(packetId)

	return key
}

func makeClientId(addr *net.UDPAddr) uint32 {
	return binary.BigEndian.Uint32(addr.IP) + uint32(addr.Port)
}
