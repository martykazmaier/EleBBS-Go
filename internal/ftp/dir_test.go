package ftp

import (
	"strings"
	"testing"

	"elebbs/internal/cfgrec"
)

func testFTPSession() *session {
	u := cfgrec.User{Security: 100, Name: "Sysop"}
	return &session{
		srv: &Server{
			grps: []cfgrec.Group{{AreaNum: 1, Name: "Main Files"}},
			areas: []cfgrec.FilesArea{
				{AreaNum: 4, Name: "Anime Shows", FilePath: `c:\files\anime`, Group: 1},
			},
		},
		user:  u,
		group: -1,
		area:  -1,
	}
}

func TestPartsFromDirListAbsoluteIsNotWildcard(t *testing.T) {
	ss := testFTPSession()
	ss.group, ss.area = 1, 4
	g, a, wild, inv := ss.partsFromDir("/Main_Files/Anime_Shows", 1, 4)
	if inv {
		t.Fatal("absolute LIST path should resolve")
	}
	if g != 1 || a != 4 {
		t.Fatalf("group/area %d/%d", g, a)
	}
	if wild != "" {
		t.Fatalf("LIST /group/area must not use the area name as a file wildcard, got %q", wild)
	}
}

func TestPartsFromDirListGroupFromRoot(t *testing.T) {
	ss := testFTPSession()
	g, a, wild, inv := ss.partsFromDir("/Main_Files", -1, -1)
	if inv {
		t.Fatal("LIST /group should resolve")
	}
	if g != 1 || a != -1 {
		t.Fatalf("LIST /group got g=%d a=%d", g, a)
	}
	if wild != "" {
		t.Fatalf("LIST /group must not use the group name as a file mask, got %q", wild)
	}
}

func TestFtpListArgDropsFlags(t *testing.T) {
	if got := ftpListArg("-a"); got != "" {
		t.Fatalf("LIST -a -> %q", got)
	}
	if got := ftpListArg("-la /Main_Files"); got != "/Main_Files" {
		t.Fatalf("LIST -la /group -> %q", got)
	}
}

func TestPartsFromDirRetrAbsoluteFile(t *testing.T) {
	ss := testFTPSession()
	g, a, wild, inv := ss.partsFromDir("/Main_Files/Anime_Shows/eday301.zip", -1, -1)
	if inv {
		t.Fatal("RETR /group/area/file should resolve")
	}
	if g != 1 || a != 4 || !strings.EqualFold(wild, "eday301.zip") {
		t.Fatalf("RETR path g=%d a=%d wild=%q", g, a, wild)
	}
}

func TestPartsFromDirCwdGroupThenArea(t *testing.T) {
	ss := testFTPSession()
	g, a, _, inv := ss.partsFromDir("Main_Files", -1, -1)
	if inv || g != 1 || a != -1 {
		t.Fatalf("CWD group got g=%d a=%d inv=%v", g, a, inv)
	}
	g, a, _, inv = ss.partsFromDir("Anime_Shows", g, a)
	if inv || g != 1 || a != 4 {
		t.Fatalf("CWD area got g=%d a=%d inv=%v", g, a, inv)
	}
}

func TestPartsFromDirDotKeepsDirectory(t *testing.T) {
	ss := testFTPSession()
	g, a, wild, _ := ss.partsFromDir(".", 1, -1)
	if g != 1 {
		t.Fatalf("LIST . in a group should keep the group, g=%d", g)
	}
	if wild != "" {
		t.Fatalf("LIST . must not filter names with %q", wild)
	}
	_ = a
}
