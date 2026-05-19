package render

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// tmpSeq disambiguates temp filenames when multiple goroutines in the
// same process call WriteSVGAtomic within the same nanosecond — a real
// possibility on macOS where time.Now() resolution can be coarser than ns.
var tmpSeq uint64

// SiblingPath maps "dir/foo.d2" to "dir/.foo.d2.svg" — a hidden sibling
// in the same directory, retaining the original filename inside the
// dot-prefixed name so multiple .d2 files in one directory do not collide.
func SiblingPath(sourcePath string) string {
	dir, file := filepath.Split(sourcePath)
	return filepath.Join(dir, "."+file+".svg")
}

// WriteSVGAtomic writes `data` to `target` via a temp-file-and-rename
// sequence so external readers never observe a partial file.
func WriteSVGAtomic(target string, data []byte) error {
	dir := filepath.Dir(target)
	base := filepath.Base(target)
	seq := atomic.AddUint64(&tmpSeq, 1)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d.%d.%d", base, os.Getpid(), time.Now().UnixNano(), seq))

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open temp %q: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write temp %q: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close temp %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %q to %q: %w", tmp, target, err)
	}
	return nil
}
