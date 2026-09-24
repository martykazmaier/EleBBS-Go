package menu

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
	"elebbs/internal/config"
	"elebbs/internal/files"
	"elebbs/internal/term"
)

func TestDownloadAreaAndNames(t *testing.T) {
	area, names := downloadAreaAndNames("GAME.ZIP", 3)
	if area != 3 || len(names) != 1 || names[0] != "GAME.ZIP" {
		t.Fatalf("file only: area=%d names=%v", area, names)
	}
	area, names = downloadAreaAndNames("5 FILE.ZIP", 1)
	if area != 5 || len(names) != 1 || names[0] != "FILE.ZIP" {
		t.Fatalf("area+file: area=%d names=%v", area, names)
	}
	area, names = downloadAreaAndNames("/A=7 README.TXT", 1)
	if area != 7 || len(names) != 1 || names[0] != "README.TXT" {
		t.Fatalf("/A=: area=%d names=%v", area, names)
	}
}

func TestDownloadAllowsAnySpeed(t *testing.T) {
	dir := t.TempDir()
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello-download")
	if err := os.WriteFile(filepath.Join(filesDir, "GAME.ZIP"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.FileBase = filepath.Join(dir, "filebase")
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	area := cfgrec.FilesArea{AreaNum: 1, Name: "Games", FilePath: filesDir}
	if err := files.WriteFDB(g, area, []files.Entry{{Hdr: cfgrec.FilesHdr{Name: "GAME.ZIP", Size: uint32(len(payload))}}}); err != nil {
		t.Fatal(err)
	}
	rec := cfgrec.EncodeProtocol(cfgrec.Protocol{
		Name:        "Auto",
		ActiveKey:   '@',
		Attribute:   1,
		DnCmdString: `*C /c rem`,
	})
	if err := os.WriteFile(filepath.Join(dir, "protocol.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	if p := config.FindProtocol(g, '@'); p.Name != "Auto" || p.DnCmdString == "" {
		t.Fatalf("protocol %+v", p)
	}

	st := &seqStream{in: []byte("\r\r")}
	line := &cfgrec.LineCfg{
		AnsiOn:           true,
		Baud:             300,
		ErrorFreeConnect: true,
		RaNodeNr:         1,
		User: cfgrec.User{
			Record:       -1,
			FileArea:     1,
			DefaultProto: '@',
			Security:     100,
			Name:         "Test User",
		},
	}
	line.Language.MenuPath = dir
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line, Files: []cfgrec.FilesArea{area}}
	eng.download("GAME.ZIP", false, true, false, false)
	out := st.out.String()
	if strings.Contains(strings.ToLower(out), "not permitted at this speed") {
		t.Fatalf("speed gate still active: %q", out)
	}
	if !bytes.Contains(st.out.Bytes(), []byte("Auto")) {
		t.Fatalf("protocol name missing: %q", out)
	}
}

func TestViewTaggedFilesEmptyDoesNotAskFilename(t *testing.T) {
	dir := t.TempDir()
	st := &seqStream{in: []byte("GAME.ZIP\r")} // would be consumed if File: was prompted
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1, FileArea: 1, Security: 100}}
	line.Telnet.NodeDirectories = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line, Files: []cfgrec.FilesArea{{AreaNum: 1, Name: "Games"}}}
	if !eng.ExecType(71, "") {
		t.Fatal("type 71")
	}
	out := st.out.String()
	if !strings.Contains(out, "currently no tagged") {
		t.Fatalf("expected empty-list message: %q", out)
	}
	if strings.Contains(out, "File:") {
		t.Fatalf("should not ask for a filename: %q", out)
	}
	if len(files.LoadTagList(dir)) != 0 {
		t.Fatal("should not have tagged from the unused input")
	}
}

func TestViewTaggedFilesShowsTagListAfterClear(t *testing.T) {
	dir := t.TempDir()
	rec := files.EncodeTagFile(files.TagFile{Name: "GAME.ZIP", AreaNum: 1, Size: 12 * 1024})
	if err := os.WriteFile(filepath.Join(dir, "taglist.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1, Security: 100}}
	line.Telnet.NodeDirectories = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line, Files: []cfgrec.FilesArea{{AreaNum: 1, Name: "Games"}}}
	if !eng.ExecType(71, "") {
		t.Fatal("type 71")
	}
	out := st.out.Bytes()
	if !bytes.Contains(out, []byte("\x1b[2J")) {
		t.Fatalf("screen not cleared: %q", out)
	}
	if !bytes.Contains(out, []byte("GAME.ZIP")) {
		t.Fatalf("taglist.ra file missing: %q", out)
	}
	if bytes.Contains(out, []byte("Tagged files: (none)")) {
		t.Fatalf("stub empty list: %q", out)
	}
}

func TestViewTaggedFilesDeleteAndClear(t *testing.T) {
	dir := t.TempDir()
	raw := files.EncodeTagFile(files.TagFile{Name: "GAME.ZIP", AreaNum: 1, Size: 1024})
	raw = append(raw, files.EncodeTagFile(files.TagFile{Name: "TOOL.ZIP", AreaNum: 1, Size: 2048})...)
	if err := os.WriteFile(filepath.Join(dir, "taglist.ra"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("D1\r\r")}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	line := &cfgrec.LineCfg{AnsiOn: true, User: cfgrec.User{Record: -1, FileArea: 1, Security: 100}}
	line.Telnet.NodeDirectories = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line, Files: []cfgrec.FilesArea{{AreaNum: 1, Name: "Games"}}}
	if !eng.ExecType(71, "") {
		t.Fatal("type 71 delete")
	}
	left := files.LoadTagList(dir)
	if len(left) != 1 || left[0].Name != "TOOL.ZIP" {
		t.Fatalf("delete #1: %+v", left)
	}

	st = &seqStream{in: []byte("CY\r")}
	tio = term.New(st, g, line)
	eng = &Engine{T: tio, G: g, Line: line, Files: []cfgrec.FilesArea{{AreaNum: 1, Name: "Games"}}}
	if !eng.ExecType(71, "") {
		t.Fatal("type 71 clear")
	}
	if tags := files.LoadTagList(dir); len(tags) != 0 {
		t.Fatalf("clear left %+v", tags)
	}
}

func TestDownloadUsesTagList(t *testing.T) {
	dir := t.TempDir()
	filesDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("tagged-download")
	if err := os.WriteFile(filepath.Join(filesDir, "GAME.ZIP"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	g := &cfgrec.GlobalCfg{}
	g.RaConfig.SysPath = dir
	g.RaConfig.MsgBasePath = dir
	g.RaConfig.FileBase = filepath.Join(dir, "filebase")
	g.RaConfig.LogFileName = filepath.Join(dir, "elebbs.log")
	area := cfgrec.FilesArea{AreaNum: 1, Name: "Games", FilePath: filesDir}
	if err := files.WriteFDB(g, area, []files.Entry{{Hdr: cfgrec.FilesHdr{Name: "GAME.ZIP", Size: uint32(len(payload))}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "taglist.ra"), files.EncodeTagFile(files.TagFile{
		Name: "GAME.ZIP", AreaNum: 1, Size: int32(len(payload)),
	}), 0644); err != nil {
		t.Fatal(err)
	}
	rec := cfgrec.EncodeProtocol(cfgrec.Protocol{
		Name:        "Auto",
		ActiveKey:   '@',
		Attribute:   1,
		DnCmdString: `*C /c rem`,
	})
	if err := os.WriteFile(filepath.Join(dir, "protocol.ra"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	st := &seqStream{in: []byte("\r\r")}
	line := &cfgrec.LineCfg{
		AnsiOn:           true,
		Baud:             300,
		ErrorFreeConnect: true,
		RaNodeNr:         1,
		User: cfgrec.User{
			Record:       -1,
			FileArea:     1,
			DefaultProto: '@',
			Security:     100,
			Name:         "Test User",
		},
	}
	line.Telnet.NodeDirectories = dir
	line.Language.MenuPath = dir
	line.Language.TextPath = dir
	tio := term.New(st, g, line)
	eng := &Engine{T: tio, G: g, Line: line, Files: []cfgrec.FilesArea{area}}
	eng.download("", false, false, false, true)
	out := st.out.String()
	if strings.Contains(out, "No files to send") {
		t.Fatalf("taglist.ra ignored: %q", out)
	}
	if !bytes.Contains(st.out.Bytes(), []byte("Auto")) {
		t.Fatalf("did not start download: %q", out)
	}
}
