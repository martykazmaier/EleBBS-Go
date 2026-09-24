package userbase

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"elebbs/internal/cfgrec"
	"elebbs/internal/crc"
	"elebbs/internal/pascal"
)

func stringsTrim(s string) string { return pascal.Trim(s) }

func cleanDir(p string) string {
	p = stringsTrim(p)
	p = strings.ReplaceAll(p, "/", `\`)
	return strings.TrimRight(p, `\`)
}

// isDriveRootRel is a Windows path like \ele\msgbase — not a drive letter,
// not UNC. Pascal opens it as {currentDrive}:\ele\msgbase. Go filepath.IsAbs
// is false for these, so Join() nested them under CONFIG.RA and missed USERS.BBS.
func isDriveRootRel(p string) bool {
	if p == "" {
		return false
	}
	if p[0] != '\\' && p[0] != '/' {
		return false
	}
	return len(p) < 2 || (p[1] != '\\' && p[1] != '/')
}

func volumeHint(g *cfgrec.GlobalCfg) string {
	for _, p := range []string{g.CfgPath, g.RaConfig.SysPath, g.RaConfig.MsgBasePath} {
		if v := filepath.VolumeName(p); v != "" {
			return v
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.VolumeName(wd)
	}
	return ""
}

func withVolume(g *cfgrec.GlobalCfg, p string) string {
	p = cleanDir(p)
	if !isDriveRootRel(p) {
		return p
	}
	if vol := volumeHint(g); vol != "" {
		return vol + p
	}
	return p
}

func suffixAfter(p, prefix string) string {
	a := strings.Trim(cleanDir(p), `\`)
	b := strings.Trim(cleanDir(prefix), `\`)
	if a == "" {
		return ""
	}
	if b == "" {
		return a
	}
	al, bl := strings.ToLower(a), strings.ToLower(b)
	if al == bl {
		return ""
	}
	if strings.HasPrefix(al, bl+`\`) {
		return a[len(b)+1:]
	}
	return a
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func absDir(g *cfgrec.GlobalCfg, p string) string {
	p = cleanDir(p)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	if isDriveRootRel(p) {
		rooted := withVolume(g, p)
		if dirExists(rooted) || fileExists(filepath.Join(rooted, cfgrec.UserBaseName)) {
			return rooted
		}
		cfgDir := ""
		if g.CfgPath != "" {
			cfgDir = filepath.Dir(g.CfgPath)
		}
		if cfgDir != "" {
			if rest := suffixAfter(p, g.RaConfig.SysPath); rest != "" {
				alt := filepath.Join(cfgDir, rest)
				if dirExists(alt) || fileExists(filepath.Join(alt, cfgrec.UserBaseName)) {
					return alt
				}
			}
			alt := filepath.Join(cfgDir, filepath.Base(p))
			if dirExists(alt) || fileExists(filepath.Join(alt, cfgrec.UserBaseName)) {
				return alt
			}
		}
		return rooted
	}
	root := cleanDir(g.RaConfig.SysPath)
	if isDriveRootRel(root) {
		root = withVolume(g, root)
	}
	if root == "" && g.CfgPath != "" {
		root = filepath.Dir(g.CfgPath)
	}
	if root != "" && !filepath.IsAbs(root) && !isDriveRootRel(root) && g.CfgPath != "" {
		root = filepath.Join(filepath.Dir(g.CfgPath), root)
	}
	if root == "" {
		return p
	}
	return filepath.Join(root, p)
}

func userDirs(g *cfgrec.GlobalCfg) []string {
	var out []string
	add := func(p string) {
		p = absDir(g, p)
		if p == "" {
			return
		}
		for _, e := range out {
			if strings.EqualFold(e, p) {
				return
			}
		}
		out = append(out, p)
	}
	add(g.RaConfig.MsgBasePath)
	add(g.RaConfig.SysPath)
	if g.CfgPath != "" {
		add(filepath.Dir(g.CfgPath))
	}
	return out
}

// userDir is the directory of the live USERS.BBS. If more than one copy
// exists, keep the largest so a 1-record shadow from a new-user apply does
// not hide the real file.
func userDir(g *cfgrec.GlobalCfg) string {
	dirs := userDirs(g)
	var best string
	var bestSize int64 = -1
	for _, dir := range dirs {
		p := filepath.Join(dir, cfgrec.UserBaseName)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		if st.Size() > bestSize {
			best = dir
			bestSize = st.Size()
		}
	}
	if best != "" {
		return best
	}
	if len(dirs) > 0 {
		return dirs[0]
	}
	return ""
}

func Path(g *cfgrec.GlobalCfg, name string) string {
	dir := userDir(g)
	if dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}

func yes(v byte) bool { return v == cfgrec.AskYes }

func setBit(attr *byte, bit byte, on bool) {
	if on {
		*attr |= bit
	} else {
		*attr &^= bit
	}
}

// NewDefaults fills a UsersRecord the way EleBBS SetNewUserDefaults does,
// and always stores '@' as DefaultProtocol.
func NewDefaults(g *cfgrec.GlobalCfg) cfgrec.User {
	now := time.Now()
	u := cfgrec.User{
		FileArea:     1,
		MsgArea:      1,
		FileGroup:    1,
		MsgGroup:     1,
		ScreenWidth:  80,
		ScreenLength: 24,
		Security:     g.RaConfig.NewSecurity,
		Credit:       int32(g.RaConfig.NewCredit),
		Flags:        g.RaConfig.NewFlags,
		Group:        uint16(g.RaConfig.NewUserGroup),
		Language:     g.RaConfig.NewUserLang,
		DateFormat:   5,
		LastTime:     now.Format("15:04"),
		LastDate:     now.Format("01-02-06"),
		FirstDate:    now.Format("01-02-06"),
		DefaultProto: cfgrec.DefaultTransferProto,
		Uploads:      0,
		UploadsK:     0,
		Record:       -1,
	}
	setBit(&u.Attribute, cfgrec.UserClrScr, g.RaConfig.ClearScreen != cfgrec.AskNo)
	setBit(&u.Attribute, cfgrec.UserMore, g.RaConfig.MorePrompt != cfgrec.AskNo)
	setBit(&u.Attribute, cfgrec.UserANSI, yes(g.RaConfig.ANSI))
	setBit(&u.Attribute2, cfgrec.User2HotKeys, yes(g.RaConfig.HotKeys))
	setBit(&u.Attribute2, cfgrec.User2Avatar, yes(g.RaConfig.AVATAR))
	if u.Attribute&cfgrec.UserANSI == 0 {
		// Telnet sessions and sysop-created users still need ANSI for the Go node.
		u.Attribute |= cfgrec.UserANSI | cfgrec.UserMore | cfgrec.UserClrScr
	}
	return u
}

// foldName matches EleBBS SearchUser: NoDoubleSpace(SUpCase(Trim(Name))).
func foldName(s string) string {
	return pascal.NoDoubleSpace(pascal.UpCase(pascal.Trim(s)))
}

func nameMatches(stored, want string) bool {
	return foldName(stored) == want && want != ""
}

func Search(g *cfgrec.GlobalCfg, name string) (cfgrec.User, bool) {
	query := foldName(name)
	if query == "" {
		return cfgrec.User{}, false
	}
	userCRC := crc.RA(query, true)
	idxPath := Path(g, cfgrec.UserBaseIdxName)
	bbsPath := Path(g, cfgrec.UserBaseName)
	if b, err := os.ReadFile(idxPath); err == nil && len(b) >= cfgrec.UsersIdxSize {
		n := len(b) / cfgrec.UsersIdxSize
		for i := 0; i < n; i++ {
			idx := cfgrec.ParseUsersIdx(b[i*cfgrec.UsersIdxSize : (i+1)*cfgrec.UsersIdxSize])
			if idx.NameCRC32 != userCRC && idx.HandleCRC32 != userCRC {
				continue
			}
			if u, ok := Read(g, i); ok && !u.Deleted() && (nameMatches(u.Name, query) || nameMatches(u.Handle, query)) {
				return u, true
			}
		}
	}
	raw, err := os.ReadFile(bbsPath)
	if err != nil {
		return cfgrec.User{}, false
	}
	n := len(raw) / cfgrec.UsersSize
	for i := 0; i < n; i++ {
		u := cfgrec.ParseUser(raw[i*cfgrec.UsersSize:(i+1)*cfgrec.UsersSize], i)
		if u.Deleted() {
			continue
		}
		if nameMatches(u.Name, query) || nameMatches(u.Handle, query) {
			return u, true
		}
	}
	return cfgrec.User{}, false
}

func Read(g *cfgrec.GlobalCfg, rec int) (cfgrec.User, bool) {
	f, err := os.Open(Path(g, cfgrec.UserBaseName))
	if err != nil {
		return cfgrec.User{}, false
	}
	defer f.Close()
	buf := make([]byte, cfgrec.UsersSize)
	if _, err := f.ReadAt(buf, int64(rec)*int64(cfgrec.UsersSize)); err != nil {
		return cfgrec.User{}, false
	}
	return cfgrec.ParseUser(buf, rec), true
}

func samePerson(onDisk, neu cfgrec.User) bool {
	qn := foldName(neu.Name)
	qh := foldName(neu.Handle)
	if qn == "" && qh == "" {
		return false
	}
	if qn != "" && (nameMatches(onDisk.Name, qn) || nameMatches(onDisk.Handle, qn)) {
		return true
	}
	if qh != "" && (nameMatches(onDisk.Name, qh) || nameMatches(onDisk.Handle, qh)) {
		return true
	}
	return false
}

func Write(g *cfgrec.GlobalCfg, u cfgrec.User) error {
	return writeUser(g, u, false)
}

func writeUser(g *cfgrec.GlobalCfg, u cfgrec.User, force bool) error {
	// Pascal UpdateUserRecord: UserRecord=-1 → no disk write.
	// Record 0 is the first user (often SysOp); the zero value must not clobber it.
	if u.Record < 0 {
		return nil
	}
	f, err := os.OpenFile(Path(g, cfgrec.UserBaseName), os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	off := int64(u.Record) * int64(cfgrec.UsersSize)
	if off < 0 || off+int64(cfgrec.UsersSize) > st.Size() {
		return fmt.Errorf("user record %d past end of file", u.Record)
	}
	buf := make([]byte, cfgrec.UsersSize)
	if _, err := f.ReadAt(buf, off); err != nil {
		return err
	}
	existing := cfgrec.ParseUser(buf, u.Record)
	if !force && existing.Name != "" && !samePerson(existing, u) {
		return fmt.Errorf("refusing to overwrite user %q with %q", existing.Name, u.Name)
	}
	_, err = f.WriteAt(cfgrec.EncodeUser(u), off)
	return err
}

func Count(g *cfgrec.GlobalCfg) (n int, path string) {
	path = Path(g, cfgrec.UserBaseName)
	users, err := List(g)
	if err != nil {
		return -1, path
	}
	return len(users), path
}

func List(g *cfgrec.GlobalCfg) ([]cfgrec.User, error) {
	raw, err := os.ReadFile(Path(g, cfgrec.UserBaseName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	n := len(raw) / cfgrec.UsersSize
	out := make([]cfgrec.User, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, cfgrec.ParseUser(raw[i*cfgrec.UsersSize:(i+1)*cfgrec.UsersSize], i))
	}
	return out, nil
}

func Append(g *cfgrec.GlobalCfg, u cfgrec.User) (cfgrec.User, error) {
	path := Path(g, cfgrec.UserBaseName)
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if os.IsNotExist(err) {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
		f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	}
	if err != nil {
		return u, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return u, err
	}
	var first []byte
	if st.Size() >= int64(cfgrec.UsersSize) {
		first = make([]byte, cfgrec.UsersSize)
		if _, err := f.ReadAt(first, 0); err != nil {
			f.Close()
			return u, err
		}
	}
	u.Record = int(st.Size()) / cfgrec.UsersSize
	_, err = f.WriteAt(cfgrec.EncodeUser(u), st.Size())
	if err == nil && len(first) > 0 {
		got := make([]byte, cfgrec.UsersSize)
		if _, rerr := f.ReadAt(got, 0); rerr == nil && !bytes.Equal(first, got) {
			_, _ = f.WriteAt(first, 0)
			f.Close()
			return u, fmt.Errorf("append clobbered existing users")
		}
	}
	f.Close()
	if err != nil {
		return u, err
	}
	return u, RebuildIndex(g)
}

func RebuildIndex(g *cfgrec.GlobalCfg) error {
	users, err := List(g)
	if err != nil {
		return err
	}
	buf := make([]byte, 0, len(users)*cfgrec.UsersIdxSize)
	for _, u := range users {
		name := u.Name
		handle := u.Handle
		idx := cfgrec.UsersIdx{NameCRC32: crc.RA(pascal.Trim(name), true), HandleCRC32: crc.RA(pascal.Trim(handle), true)}
		b := make([]byte, cfgrec.UsersIdxSize)
		putI32 := func(off int, v int32) {
			u := uint32(v)
			b[off] = byte(u)
			b[off+1] = byte(u >> 8)
			b[off+2] = byte(u >> 16)
			b[off+3] = byte(u >> 24)
		}
		putI32(0, idx.NameCRC32)
		putI32(4, idx.HandleCRC32)
		buf = append(buf, b...)
	}
	return os.WriteFile(Path(g, cfgrec.UserBaseIdxName), buf, 0644)
}

func WriteAndIndex(g *cfgrec.GlobalCfg, u cfgrec.User) error {
	if err := writeUser(g, u, true); err != nil {
		return err
	}
	return RebuildIndex(g)
}

// CheckPassword compares pw case-insensitively (RA hashes the uppercased
// password) unless strict (CONFIG.RA StrictPwdChecking) is set, in which case
// the case must match what SetPassword stored.
func CheckPassword(u cfgrec.User, pw string, strict bool) bool {
	pw = strings.TrimRight(pw, "\r\n")
	if strict {
		return (u.Password != "" && u.Password == pascal.Trim(pw)) || crc.RA(pw, false) == u.PasswordCRC
	}
	if u.Password != "" && pascal.UpCase(u.Password) == pascal.UpCase(pw) {
		return true
	}
	if crc.RA(pw, true) == u.PasswordCRC {
		return true
	}
	// Older records may have hashed the password as typed instead of uppercased.
	return crc.RA(pw, false) == u.PasswordCRC
}

// SetPassword stores pw so CheckPassword with the same strict setting matches it.
func SetPassword(u *cfgrec.User, pw string, strict bool) {
	if strict {
		u.Password = pascal.Trim(pw)
		u.PasswordCRC = crc.RA(pw, false)
		return
	}
	u.Password = pascal.UpCase(pascal.Trim(pw))
	u.PasswordCRC = crc.RA(pw, true)
}
