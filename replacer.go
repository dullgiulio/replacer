package main

import (
	"bytes"
	"io"
	"os"
)

type replacer interface {
	match(c byte) (MatchRes, int)
	matched(int) []byte
	replacement() []byte
	reset()
}

type MatchRes int

const (
	MatchResNone MatchRes = iota
	MatchResDone
	MatchResMore
)

type bytestream struct {
	src []byte
	dst []byte
	pos int
}

func newBytestream(src, dst []byte) *bytestream {
	return &bytestream{src: src, dst: dst}
}

func (b *bytestream) reset() {
	b.pos = 0
}

func (b *bytestream) match(c byte) (MatchRes, int) {
	if c != b.src[b.pos] {
		p := b.pos
		b.pos = 0
		return MatchResNone, p
	}
	b.pos++
	if b.pos >= len(b.src) {
		b.pos = 0
		return MatchResDone, 0
	}
	return MatchResMore, b.pos
}

func (b *bytestream) matched(p int) []byte {
	return b.src[0:p]
}

func (b *bytestream) replacement() []byte {
	return b.dst
}

type copyStatus int

const (
	copyStatusDrained copyStatus = iota
	copyStatusFilled
)

type copier struct {
	dst  []byte
	src  []byte
	srcs [][]byte
	pdst int
	psrc int
}

func (c *copier) from(bufs ...[]byte) {
	c.srcs = append(c.srcs, bufs...)
}

func (c *copier) to(buf []byte) {
	c.dst = buf
	c.pdst = 0
}

func (c *copier) copy() (int, copyStatus) {
	if c.dst == nil {
		return 0, copyStatusFilled
	}
	if len(c.srcs) == 0 {
		return 0, copyStatusDrained
	}
	var n int
	for {
		if c.pdst > len(c.dst) {
			return n, copyStatusFilled
		}
		if c.src == nil || c.psrc >= len(c.src) {
			if len(c.srcs) == 0 {
				return n, copyStatusDrained
			}
			c.src = c.srcs[0]
			c.psrc = 0
			c.srcs = c.srcs[1:]
		}
		// Longes amount we can copy
		ddst := len(c.dst) - c.pdst
		dist := len(c.src) - c.psrc
		if dist > ddst {
			dist = ddst
		}
		for i := 0; i < dist; i++ {
			c.dst[c.pdst] = c.src[c.psrc]
			c.pdst++
			c.psrc++
		}
		n = n + dist
	}
	return n, copyStatusDrained
}

type reader struct {
	reader  io.Reader
	buf     []byte
	re      replacer
	cp      copier
	start   int
	stop    int
	length  int
	lastpos int
	err     error
	eof     bool
	done    bool
	phantom bool
}

func newReader(r io.Reader, re replacer, buf []byte) *reader {
	return &reader{
		reader: r,
		re:     re,
		buf:    buf,
		cp:     copier{dst: make([]byte, 0)},
	}
}

func (r *reader) Read(dst []byte) (int, error) {
	var (
		n, m     int
		err      error
		cpstatus copyStatus
	)
	r.cp.to(dst)
	for {
		m, cpstatus = r.cp.copy()
		n = n + m
		if cpstatus == copyStatusFilled {
			return n, r.err
		}
		if cpstatus == copyStatusDrained && r.done == true {
			if r.eof {
				return n, io.EOF
			}
			r.length, err = r.reader.Read(r.buf)
			if err == io.EOF {
				r.eof = true
				if r.length == 0 {
					return n, io.EOF
				}
			} else {
				r.err = err
			}
			r.start = 0
			r.stop = 0
			r.done = false
		}
		var res MatchRes
		for i := r.start; i < r.length; i++ {
			res, r.lastpos = r.re.match(r.buf[i])
			if res == MatchResDone {
				r.cp.from(r.buf[r.start:r.stop], r.re.replacement())
				r.stop = i + 1
				r.start = r.stop
				return n, r.err
			}
			if res != MatchResMore {
				if r.phantom {
					r.cp.from(r.re.matched(r.lastpos))
					r.phantom = false
					r.done = true
					return n, r.err
				}
				r.stop = i + 1
			}
		}
		r.cp.from(r.buf[r.start:r.stop])
		if res == MatchResMore {
			r.phantom = true
		} else {
			r.done = true
		}
	}
	return n, r.err
}

func main() {
	var buf [10]byte
	bs := newBytestream([]byte("cloud"), []byte("toilet"))
	r := newReader(bytes.NewReader([]byte("Your clode lives in the cloud!\n")), bs, buf[:])
	io.Copy(os.Stdout, r)
}
