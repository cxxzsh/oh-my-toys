// Command portable packs the portable zip of the desktop app: the
// executable and the marker file that makes it keep its settings in a data
// folder next to it, in a folder of their own.
//
//	go run ./dev/portable -exe build/windows-amd64/oh-my-toys.exe -out oh-my-toys-0.1.0-windows-amd64-portable.zip
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/core"
)

// folder is the folder the zip unpacks into.
const folder = "oh-my-toys"

// markerText says what the marker file is for, to whoever opens it.
const markerText = `This file makes oh-my-toys portable: it keeps its settings in the "data"
folder next to it, writes nothing to the registry and does not update
itself. Delete this file to use the usual per-user folders instead.

有这个文件，oh-my-toys 就是便携版：设置保存在旁边的 data 文件夹里，
不写注册表，也不会自动更新。删掉它，就改用系统的用户目录。
`

func main() {
	exe := flag.String("exe", "", "the app's executable")
	out := flag.String("out", "", "the zip to write")
	flag.Parse()
	if *exe == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := pack(*exe, *out); err != nil {
		log.Fatal(err)
	}
}

func pack(exe, out string) error {
	f, err := os.Create(out) //nolint:gosec // G304: the developer names the zip to write
	if err != nil {
		return fmt.Errorf("create %s: %w", out, err)
	}
	zw := zip.NewWriter(f)
	err = addFile(zw, exe, filepath.Base(exe))
	if err == nil {
		err = addText(zw, core.PortableMarker, markerText)
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func addFile(zw *zip.Writer, path, name string) error {
	src, err := os.Open(path) //nolint:gosec // G304: the developer names the executable to pack
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = src.Close() }() // only read
	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("zip header of %s: %w", path, err)
	}
	h.Name, h.Method = folder+"/"+name, zip.Deflate
	w, err := zw.CreateHeader(h)
	if err != nil {
		return fmt.Errorf("add %s: %w", name, err)
	}
	if _, err := io.Copy(w, src); err != nil {
		return fmt.Errorf("pack %s: %w", name, err)
	}
	return nil
}

func addText(zw *zip.Writer, name, text string) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: folder + "/" + name, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return fmt.Errorf("add %s: %w", name, err)
	}
	if _, err := io.WriteString(w, text); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
