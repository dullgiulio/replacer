package main

import (
	"bytes"
	"io"
	"os"
)

type replacer interface {
	match(c byte) MatchRes
	replacement() []byte
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

func (b *bytestream) match(c byte) MatchRes {
	if c != b.src[b.pos] {
		b.pos = 0
		return MatchResNone
	}
	b.pos++
	if b.pos >= len(b.src) {
		b.pos = 0
		return MatchResDone
	}
	return MatchResMore
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
	reader io.Reader
	buf    []byte
	re     replacer
	cp     copier
	start  int
	stop   int
	lenght int
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
		n        int
		err      error
		cpstatus copyStatus
	)
	r.cp.to(dst)
Outer:
	for {
		n, cpstatus = r.cp.copy()
		if cpstatus == copyStatusFilled {
			break
		}
		if cpstatus == copyStatusDrained {
			r.lenght, err = r.reader.Read(r.buf)
			if r.lenght == 0 {
				break
			}
			r.start = 0
			r.stop = 0
		}
		for i := 0; i < r.lenght; i++ {
			res := r.re.match(r.buf[i])
			if res == MatchResDone {
				r.cp.from(r.buf[r.start:r.stop], r.re.replacement())
				r.start = i + 1
				r.stop = r.start
				continue Outer
			}
			if res != MatchResMore {
				r.stop++
			}
		}
		if r.start < r.stop {
			r.cp.from(r.buf[r.start:r.stop])
		}
	}
	return n, err
}

func main() {
	var buf [3]byte
	bs := newBytestream([]byte("aa"), []byte("xxx"))
	r := newReader(bytes.NewReader([]byte("aa bb cc aaaaa\n")), bs, buf[:])
	io.Copy(os.Stdout, r)
}
