package files

import (
	"testing"

	"elebbs/internal/cfgrec"
)

func TestFileAreaAccessUsesSecurityAndFlags(t *testing.T) {
	area := cfgrec.FilesArea{
		AreaNum:        1,
		Name:           "Games",
		FilePath:       `C:\files\games`,
		ListSecurity:   10,
		Security:       20,
		UploadSecurity: 50,
	}
	area.ListFlags[0] = 1 << 0 // A1 required to list
	low := cfgrec.User{Security: 5}
	listed := cfgrec.User{Security: 15}
	listed.Flags[0] = 1 << 0
	dl := cfgrec.User{Security: 25}
	dl.Flags[0] = 1 << 0
	up := cfgrec.User{Security: 50}
	up.Flags[0] = 1 << 0

	if ListAccess(area, low) {
		t.Fatal("low security must not list")
	}
	if ListAccess(area, cfgrec.User{Security: 15}) {
		t.Fatal("list flag A1 required")
	}
	if !ListAccess(area, listed) {
		t.Fatal("listed user should list")
	}
	if DownloadAccess(area, listed) {
		t.Fatal("list level must not download")
	}
	if !DownloadAccess(area, dl) {
		t.Fatal("download user should download")
	}
	if UploadAccess(area, dl) {
		t.Fatal("download level must not upload")
	}
	if !UploadAccess(area, up) {
		t.Fatal("upload user should upload")
	}
}

func TestGroupAccessRequiresFlags(t *testing.T) {
	gr := cfgrec.Group{AreaNum: 2, Name: "Public", Security: 0}
	gr.Flags[0] = 1 << 1 // A2
	u := cfgrec.User{Security: 100}
	if GroupAccess(gr, u) {
		t.Fatal("missing group flag")
	}
	u.Flags[0] = 1 << 1
	if !GroupAccess(gr, u) {
		t.Fatal("group flag present")
	}
}

func TestParseFilesAreaFlags(t *testing.T) {
	b := make([]byte, cfgrec.FilesRecSize)
	b[0], b[1] = 7, 0 // AreaNum
	b[4] = 5          // Name length
	copy(b[5:], []byte("Games"))
	b[46] = 12 // FilePath length
	copy(b[47:], []byte(`C:\files\g`))
	// UploadSecurity at offset: 2+2+41+1+41+2+2+16+2+1+1+2+1+2+2 = let's use Parse and set via writer-less layout
	a := cfgrec.ParseFilesArea(encodeFilesArea(cfgrec.FilesArea{
		AreaNum:        3,
		Name:           "Uploads",
		FilePath:       `D:\ul`,
		UploadSecurity: 40,
		Security:       20,
		ListSecurity:   10,
		UploadFlags:    cfgrec.Flag{0: 1},
		Flags:          cfgrec.Flag{1: 2},
		ListFlags:      cfgrec.Flag{2: 4},
	}))
	if a.AreaNum != 3 || a.Name != "Uploads" || a.FilePath != `D:\ul` {
		t.Fatalf("identity %+v", a)
	}
	if a.UploadSecurity != 40 || a.Security != 20 || a.ListSecurity != 10 {
		t.Fatalf("sec %+v", a)
	}
	if a.UploadFlags[0] != 1 || a.Flags[1] != 2 || a.ListFlags[2] != 4 {
		t.Fatalf("flags %+v / %+v / %+v", a.UploadFlags, a.Flags, a.ListFlags)
	}
}

func encodeFilesArea(a cfgrec.FilesArea) []byte {
	b := make([]byte, cfgrec.FilesRecSize)
	w := func(off int, s string, n int) int {
		if len(s) > n {
			s = s[:n]
		}
		b[off] = byte(len(s))
		copy(b[off+1:], s)
		return off + 1 + n
	}
	b[0] = byte(a.AreaNum)
	b[1] = byte(a.AreaNum >> 8)
	off := 4
	off = w(off, a.Name, 40)
	b[off] = a.Attrib
	off++
	off = w(off, a.FilePath, 40)
	off += 2 + 2 // kill days
	off = w(off, a.Password, 15)
	off += 2 + 1 + 1 + 2 + 1 + 2 + 2 // move, age, convert, group, attrib2, defcost, uploadarea
	put16 := func(v uint16) {
		b[off] = byte(v)
		b[off+1] = byte(v >> 8)
		off += 2
	}
	put16(a.UploadSecurity)
	copy(b[off:], a.UploadFlags[:])
	off += 4
	copy(b[off:], a.UploadNotFlags[:])
	off += 4
	put16(a.Security)
	copy(b[off:], a.Flags[:])
	off += 4
	copy(b[off:], a.NotFlags[:])
	off += 4
	put16(a.ListSecurity)
	copy(b[off:], a.ListFlags[:])
	off += 4
	copy(b[off:], a.ListNotFlags[:])
	return b
}
