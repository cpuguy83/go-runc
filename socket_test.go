//go:build linux

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package runc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSocketReceive(t *testing.T) {
	t.Run("a message with an oversized name is rejected without leaking its descriptor", func(t *testing.T) {
		s := testSocket(t)
		f := testFile(t)

		go sendFd(t, s.Path(), strings.Repeat("a", 8192), int(f.Fd()))

		if _, err := s.receive(); err == nil {
			t.Fatal("expected an error for a name which does not fit the read buffer")
		}
		if got := openFdsTo(t, f); got != 1 {
			t.Fatalf("expected only the test's own descriptor to be open, got %d", got)
		}
	})

	t.Run("a message carrying more than one descriptor is rejected without leaking them", func(t *testing.T) {
		s := testSocket(t)
		first, second := testFile(t), testFile(t)

		go sendFd(t, s.Path(), "standard", int(first.Fd()), int(second.Fd()))

		if _, err := s.receive(); err == nil {
			t.Fatal("expected an error for a message carrying more than one descriptor")
		}
		for _, f := range []*os.File{first, second} {
			if got := openFdsTo(t, f); got != 1 {
				t.Fatalf("expected only the test's own descriptor to be open, got %d", got)
			}
		}
	})

	t.Run("a message carrying no descriptor is rejected", func(t *testing.T) {
		s := testSocket(t)

		go sendFd(t, s.Path(), "standard")

		if _, err := s.receive(); err == nil {
			t.Fatal("expected an error for a message without a descriptor")
		}
	})
}

func testSocket(t *testing.T) *Socket {
	t.Helper()
	s, err := NewPidfdSocket(filepath.Join(t.TempDir(), "socket"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// testFile returns a file whose descriptor can be sent over a socket and whose
// identity is visible in procfs, so that leaked copies of it can be spotted.
func testFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// openFdsTo returns the number of descriptors in this process which refer to
// the same file as f.
func openFdsTo(t *testing.T, f *os.File) int {
	t.Helper()
	self := "/proc/self/fd"
	want, err := os.Readlink(filepath.Join(self, fmt.Sprint(f.Fd())))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(self)
	if err != nil {
		t.Fatal(err)
	}
	var open int
	for _, entry := range entries {
		// Descriptors can be closed while the directory is being read.
		if got, err := os.Readlink(filepath.Join(self, entry.Name())); err == nil && got == want {
			open++
		}
	}
	return open
}
