package relay

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"
)

func writeVarInt(dst []byte, n uint32) []byte {
	for n >= 128 {
		dst = append(dst, byte(n)|128)
		n >>= 7
	}
	return append(dst, byte(n))
}

func readVarInt(r io.ByteReader) (uint32, error) {
	var result uint32
	for shift := range 5 {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint32(b&127) << (7 * shift)
		if b < 128 {
			return result, nil
		}
	}
	return 0, errors.New("invalid Minecraft packet length")
}

func pingMinecraft(ctx context.Context, address, host string, port int) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	handshake := writeVarInt([]byte{0}, ^uint32(0))
	handshake = append(writeVarInt(handshake, uint32(len(host))), host...)
	handshake = binary.BigEndian.AppendUint16(handshake, uint16(port))
	handshake = append(handshake, 1)
	packet := append(writeVarInt(nil, uint32(len(handshake))), handshake...)
	if _, err := conn.Write(append(packet, 1, 0)); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	length, err := readVarInt(reader)
	if err != nil || length < 2 || length > 1<<17 {
		return errors.New("no valid Minecraft status packet received")
	}
	limited := bufio.NewReader(io.LimitReader(reader, int64(length)))
	packetID, err := readVarInt(limited)
	if err != nil || packetID != 0 {
		return errors.New("invalid Minecraft status response")
	}
	size, err := readVarInt(limited)
	if err != nil || size == 0 || size >= length {
		return errors.New("invalid Minecraft status length")
	}
	response := make([]byte, size)
	if _, err := io.ReadFull(limited, response); err != nil {
		return err
	}
	var status struct{ Version *struct{ Protocol int } }
	if err := json.Unmarshal(response, &status); err != nil || status.Version == nil {
		return errors.New("invalid Minecraft status JSON")
	}
	return nil
}
