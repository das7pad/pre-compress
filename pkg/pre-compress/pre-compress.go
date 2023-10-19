/*
 * Pre-Compress
 * Copyright (C) 2023 Jakob Ackermann <das7pad@outlook.com>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package precompress

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func IfSmaller(path string, m time.Time) (bool, error) {
	return IfSmallerBuffer(path, m, &bytes.Buffer{}, nil)
}

func IfSmallerBuffer(path string, m time.Time, out *bytes.Buffer, buf []byte) (bool, error) {
	s, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if !s.ModTime().Equal(m) {
		if err = os.Chtimes(path, m, m); err != nil {
			return false, err
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	out.Reset()
	out.Grow(int(s.Size()))

	w := limitedWriter{buf: out, n: int(s.Size())}
	z, err := gzip.NewWriterLevel(&w, gzip.BestCompression)
	if err != nil {
		return false, err
	}
	defer func() { _ = z.Close() }()

	n, err := io.CopyBuffer(z, f, buf)
	if err != nil {
		if errors.Is(err, errSizeThresholdExceeded) {
			return false, nil
		}
		return false, err
	}
	if err = z.Close(); err != nil {
		if errors.Is(err, errSizeThresholdExceeded) {
			return false, nil
		}
		return false, err
	}

	if int64(out.Len()) >= n {
		return false, nil
	}

	tmp := path + ".gz~"
	if err = os.WriteFile(tmp, out.Bytes(), s.Mode().Perm()); err != nil {
		return false, err
	}
	if err = os.Chtimes(tmp, m, m); err != nil {
		return false, err
	}
	if err = os.Rename(tmp, path+".gz"); err != nil {
		return false, err
	}
	return true, nil
}

var errSizeThresholdExceeded = errors.New("size threshold exceeded")

type limitedWriter struct {
	buf *bytes.Buffer
	n   int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	n, err := l.buf.Write(p)
	l.n -= n
	if l.n < 0 {
		return 0, errSizeThresholdExceeded
	}
	return n, err
}

func worker(root string, work <-chan string, m time.Time, ignoreRegex *regexp.Regexp) (int64, error) {
	n := int64(0)
	out := bytes.Buffer{}
	buf := make([]byte, 32*1024)
	for s := range work {
		if ignoreRegex.MatchString(s) {
			continue
		}
		path := filepath.Join(root, s)
		changed, err := IfSmallerBuffer(path, m, &out, buf)
		if err != nil {
			return n, err
		}
		if changed {
			n++
		}
	}
	return n, nil
}

func recurse(root string, prefix string, work chan<- string, ignore *regexp.Regexp) error {
	dirs, err := os.ReadDir(filepath.Join(root, prefix))
	if err != nil {
		return err
	}
	for i := 0; i < len(dirs); i++ {
		d := dirs[i]
		path := filepath.Join(prefix, d.Name())
		if d.Type().IsDir() {
			if ignore.MatchString(path) {
				continue
			}
			if err = recurse(root, path, work, ignore); err != nil {
				return err
			}
		} else if d.Type().IsRegular() {
			if strings.HasSuffix(d.Name(), ".gz") {
				continue
			}
			needle := d.Name() + ".gz"
			found := false
			for j := i + 1; j < len(dirs); j++ {
				if dirs[j].Name() == needle {
					found = true
					break
				}
				if dirs[j].Name() > needle {
					break
				}
			}
			if !found {
				work <- path
			}
		}
	}
	return nil
}

func Recursive(root string, m time.Time, concurrency int, ignorePattern []string) (int64, error) {
	var ignore *regexp.Regexp
	{
		var err error
		ignore, err = regexp.Compile("^" + strings.Join(ignorePattern, "|") + "$")
		if err != nil {
			return 0, err
		}
	}

	work := make(chan string, concurrency*10)
	firstErr := atomic.Pointer[error]{}
	total := atomic.Int64{}
	wg := sync.WaitGroup{}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			n, err := worker(root, work, m, ignore)
			total.Add(n)
			if err != nil {
				firstErr.CompareAndSwap(nil, &err)
			}
			for range work {
				// purge queue
			}
		}()
	}

	err := recurse(root, "", work, ignore)
	close(work)

	wg.Wait()
	if err == nil {
		if p := firstErr.Load(); p != nil {
			err = *p
		}
	}
	return total.Load(), err
}
