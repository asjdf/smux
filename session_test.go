package smux

import (
	"bytes"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"sync"
	"testing"
	"time"
)

func init() {
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()
}

// setupServer starts new server listening on a random localhost port and
// returns address of the server, function to stop the server, new client
// connection to this server or an error.
func setupServer(_ testing.TB) (addr string, stopfunc func(), client net.Conn, err error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", nil, nil, err
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleConnection(conn)
	}()
	addr = ln.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		_ = ln.Close()
		return "", nil, nil, err
	}
	return ln.Addr().String(), func() { _ = ln.Close() }, conn, nil
}

func handleConnection(conn net.Conn) {
	session, _ := Server(conn, nil)
	for {
		if stream, err := session.AcceptStream(); err == nil {
			go func(s io.ReadWriteCloser) {
				buf := make([]byte, 65536)
				for {
					n, err := s.Read(buf)
					if err != nil {
						return
					}
					_, _ = s.Write(buf[:n])
				}
			}(stream)
		} else {
			return
		}
	}
}

// setupServer starts new server listening on a random localhost port and
// returns address of the server, function to stop the server, new client
// connection to this server or an error.
func setupServerV2(_ testing.TB) (addr string, stopfunc func(), client net.Conn, err error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", nil, nil, err
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleConnectionV2(conn)
	}()
	addr = ln.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		_ = ln.Close()
		return "", nil, nil, err
	}
	return ln.Addr().String(), func() { _ = ln.Close() }, conn, nil
}

func handleConnectionV2(conn net.Conn) {
	config := DefaultConfig()
	config.Version = 2
	session, _ := Server(conn, config)
	for {
		if stream, err := session.AcceptStream(); err == nil {
			go func(s io.ReadWriteCloser) {
				buf := make([]byte, 65536)
				for {
					n, err := s.Read(buf)
					if err != nil {
						return
					}
					_, _ = s.Write(buf[:n])
				}
			}(stream)
		} else {
			return
		}
	}
}

func TestEcho(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	const N = 100
	buf := make([]byte, 10)
	var sent string
	var received string
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		_, _ = stream.Write([]byte(msg))
		sent += msg
		if n, err := stream.Read(buf); err != nil {
			t.Fatal(err)
		} else {
			received += string(buf[:n])
		}
	}
	if sent != received {
		t.Fatal("data mimatch")
	}
	_ = session.Close()
}

func TestWriteTo(t *testing.T) {
	const N = 1 << 20
	// server
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		session, _ := Server(conn, nil)
		for {
			if stream, err := session.AcceptStream(); err == nil {
				go func(s io.ReadWriteCloser) {
					numBytes := 0
					buf := make([]byte, 65536)
					for {
						n, err := s.Read(buf)
						if err != nil {
							return
						}
						_, _ = s.Write(buf[:n])
						numBytes += n

						if numBytes == N {
							_ = s.Close()
							return
						}
					}
				}(stream)
			} else {
				return
			}
		}
	}()

	addr := ln.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)

	// client
	session, _ := Client(conn, nil)
	stream, _ := session.OpenStream()
	sndbuf := make([]byte, N)
	for i := range sndbuf {
		sndbuf[i] = byte(rand.Int())
	}

	go func() {
		_, _ = stream.Write(sndbuf)
	}()

	var rcvbuf bytes.Buffer
	nw, ew := stream.WriteTo(&rcvbuf)
	if ew != io.EOF {
		t.Fatal(ew)
	}

	if nw != N {
		t.Fatal("WriteTo nw mismatch", nw)
	}

	if !bytes.Equal(sndbuf, rcvbuf.Bytes()) {
		t.Fatal("mismatched echo bytes")
	}
}

func TestWriteToV2(t *testing.T) {
	config := DefaultConfig()
	config.Version = 2
	const N = 1 << 20
	// server
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		session, _ := Server(conn, config)
		for {
			if stream, err := session.AcceptStream(); err == nil {
				go func(s io.ReadWriteCloser) {
					numBytes := 0
					buf := make([]byte, 65536)
					for {
						n, err := s.Read(buf)
						if err != nil {
							return
						}
						_, _ = s.Write(buf[:n])
						numBytes += n

						if numBytes == N {
							_ = s.Close()
							return
						}
					}
				}(stream)
			} else {
				return
			}
		}
	}()

	addr := ln.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)

	// client
	session, _ := Client(conn, config)
	stream, _ := session.OpenStream()
	sndbuf := make([]byte, N)
	for i := range sndbuf {
		sndbuf[i] = byte(rand.Int())
	}

	go func() {
		_, _ = stream.Write(sndbuf)
	}()

	var rcvbuf bytes.Buffer
	nw, ew := stream.WriteTo(&rcvbuf)
	if ew != io.EOF {
		t.Fatal(ew)
	}

	if nw != N {
		t.Fatal("WriteTo nw mismatch", nw)
	}

	if !bytes.Equal(sndbuf, rcvbuf.Bytes()) {
		t.Fatal("mismatched echo bytes")
	}
}

func TestGetDieCh(t *testing.T) {
	cs, ss, err := getSmuxStreamPair()
	if err != nil {
		t.Fatal(err)
	}
	defer func(ss *Stream) {
		_ = ss.Close()
	}(ss)
	dieCh := ss.GetDieCh()
	go func() {
		select {
		case <-dieCh:
		case <-time.Tick(time.Second):
			t.Error("wait die chan timeout")
			return
		}
	}()
	_ = cs.Close()
}

func TestSpeed(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	t.Log(stream.LocalAddr(), stream.RemoteAddr())

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		buf := make([]byte, 1024*1024)
		nrecv := 0
		for {
			n, err := stream.Read(buf)
			if err != nil {
				t.Error(err)
				break
			} else {
				nrecv += n
				if nrecv == 4096*4096 {
					break
				}
			}
		}
		_ = stream.Close()
		t.Log("time for 16MB rtt", time.Since(start))
		wg.Done()
	}()
	msg := make([]byte, 8192)
	for i := 0; i < 2048; i++ {
		_, _ = stream.Write(msg)
	}
	wg.Wait()
	_ = session.Close()
}

func TestParallel(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)

	par := 1000
	messages := 100
	var wg sync.WaitGroup
	wg.Add(par)
	for i := 0; i < par; i++ {
		stream, _ := session.OpenStream()
		go func(s *Stream) {
			buf := make([]byte, 20)
			for j := 0; j < messages; j++ {
				msg := fmt.Sprintf("hello%v", j)
				_, _ = s.Write([]byte(msg))
				if _, err := s.Read(buf); err != nil {
					break
				}
			}
			_ = s.Close()
			wg.Done()
		}(stream)
	}
	t.Log("created", session.NumStreams(), "streams")
	wg.Wait()
	_ = session.Close()
}

func TestParallelV2(t *testing.T) {
	config := DefaultConfig()
	config.Version = 2
	_, stop, cli, err := setupServerV2(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, config)

	par := 1000
	messages := 100
	var wg sync.WaitGroup
	wg.Add(par)
	for i := 0; i < par; i++ {
		stream, _ := session.OpenStream()
		go func(s *Stream) {
			buf := make([]byte, 20)
			for j := 0; j < messages; j++ {
				msg := fmt.Sprintf("hello%v", j)
				_, _ = s.Write([]byte(msg))
				if _, err := s.Read(buf); err != nil {
					break
				}
			}
			_ = s.Close()
			wg.Done()
		}(stream)
	}
	t.Log("created", session.NumStreams(), "streams")
	wg.Wait()
	_ = session.Close()
}

func TestCloseThenOpen(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	_ = session.Close()
	if _, err := session.OpenStream(); err == nil {
		t.Fatal("opened after close")
	}
}

func TestSessionDoubleClose(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	_ = session.Close()
	if err := session.Close(); err == nil {
		t.Fatal("session double close doesn't return error")
	}
}

func TestStreamDoubleClose(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	_ = stream.Close()
	if err := stream.Close(); err == nil {
		t.Fatal("stream double close doesn't return error")
	}
	_ = session.Close()
}

func TestConcurrentClose(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	numStreams := 100
	streams := make([]*Stream, 0, numStreams)
	var wg sync.WaitGroup
	wg.Add(numStreams)
	for i := 0; i < 100; i++ {
		stream, _ := session.OpenStream()
		streams = append(streams, stream)
	}
	for _, s := range streams {
		stream := s
		go func() {
			_ = stream.Close()
			wg.Done()
		}()
	}
	_ = session.Close()
	wg.Wait()
}

func TestTinyReadBuffer(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	const N = 100
	tinybuf := make([]byte, 6)
	var sent string
	var received string
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		sent += msg
		nsent, err := stream.Write([]byte(msg))
		if err != nil {
			t.Fatal("cannot write")
		}
		nrecv := 0
		for nrecv < nsent {
			if n, err := stream.Read(tinybuf); err == nil {
				nrecv += n
				received += string(tinybuf[:n])
			} else {
				t.Fatal("cannot read with tiny buffer")
			}
		}
	}

	if sent != received {
		t.Fatal("data mismatch")
	}
	_ = session.Close()
}

func TestIsClose(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	_ = session.Close()
	if !session.IsClosed() {
		t.Fatal("still open after close")
	}
}

func TestKeepAliveTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)
	go func() {
		_, _ = ln.Accept()
	}()

	cli, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func(cli net.Conn) {
		_ = cli.Close()
	}(cli)

	config := DefaultConfig()
	config.KeepAliveInterval = time.Second
	config.KeepAliveTimeout = 2 * time.Second
	session, _ := Client(cli, config)
	time.Sleep(3 * time.Second)
	if !session.IsClosed() {
		t.Fatal("keepalive-timeout failed")
	}
}

type blockWriteConn struct {
	net.Conn
}

func (c *blockWriteConn) Write(b []byte) (n int, err error) {
	forever := time.Hour * 24
	time.Sleep(forever)
	return c.Conn.Write(b)
}

func TestKeepAliveBlockWriteTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)
	go func() {
		_, _ = ln.Accept()
	}()

	cli, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func(cli net.Conn) {
		_ = cli.Close()
	}(cli)
	//when writeFrame block, keepalive in old version never timeout
	blockWriteCli := &blockWriteConn{cli}

	config := DefaultConfig()
	config.KeepAliveInterval = time.Second
	config.KeepAliveTimeout = 2 * time.Second
	session, _ := Client(blockWriteCli, config)
	time.Sleep(3 * time.Second)
	if !session.IsClosed() {
		t.Fatal("keepalive-timeout failed")
	}
}

func TestServerEcho(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)
	go func() {
		err := func() error {
			conn, err := ln.Accept()
			if err != nil {
				return err
			}
			defer func(conn net.Conn) {
				_ = conn.Close()
			}(conn)
			session, err := Server(conn, nil)
			if err != nil {
				return err
			}
			defer func(session *Session) {
				_ = session.Close()
			}(session)
			buf := make([]byte, 10)
			stream, err := session.OpenStream()
			if err != nil {
				return err
			}
			defer func(stream *Stream) {
				_ = stream.Close()
			}(stream)
			for i := 0; i < 100; i++ {
				msg := fmt.Sprintf("hello%v", i)
				_, _ = stream.Write([]byte(msg))
				n, err := stream.Read(buf)
				if err != nil {
					return err
				}
				if got := string(buf[:n]); got != msg {
					return fmt.Errorf("got: %q, want: %q", got, msg)
				}
			}
			return nil
		}()
		if err != nil {
			t.Error(err)
		}
	}()

	cli, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func(cli net.Conn) {
		_ = cli.Close()
	}(cli)
	if session, err := Client(cli, nil); err == nil {
		if stream, err := session.AcceptStream(); err == nil {
			buf := make([]byte, 65536)
			for {
				n, err := stream.Read(buf)
				if err != nil {
					break
				}
				_, _ = stream.Write(buf[:n])
			}
		} else {
			t.Fatal(err)
		}
	} else {
		t.Fatal(err)
	}
}

func TestSendWithoutRecv(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	const N = 100
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		_, _ = stream.Write([]byte(msg))
	}
	buf := make([]byte, 1)
	if _, err := stream.Read(buf); err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
}

func TestWriteAfterClose(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	_ = stream.Close()
	if _, err := stream.Write([]byte("write after close")); err == nil {
		t.Fatal("write after close failed")
	}
}

func TestReadStreamAfterSessionClose(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	_ = session.Close()
	buf := make([]byte, 10)
	if _, err := stream.Read(buf); err != nil {
		t.Log(err)
	} else {
		t.Fatal("read stream after session close succeeded")
	}
}

func TestWriteStreamAfterConnectionClose(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	_ = session.conn.Close()
	if _, err := stream.Write([]byte("write after connection close")); err == nil {
		t.Fatal("write after connection close failed")
	}
}

func TestNumStreamAfterClose(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	if _, err := session.OpenStream(); err == nil {
		if session.NumStreams() != 1 {
			t.Fatal("wrong number of streams after opened")
		}
		_ = session.Close()
		if session.NumStreams() != 0 {
			t.Fatal("wrong number of streams after session closed")
		}
	} else {
		t.Fatal(err)
	}
	_ = cli.Close()
}

func TestRandomFrame(t *testing.T) {
	addr, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	// pure random
	session, _ := Client(cli, nil)
	for i := 0; i < 100; i++ {
		rnd := make([]byte, rand.Uint32()%1024)
		_, _ = io.ReadFull(crand.Reader, rnd)
		_, _ = session.conn.Write(rnd)
	}
	_ = cli.Close()

	// double syn
	cli, err = net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	session, _ = Client(cli, nil)
	for i := 0; i < 100; i++ {
		f := newFrame(1, cmdSYN, 1000)
		_, _ = session.writeFrame(f)
	}
	_ = cli.Close()

	// random cmds
	cli, err = net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	allcmds := []byte{cmdSYN, cmdFIN, cmdPSH, cmdNOP}
	session, _ = Client(cli, nil)
	for i := 0; i < 100; i++ {
		f := newFrame(1, allcmds[rand.Int()%len(allcmds)], rand.Uint32())
		_, _ = session.writeFrame(f)
	}
	_ = cli.Close()

	// random cmds & sids
	cli, err = net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	session, _ = Client(cli, nil)
	for i := 0; i < 100; i++ {
		f := newFrame(1, byte(rand.Uint32()), rand.Uint32())
		_, _ = session.writeFrame(f)
	}
	_ = cli.Close()

	// random version
	cli, err = net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	session, _ = Client(cli, nil)
	for i := 0; i < 100; i++ {
		f := newFrame(1, byte(rand.Uint32()), rand.Uint32())
		f.ver = byte(rand.Uint32())
		_, _ = session.writeFrame(f)
	}
	_ = cli.Close()

	// incorrect size
	cli, err = net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	session, _ = Client(cli, nil)

	f := newFrame(1, byte(rand.Uint32()), rand.Uint32())
	rnd := make([]byte, rand.Uint32()%1024)
	_, _ = io.ReadFull(crand.Reader, rnd)
	f.data = rnd

	buf := make([]byte, headerSize+len(f.data))
	buf[0] = f.ver
	buf[1] = f.cmd
	binary.LittleEndian.PutUint16(buf[2:], uint16(len(rnd)+1)) /// incorrect size
	binary.LittleEndian.PutUint32(buf[4:], f.sid)
	copy(buf[headerSize:], f.data)

	_, _ = session.conn.Write(buf)
	_ = cli.Close()

	// writeFrame after die
	cli, err = net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	session, _ = Client(cli, nil)
	//close first
	_ = session.Close()
	for i := 0; i < 100; i++ {
		f := newFrame(1, byte(rand.Uint32()), rand.Uint32())
		_, _ = session.writeFrame(f)
	}
}

func TestWriteFrameInternal(t *testing.T) {
	addr, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	// pure random
	session, _ := Client(cli, nil)
	for i := 0; i < 100; i++ {
		rnd := make([]byte, rand.Uint32()%1024)
		_, _ = io.ReadFull(crand.Reader, rnd)
		_, _ = session.conn.Write(rnd)
	}
	_ = cli.Close()

	// writeFrame after die
	cli, err = net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	session, _ = Client(cli, nil)
	//close first
	_ = session.Close()
	for i := 0; i < 100; i++ {
		f := newFrame(1, byte(rand.Uint32()), rand.Uint32())
		_, _ = session.writeFrameInternal(f, time.After(session.config.KeepAliveTimeout), CLSDATA)
	}

	// random cmds
	cli, err = net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	allcmds := []byte{cmdSYN, cmdFIN, cmdPSH, cmdNOP}
	session, _ = Client(cli, nil)
	for i := 0; i < 100; i++ {
		f := newFrame(1, allcmds[rand.Int()%len(allcmds)], rand.Uint32())
		_, _ = session.writeFrameInternal(f, time.After(session.config.KeepAliveTimeout), CLSDATA)
	}
	//deadline occur
	{
		c := make(chan time.Time)
		close(c)
		f := newFrame(1, allcmds[rand.Int()%len(allcmds)], rand.Uint32())
		_, err := session.writeFrameInternal(f, c, CLSDATA)
		if !strings.Contains(err.Error(), "timeout") {
			t.Fatal("write frame with deadline failed", err)
		}
	}
	_ = cli.Close()

	{
		cli, err = net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		config := DefaultConfig()
		config.KeepAliveInterval = time.Second
		config.KeepAliveTimeout = 2 * time.Second
		session, _ = Client(&blockWriteConn{cli}, config)
		f := newFrame(1, byte(rand.Uint32()), rand.Uint32())
		c := make(chan time.Time)
		go func() {
			//die first, deadline second, better for coverage
			time.Sleep(time.Second)
			_ = session.Close()
			time.Sleep(time.Second)
			close(c)
		}()
		_, err = session.writeFrameInternal(f, c, CLSDATA)
		if !strings.Contains(err.Error(), "closed pipe") {
			t.Fatal("write frame with to closed conn failed", err)
		}
	}
}

func TestReadDeadline(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	const N = 100
	buf := make([]byte, 10)
	var readErr error
	for i := 0; i < N; i++ {
		_ = stream.SetReadDeadline(time.Now().Add(-1 * time.Minute))
		if _, readErr = stream.Read(buf); readErr != nil {
			break
		}
	}
	if readErr != nil {
		if !strings.Contains(readErr.Error(), "timeout") {
			t.Fatalf("Wrong error: %v", readErr)
		}
	} else {
		t.Fatal("No error when reading with past deadline")
	}
	_ = session.Close()
}

func TestWriteDeadline(t *testing.T) {
	_, stop, cli, err := setupServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	stream, _ := session.OpenStream()
	buf := make([]byte, 10)
	var writeErr error
	for {
		_ = stream.SetWriteDeadline(time.Now().Add(-1 * time.Minute))
		if _, writeErr = stream.Write(buf); writeErr != nil {
			if !strings.Contains(writeErr.Error(), "timeout") {
				t.Fatalf("Wrong error: %v", writeErr)
			}
			break
		}
	}
	_ = session.Close()
}

func BenchmarkAcceptClose(b *testing.B) {
	_, stop, cli, err := setupServer(b)
	if err != nil {
		b.Fatal(err)
	}
	defer stop()
	session, _ := Client(cli, nil)
	for i := 0; i < b.N; i++ {
		if stream, err := session.OpenStream(); err == nil {
			_ = stream.Close()
		} else {
			b.Fatal(err)
		}
	}
}
func BenchmarkConnSmux(b *testing.B) {
	cs, ss, err := getSmuxStreamPair()
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = cs.Close()
		_ = ss.Close()
	}()
	bench(b, cs, ss)
}

func BenchmarkConnTCP(b *testing.B) {
	cs, ss, err := getTCPConnectionPair()
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = cs.Close()
		_ = ss.Close()
	}()
	defer bench(b, cs, ss)
}

func getSmuxStreamPair() (*Stream, *Stream, error) {
	c1, c2, err := getTCPConnectionPair()
	if err != nil {
		return nil, nil, err
	}

	s, err := Server(c2, nil)
	if err != nil {
		return nil, nil, err
	}
	c, err := Client(c1, nil)
	if err != nil {
		return nil, nil, err
	}
	var ss *Stream
	done := make(chan error)
	go func() {
		var rerr error
		ss, rerr = s.AcceptStream()
		done <- rerr
		close(done)
	}()
	cs, err := c.OpenStream()
	if err != nil {
		return nil, nil, err
	}
	err = <-done
	if err != nil {
		return nil, nil, err
	}

	return cs, ss, nil
}

func getTCPConnectionPair() (net.Conn, net.Conn, error) {
	lst, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, nil, err
	}
	defer func(lst net.Listener) {
		_ = lst.Close()
	}(lst)

	var conn0 net.Conn
	var err0 error
	done := make(chan struct{})
	go func() {
		conn0, err0 = lst.Accept()
		close(done)
	}()

	conn1, err := net.Dial("tcp", lst.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	<-done
	if err0 != nil {
		return nil, nil, err0
	}
	return conn0, conn1, nil
}

func bench(b *testing.B, rd io.Reader, wr io.Writer) {
	buf := make([]byte, 128*1024)
	buf2 := make([]byte, 128*1024)
	b.SetBytes(128 * 1024)
	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		count := 0
		for {
			n, _ := rd.Read(buf2)
			count += n
			if count == 128*1024*b.N {
				return
			}
		}
	}()
	for i := 0; i < b.N; i++ {
		_, _ = wr.Write(buf)
	}
	wg.Wait()
}
